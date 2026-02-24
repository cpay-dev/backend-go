package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog/log"

	"github.com/cpay-dev/backend/internal/auth"
	"github.com/cpay-dev/backend/internal/db"
	"github.com/cpay-dev/backend/internal/events"
)

type invoiceResponse struct {
	ID             string          `json:"id"`
	ShopID         string          `json:"shop_id"`
	InvoiceNumber  int32           `json:"invoice_number"`
	Title          string          `json:"title"`
	Memo           *string         `json:"memo,omitempty"`
	Amount         string          `json:"amount"`
	TokenAddress   string          `json:"token_address"`
	ChainID        int32           `json:"chain_id"`
	Status         string          `json:"status"`
	RecipientEmail *string         `json:"recipient_email,omitempty"`
	DueDate        *string         `json:"due_date,omitempty"`
	LineItems      json.RawMessage `json:"line_items,omitempty"`
	Notes          *string         `json:"notes,omitempty"`
	PayerAddress   *string         `json:"payer_address,omitempty"`
	TxHash         *string         `json:"tx_hash,omitempty"`
	PaidAt         *string         `json:"paid_at,omitempty"`
	CreatedAt      string          `json:"created_at"`
	UpdatedAt      string          `json:"updated_at"`
}

type publicInvoiceResponse struct {
	invoiceResponse
	MerchantAddress *string `json:"merchant_address"`
}

func toInvoiceResponse(i db.Invoice) invoiceResponse {
	resp := invoiceResponse{
		ID:             i.ID,
		ShopID:         i.ShopID,
		InvoiceNumber:  i.InvoiceNumber,
		Title:          i.Title,
		Memo:           i.Memo,
		Amount:         numericToString(i.Amount),
		TokenAddress:   i.TokenAddress,
		ChainID:        i.ChainID,
		Status:         string(i.Status),
		RecipientEmail: i.RecipientEmail,
		Notes:          i.Notes,
		PayerAddress:   i.PayerAddress,
		TxHash:         i.TxHash,
		CreatedAt:      i.CreatedAt.Format(time.RFC3339),
		UpdatedAt:      i.UpdatedAt.Format(time.RFC3339),
	}
	if i.DueDate.Valid {
		s := i.DueDate.Time.Format(time.RFC3339)
		resp.DueDate = &s
	}
	if len(i.LineItems) > 0 {
		resp.LineItems = i.LineItems
	}
	if i.PaidAt.Valid {
		s := i.PaidAt.Time.Format(time.RFC3339)
		resp.PaidAt = &s
	}
	return resp
}

func toInvoiceResponseList(invoices []db.Invoice) []invoiceResponse {
	result := make([]invoiceResponse, len(invoices))
	for i, inv := range invoices {
		result[i] = toInvoiceResponse(inv)
	}
	return result
}

