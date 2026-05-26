package authsvc

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	cpayv1 "github.com/cpay-dev/cpay/internal/gen/cpay/v1"
	"github.com/cpay-dev/cpay/internal/shared/auth"
	"github.com/cpay-dev/cpay/internal/shared/ids"
	"github.com/cpay-dev/cpay/internal/shared/rpcx"
	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"google.golang.org/grpc/codes"
)

const authChallengeTTL = 10 * time.Minute

type onboardingPayload struct {
	Provider        string `json:"provider"`
	ProviderSubject string `json:"provider_subject"`
	Email           string `json:"email,omitempty"`
	DisplayName     string `json:"display_name,omitempty"`
	WalletAddress   string `json:"wallet_address,omitempty"`
	ChainID         string `json:"chain_id,omitempty"`
}

type passkeyUser struct {
	id          []byte
	name        string
	displayName string
	credentials []webauthn.Credential
}

func (u passkeyUser) WebAuthnID() []byte                         { return u.id }
func (u passkeyUser) WebAuthnName() string                       { return u.name }
func (u passkeyUser) WebAuthnDisplayName() string                { return u.displayName }
func (u passkeyUser) WebAuthnCredentials() []webauthn.Credential { return u.credentials }

func (s *Service) Signup(ctx context.Context, req *cpayv1.SignupRequest) (*cpayv1.AuthExchangeResponse, error) {
	email := strings.ToLower(strings.TrimSpace(req.GetEmail()))
	merchantName := strings.TrimSpace(req.GetMerchantName())
	if merchantName == "" {
		merchantName = strings.TrimSpace(req.GetShopName())
	}
	if merchantName == "" {
		return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "merchant_name is required")
	}
	if email == "" {
		return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "email is required")
	}

	var payload *onboardingPayload
	if token := strings.TrimSpace(req.GetOnboardingToken()); token != "" {
		loaded, err := s.consumeOnboardingToken(ctx, token)
		if err != nil {
			return nil, err
		}
		payload = loaded
		if payload.Email != "" {
			email = strings.ToLower(strings.TrimSpace(payload.Email))
		}
	} else {
		return nil, rpcx.E(codes.FailedPrecondition, "password_signup_disabled", "email and password sign-up is disabled; start with wallet or OAuth")
	}

	merchantID := ids.New()
	userID := ids.New()
	metadata, _ := json.Marshal(map[string]any{
		"shop_name":        strings.TrimSpace(req.GetShopName()),
		"shop_url":         strings.TrimSpace(req.GetShopUrl()),
		"shop_description": strings.TrimSpace(req.GetShopDescription()),
		"contact_name":     strings.TrimSpace(req.GetContactName()),
	})

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to begin signup")
	}
	defer tx.Rollback(ctx)

	if _, err = tx.Exec(ctx, `
		INSERT INTO auth.merchants(id, name, metadata, created_at, updated_at)
		VALUES($1, $2, $3::jsonb, NOW(), NOW())
	`, merchantID, merchantName, string(metadata)); err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to create merchant")
	}
	if _, err = tx.Exec(ctx, `
		INSERT INTO auth.users(id, merchant_id, email, password_hash, role, created_at, updated_at)
		VALUES($1, $2, $3, $4, 'admin', NOW(), NOW())
	`, userID, merchantID, email, nil); err != nil {
		return nil, rpcx.E(codes.AlreadyExists, "already_exists", "account already exists")
	}
	if payload != nil {
		if err = s.insertIdentityTx(ctx, tx, userID, merchantID, payload); err != nil {
			return nil, err
		}
	}
	if err = s.outbox.EnqueueTx(ctx, tx, "user", userID, &merchantID, "user.signed_up", map[string]any{
		"user_id":     userID,
		"merchant_id": merchantID,
		"email":       email,
		"role":        "admin",
	}); err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to enqueue signup event")
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to commit signup")
	}
	return s.authExchangeForUser(ctx, userID, merchantID, "admin", email)
}

func (s *Service) GoogleStart(ctx context.Context, req *cpayv1.GoogleStartRequest) (*cpayv1.GoogleStartResponse, error) {
	if strings.TrimSpace(s.cfg.GoogleClientID) == "" {
		return nil, rpcx.E(codes.FailedPrecondition, "google_not_configured", "google sign-in is not configured")
	}
	redirectURI := strings.TrimSpace(req.GetRedirectUri())
	if redirectURI == "" {
		redirectURI = s.cfg.GoogleRedirectURI
	}
	state := randomToken(32)
	challengeID := ids.New()
	payload, _ := json.Marshal(map[string]string{"state": state, "redirect_uri": redirectURI})
	if _, err := s.db.Exec(ctx, `
		INSERT INTO auth.auth_challenges(id, kind, provider, challenge_hash, payload, expires_at)
		VALUES($1, 'google_oauth', 'google', $2, $3::jsonb, NOW() + $4::interval)
	`, challengeID, hashString(state), string(payload), intervalSeconds(authChallengeTTL)); err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to create google state")
	}
	values := url.Values{}
	values.Set("client_id", s.cfg.GoogleClientID)
	values.Set("redirect_uri", redirectURI)
	values.Set("response_type", "code")
	values.Set("scope", "openid email profile")
	values.Set("state", state)
	values.Set("access_type", "offline")
	return &cpayv1.GoogleStartResponse{
		AuthorizationUrl: "https://accounts.google.com/o/oauth2/auth?" + values.Encode(),
		State:            state,
	}, nil
}

