package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cpay-dev/cpay/internal/shared/httpx"
	"github.com/cpay-dev/cpay/internal/shared/ids"
	"github.com/cpay-dev/cpay/internal/shared/middleware"
	"github.com/go-chi/chi/v5"
)

type vaultAuthorizationInput struct {
	ContractAddress string     `json:"contract_address"`
	CustomerWallet  string     `json:"customer_wallet"`
	MaxTotalAmount  float64    `json:"max_total_amount"`
	RemainingAmount float64    `json:"remaining_amount"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
}

type createSubscriptionRequest struct {
	PaymentLinkID  *string                 `json:"payment_link_id,omitempty"`
	CustomerRef    string                  `json:"customer_ref,omitempty"`
	Chain          string                  `json:"chain"`
	TokenSymbol    string                  `json:"token_symbol"`
	TokenAddress   string                  `json:"token_address,omitempty"`
	Amount         float64                 `json:"amount"`
	Currency       string                  `json:"currency"`
	IntervalUnit   string                  `json:"interval_unit"`
	IntervalCount  int                     `json:"interval_count"`
	FirstBillingAt *time.Time              `json:"first_billing_at,omitempty"`
	Vault          vaultAuthorizationInput `json:"vault"`
	Metadata       map[string]any          `json:"metadata,omitempty"`
}

func (s *Server) handleCreateSubscription(w http.ResponseWriter, r *http.Request) {
	reqAuth, ok := mustRequester(w, r)
	if !ok {
		return
	}
	var req createSubscriptionRequest
	if !s.parseJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Chain) == "" || strings.TrimSpace(req.TokenSymbol) == "" || req.Amount <= 0 {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "chain, token_symbol and positive amount are required", middleware.GetRequestID(r.Context()))
		return
	}
	if strings.TrimSpace(req.Currency) == "" {
		req.Currency = "USD"
	}
	if req.IntervalCount <= 0 {
		req.IntervalCount = 1
	}
	if req.IntervalUnit == "" {
		req.IntervalUnit = "month"
	}
	req.IntervalUnit = strings.ToLower(strings.TrimSpace(req.IntervalUnit))
	if req.IntervalUnit != "day" && req.IntervalUnit != "week" && req.IntervalUnit != "month" {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "interval_unit must be day, week or month", middleware.GetRequestID(r.Context()))
		return
	}
	if strings.TrimSpace(req.Vault.ContractAddress) == "" || strings.TrimSpace(req.Vault.CustomerWallet) == "" {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "vault contract_address and customer_wallet are required", middleware.GetRequestID(r.Context()))
		return
	}
	if req.Vault.MaxTotalAmount <= 0 {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "vault max_total_amount must be positive", middleware.GetRequestID(r.Context()))
		return
	}
	if req.Vault.RemainingAmount <= 0 {
		req.Vault.RemainingAmount = req.Vault.MaxTotalAmount
	}

	s.withIdempotency(w, r, reqAuth.MerchantID, "/v1/subscriptions", func() (int, any, error) {
		now := time.Now().UTC()
		nextBilling := now
		if req.FirstBillingAt != nil {
			nextBilling = req.FirstBillingAt.UTC()
		} else {
			nextBilling = addInterval(now, req.IntervalUnit, req.IntervalCount)
		}
		periodEnd := addInterval(nextBilling, req.IntervalUnit, req.IntervalCount)

		subID := ids.New()
		vaultID := ids.New()
		cycleID := ids.New()
		metadataRaw, _ := jsonMarshal(req.Metadata)

		var paymentLinkID any
		if req.PaymentLinkID != nil && strings.TrimSpace(*req.PaymentLinkID) != "" {
			pid, err := ids.Parse(strings.TrimSpace(*req.PaymentLinkID))
			if err != nil {
				return 0, nil, badRequest("invalid payment_link_id")
			}
			paymentLinkID = pid
		}

		tx, err := s.db.Begin(r.Context())
		if err != nil {
			return 0, nil, err
		}
		defer tx.Rollback(r.Context())

		_, err = tx.Exec(r.Context(), `
			INSERT INTO subscriptions(
				id, merchant_id, payment_link_id, customer_ref, status, chain, token_symbol, token_address,
				amount, currency, interval_unit, interval_count, next_billing_at, vault_contract_address, metadata,
				created_at, updated_at
			)
			VALUES($1, $2, $3, $4, 'active', $5, $6, $7, $8, $9, $10, $11, $12, $13, $14::jsonb, NOW(), NOW())
		`, subID, reqAuth.MerchantID, paymentLinkID, req.CustomerRef, strings.ToLower(req.Chain), strings.ToUpper(req.TokenSymbol), req.TokenAddress,
			req.Amount, strings.ToUpper(req.Currency), req.IntervalUnit, req.IntervalCount, nextBilling, req.Vault.ContractAddress, string(metadataRaw))
		if err != nil {
			return 0, nil, err
		}

		_, err = tx.Exec(r.Context(), `
			INSERT INTO vault_authorizations(
				id, merchant_id, subscription_id, chain, contract_address, customer_wallet,
				token_symbol, token_address, max_total_amount, remaining_amount, expires_at, status,
				created_at, updated_at
			)
			VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, 'active', NOW(), NOW())
		`, vaultID, reqAuth.MerchantID, subID, strings.ToLower(req.Chain), req.Vault.ContractAddress, req.Vault.CustomerWallet,
			strings.ToUpper(req.TokenSymbol), req.TokenAddress, req.Vault.MaxTotalAmount, req.Vault.RemainingAmount, req.Vault.ExpiresAt)
		if err != nil {
			return 0, nil, err
		}

		_, err = tx.Exec(r.Context(), `
			INSERT INTO subscription_cycles(
				id, subscription_id, cycle_index, period_start, period_end, due_at, status, amount,
				retry_count, created_at, updated_at
			)
			VALUES($1, $2, 1, $3, $4, $3, 'due', $5, 0, NOW(), NOW())
		`, cycleID, subID, nextBilling, periodEnd, req.Amount)
		if err != nil {
			return 0, nil, err
		}

		if err = s.enqueueEventTx(r.Context(), tx, "subscription", subID, reqAuth.MerchantID, "subscription.created", map[string]any{
			"subscription_id": subID,
			"next_billing_at": nextBilling,
			"amount":          req.Amount,
		}); err != nil {
			return 0, nil, err
		}

		if err = tx.Commit(r.Context()); err != nil {
			return 0, nil, err
		}

		return http.StatusCreated, map[string]any{
			"id":                  subID,
			"status":              "active",
			"chain":               strings.ToLower(req.Chain),
			"token_symbol":        strings.ToUpper(req.TokenSymbol),
			"amount":              req.Amount,
			"currency":            strings.ToUpper(req.Currency),
			"interval_unit":       req.IntervalUnit,
			"interval_count":      req.IntervalCount,
			"next_billing_at":     nextBilling,
			"vault_authorization": map[string]any{"id": vaultID, "remaining_amount": req.Vault.RemainingAmount},
			"first_cycle_id":      cycleID,
		}, nil
	})
}

func (s *Server) handlePauseSubscription(w http.ResponseWriter, r *http.Request) {
	reqAuth, ok := mustRequester(w, r)
	if !ok {
		return
	}
	subID, err := ids.Parse(strings.TrimSpace(chi.URLParam(r, "id")))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "invalid subscription id", middleware.GetRequestID(r.Context()))
		return
	}
	cmd, err := s.db.Exec(r.Context(), `
		UPDATE subscriptions SET status='paused', updated_at=NOW()
		WHERE id=$1 AND merchant_id=$2 AND status='active'
	`, subID, reqAuth.MerchantID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to pause subscription", middleware.GetRequestID(r.Context()))
		return
	}
	if cmd.RowsAffected() == 0 {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "subscription not found or not active", middleware.GetRequestID(r.Context()))
		return
	}
	_ = s.enqueueEvent(r.Context(), "subscription", subID, reqAuth.MerchantID, "subscription.paused", map[string]any{"subscription_id": subID})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"id": subID, "status": "paused"})
}

func (s *Server) handleResumeSubscription(w http.ResponseWriter, r *http.Request) {
	reqAuth, ok := mustRequester(w, r)
	if !ok {
		return
	}
	subID, err := ids.Parse(strings.TrimSpace(chi.URLParam(r, "id")))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "invalid subscription id", middleware.GetRequestID(r.Context()))
		return
	}
	cmd, err := s.db.Exec(r.Context(), `
		UPDATE subscriptions SET status='active', updated_at=NOW()
		WHERE id=$1 AND merchant_id=$2 AND status IN ('paused', 'past_due')
	`, subID, reqAuth.MerchantID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to resume subscription", middleware.GetRequestID(r.Context()))
		return
	}
	if cmd.RowsAffected() == 0 {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "subscription not found", middleware.GetRequestID(r.Context()))
		return
	}
	_ = s.enqueueEvent(r.Context(), "subscription", subID, reqAuth.MerchantID, "subscription.resumed", map[string]any{"subscription_id": subID})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"id": subID, "status": "active"})
}

func (s *Server) handleGetSubscriptionCycles(w http.ResponseWriter, r *http.Request) {
	reqAuth, ok := mustRequester(w, r)
	if !ok {
		return
	}
	subID, err := ids.Parse(strings.TrimSpace(chi.URLParam(r, "id")))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "invalid subscription id", middleware.GetRequestID(r.Context()))
		return
	}
	limit := 20
	if q := r.URL.Query().Get("limit"); q != "" {
		if v, err := strconv.Atoi(q); err == nil && v > 0 && v <= 200 {
			limit = v
		}
	}
	rows, err := s.db.Query(r.Context(), `
		SELECT id::text, cycle_index, period_start, period_end, due_at, status, amount::text,
			payment_intent_id::text, retry_count, last_error, created_at, updated_at
		FROM subscription_cycles c
		JOIN subscriptions s ON s.id=c.subscription_id
		WHERE c.subscription_id=$1 AND s.merchant_id=$2
		ORDER BY cycle_index DESC
		LIMIT $2
	`, subID, reqAuth.MerchantID, limit)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to list cycles", middleware.GetRequestID(r.Context()))
		return
	}
	defer rows.Close()

	items := make([]map[string]any, 0)
	for rows.Next() {
		var id string
		var idx int
		var start, end, due time.Time
		var status, amountRaw string
		var paymentIntentID, lastErr *string
		var retryCount int
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&id, &idx, &start, &end, &due, &status, &amountRaw, &paymentIntentID, &retryCount, &lastErr, &createdAt, &updatedAt); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to scan cycle", middleware.GetRequestID(r.Context()))
			return
		}
		items = append(items, map[string]any{
			"id":                id,
			"cycle_index":       idx,
			"period_start":      start,
			"period_end":        end,
			"due_at":            due,
			"status":            status,
			"amount":            parseFloatMaybe(amountRaw),
			"payment_intent_id": paymentIntentID,
			"retry_count":       retryCount,
			"last_error":        lastErr,
			"created_at":        createdAt,
			"updated_at":        updatedAt,
		})
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"subscription_id": subID, "data": items})
}

func addInterval(t time.Time, unit string, count int) time.Time {
	if count <= 0 {
		count = 1
	}
	switch strings.ToLower(unit) {
	case "day":
		return t.Add(time.Duration(count) * 24 * time.Hour)
	case "week":
		return t.Add(time.Duration(count*7) * 24 * time.Hour)
	default:
		return t.AddDate(0, count, 0)
	}
}

func jsonMarshal(v any) ([]byte, error) {
	if v == nil {
		return []byte(`{}`), nil
	}
	return json.Marshal(v)
}
