package gateway

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/cpay-dev/cpay/internal/shared/httpx"
	"github.com/cpay-dev/cpay/internal/shared/ids"
	"github.com/cpay-dev/cpay/internal/shared/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

const (
	conversionEventCheckoutPageOpened = "checkout_page_opened"
	conversionEventPayClicked         = "pay_clicked"
	conversionEventWalletConnected    = "wallet_connected"
	conversionEventPaymentMade        = "payment_made"
	conversionEventPaymentIncomplete  = "payment_incomplete"
	conversionEventCheckoutCanceled   = "checkout_canceled"
)

type checkoutConversionEventRequest struct {
	EventType string         `json:"event_type"`
	VisitorID string         `json:"visitor_id,omitempty"`
	Reason    string         `json:"reason,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type checkoutConversionStats struct {
	CheckoutPageOpened int64   `json:"checkout_page_opened"`
	PayClicked         int64   `json:"pay_clicked"`
	WalletConnected    int64   `json:"wallet_connected"`
	PaymentMade        int64   `json:"payment_made"`
	PaymentIncomplete  int64   `json:"payment_incomplete"`
	CheckoutCanceled   int64   `json:"checkout_canceled"`
	DailyVisitors      float64 `json:"daily_visitors"`
}

func (s *Server) handleCreatePublicCheckoutEvent(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(chi.URLParam(r, "session_id"))
	if sessionID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "session_id is required", middleware.GetRequestID(r.Context()))
		return
	}
	var req checkoutConversionEventRequest
	if !s.parseJSON(w, r, &req) {
		return
	}
	req.EventType = strings.TrimSpace(req.EventType)
	if !publicClientConversionEvent(req.EventType) {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "unsupported checkout event", middleware.GetRequestID(r.Context()))
		return
	}

	var merchantID, paymentLinkID string
	var paymentIntentID *string
	if err := s.db.QueryRow(r.Context(), `
		SELECT c.merchant_id::text, c.payment_link_id::text, i.id::text
		FROM checkout.checkout_sessions c
		LEFT JOIN checkout.payment_intents i ON i.checkout_session_id=c.id
		WHERE c.id::text=$1
	`, sessionID).Scan(&merchantID, &paymentLinkID, &paymentIntentID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.WriteError(w, http.StatusNotFound, "not_found", "checkout session not found", middleware.GetRequestID(r.Context()))
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to load checkout session", middleware.GetRequestID(r.Context()))
		return
	}

	if req.EventType == conversionEventCheckoutCanceled {
		_, _ = s.db.Exec(r.Context(), `
			UPDATE checkout.checkout_sessions
			SET status='canceled', updated_at=NOW()
			WHERE id::text=$1
				AND status <> 'paid'
				AND NOT EXISTS (
					SELECT 1
					FROM checkout.payment_intents i
					WHERE i.checkout_session_id=checkout.checkout_sessions.id
						AND i.status IN ('confirmed', 'overpaid', 'settled')
				)
		`, sessionID)
	}

	metadata := req.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}
	if strings.TrimSpace(req.Reason) != "" {
		metadata["reason"] = strings.TrimSpace(req.Reason)
	}
	if err := s.recordCheckoutConversionEvent(r.Context(), r, checkoutConversionEventInput{
		EventType:         req.EventType,
		MerchantID:        merchantID,
		PaymentLinkID:     paymentLinkID,
		CheckoutSessionID: sessionID,
		PaymentIntentID:   strPtrValue(paymentIntentID),
		VisitorID:         req.VisitorID,
		Metadata:          metadata,
	}); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to record checkout event", middleware.GetRequestID(r.Context()))
		return
	}

	httpx.WriteJSON(w, http.StatusAccepted, map[string]any{"recorded": true})
}

func (s *Server) handleCheckoutStats(w http.ResponseWriter, r *http.Request) {
	reqAuth, ok := mustRequester(w, r)
	if !ok {
		return
	}

	stats, err := s.checkoutConversionStats(r.Context(), reqAuth.MerchantID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to load checkout stats", middleware.GetRequestID(r.Context()))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, stats)
}

type checkoutConversionEventInput struct {
	EventType         string
	MerchantID        string
	PaymentLinkID     string
	CheckoutSessionID string
	PaymentIntentID   string
	VisitorID         string
	Metadata          map[string]any
}

func (s *Server) recordCheckoutConversionEvent(ctx context.Context, r *http.Request, input checkoutConversionEventInput) error {
	if s.db == nil || strings.TrimSpace(input.EventType) == "" || strings.TrimSpace(input.MerchantID) == "" {
		return nil
	}
	metadata := input.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}
	rawMetadata, err := json.Marshal(metadata)
	if err != nil {
		rawMetadata = []byte(`{}`)
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO checkout.conversion_events (
			id, merchant_id, payment_link_id, checkout_session_id, payment_intent_id,
			event_type, visitor_id, ip_hash, user_agent, metadata
		)
		VALUES ($1, $2, NULLIF($3, '')::ulid, NULLIF($4, '')::ulid, NULLIF($5, '')::ulid, $6, NULLIF($7, ''), NULLIF($8, ''), NULLIF($9, ''), $10::jsonb)
	`, ids.New(), input.MerchantID, input.PaymentLinkID, input.CheckoutSessionID, input.PaymentIntentID,
		input.EventType, firstNonEmpty(input.VisitorID, r.Header.Get("X-CPAY-Visitor-ID")), requestIPHash(r), r.UserAgent(), string(rawMetadata))
	return err
}

