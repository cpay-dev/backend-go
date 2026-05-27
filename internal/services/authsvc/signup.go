package authsvc

import (
	"context"
	"encoding/json"
	"strings"

	cpayv1 "github.com/cpay-dev/cpay/internal/gen/cpay/v1"
	"github.com/cpay-dev/cpay/internal/shared/ids"
	"github.com/cpay-dev/cpay/internal/shared/rpcx"
	"google.golang.org/grpc/codes"
)

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
