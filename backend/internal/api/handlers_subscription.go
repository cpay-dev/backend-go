package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/cpay-dev/backend/internal/auth"
	"github.com/cpay-dev/backend/internal/db"
	"github.com/cpay-dev/backend/internal/events"
)

// subscriptionResponse is the JSON-serialisable view of a Subscription row.
type subscriptionResponse struct {
	ID           string    `json:"id"`
	ShopID       string    `json:"shop_id"`
	Title        string    `json:"title"`
	Description  *string   `json:"description,omitempty"`
	Amount       string    `json:"amount"`
	TokenAddress string    `json:"token_address"`
	ChainID      int64     `json:"chain_id"`
	Period       string    `json:"period"`
	Active       bool      `json:"active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type publicSubscriptionResponse struct {
	subscriptionResponse
	MerchantAddress *string `json:"merchant_address"`
}

type subscriptionPaymentResponse struct {
	ID             string    `json:"id"`
	SubscriptionID string    `json:"subscription_id"`
	ShopID         string    `json:"shop_id"`
	PayerAddress   string    `json:"payer_address"`
	Amount         string    `json:"amount"`
	TokenAddress   string    `json:"token_address"`
	ChainID        int64     `json:"chain_id"`
	TxHash         string    `json:"tx_hash"`
	CreatedAt      time.Time `json:"created_at"`
}

func toSubscriptionResponse(s db.Subscription) subscriptionResponse {
	return subscriptionResponse{
		ID:           s.ID,
		ShopID:       s.ShopID,
		Title:        s.Title,
		Description:  s.Description,
		Amount:       numericToString(s.Amount),
		TokenAddress: s.TokenAddress,
		ChainID:      s.ChainID,
		Period:       string(s.Period),
		Active:       s.Active,
		CreatedAt:    s.CreatedAt,
		UpdatedAt:    s.UpdatedAt,
	}
}

func toSubscriptionPaymentResponse(p db.SubscriptionPayment) subscriptionPaymentResponse {
	return subscriptionPaymentResponse{
		ID:             p.ID,
		SubscriptionID: p.SubscriptionID,
		ShopID:         p.ShopID,
		PayerAddress:   p.PayerAddress,
		Amount:         numericToString(p.Amount),
		TokenAddress:   p.TokenAddress,
		ChainID:        p.ChainID,
		TxHash:         p.TxHash,
		CreatedAt:      p.CreatedAt,
	}
}

// createSubscription handles POST /api/subscriptions
func (h *handlers) createSubscription(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		Title        string      `json:"title"`
		Description  *string     `json:"description"`
		Amount       interface{} `json:"amount"`
		TokenAddress string      `json:"token_address"`
		ChainID      int64       `json:"chain_id"`
		Period       string      `json:"period"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Title == "" || req.Amount == nil || req.TokenAddress == "" || req.Period == "" {
		writeError(w, http.StatusBadRequest, "title, amount, token_address, period are required")
		return
	}

	shop, err := h.queries.GetShopByUserID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "shop not found — create a shop first")
		return
	}

	amount, err := parsePriceToNumeric(req.Amount)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid amount: "+err.Error())
		return
	}

	chainID := req.ChainID
	if chainID == 0 {
		chainID = 137
	}

	sub, err := h.queries.CreateSubscription(r.Context(), db.CreateSubscriptionParams{
		ShopID:       shop.ID,
		Title:        req.Title,
		Description:  req.Description,
		Amount:       amount,
		TokenAddress: req.TokenAddress,
		ChainID:      chainID,
		Period:       db.BillingPeriod(req.Period),
	})
	if err != nil {
		slog.Error("CreateSubscription failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create subscription")
		return
	}

	if evt, err := events.NewEvent(events.EventSubscriptionCreated, events.SubjectSubscriptions, map[string]string{"id": sub.ID}); err == nil {
		_ = h.eventPub.Publish(r.Context(), evt)
	}
	writeJSON(w, http.StatusCreated, toSubscriptionResponse(sub))
}

// listMySubscriptions handles GET /api/subscriptions
func (h *handlers) listMySubscriptions(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	subs, err := h.queries.ListMySubscriptions(r.Context(), userID)
	if err != nil {
		slog.Error("ListMySubscriptions failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list subscriptions")
		return
	}

	resp := make([]subscriptionResponse, 0, len(subs))
	for _, s := range subs {
		resp = append(resp, toSubscriptionResponse(s))
	}
	writeJSON(w, http.StatusOK, resp)
}