func (s *Service) GoogleConsume(ctx context.Context, req *cpayv1.GoogleConsumeRequest) (*cpayv1.AuthExchangeResponse, error) {
	redirectURI := strings.TrimSpace(req.GetRedirectUri())
	if redirectURI == "" {
		redirectURI = s.cfg.GoogleRedirectURI
	}
	payload, err := s.consumeGoogleOAuth(ctx, req.GetState(), req.GetCode(), redirectURI)
	if err != nil {
		return nil, err
	}
	return s.authOrOnboard(ctx, *payload)
}

func (s *Service) WalletChallenge(ctx context.Context, req *cpayv1.WalletChallengeRequest) (*cpayv1.WalletChallengeResponse, error) {
	if !common.IsHexAddress(req.GetAddress()) {
		return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "address is invalid")
	}
	address := common.HexToAddress(req.GetAddress()).Hex()
	nonce := randomToken(24)
	issuedAt := time.Now().UTC().Format(time.RFC3339)
	message := s.walletMessage(address, strings.TrimSpace(req.GetChainId()), nonce, issuedAt)
	challengeID := ids.New()
	payload, _ := json.Marshal(map[string]string{
		"address":   address,
		"chain_id":  strings.TrimSpace(req.GetChainId()),
		"nonce":     nonce,
		"issued_at": issuedAt,
	})
	if _, err := s.db.Exec(ctx, `
		INSERT INTO auth.auth_challenges(id, kind, provider, provider_subject, challenge_hash, payload, expires_at)
		VALUES($1, 'wallet_login', 'wallet', $2, $3, $4::jsonb, NOW() + $5::interval)
	`, challengeID, strings.ToLower(address), hashString(message), string(payload), intervalSeconds(authChallengeTTL)); err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to create wallet challenge")
	}
	return &cpayv1.WalletChallengeResponse{ChallengeId: challengeID, Message: message}, nil
}

func (s *Service) WalletVerify(ctx context.Context, req *cpayv1.WalletVerifyRequest) (*cpayv1.AuthExchangeResponse, error) {
	challengeID, err := parseID(req.GetChallengeId(), "challenge_id")
	if err != nil {
		return nil, err
	}
	address, err := s.consumeWalletChallenge(ctx, challengeID, req.GetAddress(), req.GetMessage(), req.GetSignature())
	if err != nil {
		return nil, err
	}
	return s.authOrOnboard(ctx, onboardingPayload{
		Provider:        "wallet",
		ProviderSubject: strings.ToLower(address),
		WalletAddress:   address,
		ChainID:         strings.TrimSpace(req.GetChainId()),
	})
}

func (s *Service) BeginPasskeyRegistration(ctx context.Context, req *cpayv1.PasskeyOptionsRequest) (*cpayv1.PasskeyOptionsResponse, error) {
	userID, err := parseID(req.GetUserId(), "user_id")
	if err != nil {
		return nil, err
	}
	user, merchantID, err := s.loadPasskeyUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	wa, err := s.webAuthn()
	if err != nil {
		return nil, err
	}
	creation, session, err := wa.BeginMediatedRegistration(user, protocol.MediationDefault,
		webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementRequired),
		webauthn.WithExclusions(webauthn.Credentials(user.WebAuthnCredentials()).CredentialDescriptors()),
		webauthn.WithExtensions(map[string]any{"credProps": true}),
	)
	if err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to create passkey options")
	}
	return s.savePasskeyOptions(ctx, "passkey_register", userID, merchantID, creation, session)
}

func (s *Service) FinishPasskeyRegistration(ctx context.Context, req *cpayv1.PasskeyVerifyRequest) (*cpayv1.ProfileSecurityResponse, error) {
	challengeID, err := parseID(req.GetChallengeId(), "challenge_id")
	if err != nil {
		return nil, err
	}
	userID, merchantID, session, err := s.consumePasskeySession(ctx, challengeID, "passkey_register")
	if err != nil {
		return nil, err
	}
	user, _, err := s.loadPasskeyUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	wa, err := s.webAuthn()
	if err != nil {
		return nil, err
	}
	httpReq := webAuthnRequest(req.GetCredentialJson())
	credential, err := wa.FinishRegistration(user, *session, httpReq)
	if err != nil {
		return nil, rpcx.E(codes.Unauthenticated, "invalid_passkey", "passkey registration failed")
	}
	if err = s.insertPasskeyCredential(ctx, userID, merchantID, credential); err != nil {
		return nil, err
	}
	return s.profileSecurity(ctx, userID, merchantID)
}

