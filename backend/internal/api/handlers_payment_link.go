package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/cpay-dev/backend/internal/auth"
	"github.com/cpay-dev/backend/internal/db"
	"github.com/cpay-dev/backend/internal/events"
)

type paymentLinkResponse struct {
	ID           string  `json:"id"`
	ShopID       string  `json:"shop_id"`
	Title        string  `json:"title"`
	Description  *string `json:"description"`
	Amount       string  `json:"amount"`
	TokenAddress string  `json:"token_address"`
	ChainID      int32   `json:"chain_id"`
	MaxUses      *int32  `json:"max_uses"`
	UseCount     int32   `json:"use_count"`
	Active       bool    `json:"active"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`
}

type publicPaymentLinkResponse struct {
	paymentLinkResponse
	MerchantAddress string `json:"merchant_address"`
}

func toPaymentLinkResponse(l db.PaymentLink) paymentLinkResponse {
	return paymentLinkResponse{
		ID:           l.ID,
		ShopID:       l.ShopID,
		Title:        l.Title,
		Description:  l.Description,
		Amount:       numericToString(l.Amount),
		TokenAddress: l.TokenAddress,
		ChainID:      l.ChainID,
		MaxUses:      l.MaxUses,
		UseCount:     l.UseCount,
		Active:       l.Active,
		CreatedAt:    l.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:    l.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func toPaymentLinkResponseList(links []db.PaymentLink) []paymentLinkResponse {
	result := make([]paymentLinkResponse, len(links))
	for i, l := range links {
		result[i] = toPaymentLinkResponse(l)
	}
	return result
}

func (h *handlers) createPaymentLink(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	role := auth.RoleFromContext(r.Context())

	if role != "merchant" {
		writeError(w, http.StatusForbidden, "only merchants can create payment links")
		return
	}

	shop, err := h.queries.GetShopByUserID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "create a shop first")
		return
	}

	var req struct {
		Title        string      `json:"title"`
		Description  *string     `json:"description"`
		Amount       interface{} `json:"amount"`
		TokenAddress string      `json:"token_address"`
		ChainID      int32       `json:"chain_id"`
		MaxUses      *int32      `json:"max_uses"`
	}
	dec := json.NewDecoder(r.Body)
	dec.UseNumber()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	if req.Amount == nil {
		writeError(w, http.StatusBadRequest, "amount is required")
		return
	}

	amount, err := parsePriceToNumeric(req.Amount)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid amount: "+err.Error())
		return
	}

	if req.TokenAddress == "" {
		req.TokenAddress = "0x3c499c542cEF5E3811e1192ce70d8cC03d5c3359"
	}
	if req.ChainID == 0 {
		req.ChainID = 137
	}

	link, err := h.queries.CreatePaymentLink(r.Context(), db.CreatePaymentLinkParams{
		ShopID:       shop.ID,
		Title:        req.Title,
		Description:  req.Description,
		Amount:       amount,
		TokenAddress: req.TokenAddress,
		ChainID:      req.ChainID,
		MaxUses:      req.MaxUses,
	})
	if err != nil {
		slog.Error("failed to create payment link", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create payment link")
		return
	}

	evt, err := events.NewEvent(events.EventPaymentLinkCreated, events.SubjectPaymentLinks, map[string]string{
		"payment_link_id": link.ID,
		"shop_id":         shop.ID,
	})
	if err == nil {
		if pubErr := h.eventPub.Publish(r.Context(), evt); pubErr != nil {
			slog.Error("failed to publish payment link created event", "error", pubErr)
		}
	}

	writeJSON(w, http.StatusCreated, toPaymentLinkResponse(link))
}

func (h *handlers) listMyPaymentLinks(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	shop, err := h.queries.GetShopByUserID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "shop not found")
		return
	}

	links, err := h.queries.ListShopPaymentLinks(r.Context(), shop.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list payment links")
		return
	}

	writeJSON(w, http.StatusOK, toPaymentLinkResponseList(links))
}

func (h *handlers) getPublicPaymentLink(w http.ResponseWriter, r *http.Request) {
	linkID := chi.URLParam(r, "id")

	link, err := h.queries.GetPublicPaymentLink(r.Context(), linkID)
	if err != nil || !link.Active {
		writeError(w, http.StatusNotFound, "payment link not found")
		return
	}

	if link.MaxUses != nil && link.UseCount >= *link.MaxUses {
		writeError(w, http.StatusGone, "payment link has reached its maximum uses")
		return
	}

	resp := publicPaymentLinkResponse{
		paymentLinkResponse: paymentLinkResponse{
			ID:           link.ID,
			ShopID:       link.ShopID,
			Title:        link.Title,
			Description:  link.Description,
			Amount:       numericToString(link.Amount),
			TokenAddress: link.TokenAddress,
			ChainID:      link.ChainID,
			MaxUses:      link.MaxUses,
			UseCount:     link.UseCount,
			Active:       link.Active,
			CreatedAt:    link.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			UpdatedAt:    link.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		},
		MerchantAddress: link.MerchantAddress,
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *handlers) recordPaymentLinkUse(w http.ResponseWriter, r *http.Request) {
	linkID := chi.URLParam(r, "id")

	link, err := h.queries.GetPaymentLinkByID(r.Context(), linkID)
	if err != nil || !link.Active {
		writeError(w, http.StatusNotFound, "payment link not found")
		return
	}

	if link.MaxUses != nil && link.UseCount >= *link.MaxUses {
		writeError(w, http.StatusGone, "payment link has reached its maximum uses")
		return
	}

	updated, err := h.queries.IncrementPaymentLinkUseCount(r.Context(), linkID)
	if err != nil {
		slog.Error("failed to increment payment link use count", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to record use")
		return
	}

	evt, err := events.NewEvent(events.EventPaymentLinkUsed, events.SubjectPaymentLinks, map[string]string{
		"payment_link_id": linkID,
		"shop_id":         link.ShopID,
	})
	if err == nil {
		if pubErr := h.eventPub.Publish(r.Context(), evt); pubErr != nil {
			slog.Error("failed to publish payment link used event", "error", pubErr)
		}
	}

	writeJSON(w, http.StatusOK, map[string]int32{"use_count": updated.UseCount})
}

func (h *handlers) setPaymentLinkActive(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	linkID := chi.URLParam(r, "id")

	shop, err := h.queries.GetShopByUserID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}

	existing, err := h.queries.GetPaymentLinkByID(r.Context(), linkID)
	if err != nil {
		writeError(w, http.StatusNotFound, "payment link not found")
		return
	}
	if existing.ShopID != shop.ID {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}

	var req struct {
		Active bool `json:"active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.queries.SetPaymentLinkActive(r.Context(), db.SetPaymentLinkActiveParams{
		ID:     linkID,
		Active: req.Active,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update payment link")
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"active": req.Active})
}

func (h *handlers) deletePaymentLink(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	linkID := chi.URLParam(r, "id")

	shop, err := h.queries.GetShopByUserID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}

	existing, err := h.queries.GetPaymentLinkByID(r.Context(), linkID)
	if err != nil {
		writeError(w, http.StatusNotFound, "payment link not found")
		return
	}
	if existing.ShopID != shop.ID {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}

	if err := h.queries.DeletePaymentLink(r.Context(), linkID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete payment link")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