// getPublicSubscription handles GET /api/sub/:id (public)
func (h *handlers) getPublicSubscription(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	row, err := h.queries.GetPublicSubscription(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "subscription not found")
		return
	}

	// If CF address cache is empty, derive it from chain service and backfill.
	merchantAddr := row.MerchantAddress
	if merchantAddr == nil && row.MerchantWallet != "" {
		addr, err := h.resolveMerchantCFAddress(r.Context(), row.MerchantWallet)
		if err == nil {
			merchantAddr = &addr
		}
	}

	resp := publicSubscriptionResponse{
		subscriptionResponse: subscriptionResponse{
			ID:           row.ID,
			ShopID:       row.ShopID,
			Title:        row.Title,
			Description:  row.Description,
			Amount:       numericToString(row.Amount),
			TokenAddress: row.TokenAddress,
			ChainID:      row.ChainID,
			Period:       string(row.Period),
			Active:       row.Active,
			CreatedAt:    row.CreatedAt,
			UpdatedAt:    row.UpdatedAt,
		},
		MerchantAddress: merchantAddr,
	}
	writeJSON(w, http.StatusOK, resp)
}

// toggleSubscriptionActive handles PATCH /api/subscriptions/:id/active
func (h *handlers) toggleSubscriptionActive(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	userID := auth.UserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		Active bool `json:"active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	sub, err := h.queries.ToggleSubscriptionActive(r.Context(), db.ToggleSubscriptionActiveParams{
		ID:     id,
		Active: req.Active,
	})
	if err != nil {
		writeError(w, http.StatusNotFound, "subscription not found")
		return
	}

	writeJSON(w, http.StatusOK, toSubscriptionResponse(sub))
}

// deleteSubscription handles DELETE /api/subscriptions/:id
func (h *handlers) deleteSubscription(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	userID := auth.UserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	if err := h.queries.DeleteSubscription(r.Context(), id); err != nil {
		slog.Error("DeleteSubscription failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete subscription")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// checkSubscriptionPayment handles GET /api/sub/:id/payment?payer=0x... — check if a payer has already paid this subscription.
func (h *handlers) checkSubscriptionPayment(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "id")
	payer := r.URL.Query().Get("payer")
	if payer == "" {
		writeError(w, http.StatusBadRequest, "payer query param required")
		return
	}

	payment, err := h.queries.GetSubscriptionPaymentByPayer(r.Context(), db.GetSubscriptionPaymentByPayerParams{
		SubscriptionID: subID,
		PayerAddress:   payer,
	})
	if err != nil {
		writeJSON(w, http.StatusOK, nil)
		return
	}

	writeJSON(w, http.StatusOK, payment)
}

// recordSubscriptionPayment handles POST /api/sub/:id/payment (public — payer calls this)
func (h *handlers) recordSubscriptionPayment(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "id")

	var req struct {
		PayerAddress string      `json:"payer_address"`
		TokenAddress string      `json:"token_address"`
		ChainID      int64       `json:"chain_id"`
		Amount       interface{} `json:"amount"`
		TxHash       string      `json:"tx_hash"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.TxHash == "" || req.TokenAddress == "" || req.PayerAddress == "" || req.Amount == nil {
		writeError(w, http.StatusBadRequest, "payer_address, tx_hash, token_address, amount are required")
		return
	}

	// Fetch subscription to get shop_id
	sub, err := h.queries.GetSubscription(r.Context(), subID)
	if err != nil {
		writeError(w, http.StatusNotFound, "subscription not found")
		return
	}

	amount, err := parsePriceToNumeric(req.Amount)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid amount: "+err.Error())
		return
	}

	chainID := req.ChainID
	if chainID == 0 {
		chainID = sub.ChainID
	}

	payment, err := h.queries.CreateSubscriptionPayment(r.Context(), db.CreateSubscriptionPaymentParams{
		SubscriptionID: subID,
		ShopID:         sub.ShopID,
		PayerAddress:   req.PayerAddress,
		Amount:         amount,
		TokenAddress:   req.TokenAddress,
		ChainID:        chainID,
		TxHash:         req.TxHash,
	})
	if err != nil {
		slog.Error("CreateSubscriptionPayment failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to record subscription payment")
		return
	}

	if evt, err := events.NewEvent(events.EventSubscriptionPayment, events.SubjectSubscriptions, map[string]string{
		"id": payment.ID, "subscription_id": subID, "tx_hash": payment.TxHash,
	}); err == nil {
		_ = h.eventPub.Publish(r.Context(), evt)
	}
	writeJSON(w, http.StatusCreated, toSubscriptionPaymentResponse(payment))
}

// listSubscriptionPayments handles GET /api/subscriptions/:id/payments (authenticated)
func (h *handlers) listSubscriptionPayments(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "id")
	userID := auth.UserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	payments, err := h.queries.ListSubscriptionPayments(r.Context(), subID)
	if err != nil {
		slog.Error("ListSubscriptionPayments failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list subscription payments")
		return
	}

	resp := make([]subscriptionPaymentResponse, 0, len(payments))
	for _, p := range payments {
		resp = append(resp, toSubscriptionPaymentResponse(p))
	}
	writeJSON(w, http.StatusOK, resp)
}