func (s *Service) BeginPasskeyLogin(ctx context.Context, req *cpayv1.PasskeyOptionsRequest) (*cpayv1.PasskeyOptionsResponse, error) {
	wa, err := s.webAuthn()
	if err != nil {
		return nil, err
	}
	var assertion *protocol.CredentialAssertion
	var session *webauthn.SessionData
	var userID, merchantID string
	email := strings.ToLower(strings.TrimSpace(req.GetEmail()))
	if email != "" {
		user, mid, err := s.loadPasskeyUserByEmail(ctx, email)
		if err != nil {
			return nil, err
		}
		userID = string(user.WebAuthnID())
		merchantID = mid
		assertion, session, err = wa.BeginMediatedLogin(user, protocol.MediationDefault)
		if err != nil {
			return nil, rpcx.E(codes.FailedPrecondition, "no_passkey", "no passkey is registered for this account")
		}
	} else {
		assertion, session, err = wa.BeginDiscoverableMediatedLogin(protocol.MediationDefault)
		if err != nil {
			return nil, rpcx.E(codes.Internal, "internal_error", "failed to create passkey options")
		}
	}
	return s.savePasskeyOptions(ctx, "passkey_login", userID, merchantID, assertion, session)
}

func (s *Service) FinishPasskeyLogin(ctx context.Context, req *cpayv1.PasskeyVerifyRequest) (*cpayv1.AuthExchangeResponse, error) {
	challengeID, err := parseID(req.GetChallengeId(), "challenge_id")
	if err != nil {
		return nil, err
	}
	_, _, session, err := s.consumePasskeySession(ctx, challengeID, "passkey_login")
	if err != nil {
		return nil, err
	}
	wa, err := s.webAuthn()
	if err != nil {
		return nil, err
	}
	validatedUser, credential, err := wa.FinishPasskeyLogin(func(rawID, userHandle []byte) (webauthn.User, error) {
		return s.loadPasskeyUserByCredential(ctx, rawID, userHandle)
	}, *session, webAuthnRequest(req.GetCredentialJson()))
	if err != nil {
		return nil, rpcx.E(codes.Unauthenticated, "invalid_passkey", "passkey sign-in failed")
	}
	user := validatedUser.(passkeyUser)
	userID := string(user.WebAuthnID())
	merchantID, role, email, err := s.lookupUserAuth(ctx, userID)
	if err != nil {
		return nil, err
	}
	credRaw, _ := json.Marshal(credential)
	credentialID := credentialIDString(credential.ID)
	_, _ = s.db.Exec(ctx, `
		UPDATE auth.passkey_credentials
		SET credential=$2::jsonb, sign_count=$3, last_used_at=NOW(), updated_at=NOW()
		WHERE credential_id=$1
	`, credentialID, string(credRaw), int64(credential.Authenticator.SignCount))
	return s.authExchangeForUser(ctx, userID, merchantID, role, email)
}

func (s *Service) GetProfileSecurity(ctx context.Context, req *cpayv1.ProfileSecurityRequest) (*cpayv1.ProfileSecurityResponse, error) {
	userID, err := parseID(req.GetUserId(), "user_id")
	if err != nil {
		return nil, err
	}
	merchantID, err := parseID(req.GetMerchantId(), "merchant_id")
	if err != nil {
		return nil, err
	}
	return s.profileSecurity(ctx, userID, merchantID)
}

func (s *Service) LinkGoogle(ctx context.Context, req *cpayv1.LinkGoogleRequest) (*cpayv1.ProfileSecurityResponse, error) {
	userID, err := parseID(req.GetUserId(), "user_id")
	if err != nil {
		return nil, err
	}
	merchantID, err := parseID(req.GetMerchantId(), "merchant_id")
	if err != nil {
		return nil, err
	}
	state := strings.TrimSpace(req.GetState())
	redirectURI := strings.TrimSpace(req.GetRedirectUri())
	if redirectURI == "" {
		redirectURI = s.cfg.GoogleRedirectURI
	}
	payload, err := s.consumeGoogleOAuth(ctx, state, req.GetCode(), redirectURI)
	if err != nil {
		return nil, err
	}
	if err = s.insertIdentityTx(ctx, s.db, userID, merchantID, payload); err != nil {
		return nil, err
	}
	return s.profileSecurity(ctx, userID, merchantID)
}

