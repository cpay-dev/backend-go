package api

import (
	"encoding/json"
	"net/http"
	"strings"

	cryptox "github.com/cpay-dev/cpay/internal/shared/crypto"
	"github.com/cpay-dev/cpay/internal/shared/httpx"
	"github.com/cpay-dev/cpay/internal/shared/ids"
	"github.com/cpay-dev/cpay/internal/shared/middleware"
	"github.com/cpay-dev/cpay/internal/shared/random"
)

type createWebhookEndpointRequest struct {
	URL         string   `json:"url"`
	Description string   `json:"description,omitempty"`
	Events      []string `json:"events,omitempty"`
	MaxRetries  *int     `json:"max_retries,omitempty"`
}

func (s *Server) handleCreateWebhookEndpoint(w http.ResponseWriter, r *http.Request) {
	reqAuth, ok := mustRequester(w, r)
	if !ok {
		return
	}
	var req createWebhookEndpointRequest
	if !s.parseJSON(w, r, &req) {
		return
	}
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(req.URL)), "http://") && !strings.HasPrefix(strings.ToLower(strings.TrimSpace(req.URL)), "https://") {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "url must be http or https", middleware.GetRequestID(r.Context()))
		return
	}
	if len(req.Events) == 0 {
		req.Events = []string{"*"}
	}
	maxRetries := s.cfg.WebhookMaxRetries
	if req.MaxRetries != nil && *req.MaxRetries > 0 {
		maxRetries = *req.MaxRetries
	}

	s.withIdempotency(w, r, reqAuth.MerchantID, "/v1/webhook_endpoints", func() (int, any, error) {
		secret, err := random.PrefixedToken("whsec_", 24)
		if err != nil {
			return 0, nil, err
		}
		encryptedSecret, err := cryptox.EncryptString(s.encryptKey, secret)
		if err != nil {
			return 0, nil, err
		}
		eventsRaw, _ := json.Marshal(req.Events)
		id := ids.New()
		_, err = s.db.Exec(r.Context(), `
			INSERT INTO webhook_endpoints(
				id, merchant_id, url, description, enabled, events, secret_encrypted, max_retries, created_at, updated_at
			)
			VALUES($1, $2, $3, $4, TRUE, $5::jsonb, $6, $7, NOW(), NOW())
		`, id, reqAuth.MerchantID, strings.TrimSpace(req.URL), req.Description, string(eventsRaw), encryptedSecret, maxRetries)
		if err != nil {
			return 0, nil, err
		}
		_ = s.enqueueEvent(r.Context(), "webhook_endpoint", id, reqAuth.MerchantID, "webhook_endpoint.created", map[string]any{
			"webhook_endpoint_id": id,
			"url":                 strings.TrimSpace(req.URL),
			"events":              req.Events,
		})
		return http.StatusCreated, map[string]any{
			"id":          id,
			"url":         strings.TrimSpace(req.URL),
			"description": req.Description,
			"events":      req.Events,
			"max_retries": maxRetries,
			"secret":      secret,
		}, nil
	})
}
