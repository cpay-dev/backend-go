package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"github.com/cpay-dev/backend/internal/auth"
	"github.com/cpay-dev/backend/internal/db"
)

var allowedWebhookEvents = map[string]bool{
	"product.created":      true,
	"payment.received":     true,
	"subscription.created": true,
	"subscription.payment": true,
	"withdrawal.completed": true,
	"invoice.created":      true,
	"invoice.sent":         true,
	"invoice.paid":         true,
	"invoice.cancelled":    true,
}

type webhookResponse struct {
	ID         string   `json:"id"`
	ShopID     string   `json:"shop_id"`
	URL        string   `json:"url"`
	EventTypes []string `json:"event_types"`
	Active     bool     `json:"active"`
	CreatedAt  string   `json:"created_at"`
	UpdatedAt  string   `json:"updated_at"`
}

func toWebhookResponse(w db.Webhook) webhookResponse {
	return webhookResponse{
		ID:         w.ID,
		ShopID:     w.ShopID,
		URL:        w.Url,
		EventTypes: w.EventTypes,
		Active:     w.Active,
		CreatedAt:  w.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:  w.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func (h *handlers) createWebhook(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	role := auth.RoleFromContext(r.Context())
	if role != "merchant" {
		writeError(w, http.StatusForbidden, "only merchants can manage webhooks")
		return
	}

	shop, err := h.queries.GetShopByUserID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "create a shop first")
		return
	}

	var req struct {
		URL        string   `json:"url"`
		EventTypes []string `json:"event_types"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.URL == "" || len(req.EventTypes) == 0 {
		writeError(w, http.StatusBadRequest, "url and event_types are required")
		return
	}

	for _, et := range req.EventTypes {
		if !allowedWebhookEvents[et] {
			writeError(w, http.StatusBadRequest, "invalid event type: "+et)
			return
		}
	}

	secretBytes := make([]byte, 32)
	if _, err := rand.Read(secretBytes); err != nil {
		log.Error().Err(err).Msg("failed to generate webhook secret")
		writeError(w, http.StatusInternalServerError, "failed to generate secret")
		return
	}
	secret := hex.EncodeToString(secretBytes)

	webhook, err := h.queries.CreateWebhook(r.Context(), db.CreateWebhookParams{
		ShopID:     shop.ID,
		Url:        req.URL,
		Secret:     secret,
		EventTypes: req.EventTypes,
	})
	if err != nil {
		log.Error().Err(err).Msg("failed to create webhook")
		writeError(w, http.StatusInternalServerError, "failed to create webhook")
		return
	}

	// Return secret only on creation
	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"webhook": toWebhookResponse(webhook),
		"secret":  secret,
	})
}

func (h *handlers) listWebhooks(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	shop, err := h.queries.GetShopByUserID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "shop not found")
		return
	}

	webhooks, err := h.queries.ListWebhooksByShop(r.Context(), shop.ID)
	if err != nil {
		log.Error().Err(err).Msg("ListWebhooksByShop failed")
		writeError(w, http.StatusInternalServerError, "failed to list webhooks")
		return
	}

	result := make([]webhookResponse, len(webhooks))
	for i, wh := range webhooks {
		result[i] = toWebhookResponse(wh)
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *handlers) updateWebhook(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	webhookID := chi.URLParam(r, "id")

	shop, err := h.queries.GetShopByUserID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}

	existing, err := h.queries.GetWebhookByID(r.Context(), webhookID)
	if err != nil {
		writeError(w, http.StatusNotFound, "webhook not found")
		return
	}
	if existing.ShopID != shop.ID {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}

	var req struct {
		URL        string   `json:"url"`
		EventTypes []string `json:"event_types"`
		Active     bool     `json:"active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.URL == "" || len(req.EventTypes) == 0 {
		writeError(w, http.StatusBadRequest, "url and event_types are required")
		return
	}

	for _, et := range req.EventTypes {
		if !allowedWebhookEvents[et] {
			writeError(w, http.StatusBadRequest, "invalid event type: "+et)
			return
		}
	}

	updated, err := h.queries.UpdateWebhook(r.Context(), db.UpdateWebhookParams{
		ID:         webhookID,
		Url:        req.URL,
		EventTypes: req.EventTypes,
		Active:     req.Active,
	})
	if err != nil {
		log.Error().Err(err).Msg("failed to update webhook")
		writeError(w, http.StatusInternalServerError, "failed to update webhook")
		return
	}

	writeJSON(w, http.StatusOK, toWebhookResponse(updated))
}

func (h *handlers) deleteWebhook(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	webhookID := chi.URLParam(r, "id")

	shop, err := h.queries.GetShopByUserID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}

	existing, err := h.queries.GetWebhookByID(r.Context(), webhookID)
	if err != nil {
		writeError(w, http.StatusNotFound, "webhook not found")
		return
	}
	if existing.ShopID != shop.ID {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}

	if err := h.queries.DeleteWebhook(r.Context(), webhookID); err != nil {
		log.Error().Err(err).Msg("failed to delete webhook")
		writeError(w, http.StatusInternalServerError, "failed to delete webhook")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *handlers) listWebhookDeliveries(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	webhookID := chi.URLParam(r, "id")

	shop, err := h.queries.GetShopByUserID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}

	existing, err := h.queries.GetWebhookByID(r.Context(), webhookID)
	if err != nil {
		writeError(w, http.StatusNotFound, "webhook not found")
		return
	}
	if existing.ShopID != shop.ID {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}

	deliveries, err := h.queries.ListWebhookDeliveries(r.Context(), webhookID)
	if err != nil {
		log.Error().Err(err).Msg("ListWebhookDeliveries failed")
		writeError(w, http.StatusInternalServerError, "failed to list deliveries")
		return
	}

	writeJSON(w, http.StatusOK, deliveries)
}