func (s *Service) LinkWallet(ctx context.Context, req *cpayv1.LinkWalletRequest) (*cpayv1.ProfileSecurityResponse, error) {
	userID, err := parseID(req.GetUserId(), "user_id")
	if err != nil {
		return nil, err
	}
	merchantID, err := parseID(req.GetMerchantId(), "merchant_id")
	if err != nil {
		return nil, err
	}
	challengeID, err := parseID(req.GetChallengeId(), "challenge_id")
	if err != nil {
		return nil, err
	}
	address, err := s.consumeWalletChallenge(ctx, challengeID, req.GetAddress(), req.GetMessage(), req.GetSignature())
	if err != nil {
		return nil, err
	}
	payload := &onboardingPayload{
		Provider:        "wallet",
		ProviderSubject: strings.ToLower(address),
		WalletAddress:   address,
		ChainID:         strings.TrimSpace(req.GetChainId()),
	}
	if err = s.insertIdentityTx(ctx, s.db, userID, merchantID, payload); err != nil {
		return nil, err
	}
	return s.profileSecurity(ctx, userID, merchantID)
}

func (s *Service) DeleteIdentity(ctx context.Context, req *cpayv1.DeleteIdentityRequest) (*cpayv1.DeleteAuthMethodResponse, error) {
	userID, err := parseID(req.GetUserId(), "user_id")
	if err != nil {
		return nil, err
	}
	merchantID, err := parseID(req.GetMerchantId(), "merchant_id")
	if err != nil {
		return nil, err
	}
	id, err := parseID(req.GetId(), "id")
	if err != nil {
		return nil, err
	}
	if err = s.ensureAlternativeAuthMethod(ctx, userID, merchantID, "identity", id); err != nil {
		return nil, err
	}
	cmd, err := s.db.Exec(ctx, `DELETE FROM auth.identities WHERE id=$1 AND user_id=$2 AND merchant_id=$3`, id, userID, merchantID)
	if err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to delete identity")
	}
	if cmd.RowsAffected() == 0 {
		return nil, rpcx.E(codes.NotFound, "not_found", "identity not found")
	}
	return &cpayv1.DeleteAuthMethodResponse{Id: id, Deleted: true}, nil
}

func (s *Service) DeletePasskey(ctx context.Context, req *cpayv1.DeletePasskeyRequest) (*cpayv1.DeleteAuthMethodResponse, error) {
	userID, err := parseID(req.GetUserId(), "user_id")
	if err != nil {
		return nil, err
	}
	merchantID, err := parseID(req.GetMerchantId(), "merchant_id")
	if err != nil {
		return nil, err
	}
	id, err := parseID(req.GetId(), "id")
	if err != nil {
		return nil, err
	}
	if err = s.ensureAlternativeAuthMethod(ctx, userID, merchantID, "passkey", id); err != nil {
		return nil, err
	}
	cmd, err := s.db.Exec(ctx, `DELETE FROM auth.passkey_credentials WHERE id=$1 AND user_id=$2 AND merchant_id=$3`, id, userID, merchantID)
	if err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to delete passkey")
	}
	if cmd.RowsAffected() == 0 {
		return nil, rpcx.E(codes.NotFound, "not_found", "passkey not found")
	}
	return &cpayv1.DeleteAuthMethodResponse{Id: id, Deleted: true}, nil
}

func (s *Service) webAuthn() (*webauthn.WebAuthn, error) {
	origin := strings.TrimRight(s.cfg.PublicWebOrigin, "/")
	wa, err := webauthn.New(&webauthn.Config{
		RPDisplayName: s.cfg.WebAuthnRPName,
		RPID:          s.cfg.WebAuthnRPID,
		RPOrigins:     []string{origin},
	})
	if err != nil {
		return nil, rpcx.E(codes.FailedPrecondition, "passkey_not_configured", "passkeys are not configured")
	}
	return wa, nil
}

func (s *Service) authOrOnboard(ctx context.Context, payload onboardingPayload) (*cpayv1.AuthExchangeResponse, error) {
	var userID, merchantID, role, email string
	err := s.db.QueryRow(ctx, `
		SELECT u.id::text, u.merchant_id::text, u.role, lower(u.email)
		FROM auth.identities i
		JOIN auth.users u ON u.id=i.user_id
		WHERE i.provider=$1 AND i.provider_subject=$2
		LIMIT 1
	`, payload.Provider, payload.ProviderSubject).Scan(&userID, &merchantID, &role, &email)
	if err == nil {
		return s.authExchangeForUser(ctx, userID, merchantID, role, email)
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, rpcx.E(codes.Internal, "internal_error", "identity lookup failed")
	}
	token, err := s.createOnboardingToken(ctx, payload)
	if err != nil {
		return nil, err
	}
	return &cpayv1.AuthExchangeResponse{
		OnboardingRequired: true,
		OnboardingToken:    token,
		Email:              payload.Email,
		Provider:           payload.Provider,
	}, nil
}

