package authsvc

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	cpayv1 "github.com/cpay-dev/cpay/internal/gen/cpay/v1"
	"github.com/cpay-dev/cpay/internal/shared/ids"
	"github.com/cpay-dev/cpay/internal/shared/rpcx"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"google.golang.org/grpc/codes"
)

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

func webAuthnRequest(raw string) *http.Request {
	return &http.Request{
		Method: http.MethodPost,
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body:   io.NopCloser(bytes.NewBufferString(raw)),
	}
}
