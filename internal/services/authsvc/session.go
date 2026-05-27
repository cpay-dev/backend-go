package authsvc

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	cpayv1 "github.com/cpay-dev/cpay/internal/gen/cpay/v1"
	"github.com/cpay-dev/cpay/internal/shared/auth"
	"github.com/cpay-dev/cpay/internal/shared/ids"
	"github.com/cpay-dev/cpay/internal/shared/rpcx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"google.golang.org/grpc/codes"
)

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
