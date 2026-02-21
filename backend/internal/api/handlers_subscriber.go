package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/cpay-dev/backend/internal/billing"
	"github.com/cpay-dev/backend/internal/db"
	"github.com/cpay-dev/backend/internal/events"
)

type subscriberResponse struct {
	ID             string  `json:"id"`
	SubscriptionID string  `json:"subscription_id"`
	ShopID         string  `json:"shop_id"`
	PayerAddress   string  `json:"payer_address"`
	PayerEmail     string  `json:"payer_email"`
	Status         string  `json:"status"`
	Method         string  `json:"method"`
	PeriodsPaid    int32   `json:"periods_paid"`
	PeriodsUsed    int32   `json:"periods_used"`
	ApprovedAmount *string `json:"approved_amount,omitempty"`
	SpentAmount    *string `json:"spent_amount,omitempty"`
	NextDue        *string `json:"next_due,omitempty"`
	CancelledAt    *string `json:"cancelled_at,omitempty"`
	ExpiresAt      *string `json:"expires_at,omitempty"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
}

func toSubscriberResponse(s db.Subscriber) subscriberResponse {
	r := subscriberResponse{
		ID:             s.ID,
		SubscriptionID: s.SubscriptionID,
		ShopID:         s.ShopID,
		PayerAddress:   s.PayerAddress,
		PayerEmail:     s.PayerEmail,
		Status:         string(s.Status),
		Method:         string(s.Method),
		PeriodsPaid:    s.PeriodsPaid,
		PeriodsUsed:    s.PeriodsUsed,
		ApprovedAmount: s.ApprovedAmount,
		SpentAmount:    s.SpentAmount,
		CreatedAt:      s.CreatedAt.Format(time.RFC3339),
		UpdatedAt:      s.UpdatedAt.Format(time.RFC3339),
	}
	if s.NextDue.Valid {
		t := s.NextDue.Time.Format(time.RFC3339)
		r.NextDue = &t
	}
	if s.CancelledAt.Valid {
		t := s.CancelledAt.Time.Format(time.RFC3339)
		r.CancelledAt = &t
	}
	if s.ExpiresAt.Valid {
		t := s.ExpiresAt.Time.Format(time.RFC3339)
		r.ExpiresAt = &t
	}
	return r
}

// createSubscriber handles POST /api/sub/:id/subscribe
func (h *handlers) createSubscriber(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "id")

	var req struct {
		PayerAddress   string  `json:"payer_address"`
		Email          string  `json:"email"`
		Method         string  `json:"method"` // "prepaid" or "approved"
		Periods        int32   `json:"periods"`
		ApprovedAmount *string `json:"approved_amount"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.PayerAddress == "" || req.Email == "" || req.Method == "" {
		writeError(w, http.StatusBadRequest, "payer_address, email, method are required")
		return
	}
	if req.Method != "prepaid" && req.Method != "approved" {
		writeError(w, http.StatusBadRequest, "method must be 'prepaid' or 'approved'")
		return
	}

	sub, err := h.queries.GetSubscription(r.Context(), subID)
	if err != nil {
		writeError(w, http.StatusNotFound, "subscription not found")
		return
	}
	if !sub.Active {
		writeError(w, http.StatusBadRequest, "subscription is not active")
		return
	}

	// Check for existing active subscriber
	existing, err := h.queries.GetActiveSubscriber(r.Context(), db.GetActiveSubscriberParams{
		SubscriptionID: subID,
		PayerAddress:   req.PayerAddress,
	})
	if err == nil && existing.ID != "" {
		writeError(w, http.StatusConflict, "already subscribed")
		return
	}

	nextDue := nextDueFromNow(sub.Period)

	var periodsPaid int32
	if req.Method == "prepaid" {
		periodsPaid = req.Periods
		if periodsPaid < 1 {
			periodsPaid = 1
		}
	}

	subscriber, err := h.queries.CreateSubscriber(r.Context(), db.CreateSubscriberParams{
		SubscriptionID: subID,
		ShopID:         sub.ShopID,
		PayerAddress:   req.PayerAddress,
		PayerEmail:     req.Email,
		Method:         db.PaymentMethod(req.Method),
		PeriodsPaid:    periodsPaid,
		ApprovedAmount: req.ApprovedAmount,
		NextDue:        pgtype.Timestamptz{Time: nextDue, Valid: true},
	})
	if err != nil {
		log.Error().Err(err).Msg("CreateSubscriber failed")
		writeError(w, http.StatusInternalServerError, "failed to create subscriber")
		return
	}

	if evt, err := events.NewEvent(events.EventSubscriberCreated, events.SubjectSubscribers, map[string]string{
		"id": subscriber.ID, "subscription_id": subID, "payer": req.PayerAddress,
	}); err == nil {
		_ = h.eventPub.Publish(r.Context(), evt)
	}

	writeJSON(w, http.StatusCreated, toSubscriberResponse(subscriber))
}