func (h *handlers) createInvoice(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	role := auth.RoleFromContext(r.Context())

	if role != "merchant" {
		writeError(w, http.StatusForbidden, "only merchants can create invoices")
		return
	}

	shop, err := h.queries.GetShopByUserID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "create a shop first")
		return
	}

	var req struct {
		Title          string          `json:"title"`
		Memo           *string         `json:"memo"`
		Amount         interface{}     `json:"amount"`
		TokenAddress   string          `json:"token_address"`
		ChainID        int32           `json:"chain_id"`
		RecipientEmail *string         `json:"recipient_email"`
		DueDate        *string         `json:"due_date"`
		LineItems      json.RawMessage `json:"line_items"`
		Notes          *string         `json:"notes"`
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
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid amount: %v", err))
		return
	}

	if req.TokenAddress == "" {
		req.TokenAddress = "0x41E94Eb019C0762f9Bfcf9Fb1E58725BfB0e7582"
	}
	if req.ChainID == 0 {
		req.ChainID = 80002
	}

	var dueDate pgtype.Timestamptz
	if req.DueDate != nil {
		t, err := time.Parse(time.RFC3339, *req.DueDate)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid due_date format (use RFC3339)")
			return
		}
		dueDate = pgtype.Timestamptz{Time: t, Valid: true}
	}

	var lineItems []byte
	if len(req.LineItems) > 0 {
		lineItems = req.LineItems
	}

	invoice, err := h.queries.CreateInvoice(r.Context(), db.CreateInvoiceParams{
		ShopID:         shop.ID,
		Title:          req.Title,
		Memo:           req.Memo,
		Amount:         amount,
		TokenAddress:   req.TokenAddress,
		ChainID:        req.ChainID,
		Status:         db.InvoiceStatusDraft,
		RecipientEmail: req.RecipientEmail,
		DueDate:        dueDate,
		LineItems:      lineItems,
		Notes:          req.Notes,
	})
	if err != nil {
		log.Error().Err(err).Msg("failed to create invoice")
		writeError(w, http.StatusInternalServerError, "failed to create invoice")
		return
	}

	evt, err := events.NewEvent(events.EventInvoiceCreated, events.SubjectInvoices, map[string]string{
		"invoice_id": invoice.ID,
		"shop_id":    shop.ID,
		"title":      invoice.Title,
		"amount":     numericToString(invoice.Amount),
	})
	if err == nil {
		if pubErr := h.eventPub.Publish(r.Context(), evt); pubErr != nil {
			log.Error().Err(pubErr).Msg("failed to publish invoice created event")
		}
	}

	writeJSON(w, http.StatusCreated, toInvoiceResponse(invoice))
}

func (h *handlers) listMyInvoices(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	shop, err := h.queries.GetShopByUserID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "shop not found")
		return
	}

	invoices, err := h.queries.ListShopInvoices(r.Context(), shop.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list invoices")
		return
	}

	writeJSON(w, http.StatusOK, toInvoiceResponseList(invoices))
}

func (h *handlers) getInvoice(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	invoiceID := chi.URLParam(r, "id")

	shop, err := h.queries.GetShopByUserID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}

	invoice, err := h.queries.GetInvoiceByID(r.Context(), invoiceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "invoice not found")
		return
	}
	if invoice.ShopID != shop.ID {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}

	writeJSON(w, http.StatusOK, toInvoiceResponse(invoice))
}

func (h *handlers) updateInvoice(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	invoiceID := chi.URLParam(r, "id")

	shop, err := h.queries.GetShopByUserID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}

	existing, err := h.queries.GetInvoiceByID(r.Context(), invoiceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "invoice not found")
		return
	}
	if existing.ShopID != shop.ID {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}
	if existing.Status != db.InvoiceStatusDraft {
		writeError(w, http.StatusBadRequest, "only draft invoices can be edited")
		return
	}

	var req struct {
		Title          string          `json:"title"`
		Memo           *string         `json:"memo"`
		Amount         interface{}     `json:"amount"`
		TokenAddress   string          `json:"token_address"`
		ChainID        int32           `json:"chain_id"`
		RecipientEmail *string         `json:"recipient_email"`
		DueDate        *string         `json:"due_date"`
		LineItems      json.RawMessage `json:"line_items"`
		Notes          *string         `json:"notes"`
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
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid amount: %v", err))
		return
	}

	if req.TokenAddress == "" {
		req.TokenAddress = existing.TokenAddress
	}
	if req.ChainID == 0 {
		req.ChainID = existing.ChainID
	}

	var dueDate pgtype.Timestamptz
	if req.DueDate != nil {
		t, err := time.Parse(time.RFC3339, *req.DueDate)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid due_date format (use RFC3339)")
			return
		}
		dueDate = pgtype.Timestamptz{Time: t, Valid: true}
	}

	var lineItems []byte
	if len(req.LineItems) > 0 {
		lineItems = req.LineItems
	}

	updated, err := h.queries.UpdateInvoice(r.Context(), db.UpdateInvoiceParams{
		ID:             invoiceID,
		Title:          req.Title,
		Memo:           req.Memo,
		Amount:         amount,
		TokenAddress:   req.TokenAddress,
		ChainID:        req.ChainID,
		RecipientEmail: req.RecipientEmail,
		DueDate:        dueDate,
		LineItems:      lineItems,
		Notes:          req.Notes,
	})
	if err != nil {
		log.Error().Err(err).Msg("failed to update invoice")
		writeError(w, http.StatusInternalServerError, "failed to update invoice")
		return
	}

	evt, err := events.NewEvent(events.EventInvoiceUpdated, events.SubjectInvoices, map[string]string{
		"invoice_id": updated.ID,
		"shop_id":    shop.ID,
	})
	if err == nil {
		if pubErr := h.eventPub.Publish(r.Context(), evt); pubErr != nil {
			log.Error().Err(pubErr).Msg("failed to publish invoice updated event")
		}
	}

	writeJSON(w, http.StatusOK, toInvoiceResponse(updated))
}

