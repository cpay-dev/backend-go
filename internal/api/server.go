package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cpay-dev/cpay/internal/platform/storage"
	"github.com/cpay-dev/cpay/internal/shared/auth"
	"github.com/cpay-dev/cpay/internal/shared/chain"
	"github.com/cpay-dev/cpay/internal/shared/config"
	"github.com/cpay-dev/cpay/internal/shared/httpx"
	"github.com/cpay-dev/cpay/internal/shared/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"
	"golang.org/x/crypto/bcrypt"
)

type authContextKey string

const requesterKey authContextKey = "requester"

type requester struct {
	MerchantID uuid.UUID
	UserID     *uuid.UUID
	Role       string
	APIKeyID   *uuid.UUID
}

type Server struct {
	cfg        config.Config
	log        zerolog.Logger
	db         *pgxpool.Pool
	minio      *storage.MinIO
	nats       *nats.Conn
	chain      chain.ChainAdapter
	encryptKey []byte
}

func NewServer(cfg config.Config, log zerolog.Logger, db *pgxpool.Pool, minio *storage.MinIO, natsConn *nats.Conn, chainAdapter chain.ChainAdapter, encryptKey []byte) *Server {
	return &Server{
		cfg:        cfg,
		log:        log,
		db:         db,
		minio:      minio,
		nats:       natsConn,
		chain:      chainAdapter,
		encryptKey: encryptKey,
	}
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Idempotency)
	r.Use(s.recovery)
	r.Use(s.accessLog)

	r.Get("/health", httpx.HealthHandler("api-gateway"))

	r.Route("/v1", func(r chi.Router) {
		r.Post("/auth/login", s.handleLogin)
		r.Post("/auth/refresh", s.handleRefresh)

		r.Get("/public/payment_links/{code}", s.handleGetPublicPaymentLink)

		r.Group(func(r chi.Router) {
			r.Use(s.authn)

			r.Get("/me", s.handleMe)

			r.Post("/api_keys", s.handleCreateAPIKey)
			r.Get("/api_keys", s.handleListAPIKeys)
			r.Post("/api_keys/{id}/revoke", s.handleRevokeAPIKey)

			r.Post("/payment_links", s.handleCreatePaymentLink)
			r.Get("/payment_links", s.handleListPaymentLinks)
			r.Get("/payment_links/{id}", s.handleGetPaymentLink)
			r.Patch("/payment_links/{id}", s.handleUpdatePaymentLink)
			r.Post("/payment_links/{id}/archive", s.handleArchivePaymentLink)

			r.Post("/payment_links/{id}/sessions", s.handleCreateCheckoutSession)
			r.Get("/checkout/{session_id}", s.handleGetCheckoutSession)
			r.Post("/checkout/{session_id}/confirm", s.handleConfirmCheckoutSession)
			r.Get("/payments/{id}", s.handleGetPaymentIntent)

			r.Post("/webhook_endpoints", s.handleCreateWebhookEndpoint)

			r.Post("/subscriptions", s.handleCreateSubscription)
			r.Post("/subscriptions/{id}/pause", s.handlePauseSubscription)
			r.Post("/subscriptions/{id}/resume", s.handleResumeSubscription)
			r.Get("/subscriptions/{id}/cycles", s.handleGetSubscriptionCycles)
		})
	})

	return r
}

func (s *Server) accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &respWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		s.log.Info().
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Int("status", rw.status).
			Dur("latency", time.Since(start)).
			Str("request_id", middleware.GetRequestID(r.Context())).
			Msg("http request")
	})
}

func (s *Server) recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				s.log.Error().Interface("panic", rec).Msg("panic recovered")
				httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "internal error", middleware.GetRequestID(r.Context()))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) authn(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authz := strings.TrimSpace(r.Header.Get("Authorization"))
		if strings.HasPrefix(strings.ToLower(authz), "bearer ") {
			token := strings.TrimSpace(authz[7:])
			claims, err := auth.ParseJWT(s.cfg.JWTSecret, token)
			if err != nil || claims.TokenType != "access" {
				httpx.WriteError(w, http.StatusUnauthorized, "invalid_token", "invalid access token", middleware.GetRequestID(r.Context()))
				return
			}
			merchantID, err := uuid.Parse(claims.MerchantID)
			if err != nil {
				httpx.WriteError(w, http.StatusUnauthorized, "invalid_token", "invalid merchant in token", middleware.GetRequestID(r.Context()))
				return
			}
			uid, err := uuid.Parse(claims.UserID)
			if err != nil {
				httpx.WriteError(w, http.StatusUnauthorized, "invalid_token", "invalid user in token", middleware.GetRequestID(r.Context()))
				return
			}
			req := requester{MerchantID: merchantID, Role: claims.Role, UserID: &uid}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requesterKey, req)))
			return
		}

		apiKey := strings.TrimSpace(r.Header.Get("X-API-Key"))
		if apiKey == "" {
			httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "missing credentials", middleware.GetRequestID(r.Context()))
			return
		}
		hash := auth.HashToken(apiKey)
		var keyIDStr string
		var merchantIDStr string
		err := s.db.QueryRow(r.Context(), `
			SELECT id::text, merchant_id::text
			FROM api_keys
			WHERE key_hash=$1 AND revoked_at IS NULL
		`, hash).Scan(&keyIDStr, &merchantIDStr)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "invalid api key", middleware.GetRequestID(r.Context()))
				return
			}
			httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "auth lookup failed", middleware.GetRequestID(r.Context()))
			return
		}

		keyID, _ := uuid.Parse(keyIDStr)
		merchantID, _ := uuid.Parse(merchantIDStr)
		_, _ = s.db.Exec(r.Context(), `UPDATE api_keys SET last_used_at=NOW() WHERE id=$1`, keyID)

		req := requester{MerchantID: merchantID, APIKeyID: &keyID, Role: "api_key"}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requesterKey, req)))
	})
}