// cancelSubscription handles POST /api/sub/:id/cancel
func (h *handlers) cancelSubscription(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "id")

	var req struct {
		PayerAddress string `json:"payer_address"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.PayerAddress == "" {
		writeError(w, http.StatusBadRequest, "payer_address is required")
		return
	}

	existing, err := h.queries.GetActiveSubscriber(r.Context(), db.GetActiveSubscriberParams{
		SubscriptionID: subID,
		PayerAddress:   req.PayerAddress,
	})
	if err != nil {
		writeError(w, http.StatusNotFound, "active subscriber not found")
		return
	}

	subscriber, err := h.queries.CancelSubscriber(r.Context(), existing.ID)
	if err != nil {
		log.Error().Err(err).Msg("CancelSubscriber failed")
		writeError(w, http.StatusInternalServerError, "failed to cancel subscriber")
		return
	}

	if evt, err := events.NewEvent(events.EventSubscriberCancelled, events.SubjectSubscribers, map[string]string{
		"id": subscriber.ID, "subscription_id": subID, "payer": req.PayerAddress,
		"email": subscriber.PayerEmail,
	}); err == nil {
		_ = h.eventPub.Publish(r.Context(), evt)
	}

	writeJSON(w, http.StatusOK, toSubscriberResponse(subscriber))
}

// getSubscriber handles GET /api/sub/:id/subscriber?payer=0x...
func (h *handlers) getSubscriber(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "id")
	payer := r.URL.Query().Get("payer")
	if payer == "" {
		writeError(w, http.StatusBadRequest, "payer query param required")
		return
	}

	subscriber, err := h.queries.GetSubscriber(r.Context(), db.GetSubscriberParams{
		SubscriptionID: subID,
		PayerAddress:   payer,
	})
	if err != nil {
		writeJSON(w, http.StatusOK, nil)
		return
	}

	writeJSON(w, http.StatusOK, toSubscriberResponse(subscriber))
}

// getRelayerAddress handles GET /api/relayer-address
func (h *handlers) getRelayerAddress(w http.ResponseWriter, r *http.Request) {
	addr, err := h.chainClient.GetRelayerAddress(r.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "relayer not available")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"address": addr})
}

// listSubscribers handles GET /api/subscriptions/:id/subscribers (authenticated)
func (h *handlers) listSubscribers(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "id")

	subscribers, err := h.queries.ListSubscribers(r.Context(), subID)
	if err != nil {
		log.Error().Err(err).Msg("ListSubscribers failed")
		writeError(w, http.StatusInternalServerError, "failed to list subscribers")
		return
	}

	resp := make([]subscriberResponse, 0, len(subscribers))
	for _, s := range subscribers {
		resp = append(resp, toSubscriberResponse(s))
	}
	writeJSON(w, http.StatusOK, resp)
}

func nextDueFromNow(period db.BillingPeriod) time.Time {
	return billing.NextDueFrom(time.Now(), period)
}
