package api

import (
	"log/slog"
	"net/http"

	"github.com/cpay-dev/backend/internal/auth"
	"github.com/cpay-dev/backend/internal/db"
)

const cfChainID = 80002 // Polygon Amoy testnet
const cfTokenAddress = "0x41E94Eb019C0762f9Bfcf9Fb1E58725BfB0e7582" // test USDC on Amoy

// getMyCFAddress returns (and caches) the counterfactual account address for the
// authenticated merchant on Polygon Amoy (chain 80002).
func (h *handlers) getMyCFAddress(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	wallet := auth.WalletFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// Try cache first
	existing, err := h.queries.GetCounterfactualAccount(r.Context(), db.GetCounterfactualAccountParams{
		UserID:  userID,
		ChainID: cfChainID,
	})
	if err == nil {
		writeJSON(w, http.StatusOK, map[string]string{"address": existing.Address})
		return
	}

	// Derive via chain service
	addr, err := h.chainClient.GetCounterfactualAddress(r.Context(), cfChainID, wallet)
	if err != nil {
		slog.Error("GetCounterfactualAddress failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to derive counterfactual address")
		return
	}

	// Cache it
	_, err = h.queries.UpsertCounterfactualAccount(r.Context(), db.UpsertCounterfactualAccountParams{
		UserID:  userID,
		ChainID: cfChainID,
		Address: addr,
	})
	if err != nil {
		slog.Warn("failed to cache CF address", "error", err)
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
		tokenAddress = cfTokenAddress
	}

	// Get CF address (from cache or derive)
	cfAcct, err := h.queries.GetCounterfactualAccount(r.Context(), db.GetCounterfactualAccountParams{
		UserID:  userID,
		ChainID: cfChainID,
	})
	cfAddr := ""
	if err != nil {
		addr, err2 := h.chainClient.GetCounterfactualAddress(r.Context(), cfChainID, wallet)
		if err2 != nil {
			slog.Error("GetCounterfactualAddress failed", "error", err2)
			writeError(w, http.StatusInternalServerError, "failed to derive counterfactual address")
			return
		}
		cfAddr = addr
	} else {
		cfAddr = cfAcct.Address
	}

	balance, err := h.chainClient.GetTokenBalance(r.Context(), cfChainID, tokenAddress, cfAddr)
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

// getCFWithdrawInfo returns the CF wallet address, token balance, and whether it's deployed.
// The frontend uses this to decide whether to deploy the wallet first, then call execute().
func (h *handlers) getCFWithdrawInfo(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	wallet := auth.WalletFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	tokenAddress := r.URL.Query().Get("token")
	if tokenAddress == "" {
		tokenAddress = cfTokenAddress
	}

	info, err := h.chainClient.GetCFWalletInfo(r.Context(), cfChainID, wallet, tokenAddress)
	if err != nil {
		slog.Error("GetCFWalletInfo failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get CF wallet info")
		return
	}

	// Also cache the CF address if not already cached
	_, cacheErr := h.queries.GetCounterfactualAccount(r.Context(), db.GetCounterfactualAccountParams{
		UserID:  userID,
		ChainID: cfChainID,
	})
	if cacheErr != nil {
		_, _ = h.queries.UpsertCounterfactualAccount(r.Context(), db.UpsertCounterfactualAccountParams{
			UserID:  userID,
			ChainID: cfChainID,
			Address: info.CFAddress,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"cf_address":   info.CFAddress,
		"balance":      info.Balance,
		"is_deployed":  info.IsDeployed,
		"token":        tokenAddress,
		"chain_id":     cfChainID,
	})
}
