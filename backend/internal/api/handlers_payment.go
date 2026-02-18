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

// paymentResponse is the JSON-serialisable view of a Payment row.
type paymentResponse struct {
	ID            string    `json:"id"`
	ShopID        string    `json:"shop_id"`
	Kind          string    `json:"kind"`
	ProductID     *string   `json:"product_id,omitempty"`
	PaymentLinkID *string   `json:"payment_link_id,omitempty"`
	PayerAddress  *string   `json:"payer_address,omitempty"`
	TokenAddress  string    `json:"token_address"`
	ChainID       int64     `json:"chain_id"`
	Amount        string    `json:"amount"`
	TxHash        string    `json:"tx_hash"`
	CreatedAt     time.Time `json:"created_at"`
}

func toPaymentResponse(p db.Payment) paymentResponse {
	return paymentResponse{
		ID:            p.ID,
		ShopID:        p.ShopID,
		Kind:          string(p.Kind),
		ProductID:     p.ProductID,
		PaymentLinkID: p.PaymentLinkID,
		PayerAddress:  p.PayerAddress,
		TokenAddress:  p.TokenAddress,
		ChainID:       p.ChainID,
		Amount:        numericToString(p.Amount),
		TxHash:        p.TxHash,
		CreatedAt:     p.CreatedAt,
	}
}

func publishPaymentEvent(h *handlers, r *http.Request, payment db.Payment) {
	evt, err := events.NewEvent(events.EventPaymentRecorded, events.SubjectPayments, map[string]string{
		"payment_id": payment.ID,
		"shop_id":    payment.ShopID,
		"kind":       string(payment.Kind),
		"tx_hash":    payment.TxHash,
	})
	if err == nil {
		_ = h.eventPub.Publish(r.Context(), evt)
	}
}

// recordPaymentLinkPayment handles POST /api/pay/:id/payment — anonymous (no JWT) payment
// recording for payment links (payer may not be a registered user).
func (h *handlers) recordPaymentLinkPayment(w http.ResponseWriter, r *http.Request) {
	linkID := chi.URLParam(r, "id")

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
	if req.TxHash == "" || req.TokenAddress == "" || req.Amount == nil {
		writeError(w, http.StatusBadRequest, "tx_hash, token_address, amount are required")
		return
	}

	// Fetch link to get shop_id
	link, err := h.queries.GetPublicPaymentLink(r.Context(), linkID)
	if err != nil {
		writeError(w, http.StatusNotFound, "payment link not found")
		return
	}

	amount, err := parsePriceToNumeric(req.Amount)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid amount: "+err.Error())
		return
	}

	payer := req.PayerAddress
	linkIDRef := linkID
	payment, err := h.queries.CreatePayment(r.Context(), db.CreatePaymentParams{
		ShopID:        link.ShopID,
		Kind:          db.PaymentKindPaymentLink,
		ProductID:     nil,
		PaymentLinkID: &linkIDRef,
		PayerAddress:  &payer,
		TokenAddress:  req.TokenAddress,
		ChainID:       req.ChainID,
		Amount:        amount,
		TxHash:        req.TxHash,
	})
	if err != nil {
		slog.Error("CreatePayment (link) failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to record payment")
		return
	}

	publishPaymentEvent(h, r, payment)
	writeJSON(w, http.StatusCreated, toPaymentResponse(payment))
}

// checkProductPayment handles GET /api/p/:id/payment?payer=0x... — check if a payer has already paid.
func (h *handlers) checkProductPayment(w http.ResponseWriter, r *http.Request) {
	productID := chi.URLParam(r, "id")
	payer := r.URL.Query().Get("payer")
	if payer == "" {
		writeError(w, http.StatusBadRequest, "payer query param required")
		return
	}

	payment, err := h.queries.GetPaymentByProductAndPayer(r.Context(), db.GetPaymentByProductAndPayerParams{
		ProductID:    &productID,
		PayerAddress: &payer,
	})
	if err != nil {
		// No payment found — return null, not an error
		writeJSON(w, http.StatusOK, nil)
		return
	}

	writeJSON(w, http.StatusOK, toPaymentResponse(payment))
}

// recordProductPayment handles POST /api/p/:id/payment — anonymous product purchase recording.
func (h *handlers) recordProductPayment(w http.ResponseWriter, r *http.Request) {
	productID := chi.URLParam(r, "id")

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
	if req.TxHash == "" || req.TokenAddress == "" || req.Amount == nil {
		writeError(w, http.StatusBadRequest, "tx_hash, token_address, amount are required")
		return
	}

	// Fetch product to get shop_id
	product, err := h.queries.GetPublicProduct(r.Context(), productID)
	if err != nil {
		writeError(w, http.StatusNotFound, "product not found")
		return
	}

	amount, err := parsePriceToNumeric(req.Amount)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid amount: "+err.Error())
		return
	}

	payer := req.PayerAddress
	pidRef := productID
	payment, err := h.queries.CreatePayment(r.Context(), db.CreatePaymentParams{
		ShopID:        product.ShopID,
		Kind:          db.PaymentKindProduct,
		ProductID:     &pidRef,
		PaymentLinkID: nil,
		PayerAddress:  &payer,
		TokenAddress:  req.TokenAddress,
		ChainID:       req.ChainID,
		Amount:        amount,
		TxHash:        req.TxHash,
	})
	if err != nil {
		slog.Error("CreatePayment (product) failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to record payment")
		return
	}

	publishPaymentEvent(h, r, payment)
	writeJSON(w, http.StatusCreated, toPaymentResponse(payment))
}

// listMyPayments handles GET /api/payments — returns all payments for the merchant's shop.
func (h *handlers) listMyPayments(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	shop, err := h.queries.GetShopByUserID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "shop not found")
		return
	}

	payments, err := h.queries.ListPaymentsByShop(r.Context(), shop.ID)
	if err != nil {
		slog.Error("ListPaymentsByShop failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list payments")
		return
	}

	resp := make([]paymentResponse, 0, len(payments))
	for _, p := range payments {
		resp = append(resp, toPaymentResponse(p))
	}
	writeJSON(w, http.StatusOK, resp)
}

// listProductPayments handles GET /api/products/:id/payments
func (h *handlers) listProductPayments(w http.ResponseWriter, r *http.Request) {
	productID := chi.URLParam(r, "id")

	userID := auth.UserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// Verify ownership
	product, err := h.queries.GetProductByID(r.Context(), productID)
	if err != nil {
		writeError(w, http.StatusNotFound, "product not found")
		return
	}
	shop, err := h.queries.GetShopByUserID(r.Context(), userID)
	if err != nil || shop.ID != product.ShopID {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	payments, err := h.queries.ListPaymentsByProduct(r.Context(), &productID)
	if err != nil {
		slog.Error("ListPaymentsByProduct failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list payments")
		return
	}

	resp := make([]paymentResponse, 0, len(payments))
	for _, p := range payments {
		resp = append(resp, toPaymentResponse(p))
	}
	writeJSON(w, http.StatusOK, resp)
}