func (h *handlers) sendInvoice(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	invoiceID := chi.URLParam(r, "id")

	shop, err := h.queries.GetShopByUserID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}

	existing, err := h.queries.GetInvoiceByID(r.Context(), invoiceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "invoice not found")
		return
	}
	if existing.ShopID != shop.ID {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}
	if existing.Status != db.InvoiceStatusDraft {
		writeError(w, http.StatusBadRequest, "only draft invoices can be sent")
		return
	}

	updated, err := h.queries.UpdateInvoiceStatus(r.Context(), db.UpdateInvoiceStatusParams{
		ID:     invoiceID,
		Status: db.InvoiceStatusSent,
	})
	if err != nil {
		log.Error().Err(err).Msg("failed to send invoice")
		writeError(w, http.StatusInternalServerError, "failed to send invoice")
		return
	}

	evt, err := events.NewEvent(events.EventInvoiceSent, events.SubjectInvoices, map[string]string{
		"invoice_id": updated.ID,
		"shop_id":    shop.ID,
		"title":      updated.Title,
		"amount":     numericToString(updated.Amount),
	})
	if err == nil {
		if pubErr := h.eventPub.Publish(r.Context(), evt); pubErr != nil {
			log.Error().Err(pubErr).Msg("failed to publish invoice sent event")
		}
	}

	writeJSON(w, http.StatusOK, toInvoiceResponse(updated))
}

func (h *handlers) cancelInvoice(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	invoiceID := chi.URLParam(r, "id")

	shop, err := h.queries.GetShopByUserID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}

	existing, err := h.queries.GetInvoiceByID(r.Context(), invoiceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "invoice not found")
		return
	}
	if existing.ShopID != shop.ID {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}
	if existing.Status != db.InvoiceStatusSent {
		writeError(w, http.StatusBadRequest, "only sent invoices can be cancelled")
		return
	}

	updated, err := h.queries.UpdateInvoiceStatus(r.Context(), db.UpdateInvoiceStatusParams{
		ID:     invoiceID,
		Status: db.InvoiceStatusCancelled,
	})
	if err != nil {
		log.Error().Err(err).Msg("failed to cancel invoice")
		writeError(w, http.StatusInternalServerError, "failed to cancel invoice")
		return
	}

	evt, err := events.NewEvent(events.EventInvoiceCancelled, events.SubjectInvoices, map[string]string{
		"invoice_id": updated.ID,
		"shop_id":    shop.ID,
	})
	if err == nil {
		if pubErr := h.eventPub.Publish(r.Context(), evt); pubErr != nil {
			log.Error().Err(pubErr).Msg("failed to publish invoice cancelled event")
		}
	}

	writeJSON(w, http.StatusOK, toInvoiceResponse(updated))
}

