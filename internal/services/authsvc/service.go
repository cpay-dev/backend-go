package authsvc

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	cpayv1 "github.com/cpay-dev/cpay/internal/gen/cpay/v1"
	"github.com/cpay-dev/cpay/internal/platform/outbox"
	"github.com/cpay-dev/cpay/internal/shared/auth"
	"github.com/cpay-dev/cpay/internal/shared/config"
	"github.com/cpay-dev/cpay/internal/shared/rpcx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc/codes"
)

type Service struct {
	cpayv1.UnimplementedAuthServiceServer

	cfg    config.Config
	log    zerolog.Logger
	db     *pgxpool.Pool
	outbox *outbox.Publisher
}

func New(cfg config.Config, log zerolog.Logger, db *pgxpool.Pool) *Service {
	return &Service{
		cfg:    cfg,
		log:    log,
		db:     db,
		outbox: outbox.New(db, cfg.ServiceName),
	}
}

func (s *Service) EnsureBootstrap(ctx context.Context) error {
	var count int
	if err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM auth.users`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	merchantID := uuid.New()
	userID := uuid.New()
	passHash, err := bcrypt.GenerateFromPassword([]byte(s.cfg.BootstrapAdminPass), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err = tx.Exec(ctx, `
		INSERT INTO auth.merchants(id, name, created_at, updated_at)
		VALUES($1, $2, NOW(), NOW())
	`, merchantID, s.cfg.BootstrapMerchantName); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `
		INSERT INTO auth.users(id, merchant_id, email, password_hash, role, created_at, updated_at)
		VALUES($1, $2, $3, $4, 'admin', NOW(), NOW())
	`, userID, merchantID, strings.ToLower(strings.TrimSpace(s.cfg.BootstrapAdminEmail)), string(passHash)); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}

func (s *Service) Login(ctx context.Context, req *cpayv1.LoginRequest) (*cpayv1.LoginResponse, error) {
	email := strings.ToLower(strings.TrimSpace(req.GetEmail()))
	password := req.GetPassword()
	if email == "" || password == "" {
		return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "email and password are required")
	}

	var userID, merchantID, role, passHash string
	err := s.db.QueryRow(ctx, `
		SELECT id::text, merchant_id::text, role, password_hash
		FROM auth.users
		WHERE lower(email)=lower($1)
		LIMIT 1
	`, email).Scan(&userID, &merchantID, &role, &passHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, rpcx.E(codes.Unauthenticated, "invalid_credentials", "invalid credentials")
		}
		return nil, rpcx.E(codes.Internal, "internal_error", "login lookup failed")
	}

	if err = bcrypt.CompareHashAndPassword([]byte(passHash), []byte(password)); err != nil {
		return nil, rpcx.E(codes.Unauthenticated, "invalid_credentials", "invalid credentials")
	}

	access, err := auth.GenerateJWT(s.cfg.JWTSecret, "access", userID, merchantID, role, s.cfg.JWTAccessTTL)
	if err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to create access token")
	}
	refresh, err := auth.GenerateJWT(s.cfg.JWTSecret, "refresh", userID, merchantID, role, s.cfg.JWTRefreshTTL)
	if err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to create refresh token")
	}

	return &cpayv1.LoginResponse{
		Tokens: &cpayv1.TokenPair{
			AccessToken:  access,
			RefreshToken: refresh,
			TokenType:    "Bearer",
			ExpiresIn:    int32(s.cfg.JWTAccessTTL.Seconds()),
		},
		User: &cpayv1.UserInfo{Id: userID, MerchantId: merchantID, Role: role},
	}, nil
}

func (s *Service) Refresh(ctx context.Context, req *cpayv1.RefreshRequest) (*cpayv1.RefreshResponse, error) {
	claims, err := auth.ParseJWT(s.cfg.JWTSecret, strings.TrimSpace(req.GetRefreshToken()))
	if err != nil || claims.TokenType != "refresh" {
		return nil, rpcx.E(codes.Unauthenticated, "invalid_token", "invalid refresh token")
	}
	access, err := auth.GenerateJWT(s.cfg.JWTSecret, "access", claims.UserID, claims.MerchantID, claims.Role, s.cfg.JWTAccessTTL)
	if err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to create access token")
	}
	refresh, err := auth.GenerateJWT(s.cfg.JWTSecret, "refresh", claims.UserID, claims.MerchantID, claims.Role, s.cfg.JWTRefreshTTL)
	if err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to create refresh token")
	}
	return &cpayv1.RefreshResponse{Tokens: &cpayv1.TokenPair{
		AccessToken:  access,
		RefreshToken: refresh,
		TokenType:    "Bearer",
		ExpiresIn:    int32(s.cfg.JWTAccessTTL.Seconds()),
	}}, nil
}

func (s *Service) CreateApiKey(ctx context.Context, req *cpayv1.CreateApiKeyRequest) (*cpayv1.CreateApiKeyResponse, error) {
	merchantID, err := parseUUID(req.GetMerchantId(), "merchant_id")
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(req.GetName())
	if name == "" {
		return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "name is required")
	}
	scopes := req.GetScopes()
	if len(scopes) == 0 {
		scopes = []string{"*"}
	}

	plain, prefix, hash, err := auth.GenerateAPIKey()
	if err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to generate api key")
	}
	keyID := uuid.New()
	scopesRaw, _ := json.Marshal(scopes)
	_, err = s.db.Exec(ctx, `
		INSERT INTO auth.api_keys(id, merchant_id, name, key_prefix, key_hash, scopes, created_at)
		VALUES($1, $2, $3, $4, $5, $6::jsonb, NOW())
	`, keyID, merchantID, name, prefix, hash, string(scopesRaw))
	if err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to create api key")
	}

	_ = s.outbox.Enqueue(ctx, "api_key", keyID.String(), &merchantID, "api_key.created", map[string]any{
		"api_key_id": keyID,
		"name":       name,
		"scopes":     scopes,
	})

	now := time.Now().UTC().Format(time.RFC3339Nano)
	return &cpayv1.CreateApiKeyResponse{
		ApiKey: &cpayv1.ApiKey{
			Id:        keyID.String(),
			Name:      name,
			Prefix:    prefix,
			Scopes:    scopes,
			CreatedAt: now,
		},
		PlainKey: plain,
	}, nil
}

func (s *Service) ListApiKeys(ctx context.Context, req *cpayv1.ListApiKeysRequest) (*cpayv1.ListApiKeysResponse, error) {
	merchantID, err := parseUUID(req.GetMerchantId(), "merchant_id")
	if err != nil {
		return nil, err
	}

	rows, err := s.db.Query(ctx, `
		SELECT id::text, name, key_prefix, scopes, revoked_at, last_used_at, created_at
		FROM auth.api_keys
		WHERE merchant_id=$1
		ORDER BY created_at DESC
	`, merchantID)
	if err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to list api keys")
	}
	defer rows.Close()

	items := make([]*cpayv1.ApiKey, 0)
	for rows.Next() {
		var id, name, prefix string
		var scopesRaw []byte
		var revokedAt, lastUsedAt *time.Time
		var createdAt time.Time
		if err := rows.Scan(&id, &name, &prefix, &scopesRaw, &revokedAt, &lastUsedAt, &createdAt); err != nil {
			return nil, rpcx.E(codes.Internal, "internal_error", "failed to scan api key")
		}
		scopes := []string{}
		_ = json.Unmarshal(scopesRaw, &scopes)
		items = append(items, &cpayv1.ApiKey{
			Id:         id,
			Name:       name,
			Prefix:     prefix,
			Scopes:     scopes,
			RevokedAt:  formatTimePtr(revokedAt),
			LastUsedAt: formatTimePtr(lastUsedAt),
			CreatedAt:  createdAt.UTC().Format(time.RFC3339Nano),
		})
	}

	return &cpayv1.ListApiKeysResponse{Data: items}, nil
}

func (s *Service) RevokeApiKey(ctx context.Context, req *cpayv1.RevokeApiKeyRequest) (*cpayv1.RevokeApiKeyResponse, error) {
	merchantID, err := parseUUID(req.GetMerchantId(), "merchant_id")
	if err != nil {
		return nil, err
	}
	keyID, err := parseUUID(req.GetId(), "id")
	if err != nil {
		return nil, err
	}

	cmd, err := s.db.Exec(ctx, `
		UPDATE auth.api_keys
		SET revoked_at=NOW()
		WHERE id=$1 AND merchant_id=$2 AND revoked_at IS NULL
	`, keyID, merchantID)
	if err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to revoke api key")
	}
	if cmd.RowsAffected() == 0 {
		return nil, rpcx.E(codes.NotFound, "not_found", "api key not found")
	}

	_ = s.outbox.Enqueue(ctx, "api_key", keyID.String(), &merchantID, "api_key.revoked", map[string]any{"api_key_id": keyID})
	return &cpayv1.RevokeApiKeyResponse{Id: keyID.String(), Revoked: true}, nil
}

func (s *Service) ValidateCredential(ctx context.Context, req *cpayv1.ValidateCredentialRequest) (*cpayv1.ValidateCredentialResponse, error) {
	authorization := strings.TrimSpace(req.GetAuthorization())
	if strings.HasPrefix(strings.ToLower(authorization), "bearer ") {
		token := strings.TrimSpace(authorization[7:])
		claims, err := auth.ParseJWT(s.cfg.JWTSecret, token)
		if err != nil || claims.TokenType != "access" {
			return nil, rpcx.E(codes.Unauthenticated, "invalid_token", "invalid access token")
		}
		if _, err := uuid.Parse(claims.MerchantID); err != nil {
			return nil, rpcx.E(codes.Unauthenticated, "invalid_token", "invalid merchant in token")
		}
		if _, err := uuid.Parse(claims.UserID); err != nil {
			return nil, rpcx.E(codes.Unauthenticated, "invalid_token", "invalid user in token")
		}
		return &cpayv1.ValidateCredentialResponse{Principal: &cpayv1.Principal{
			MerchantId: claims.MerchantID,
			UserId:     claims.UserID,
			Role:       claims.Role,
			IsUser:     true,
		}}, nil
	}

	apiKey := strings.TrimSpace(req.GetApiKey())
	if apiKey == "" {
		return nil, rpcx.E(codes.Unauthenticated, "unauthorized", "missing credentials")
	}
	hash := auth.HashToken(apiKey)

	var keyID string
	var merchantID string
	err := s.db.QueryRow(ctx, `
		SELECT id::text, merchant_id::text
		FROM auth.api_keys
		WHERE key_hash=$1 AND revoked_at IS NULL
	`, hash).Scan(&keyID, &merchantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, rpcx.E(codes.Unauthenticated, "unauthorized", "invalid api key")
		}
		return nil, rpcx.E(codes.Internal, "internal_error", "auth lookup failed")
	}

	if _, err := uuid.Parse(merchantID); err != nil {
		return nil, rpcx.E(codes.Unauthenticated, "unauthorized", "invalid api key")
	}
	_, _ = s.db.Exec(ctx, `UPDATE auth.api_keys SET last_used_at=NOW() WHERE id=$1`, keyID)

	return &cpayv1.ValidateCredentialResponse{Principal: &cpayv1.Principal{
		MerchantId: merchantID,
		Role:       "api_key",
		ApiKeyId:   keyID,
		IsUser:     false,
	}}, nil
}

func parseUUID(raw, field string) (uuid.UUID, error) {
	id, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil {
		return uuid.Nil, rpcx.E(codes.InvalidArgument, "invalid_request", field+" is invalid")
	}
	return id, nil
}

func formatTimePtr(v *time.Time) string {
	if v == nil {
		return ""
	}
	return v.UTC().Format(time.RFC3339Nano)
}
