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

func (h *handlers) createShop(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	role := auth.RoleFromContext(r.Context())

	if role != "merchant" {
		writeError(w, http.StatusForbidden, "only merchants can create shops")
		return
	}

	var req struct {
		Name      string  `json:"name"`
		Website   *string `json:"website"`
		Title     *string `json:"title"`
		AvatarURL *string `json:"avatar_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	shop, err := h.queries.CreateShop(r.Context(), db.CreateShopParams{
		UserID:    userID,
		Name:      req.Name,
		Website:   req.Website,
		Title:     req.Title,
		AvatarUrl: req.AvatarURL,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create shop")
		return
	}

	evt, err := events.NewEvent(events.EventShopCreated, events.SubjectShops, map[string]string{
		"shop_id": shop.ID,
		"user_id": userID,
	})
	if err == nil {
		if pubErr := h.eventPub.Publish(r.Context(), evt); pubErr != nil {
			slog.Error("failed to publish shop created event", "error", pubErr)
		}
	}

	writeJSON(w, http.StatusCreated, shop)
}

func (h *handlers) getMyShop(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	shop, err := h.queries.GetShopByUserID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "shop not found")
		return
	}

	writeJSON(w, http.StatusOK, shop)
}

func (h *handlers) updateShop(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	shopID := chi.URLParam(r, "id")

	shop, err := h.queries.GetShopByID(r.Context(), shopID)
	if err != nil {
		writeError(w, http.StatusNotFound, "shop not found")
		return
	}
	if shop.UserID != userID {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}

	var req struct {
		Name      string  `json:"name"`
		Website   *string `json:"website"`
		Title     *string `json:"title"`
		AvatarURL *string `json:"avatar_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	updated, err := h.queries.UpdateShop(r.Context(), db.UpdateShopParams{
		ID:        shopID,
		Name:      req.Name,
		Website:   req.Website,
		Title:     req.Title,
		AvatarUrl: req.AvatarURL,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update shop")
		return
	}

	writeJSON(w, http.StatusOK, updated)
}