func (h *handlers) deleteInvoice(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	invoiceID := chi.URLParam(r, "id")

	shop, err := h.queries.GetShopByUserID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}

	existing, err := h.queries.GetInvoiceByID(r.Context(), invoiceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "invoice not found")
		return
	}
	if existing.ShopID != shop.ID {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}
	if existing.Status != db.InvoiceStatusDraft {
		writeError(w, http.StatusBadRequest, "only draft invoices can be deleted")
		return
	}

	if err := h.queries.DeleteInvoice(r.Context(), invoiceID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete invoice")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *handlers) getPublicInvoice(w http.ResponseWriter, r *http.Request) {
	invoiceID := chi.URLParam(r, "id")

	invoice, err := h.queries.GetPublicInvoice(r.Context(), invoiceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "invoice not found")
		return
	}

	merchantAddr := invoice.MerchantAddress
	if merchantAddr == nil && invoice.MerchantWallet != "" {
		addr, err := h.resolveMerchantCFAddress(r.Context(), invoice.MerchantWallet)
		if err == nil {
			merchantAddr = &addr
		}
	}

	resp := publicInvoiceResponse{
		invoiceResponse: invoiceResponse{
			ID:             invoice.ID,
			ShopID:         invoice.ShopID,
			InvoiceNumber:  invoice.InvoiceNumber,
			Title:          invoice.Title,
			Memo:           invoice.Memo,
			Amount:         numericToString(invoice.Amount),
			TokenAddress:   invoice.TokenAddress,
			ChainID:        invoice.ChainID,
			Status:         string(invoice.Status),
			RecipientEmail: invoice.RecipientEmail,
			Notes:          invoice.Notes,
			PayerAddress:   invoice.PayerAddress,
			TxHash:         invoice.TxHash,
			CreatedAt:      invoice.CreatedAt.Format(time.RFC3339),
			UpdatedAt:      invoice.UpdatedAt.Format(time.RFC3339),
		},
		MerchantAddress: merchantAddr,
	}
	if invoice.DueDate.Valid {
		s := invoice.DueDate.Time.Format(time.RFC3339)
		resp.DueDate = &s
	}
	if len(invoice.LineItems) > 0 {
		resp.LineItems = invoice.LineItems
	}
	if invoice.PaidAt.Valid {
		s := invoice.PaidAt.Time.Format(time.RFC3339)
		resp.PaidAt = &s
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *handlers) recordInvoicePayment(w http.ResponseWriter, r *http.Request) {
	invoiceID := chi.URLParam(r, "id")

	invoice, err := h.queries.GetPublicInvoice(r.Context(), invoiceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "invoice not found")
		return
	}
	if invoice.Status != db.InvoiceStatusSent {
		writeError(w, http.StatusBadRequest, "invoice is not payable")
		return
	}

	var req struct {
		PayerAddress string `json:"payer_address"`
		TxHash       string `json:"tx_hash"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.TxHash == "" {
		writeError(w, http.StatusBadRequest, "tx_hash is required")
		return
	}

	updated, err := h.queries.RecordInvoicePayment(r.Context(), db.RecordInvoicePaymentParams{
		ID:           invoiceID,
		PayerAddress: &req.PayerAddress,
		TxHash:       &req.TxHash,
	})
	if err != nil {
		log.Error().Err(err).Msg("failed to record invoice payment")
		writeError(w, http.StatusInternalServerError, "failed to record payment")
		return
	}

	evt, err := events.NewEvent(events.EventInvoicePaid, events.SubjectInvoices, map[string]string{
		"invoice_id":   updated.ID,
		"shop_id":      updated.ShopID,
		"title":        updated.Title,
		"amount":       numericToString(updated.Amount),
		"tx_hash":      req.TxHash,
		"payer_address": req.PayerAddress,
	})
	if err == nil {
		if pubErr := h.eventPub.Publish(r.Context(), evt); pubErr != nil {
			log.Error().Err(pubErr).Msg("failed to publish invoice paid event")
		}
	}

	writeJSON(w, http.StatusCreated, toInvoiceResponse(updated))
}
