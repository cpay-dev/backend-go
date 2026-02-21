package api

import (
	"encoding/json"
	"net/http"

	"github.com/rs/zerolog/log"

	"github.com/cpay-dev/backend/internal/auth"
	"github.com/cpay-dev/backend/internal/db"
	"github.com/cpay-dev/backend/internal/events"
)

func (h *handlers) getProfile(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	user, err := h.queries.GetUserByID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	writeJSON(w, http.StatusOK, user)
}

func (h *handlers) updateEmail(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Email == "" {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}

	updated, err := h.queries.UpdateUserEmail(r.Context(), db.UpdateUserEmailParams{
		ID:    userID,
		Email: &req.Email,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update email")
		return
	}

	writeJSON(w, http.StatusOK, updated)
}

func (h *handlers) setRole(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	var req struct {
		Role string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Role != "merchant" && req.Role != "user" {
		writeError(w, http.StatusBadRequest, "role must be 'merchant' or 'user'")
		return
	}

	// Check if role is already set
	user, err := h.queries.GetUserByID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if user.Role != nil {
		writeError(w, http.StatusConflict, "role already set")
		return
	}

	updated, err := h.queries.UpdateUserRole(r.Context(), db.UpdateUserRoleParams{
		Role: &req.Role,
		ID:   userID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update role")
		return
	}

	// Publish event
	evt, err := events.NewEvent(events.EventUserUpdated, events.SubjectUsers, map[string]string{
		"user_id": userID,
		"role":    req.Role,
	})
	if err == nil {
		if pubErr := h.eventPub.Publish(r.Context(), evt); pubErr != nil {
			log.Error().Err(pubErr).Msg("failed to publish user updated event")
		}
	}

	// Re-issue JWT with the new role
	token, err := h.authSvc.IssueJWT(updated.ID, updated.WalletAddress, updated.Role)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to issue token")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"token": token,
		"user":  updated,
	})
}
