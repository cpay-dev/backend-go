package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/rs/zerolog/log"

	"github.com/cpay-dev/backend/internal/auth"
	"github.com/cpay-dev/backend/internal/db"
	"github.com/cpay-dev/backend/internal/events"
)

func (h *handlers) recordWithdrawal(w http.ResponseWriter, r *http.Request) {
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

	var req struct {
		TokenAddress string      `json:"token_address"`
		ChainID      int64       `json:"chain_id"`
		Amount       interface{} `json:"amount"`
		TxHash       string      `json:"tx_hash"`
	}
	dec := json.NewDecoder(r.Body)
	dec.UseNumber()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.TxHash == "" || req.TokenAddress == "" || req.Amount == nil {
		writeError(w, http.StatusBadRequest, "tx_hash, token_address, amount are required")
		return
	}

	amount, err := parsePriceToNumeric(req.Amount)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid amount: "+err.Error())
		return
	}

	withdrawal, err := h.queries.CreateWithdrawal(r.Context(), db.CreateWithdrawalParams{
		ShopID:       shop.ID,
		TokenAddress: req.TokenAddress,
		ChainID:      req.ChainID,
		Amount:       amount,
		TxHash:       req.TxHash,
	})
	if err != nil {
		log.Error().Err(err).Msg("CreateWithdrawal failed")
		writeError(w, http.StatusInternalServerError, "failed to record withdrawal")
		return
	}

	evt, err := events.NewEvent(events.EventWithdrawalCompleted, events.SubjectWithdrawals, map[string]string{
		"withdrawal_id": withdrawal.ID,
		"shop_id":       shop.ID,
		"token_address": req.TokenAddress,
		"chain_id":      fmt.Sprintf("%d", req.ChainID),
		"amount":        numericToString(withdrawal.Amount),
		"tx_hash":       req.TxHash,
	})
	if err == nil && h.eventPub != nil {
		if pubErr := h.eventPub.Publish(r.Context(), evt); pubErr != nil {
			log.Error().Err(pubErr).Msg("failed to publish withdrawal event")
		}
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"id":      withdrawal.ID,
		"tx_hash": withdrawal.TxHash,
	})
}
