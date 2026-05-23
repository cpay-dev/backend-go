package gateway

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	cpayv1 "github.com/cpay-dev/cpay/internal/gen/cpay/v1"
	"github.com/cpay-dev/cpay/internal/platform/outbox"
	"github.com/cpay-dev/cpay/internal/shared/config"
	"github.com/cpay-dev/cpay/internal/shared/httpx"
	"github.com/cpay-dev/cpay/internal/shared/ids"
	"github.com/cpay-dev/cpay/internal/shared/middleware"
	"github.com/cpay-dev/cpay/internal/shared/rpcx"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

type authContextKey string

const requesterKey authContextKey = "requester"

type requester struct {
	MerchantID string
	UserID     *string
	Role       string
	APIKeyID   *string
	IsUser     bool
}

type gatewayOutboxPublisher interface {
	Enqueue(ctx context.Context, aggregateType, aggregateID string, merchantID *string, eventType string, payload any) error
}

type gatewayObjectStore interface {
	PutObjectBytes(ctx context.Context, objectKey, contentType string, payload []byte) error
	GetObjectBytes(ctx context.Context, objectKey string) ([]byte, string, error)
}

type Server struct {
	cfg            config.Config
	log            zerolog.Logger
	db             *pgxpool.Pool
	mediaStore     gatewayObjectStore
	outbox         gatewayOutboxPublisher
	authClient     cpayv1.AuthServiceClient
	linkClient     cpayv1.PaymentLinkServiceClient
	checkoutClient cpayv1.CheckoutServiceClient
}

func NewServer(
	cfg config.Config,
	log zerolog.Logger,
	db *pgxpool.Pool,
	mediaStore gatewayObjectStore,
	authClient cpayv1.AuthServiceClient,
	linkClient cpayv1.PaymentLinkServiceClient,
	checkoutClient cpayv1.CheckoutServiceClient,
) *Server {
	return &Server{
		cfg:            cfg,
		log:            log,
		db:             db,
		mediaStore:     mediaStore,
		outbox:         outbox.New(db, cfg.ServiceName),
		authClient:     authClient,
		linkClient:     linkClient,
		checkoutClient: checkoutClient,
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
		r.Post("/auth/signup", s.handleSignup)
		r.Post("/auth/google/start", s.handleGoogleStart)
		r.Post("/auth/google/consume", s.handleGoogleConsume)
		r.Post("/auth/wallet/challenge", s.handleWalletChallenge)
		r.Post("/auth/wallet/verify", s.handleWalletVerify)
		r.Post("/auth/passkeys/login/options", s.handleBeginPasskeyLogin)
		r.Post("/auth/passkeys/login/verify", s.handleFinishPasskeyLogin)

		r.Get("/public/payment_links/{code}", s.handleGetPublicPaymentLink)
		r.Get("/public/products/{id}", s.handleGetPublicProduct)
		r.Get("/products/{id}", s.handleGetPublicProduct)
		r.Post("/public/payment_links/{id}/sessions", s.handleCreatePublicCheckoutSession)
		r.Get("/public/checkout/{session_id}", s.handleGetPublicCheckoutSession)
		r.Post("/public/checkout/{session_id}/events", s.handleCreatePublicCheckoutEvent)
		r.Post("/public/checkout/{session_id}/confirm", s.handleConfirmPublicCheckoutSession)
		r.Get("/public/product_images/{merchant_id}/{product_id}/{image_id}", s.handleGetProductImage)

		r.Group(func(r chi.Router) {
			r.Use(s.authn)

			r.Get("/me", s.handleMe)
			r.Get("/profile/security", s.handleGetProfileSecurity)
			r.Post("/profile/security/google/consume", s.handleLinkGoogle)
			r.Post("/profile/security/wallet/verify", s.handleLinkWallet)
			r.Delete("/profile/security/identities/{id}", s.handleDeleteIdentity)
			r.Delete("/profile/security/passkeys/{id}", s.handleDeletePasskey)
			r.Post("/auth/passkeys/register/options", s.handleBeginPasskeyRegistration)
			r.Post("/auth/passkeys/register/verify", s.handleFinishPasskeyRegistration)
			r.Get("/merchant/settings", s.handleGetMerchantSettings)
			r.Patch("/merchant/settings", s.handleUpdateMerchantSettings)

			r.Post("/api_keys", s.handleCreateAPIKey)
			r.Get("/api_keys", s.handleListAPIKeys)
			r.Post("/api_keys/{id}/revoke", s.handleRevokeAPIKey)

			r.Post("/products", s.handleCreateProduct)
			r.Get("/products", s.handleListProducts)
			r.Post("/products/{id}", s.handleUpdateProduct)
			r.Post("/products/{id}/image", s.handleUploadProductImage)
			r.Delete("/products/{id}", s.handleDeleteProduct)

			r.Post("/payment_links", s.handleCreatePaymentLink)
			r.Get("/payment_links", s.handleListPaymentLinks)
			r.Get("/payment_links/{id}", s.handleGetPaymentLink)
			r.Patch("/payment_links/{id}", s.handleUpdatePaymentLink)
			r.Post("/payment_links/{id}/archive", s.handleArchivePaymentLink)

			r.Post("/payment_links/{id}/sessions", s.handleCreateCheckoutSession)
			r.Get("/checkout/stats", s.handleCheckoutStats)
			r.Get("/checkout/{session_id}", s.handleGetCheckoutSession)
			r.Post("/checkout/{session_id}/confirm", s.handleConfirmCheckoutSession)
			r.Post("/mock_transfers", s.handleCreateMockTransfer)
			r.Get("/payments", s.handleListPayments)
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
		authorization := strings.TrimSpace(r.Header.Get("Authorization"))
		apiKey := strings.TrimSpace(r.Header.Get("X-API-Key"))
		if authorization == "" && apiKey == "" {
			httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "missing credentials", middleware.GetRequestID(r.Context()))
			return
		}

		ctx := s.rpcContext(r.Context())
		resp, err := s.authClient.ValidateCredential(ctx, &cpayv1.ValidateCredentialRequest{
			Authorization: authorization,
			ApiKey:        apiKey,
		})
		if err != nil {
			s.writeRPCError(w, r, err)
			return
		}
		if resp.GetPrincipal() == nil {
			httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "invalid credentials", middleware.GetRequestID(r.Context()))
			return
		}
		principal := resp.GetPrincipal()
		merchantID, err := ids.Parse(principal.GetMerchantId())
		if err != nil {
			httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "invalid merchant", middleware.GetRequestID(r.Context()))
			return
		}
		var userID *string
		if strings.TrimSpace(principal.GetUserId()) != "" {
			uid, err := ids.Parse(principal.GetUserId())
			if err == nil {
				userID = &uid
			}
		}
		var apiKeyID *string
		if strings.TrimSpace(principal.GetApiKeyId()) != "" {
			kid, err := ids.Parse(principal.GetApiKeyId())
			if err == nil {
				apiKeyID = &kid
			}
		}
		req := requester{
			MerchantID: merchantID,
			UserID:     userID,
			Role:       principal.GetRole(),
			APIKeyID:   apiKeyID,
			IsUser:     principal.GetIsUser(),
		}
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
	if !req.IsUser || req.UserID == nil {
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
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "invalid json: "+err.Error(), middleware.GetRequestID(r.Context()))
		return false
	}
	return true
}