func (s *Server) recordCheckoutConversionEventBySession(ctx context.Context, r *http.Request, sessionID string, eventType string, metadata map[string]any) {
	if s.db == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	var merchantID, paymentLinkID string
	var paymentIntentID *string
	if err := s.db.QueryRow(ctx, `
		SELECT c.merchant_id::text, c.payment_link_id::text, i.id::text
		FROM checkout.checkout_sessions c
		LEFT JOIN checkout.payment_intents i ON i.checkout_session_id=c.id
		WHERE c.id::text=$1
	`, sessionID).Scan(&merchantID, &paymentLinkID, &paymentIntentID); err != nil {
		s.log.Warn().Err(err).Str("session_id", sessionID).Str("event_type", eventType).Msg("failed to lookup checkout conversion event session")
		return
	}
	if err := s.recordCheckoutConversionEvent(ctx, r, checkoutConversionEventInput{
		EventType:         eventType,
		MerchantID:        merchantID,
		PaymentLinkID:     paymentLinkID,
		CheckoutSessionID: sessionID,
		PaymentIntentID:   strPtrValue(paymentIntentID),
		Metadata:          metadata,
	}); err != nil {
		s.log.Warn().Err(err).Str("session_id", sessionID).Str("event_type", eventType).Msg("failed to record checkout conversion event")
	}
}

func (s *Server) checkoutConversionStats(ctx context.Context, merchantID string) (checkoutConversionStats, error) {
	var stats checkoutConversionStats
	var firstPageOpen, lastPageOpen *time.Time
	if err := s.db.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE event_type='checkout_page_opened') AS checkout_page_opened,
			COUNT(*) FILTER (WHERE event_type='pay_clicked') AS pay_clicked,
			COUNT(*) FILTER (WHERE event_type='wallet_connected') AS wallet_connected,
			COUNT(DISTINCT payment_intent_id) FILTER (WHERE event_type='payment_made' AND payment_intent_id IS NOT NULL) AS payment_made,
			COUNT(DISTINCT checkout_session_id) FILTER (WHERE event_type='payment_incomplete' AND checkout_session_id IS NOT NULL) AS logged_incomplete,
			COUNT(DISTINCT checkout_session_id) FILTER (WHERE event_type='checkout_canceled' AND checkout_session_id IS NOT NULL) AS logged_canceled,
			MIN(created_at) FILTER (WHERE event_type='checkout_page_opened') AS first_page_open,
			MAX(created_at) FILTER (WHERE event_type='checkout_page_opened') AS last_page_open
		FROM checkout.conversion_events
		WHERE merchant_id::text=$1
	`, merchantID).Scan(
		&stats.CheckoutPageOpened,
		&stats.PayClicked,
		&stats.WalletConnected,
		&stats.PaymentMade,
		&stats.PaymentIncomplete,
		&stats.CheckoutCanceled,
		&firstPageOpen,
		&lastPageOpen,
	); err != nil {
		return stats, err
	}

	var derivedIncomplete, derivedCanceled int64
	if err := s.db.QueryRow(ctx, `
		WITH session_outcomes AS (
			SELECT
				c.id,
				c.status,
				c.expires_at,
				EXISTS (
					SELECT 1 FROM checkout.payment_intents i
					WHERE i.checkout_session_id=c.id AND i.status IN ('confirmed', 'overpaid', 'settled')
				) AS paid,
				EXISTS (
					SELECT 1 FROM checkout.conversion_events e
					WHERE e.checkout_session_id=c.id AND e.event_type='payment_incomplete'
				) AS logged_incomplete,
				EXISTS (
					SELECT 1 FROM checkout.conversion_events e
					WHERE e.checkout_session_id=c.id AND e.event_type='checkout_canceled'
				) AS logged_canceled
			FROM checkout.checkout_sessions c
			WHERE c.merchant_id::text=$1
		)
		SELECT
			COUNT(*) FILTER (WHERE NOT paid AND (status IN ('expired', 'failed', 'canceled') OR expires_at < NOW() OR logged_incomplete OR logged_canceled)),
			COUNT(*) FILTER (WHERE NOT paid AND (status='canceled' OR logged_canceled))
		FROM session_outcomes
	`, merchantID).Scan(&derivedIncomplete, &derivedCanceled); err != nil {
		return stats, err
	}
	if derivedIncomplete > stats.PaymentIncomplete {
		stats.PaymentIncomplete = derivedIncomplete
	}
	if derivedCanceled > stats.CheckoutCanceled {
		stats.CheckoutCanceled = derivedCanceled
	}
	if firstPageOpen != nil && lastPageOpen != nil {
		days := int(lastPageOpen.Truncate(24*time.Hour).Sub(firstPageOpen.Truncate(24*time.Hour))/(24*time.Hour)) + 1
		if days < 1 {
			days = 1
		}
		stats.DailyVisitors = float64(stats.CheckoutPageOpened) / float64(days)
	}
	return stats, nil
}

func publicClientConversionEvent(eventType string) bool {
	switch eventType {
	case conversionEventWalletConnected, conversionEventPaymentIncomplete, conversionEventCheckoutCanceled:
		return true
	default:
		return false
	}
}

func requestIPHash(r *http.Request) string {
	raw := firstNonEmpty(r.Header.Get("CF-Connecting-IP"), r.Header.Get("X-Real-IP"))
	if raw == "" {
		xff := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
		if len(xff) > 0 {
			raw = strings.TrimSpace(xff[0])
		}
	}
	if raw == "" {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err == nil {
			raw = host
		} else {
			raw = r.RemoteAddr
		}
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func strPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