func (s *Service) authExchangeForUser(ctx context.Context, userID, merchantID, role, email string) (*cpayv1.AuthExchangeResponse, error) {
	access, err := auth.GenerateJWT(s.cfg.JWTSecret, "access", userID, merchantID, role, s.cfg.JWTAccessTTL)
	if err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to create access token")
	}
	refresh, err := auth.GenerateJWT(s.cfg.JWTSecret, "refresh", userID, merchantID, role, s.cfg.JWTRefreshTTL)
	if err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to create refresh token")
	}
	return &cpayv1.AuthExchangeResponse{
		Tokens: &cpayv1.TokenPair{AccessToken: access, RefreshToken: refresh, TokenType: "Bearer", ExpiresIn: int32(s.cfg.JWTAccessTTL.Seconds())},
		User:   &cpayv1.UserInfo{Id: userID, MerchantId: merchantID, Role: role, Email: email},
	}, nil
}

func (s *Service) createOnboardingToken(ctx context.Context, payload onboardingPayload) (string, error) {
	token := randomToken(32)
	raw, _ := json.Marshal(payload)
	if _, err := s.db.Exec(ctx, `
		INSERT INTO auth.auth_challenges(id, kind, provider, provider_subject, challenge_hash, payload, expires_at)
		VALUES($1, 'onboarding', $2, $3, $4, $5::jsonb, NOW() + $6::interval)
	`, ids.New(), payload.Provider, payload.ProviderSubject, hashString(token), string(raw), intervalSeconds(30*time.Minute)); err != nil {
		return "", rpcx.E(codes.Internal, "internal_error", "failed to create onboarding token")
	}
	return token, nil
}

func (s *Service) consumeOnboardingToken(ctx context.Context, token string) (*onboardingPayload, error) {
	var raw []byte
	err := s.db.QueryRow(ctx, `
		UPDATE auth.auth_challenges
		SET consumed_at=NOW()
		WHERE kind='onboarding' AND challenge_hash=$1 AND consumed_at IS NULL AND expires_at > NOW()
		RETURNING payload
	`, hashString(token)).Scan(&raw)
	if err != nil {
		return nil, rpcx.E(codes.Unauthenticated, "invalid_onboarding", "onboarding token is invalid or expired")
	}
	var payload onboardingPayload
	if err = json.Unmarshal(raw, &payload); err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "onboarding payload is invalid")
	}
	return &payload, nil
}