func (s *Server) rpcContext(ctx context.Context) context.Context {
	return rpcx.WithOutboundMetadata(ctx, middleware.GetRequestID(ctx), middleware.GetIdempotencyKey(ctx))
}

func (s *Server) writeRPCError(w http.ResponseWriter, r *http.Request, err error) {
	code, reason, msg := rpcx.Parse(err)
	httpx.WriteError(w, rpcx.HTTPFromGRPC(code), reason, msg, middleware.GetRequestID(r.Context()))
}

func (s *Server) withIdempotency(w http.ResponseWriter, r *http.Request, merchantID string, endpoint string, fn func() (int, any, error)) {
	idempotencyKey := middleware.GetIdempotencyKey(r.Context())
	if idempotencyKey == "" {
		status, payload, err := fn()
		if err != nil {
			s.writeRPCError(w, r, err)
			return
		}
		httpx.WriteJSON(w, status, payload)
		return
	}

	var existingStatus int
	var existingResp []byte
	err := s.db.QueryRow(r.Context(), `
		SELECT response_status, response_body::text
		FROM platform.idempotency_keys
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
		s.writeRPCError(w, r, runErr)
		return
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to serialize response", middleware.GetRequestID(r.Context()))
		return
	}
	_, _ = s.db.Exec(r.Context(), `
		INSERT INTO platform.idempotency_keys(id, merchant_id, endpoint, idempotency_key, request_hash, response_status, response_body, expires_at)
		VALUES($1, $2, $3, $4, $5, $6, $7::jsonb, NOW() + INTERVAL '24 hours')
		ON CONFLICT (merchant_id, endpoint, idempotency_key) DO NOTHING
	`, ids.New(), merchantID, endpoint, idempotencyKey, hashRequest(r), status, string(payloadBytes))

	httpx.WriteJSON(w, status, payload)
}

func hashRequest(r *http.Request) string {
	sum := sha256.Sum256([]byte(r.Method + ":" + r.URL.Path))
	return hex.EncodeToString(sum[:])
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

type respWriter struct {
	http.ResponseWriter
	status int
}

func (rw *respWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}
