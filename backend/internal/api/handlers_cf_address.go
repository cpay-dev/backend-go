package api

import (
	"context"
	"net/http"

	"github.com/rs/zerolog/log"

	"github.com/cpay-dev/backend/internal/auth"
	"github.com/cpay-dev/backend/internal/db"
)

const cfChainID = 80002 // Polygon Amoy testnet
const cfTokenAddress = "0x41E94Eb019C0762f9Bfcf9Fb1E58725BfB0e7582" // USDC on Amoy

// resolveMerchantCFAddress derives the CF address for a merchant wallet via the
// chain service. Used by public endpoints when the cache is empty.
func (h *handlers) resolveMerchantCFAddress(ctx context.Context, wallet string) (string, error) {
	addr, err := h.chainClient.GetCounterfactualAddress(ctx, cfChainID, wallet)
	if err != nil {
		log.Error().Err(err).Str("wallet", wallet).Msg("resolveMerchantCFAddress failed")
		return "", err
	}
	return addr, nil
}

// getMyCFAddress returns (and caches) the counterfactual account address for the
// authenticated merchant on Polygon Amoy (chain 80002).
func (h *handlers) getMyCFAddress(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	wallet := auth.WalletFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// Always derive from chain service (source of truth) and upsert cache.
	// This ensures stale cached addresses get corrected after bytecode changes.
	addr, err := h.chainClient.GetCounterfactualAddress(r.Context(), cfChainID, wallet)
	if err != nil {
		log.Error().Err(err).Msg("GetCounterfactualAddress failed")
		writeError(w, http.StatusInternalServerError, "failed to derive counterfactual address")
		return
	}

	_, err = h.queries.UpsertCounterfactualAccount(r.Context(), db.UpsertCounterfactualAccountParams{
		UserID:  userID,
		ChainID: cfChainID,
		Address: addr,
	})
	if err != nil {
		log.Warn().Err(err).Msg("failed to cache CF address")
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

	// Always derive from chain service to avoid stale cache.
	cfAddr, err := h.chainClient.GetCounterfactualAddress(r.Context(), cfChainID, wallet)
	if err != nil {
		log.Error().Err(err).Msg("GetCounterfactualAddress failed")
		writeError(w, http.StatusInternalServerError, "failed to derive counterfactual address")
		return
	}

	// Backfill cache.
	_, _ = h.queries.UpsertCounterfactualAccount(r.Context(), db.UpsertCounterfactualAccountParams{
		UserID:  userID,
		ChainID: cfChainID,
		Address: cfAddr,
	})

	balance, err := h.chainClient.GetTokenBalance(r.Context(), cfChainID, tokenAddress, cfAddr)
	if err != nil {
		log.Error().Err(err).Msg("GetTokenBalance failed")
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
		log.Error().Err(err).Msg("GetCFWalletInfo failed")
		writeError(w, http.StatusInternalServerError, "failed to get CF wallet info")
		return
	}

	// Always upsert the cached CF address so stale entries get corrected
	// (e.g. after a bytecode change the chain service derives a new address).
	_, _ = h.queries.UpsertCounterfactualAccount(r.Context(), db.UpsertCounterfactualAccountParams{
		UserID:  userID,
		ChainID: cfChainID,
		Address: info.CFAddress,
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"cf_address":   info.CFAddress,
		"balance":      info.Balance,
		"is_deployed":  info.IsDeployed,
		"token":        tokenAddress,
		"chain_id":     cfChainID,
	})
}