type txExec interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func (s *Service) insertIdentityTx(ctx context.Context, tx txExec, userID, merchantID string, payload *onboardingPayload) error {
	subject := payload.ProviderSubject
	email := payload.Email
	displayName := payload.DisplayName
	meta, _ := json.Marshal(payload)
	if payload.Provider == "wallet" {
		subject = strings.ToLower(payload.WalletAddress)
		if subject == "" {
			subject = strings.ToLower(payload.ProviderSubject)
		}
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO auth.identities(id, user_id, merchant_id, provider, provider_subject, email, display_name, metadata, created_at, updated_at)
		VALUES($1, $2, $3, $4, $5, $6, $7, $8::jsonb, NOW(), NOW())
	`, ids.New(), userID, merchantID, payload.Provider, subject, nullString(email), nullString(displayName), string(meta))
	if err != nil {
		return rpcx.E(codes.AlreadyExists, "already_exists", "identity already belongs to another account")
	}
	return nil
}

func (s *Service) savePasskeyOptions(ctx context.Context, kind, userID, merchantID string, publicKey any, session *webauthn.SessionData) (*cpayv1.PasskeyOptionsResponse, error) {
	challengeID := ids.New()
	publicRaw, _ := json.Marshal(publicKey)
	sessionRaw, _ := json.Marshal(session)
	if _, err := s.db.Exec(ctx, `
		INSERT INTO auth.auth_challenges(id, kind, user_id, merchant_id, session_data, expires_at)
		VALUES($1, $2, $3, $4, $5::jsonb, NOW() + $6::interval)
	`, challengeID, kind, nullString(userID), nullString(merchantID), string(sessionRaw), intervalSeconds(authChallengeTTL)); err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to save passkey challenge")
	}
	return &cpayv1.PasskeyOptionsResponse{ChallengeId: challengeID, PublicKeyJson: string(publicRaw)}, nil
}

func (s *Service) consumePasskeySession(ctx context.Context, challengeID, kind string) (string, string, *webauthn.SessionData, error) {
	var userID, merchantID sql.NullString
	var raw []byte
	err := s.db.QueryRow(ctx, `
		UPDATE auth.auth_challenges
		SET consumed_at=NOW()
		WHERE id=$1 AND kind=$2 AND consumed_at IS NULL AND expires_at > NOW()
		RETURNING user_id::text, merchant_id::text, session_data
	`, challengeID, kind).Scan(&userID, &merchantID, &raw)
	if err != nil {
		return "", "", nil, rpcx.E(codes.Unauthenticated, "invalid_challenge", "passkey challenge is invalid or expired")
	}
	var session webauthn.SessionData
	if err = json.Unmarshal(raw, &session); err != nil {
		return "", "", nil, rpcx.E(codes.Internal, "internal_error", "passkey session is invalid")
	}
	return userID.String, merchantID.String, &session, nil
}

func (s *Service) loadPasskeyUser(ctx context.Context, userID string) (passkeyUser, string, error) {
	var email, merchantID string
	err := s.db.QueryRow(ctx, `SELECT lower(email), merchant_id::text FROM auth.users WHERE id=$1`, userID).Scan(&email, &merchantID)
	if err != nil {
		return passkeyUser{}, "", rpcx.E(codes.NotFound, "not_found", "user not found")
	}
	creds, err := s.loadCredentials(ctx, userID)
	if err != nil {
		return passkeyUser{}, "", err
	}
	return passkeyUser{id: []byte(userID), name: email, displayName: email, credentials: creds}, merchantID, nil
}

func (s *Service) loadPasskeyUserByEmail(ctx context.Context, email string) (passkeyUser, string, error) {
	var userID string
	var merchantID string
	err := s.db.QueryRow(ctx, `SELECT id::text, merchant_id::text FROM auth.users WHERE lower(email)=lower($1)`, email).Scan(&userID, &merchantID)
	if err != nil {
		return passkeyUser{}, "", rpcx.E(codes.NotFound, "not_found", "account not found")
	}
	user, _, err := s.loadPasskeyUser(ctx, userID)
	return user, merchantID, err
}

func (s *Service) loadPasskeyUserByCredential(ctx context.Context, rawID, userHandle []byte) (webauthn.User, error) {
	credentialID := credentialIDString(rawID)
	var userID string
	err := s.db.QueryRow(ctx, `SELECT user_id::text FROM auth.passkey_credentials WHERE credential_id=$1`, credentialID).Scan(&userID)
	if err != nil && len(userHandle) > 0 {
		userID = string(userHandle)
		err = nil
	}
	if err != nil {
		return nil, fmt.Errorf("credential not found")
	}
	user, _, err := s.loadPasskeyUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (s *Service) loadCredentials(ctx context.Context, userID string) ([]webauthn.Credential, error) {
	rows, err := s.db.Query(ctx, `SELECT credential FROM auth.passkey_credentials WHERE user_id=$1`, userID)
	if err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to load passkeys")
	}
	defer rows.Close()
	var creds []webauthn.Credential
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			return nil, rpcx.E(codes.Internal, "internal_error", "failed to scan passkey")
		}
		var cred webauthn.Credential
		if err = json.Unmarshal(raw, &cred); err != nil {
			return nil, rpcx.E(codes.Internal, "internal_error", "failed to decode passkey")
		}
		creds = append(creds, cred)
	}
	return creds, nil
}

func (s *Service) insertPasskeyCredential(ctx context.Context, userID, merchantID string, credential *webauthn.Credential) error {
	raw, _ := json.Marshal(credential)
	transports, _ := json.Marshal(credential.Transport)
	_, err := s.db.Exec(ctx, `
		INSERT INTO auth.passkey_credentials(id, user_id, merchant_id, credential_id, credential, transports, sign_count, created_at, updated_at)
		VALUES($1, $2, $3, $4, $5::jsonb, $6::jsonb, $7, NOW(), NOW())
	`, ids.New(), userID, merchantID, credentialIDString(credential.ID), string(raw), string(transports), int64(credential.Authenticator.SignCount))
	if err != nil {
		return rpcx.E(codes.AlreadyExists, "already_exists", "passkey is already registered")
	}
	return nil
}

func (s *Service) profileSecurity(ctx context.Context, userID, merchantID string) (*cpayv1.ProfileSecurityResponse, error) {
	identities := []*cpayv1.Identity{}
	rows, err := s.db.Query(ctx, `
		SELECT id::text, provider, provider_subject, COALESCE(email, ''), COALESCE(display_name, ''), created_at
		FROM auth.identities
		WHERE user_id=$1 AND merchant_id=$2
		ORDER BY created_at DESC
	`, userID, merchantID)
	if err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to list identities")
	}
	defer rows.Close()
	for rows.Next() {
		var item cpayv1.Identity
		var createdAt time.Time
		if err = rows.Scan(&item.Id, &item.Provider, &item.ProviderSubject, &item.Email, &item.DisplayName, &createdAt); err != nil {
			return nil, rpcx.E(codes.Internal, "internal_error", "failed to scan identity")
		}
		item.CreatedAt = createdAt.UTC().Format(time.RFC3339Nano)
		identities = append(identities, &item)
	}
	passkeys := []*cpayv1.PasskeyCredentialInfo{}
	rows, err = s.db.Query(ctx, `
		SELECT id::text, credential_id, COALESCE(name, ''), created_at, last_used_at
		FROM auth.passkey_credentials
		WHERE user_id=$1 AND merchant_id=$2
		ORDER BY created_at DESC
	`, userID, merchantID)
	if err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to list passkeys")
	}
	defer rows.Close()
	for rows.Next() {
		var item cpayv1.PasskeyCredentialInfo
		var createdAt time.Time
		var lastUsedAt *time.Time
		if err = rows.Scan(&item.Id, &item.CredentialId, &item.Name, &createdAt, &lastUsedAt); err != nil {
			return nil, rpcx.E(codes.Internal, "internal_error", "failed to scan passkey")
		}
		item.CreatedAt = createdAt.UTC().Format(time.RFC3339Nano)
		item.LastUsedAt = formatTimePtr(lastUsedAt)
		passkeys = append(passkeys, &item)
	}
	return &cpayv1.ProfileSecurityResponse{Identities: identities, Passkeys: passkeys}, nil
}

func (s *Service) lookupUserAuth(ctx context.Context, userID string) (string, string, string, error) {
	var merchantID, role, email string
	err := s.db.QueryRow(ctx, `SELECT merchant_id::text, role, lower(email) FROM auth.users WHERE id=$1`, userID).Scan(&merchantID, &role, &email)
	if err != nil {
		return "", "", "", rpcx.E(codes.NotFound, "not_found", "user not found")
	}
	return merchantID, role, email, nil
}

func (s *Service) ensureAlternativeAuthMethod(ctx context.Context, userID, merchantID, deletingKind, deletingID string) error {
	var exists bool
	if err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM auth.users WHERE id=$1 AND merchant_id=$2)`, userID, merchantID).Scan(&exists); err != nil || !exists {
		return rpcx.E(codes.NotFound, "not_found", "user not found")
	}
	var identities, passkeys int
	_ = s.db.QueryRow(ctx, `SELECT COUNT(*) FROM auth.identities WHERE user_id=$1 AND merchant_id=$2 AND ($3 <> 'identity' OR id::text <> $4)`, userID, merchantID, deletingKind, deletingID).Scan(&identities)
	_ = s.db.QueryRow(ctx, `SELECT COUNT(*) FROM auth.passkey_credentials WHERE user_id=$1 AND merchant_id=$2 AND ($3 <> 'passkey' OR id::text <> $4)`, userID, merchantID, deletingKind, deletingID).Scan(&passkeys)
	if identities == 0 && passkeys == 0 {
		return rpcx.E(codes.FailedPrecondition, "last_auth_method", "add another sign-in method before removing this one")
	}
	return nil
}