func requesterFromContext(ctx context.Context) (requester, bool) {
	v := ctx.Value(requesterKey)
	r, ok := v.(requester)
	return r, ok
}

func mustRequester(w http.ResponseWriter, r *http.Request) (requester, bool) {
	req, ok := requesterFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "missing requester", middleware.GetRequestID(r.Context()))
		return requester{}, false
	}
	return req, true
}

func (s *Server) requireUser(w http.ResponseWriter, r *http.Request) (requester, bool) {
	req, ok := mustRequester(w, r)
	if !ok {
		return requester{}, false
	}
	if req.UserID == nil {
		httpx.WriteError(w, http.StatusForbidden, "forbidden", "user token required", middleware.GetRequestID(r.Context()))
		return requester{}, false
	}
	return req, true
}

func (s *Server) parseJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", fmt.Sprintf("invalid json: %v", err), middleware.GetRequestID(r.Context()))
		return false
	}
	return true
}

func (s *Server) withIdempotency(w http.ResponseWriter, r *http.Request, merchantID uuid.UUID, endpoint string, fn func() (int, any, error)) {
	idempotencyKey := middleware.GetIdempotencyKey(r.Context())
	if idempotencyKey == "" {
		status, payload, err := fn()
		if err != nil {
			s.writeErr(w, r, err)
			return
		}
		httpx.WriteJSON(w, status, payload)
		return
	}

	var existingStatus int
	var existingResp []byte
	err := s.db.QueryRow(r.Context(), `
		SELECT response_status, response_body::text
		FROM idempotency_keys
		WHERE merchant_id=$1 AND endpoint=$2 AND idempotency_key=$3
	`, merchantID, endpoint, idempotencyKey).Scan(&existingStatus, &existingResp)
	if err == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(existingStatus)
		_, _ = w.Write(existingResp)
		return
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "idempotency lookup failed", middleware.GetRequestID(r.Context()))
		return
	}

	status, payload, runErr := fn()
	if runErr != nil {
		s.writeErr(w, r, runErr)
		return
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to serialize response", middleware.GetRequestID(r.Context()))
		return
	}
	_, _ = s.db.Exec(r.Context(), `
		INSERT INTO idempotency_keys(id, merchant_id, endpoint, idempotency_key, request_hash, response_status, response_body, expires_at)
		VALUES($1, $2, $3, $4, $5, $6, $7::jsonb, NOW() + INTERVAL '24 hours')
		ON CONFLICT (merchant_id, endpoint, idempotency_key) DO NOTHING
	`, uuid.New(), merchantID, endpoint, idempotencyKey, hashRequest(r), status, string(payloadBytes))

	httpx.WriteJSON(w, status, payload)
}

func hashRequest(r *http.Request) string {
	sum := sha256.Sum256([]byte(r.Method + ":" + r.URL.Path))
	return hex.EncodeToString(sum[:])
}

func (s *Server) writeErr(w http.ResponseWriter, r *http.Request, err error) {
	type coded interface {
		Code() string
		Status() int
		Message() string
	}
	if c, ok := err.(coded); ok {
		httpx.WriteError(w, c.Status(), c.Code(), c.Message(), middleware.GetRequestID(r.Context()))
		return
	}
	s.log.Error().Err(err).Msg("handler error")
	httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "internal error", middleware.GetRequestID(r.Context()))
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	req, ok := mustRequester(w, r)
	if !ok {
		return
	}
	resp := map[string]any{
		"merchant_id": req.MerchantID,
		"role":        req.Role,
	}
	if req.UserID != nil {
		resp["user_id"] = req.UserID
	}
	if req.APIKeyID != nil {
		resp["api_key_id"] = req.APIKeyID
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (s *Server) EnsureBootstrap(ctx context.Context) error {
	var count int
	if err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
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
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()
	if _, err = tx.Exec(ctx, `
		INSERT INTO merchants(id, name, created_at, updated_at)
		VALUES($1, $2, NOW(), NOW())
	`, merchantID, s.cfg.BootstrapMerchantName); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `
		INSERT INTO users(id, merchant_id, email, password_hash, role, created_at, updated_at)
		VALUES($1, $2, $3, $4, 'admin', NOW(), NOW())
	`, userID, merchantID, strings.ToLower(strings.TrimSpace(s.cfg.BootstrapAdminEmail)), string(passHash)); err != nil {
		return err
	}
	err = tx.Commit(ctx)
	if err != nil {
		return err
	}
	s.log.Info().Str("email", s.cfg.BootstrapAdminEmail).Msg("bootstrap admin user created")
	return nil
}

type apiError struct {
	status  int
	code    string
	message string
}

func (e apiError) Error() string   { return e.message }
func (e apiError) Status() int     { return e.status }
func (e apiError) Code() string    { return e.code }
func (e apiError) Message() string { return e.message }

func badRequest(msg string) error {
	return apiError{status: http.StatusBadRequest, code: "invalid_request", message: msg}
}

func notFound(msg string) error {
	return apiError{status: http.StatusNotFound, code: "not_found", message: msg}
}

type respWriter struct {
	http.ResponseWriter
	status int
}

func (rw *respWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}
