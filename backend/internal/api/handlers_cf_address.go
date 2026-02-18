package api

import (
	"log/slog"
	"net/http"

	"github.com/cpay-dev/backend/internal/auth"
	"github.com/cpay-dev/backend/internal/db"
)

// getMyCFAddress returns (and caches) the counterfactual account address for the
// authenticated merchant on chain 137 (Polygon).
func (h *handlers) getMyCFAddress(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	wallet := auth.WalletFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	const chainID = 137

	// Try cache first
	existing, err := h.queries.GetCounterfactualAccount(r.Context(), db.GetCounterfactualAccountParams{
		UserID:  userID,
		ChainID: chainID,
	})
	if err == nil {
		writeJSON(w, http.StatusOK, map[string]string{"address": existing.Address})
		return
	}

	// Derive via chain service
	addr, err := h.chainClient.GetCounterfactualAddress(r.Context(), chainID, wallet)
	if err != nil {
		slog.Error("GetCounterfactualAddress failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to derive counterfactual address")
		return
	}

	// Cache it
	_, err = h.queries.UpsertCounterfactualAccount(r.Context(), db.UpsertCounterfactualAccountParams{
		UserID:  userID,
		ChainID: chainID,
		Address: addr,
	})
	if err != nil {
		slog.Warn("failed to cache CF address", "error", err)
		// Non-fatal: still return the derived address
	}

	writeJSON(w, http.StatusOK, map[string]string{"address": addr})
}

// getCFBalance returns the ERC-20 token balance for the merchant's counterfactual account.
func (h *handlers) getCFBalance(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	wallet := auth.WalletFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	tokenAddress := r.URL.Query().Get("token")
	if tokenAddress == "" {
		writeError(w, http.StatusBadRequest, "token query param required")
		return
	}

	const chainID = 137

	// Get CF address (from cache or derive)
	cfAcct, err := h.queries.GetCounterfactualAccount(r.Context(), db.GetCounterfactualAccountParams{
		UserID:  userID,
		ChainID: chainID,
	})
	cfAddr := ""
	if err != nil {
		// Not cached — derive it
		addr, err2 := h.chainClient.GetCounterfactualAddress(r.Context(), chainID, wallet)
		if err2 != nil {
			slog.Error("GetCounterfactualAddress failed", "error", err2)
			writeError(w, http.StatusInternalServerError, "failed to derive counterfactual address")
			return
		}
		cfAddr = addr
	} else {
		cfAddr = cfAcct.Address
	}

	balance, err := h.chainClient.GetTokenBalance(r.Context(), chainID, tokenAddress, cfAddr)
	if err != nil {
		slog.Error("GetTokenBalance failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to fetch token balance")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"address": cfAddr,
		"balance": balance,
	})
}