type googleClaims struct {
	Sub           string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified string `json:"email_verified"`
	Aud           string `json:"aud"`
	Name          string `json:"name"`
}

func (s *Service) consumeGoogleOAuth(ctx context.Context, state, code, redirectURI string) (*onboardingPayload, error) {
	state = strings.TrimSpace(state)
	if state == "" || strings.TrimSpace(code) == "" {
		return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "code and state are required")
	}

	var challengeID string
	var statePayload []byte
	err := s.db.QueryRow(ctx, `
		SELECT id::text, payload
		FROM auth.auth_challenges
		WHERE kind='google_oauth' AND challenge_hash=$1 AND consumed_at IS NULL AND expires_at > NOW()
		LIMIT 1
	`, hashString(state)).Scan(&challengeID, &statePayload)
	if err != nil || !googleStateRedirectMatches(statePayload, redirectURI) {
		return nil, rpcx.E(codes.Unauthenticated, "invalid_state", "google state is invalid or expired")
	}

	claims, err := s.exchangeGoogleCode(ctx, code, redirectURI)
	if err != nil {
		return nil, err
	}
	_, _ = s.db.Exec(ctx, `UPDATE auth.auth_challenges SET consumed_at=NOW() WHERE id=$1`, challengeID)
	return &onboardingPayload{
		Provider:        "google",
		ProviderSubject: claims.Sub,
		Email:           strings.ToLower(strings.TrimSpace(claims.Email)),
		DisplayName:     strings.TrimSpace(claims.Name),
	}, nil
}

