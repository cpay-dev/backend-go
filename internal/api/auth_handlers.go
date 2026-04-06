package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/cpay-dev/cpay/internal/shared/auth"
	"github.com/cpay-dev/cpay/internal/shared/httpx"
	"github.com/cpay-dev/cpay/internal/shared/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type createAPIKeyRequest struct {
	Name   string   `json:"name"`
	Scopes []string `json:"scopes"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if !s.parseJSON(w, r, &req) {
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if email == "" || req.Password == "" {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "email and password are required", middleware.GetRequestID(r.Context()))
		return
	}

	var userIDStr, merchantIDStr, role, passHash string
	err := s.db.QueryRow(r.Context(), `
		SELECT id::text, merchant_id::text, role, password_hash
		FROM users
		WHERE lower(email)=lower($1)
		LIMIT 1
	`, email).Scan(&userIDStr, &merchantIDStr, &role, &passHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.WriteError(w, http.StatusUnauthorized, "invalid_credentials", "invalid credentials", middleware.GetRequestID(r.Context()))
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "login lookup failed", middleware.GetRequestID(r.Context()))
		return
	}

	if err = bcrypt.CompareHashAndPassword([]byte(passHash), []byte(req.Password)); err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid_credentials", "invalid credentials", middleware.GetRequestID(r.Context()))
		return
	}

	access, err := auth.GenerateJWT(s.cfg.JWTSecret, "access", userIDStr, merchantIDStr, role, s.cfg.JWTAccessTTL)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to create access token", middleware.GetRequestID(r.Context()))
		return
	}
	refresh, err := auth.GenerateJWT(s.cfg.JWTSecret, "refresh", userIDStr, merchantIDStr, role, s.cfg.JWTRefreshTTL)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to create refresh token", middleware.GetRequestID(r.Context()))
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"access_token":  access,
		"refresh_token": refresh,
		"token_type":    "Bearer",
		"expires_in":    int(s.cfg.JWTAccessTTL.Seconds()),
		"user": map[string]any{
			"id":          userIDStr,
			"merchant_id": merchantIDStr,
			"role":        role,
		},
	})
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if !s.parseJSON(w, r, &req) {
		return
	}
	claims, err := auth.ParseJWT(s.cfg.JWTSecret, strings.TrimSpace(req.RefreshToken))
	if err != nil || claims.TokenType != "refresh" {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid_token", "invalid refresh token", middleware.GetRequestID(r.Context()))
		return
	}
	access, err := auth.GenerateJWT(s.cfg.JWTSecret, "access", claims.UserID, claims.MerchantID, claims.Role, s.cfg.JWTAccessTTL)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to create access token", middleware.GetRequestID(r.Context()))
		return
	}
	refresh, err := auth.GenerateJWT(s.cfg.JWTSecret, "refresh", claims.UserID, claims.MerchantID, claims.Role, s.cfg.JWTRefreshTTL)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to create refresh token", middleware.GetRequestID(r.Context()))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"access_token":  access,
		"refresh_token": refresh,
		"token_type":    "Bearer",
		"expires_in":    int(s.cfg.JWTAccessTTL.Seconds()),
	})
}

func (s *Server) handleCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	reqAuth, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	var req createAPIKeyRequest
	if !s.parseJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "name is required", middleware.GetRequestID(r.Context()))
		return
	}
	if len(req.Scopes) == 0 {
		req.Scopes = []string{"*"}
	}

	s.withIdempotency(w, r, reqAuth.MerchantID, "/v1/api_keys", func() (int, any, error) {
		plain, prefix, hash, err := auth.GenerateAPIKey()
		if err != nil {
			return 0, nil, err
		}
		id := uuid.New()
		scopesRaw, _ := json.Marshal(req.Scopes)
		_, err = s.db.Exec(r.Context(), `
			INSERT INTO api_keys(id, merchant_id, name, key_prefix, key_hash, scopes, created_at)
			VALUES($1, $2, $3, $4, $5, $6::jsonb, NOW())
		`, id, reqAuth.MerchantID, req.Name, prefix, hash, string(scopesRaw))
		if err != nil {
			return 0, nil, err
		}
		_ = s.enqueueEvent(r.Context(), "api_key", id.String(), reqAuth.MerchantID, "api_key.created", map[string]any{
			"api_key_id": id,
			"name":       req.Name,
			"scopes":     req.Scopes,
		})
		return http.StatusCreated, map[string]any{
			"id":         id,
			"name":       req.Name,
			"prefix":     prefix,
			"key":        plain,
			"scopes":     req.Scopes,
			"created_at": timeNowUTC(),
		}, nil
	})
}

func (s *Server) handleListAPIKeys(w http.ResponseWriter, r *http.Request) {
	reqAuth, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	rows, err := s.db.Query(r.Context(), `
		SELECT id::text, name, key_prefix, scopes, revoked_at, last_used_at, created_at
		FROM api_keys
		WHERE merchant_id=$1
		ORDER BY created_at DESC
	`, reqAuth.MerchantID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to list api keys", middleware.GetRequestID(r.Context()))
		return
	}
	defer rows.Close()

	items := make([]map[string]any, 0)
	for rows.Next() {
		var id, name, prefix string
		var scopesRaw []byte
		var revokedAt, lastUsedAt *time.Time
		var createdAt time.Time
		if err := rows.Scan(&id, &name, &prefix, &scopesRaw, &revokedAt, &lastUsedAt, &createdAt); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to scan api key", middleware.GetRequestID(r.Context()))
			return
		}
		var scopes []string
		_ = json.Unmarshal(scopesRaw, &scopes)
		item := map[string]any{"id": id, "name": name, "prefix": prefix, "scopes": scopes, "revoked_at": revokedAt, "last_used_at": lastUsedAt, "created_at": createdAt}
		items = append(items, item)
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (s *Server) handleRevokeAPIKey(w http.ResponseWriter, r *http.Request) {
	reqAuth, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "invalid api key id", middleware.GetRequestID(r.Context()))
		return
	}
	cmd, err := s.db.Exec(r.Context(), `
		UPDATE api_keys
		SET revoked_at=NOW()
		WHERE id=$1 AND merchant_id=$2 AND revoked_at IS NULL
	`, id, reqAuth.MerchantID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to revoke api key", middleware.GetRequestID(r.Context()))
		return
	}
	if cmd.RowsAffected() == 0 {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "api key not found", middleware.GetRequestID(r.Context()))
		return
	}
	_ = s.enqueueEvent(r.Context(), "api_key", id.String(), reqAuth.MerchantID, "api_key.revoked", map[string]any{"api_key_id": id})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"id": id, "revoked": true})
}

func timeNowUTC() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}
