package api

import (
	"encoding/json"
	"net/http"
)

func (h *handlers) generateNonce(w http.ResponseWriter, r *http.Request) {
	nonce, err := h.authSvc.GenerateNonce()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate nonce")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"nonce": nonce})
}

func (h *handlers) verifySIWE(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Message   string `json:"message"`
		Signature string `json:"signature"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	wallet, err := h.authSvc.VerifySIWE(req.Message, req.Signature)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "SIWE verification failed")
		return
	}

	user, err := h.queries.GetUserByWallet(r.Context(), wallet)
	if err != nil {
		user, err = h.queries.CreateUser(r.Context(), wallet)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create user")
			return
		}
	}

	token, err := h.authSvc.IssueJWT(user.ID, user.WalletAddress, user.Role)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to issue token")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"token": token,
		"user":  user,
	})
}