func (s *Service) exchangeGoogleCode(ctx context.Context, code, redirectURI string) (*googleClaims, error) {
	if s.cfg.GoogleClientID == "" || s.cfg.GoogleClientSecret == "" {
		return nil, rpcx.E(codes.FailedPrecondition, "google_not_configured", "google sign-in is not configured")
	}
	form := url.Values{}
	form.Set("client_id", s.cfg.GoogleClientID)
	form.Set("client_secret", s.cfg.GoogleClientSecret)
	form.Set("code", code)
	form.Set("grant_type", "authorization_code")
	form.Set("redirect_uri", redirectURI)
	httpReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://oauth2.googleapis.com/token", strings.NewReader(form.Encode()))
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, rpcx.E(codes.Unavailable, "google_unavailable", "google token exchange failed")
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, rpcx.E(codes.Unauthenticated, "google_rejected", "google token exchange was rejected")
	}
	var tokenResp struct {
		IDToken string `json:"id_token"`
	}
	if err = json.Unmarshal(body, &tokenResp); err != nil || tokenResp.IDToken == "" {
		return nil, rpcx.E(codes.Unauthenticated, "google_rejected", "google did not return an identity token")
	}
	infoURL := "https://oauth2.googleapis.com/tokeninfo?id_token=" + url.QueryEscape(tokenResp.IDToken)
	infoReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, infoURL, nil)
	infoResp, err := http.DefaultClient.Do(infoReq)
	if err != nil {
		return nil, rpcx.E(codes.Unavailable, "google_unavailable", "google identity verification failed")
	}
	defer infoResp.Body.Close()
	infoBody, _ := io.ReadAll(infoResp.Body)
	if infoResp.StatusCode < 200 || infoResp.StatusCode >= 300 {
		return nil, rpcx.E(codes.Unauthenticated, "google_rejected", "google identity token is invalid")
	}
	var claims googleClaims
	if err = json.Unmarshal(infoBody, &claims); err != nil {
		return nil, rpcx.E(codes.Unauthenticated, "google_rejected", "google identity token is invalid")
	}
	if claims.Aud != s.cfg.GoogleClientID || claims.Sub == "" || strings.ToLower(claims.EmailVerified) != "true" || strings.TrimSpace(claims.Email) == "" {
		return nil, rpcx.E(codes.Unauthenticated, "google_rejected", "google identity token is invalid")
	}
	return &claims, nil
}

func googleStateRedirectMatches(raw []byte, redirectURI string) bool {
	var payload struct {
		RedirectURI string `json:"redirect_uri"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return false
	}
	return strings.TrimSpace(payload.RedirectURI) == strings.TrimSpace(redirectURI)
}

func (s *Service) consumeWalletChallenge(ctx context.Context, challengeID, rawAddress, message, signature string) (string, error) {
	if !common.IsHexAddress(rawAddress) {
		return "", rpcx.E(codes.InvalidArgument, "invalid_request", "address is invalid")
	}
	address := common.HexToAddress(rawAddress).Hex()

	err := s.db.QueryRow(ctx, `
		SELECT id::text
		FROM auth.auth_challenges
		WHERE id=$1 AND kind='wallet_login' AND provider_subject=$2 AND challenge_hash=$3 AND consumed_at IS NULL AND expires_at > NOW()
	`, challengeID, strings.ToLower(address), hashString(message)).Scan(&challengeID)
	if err != nil {
		return "", rpcx.E(codes.Unauthenticated, "invalid_challenge", "wallet challenge is invalid or expired")
	}
	if !verifyPersonalSignature(address, message, signature) {
		return "", rpcx.E(codes.Unauthenticated, "invalid_signature", "wallet signature is invalid")
	}
	_, _ = s.db.Exec(ctx, `UPDATE auth.auth_challenges SET consumed_at=NOW() WHERE id=$1`, challengeID)
	return address, nil
}

func (s *Service) walletMessage(address, chainID, nonce, issuedAt string) string {
	host := strings.TrimPrefix(strings.TrimPrefix(s.cfg.PublicWebOrigin, "https://"), "http://")
	if slash := strings.Index(host, "/"); slash >= 0 {
		host = host[:slash]
	}
	lines := []string{
		host + " wants you to sign in with your Ethereum account:",
		address,
		"",
		"Sign in to cpay.dev.",
		"",
		"URI: " + s.cfg.PublicWebOrigin,
		"Version: 1",
		"Nonce: " + nonce,
		"Issued At: " + issuedAt,
	}
	if strings.TrimSpace(chainID) != "" {
		lines = append(lines, "Chain ID: "+strings.TrimSpace(chainID))
	}
	return strings.Join(lines, "\n")
}

func verifyPersonalSignature(address, message, signature string) bool {
	sig, err := hex.DecodeString(strings.TrimPrefix(signature, "0x"))
	if err != nil || len(sig) != 65 {
		return false
	}
	if sig[64] >= 27 {
		sig[64] -= 27
	}
	pub, err := crypto.SigToPub(accounts.TextHash([]byte(message)), sig)
	if err != nil {
		return false
	}
	recovered := crypto.PubkeyToAddress(*pub)
	return strings.EqualFold(recovered.Hex(), common.HexToAddress(address).Hex())
}

func webAuthnRequest(raw string) *http.Request {
	return &http.Request{
		Method: http.MethodPost,
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body:   io.NopCloser(bytes.NewBufferString(raw)),
	}
}

func randomToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func hashString(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func credentialIDString(raw []byte) string {
	return base64.RawURLEncoding.EncodeToString(raw)
}

func intervalSeconds(d time.Duration) string {
	return fmt.Sprintf("%d seconds", int(d.Seconds()))
}

func nullString(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return v
}
