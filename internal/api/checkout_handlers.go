package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/cpay-dev/cpay/internal/domain/payment"
	"github.com/cpay-dev/cpay/internal/platform/storage"
	"github.com/cpay-dev/cpay/internal/shared/chain"
	cryptox "github.com/cpay-dev/cpay/internal/shared/crypto"
	"github.com/cpay-dev/cpay/internal/shared/httpx"
	"github.com/cpay-dev/cpay/internal/shared/ids"
	"github.com/cpay-dev/cpay/internal/shared/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type createCheckoutSessionRequest struct {
	Amount          *float64       `json:"amount,omitempty"`
	Chain           string         `json:"chain"`
	TokenSymbol     string         `json:"token_symbol"`
	TokenAddress    string         `json:"token_address,omitempty"`
	CustomerEmail   string         `json:"customer_email,omitempty"`
	CustomerName    string         `json:"customer_name,omitempty"`
	CustomerPhone   string         `json:"customer_phone,omitempty"`
	CustomerAddress map[string]any `json:"customer_address,omitempty"`
	SuccessURL      string         `json:"success_url,omitempty"`
	ExpiresInSec    int            `json:"expires_in_sec,omitempty"`
	Metadata        map[string]any `json:"metadata,omitempty"`
}

type confirmCheckoutRequest struct {
	TxHash         string         `json:"tx_hash"`
	ReceivedAmount float64        `json:"received_amount"`
	Confirmations  int            `json:"confirmations"`
	BlockNumber    *int64         `json:"block_number,omitempty"`
	FromAddress    string         `json:"from_address,omitempty"`
	ToAddress      string         `json:"to_address,omitempty"`
	RawPayload     map[string]any `json:"raw_payload,omitempty"`
}

func (s *Server) handleCreateCheckoutSession(w http.ResponseWriter, r *http.Request) {
	reqAuth, ok := mustRequester(w, r)
	if !ok {
		return
	}
	linkIdentifier := strings.TrimSpace(chi.URLParam(r, "id"))
	if linkIdentifier == "" {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "link id is required", middleware.GetRequestID(r.Context()))
		return
	}

	var req createCheckoutSessionRequest
	if !s.parseJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Chain) == "" || strings.TrimSpace(req.TokenSymbol) == "" {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "chain and token_symbol are required", middleware.GetRequestID(r.Context()))
		return
	}

	s.withIdempotency(w, r, reqAuth.MerchantID, "/v1/payment_links/:id/sessions", func() (int, any, error) {
		query := `
			SELECT p.id::text, p.code, p.title, p.pricing_mode, COALESCE(p.amount::text, ''), COALESCE(p.min_amount::text, ''),
				COALESCE(p.max_amount::text, ''), COALESCE(p.adjust_percent::text, '0'), p.currency, p.reusable, p.max_payments,
				p.expires_at, p.status, p.allowed_tokens, p.after_payment_type, p.redirect_url,
				COALESCE(l.add_invoice_pdf, false)
			FROM payment_links p
			LEFT JOIN link_options l ON l.payment_link_id = p.id
			WHERE p.merchant_id=$1 AND (p.id::text=$2 OR p.code=$2)
		`
		var linkIDStr, code, title, pricingMode, amountRaw, minRaw, maxRaw, adjustRaw, currency, afterType string
		var reusable bool
		var maxPayments *int
		var linkExpiresAt *time.Time
		var linkStatus string
		var allowedRaw []byte
		var redirectURL *string
		var addInvoice bool
		err := s.db.QueryRow(r.Context(), query, reqAuth.MerchantID, linkIdentifier).Scan(
			&linkIDStr, &code, &title, &pricingMode, &amountRaw, &minRaw, &maxRaw, &adjustRaw, &currency, &reusable,
			&maxPayments, &linkExpiresAt, &linkStatus, &allowedRaw, &afterType, &redirectURL, &addInvoice,
		)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return 0, nil, notFound("payment link not found")
			}
			return 0, nil, err
		}
		if linkStatus != "active" {
			return 0, nil, badRequest("payment link is not active")
		}
		if linkExpiresAt != nil && linkExpiresAt.Before(time.Now().UTC()) {
			return 0, nil, badRequest("payment link is expired")
		}
		if maxPayments != nil {
			var paidCount int
			if err := s.db.QueryRow(r.Context(), `
				SELECT COUNT(*)
				FROM payment_intents
				WHERE payment_link_id=$1 AND status IN ('confirmed', 'overpaid', 'settled')
			`, linkIDStr).Scan(&paidCount); err != nil {
				return 0, nil, err
			}
			if paidCount >= *maxPayments {
				return 0, nil, badRequest("payment link has reached max payments")
			}
		}

		amount := 0.0
		if strings.ToLower(pricingMode) == "fixed" {
			amountAny := parseFloatMaybe(amountRaw)
			if f, ok := amountAny.(float64); ok {
				amount = f
			}
			if amount <= 0 {
				return 0, nil, badRequest("invalid fixed amount")
			}
		} else {
			if req.Amount == nil || *req.Amount <= 0 {
				return 0, nil, badRequest("amount is required for open links")
			}
			amount = *req.Amount
			if minAny := parseFloatMaybe(minRaw); minAny != nil {
				if minVal, ok := minAny.(float64); ok && amount < minVal {
					return 0, nil, badRequest("amount is below min_amount")
				}
			}
			if maxAny := parseFloatMaybe(maxRaw); maxAny != nil {
				if maxVal, ok := maxAny.(float64); ok && maxVal > 0 && amount > maxVal {
					return 0, nil, badRequest("amount is above max_amount")
				}
			}
		}
		adjustAny := parseFloatMaybe(adjustRaw)
		if adjustVal, ok := adjustAny.(float64); ok && adjustVal != 0 {
			amount = amount + (amount * adjustVal / 100)
		}

		var allowedTokens []payment.AllowedToken
		if err := json.Unmarshal(allowedRaw, &allowedTokens); err != nil {
			return 0, nil, err
		}
		if !payment.TokenAllowed(allowedTokens, req.Chain, req.TokenSymbol, req.TokenAddress) {
			return 0, nil, badRequest("token is not allowed for this link")
		}

		expiresIn := 30 * time.Minute
		if req.ExpiresInSec > 0 {
			if req.ExpiresInSec < 60 {
				req.ExpiresInSec = 60
			}
			if req.ExpiresInSec > 86400 {
				req.ExpiresInSec = 86400
			}
			expiresIn = time.Duration(req.ExpiresInSec) * time.Second
		}
		sessionExpiresAt := time.Now().UTC().Add(expiresIn)
		if linkExpiresAt != nil && linkExpiresAt.Before(sessionExpiresAt) {
			sessionExpiresAt = *linkExpiresAt
		}

		requiredConf := s.cfg.ConfirmationForChain(req.Chain)
		minAccept, maxAccept := payment.ComputeBounds(amount, s.cfg.DefaultTolerancePercent)
		sessionID := ids.New()
		intentID := ids.New()
		addressID := ids.New()
		wallet, err := s.chain.GenerateDepositWallet(r.Context(), req.Chain, intentID)
		if err != nil {
			return 0, nil, err
		}
		walletType := strings.TrimSpace(wallet.WalletType)
		if walletType == "" {
			walletType = chain.WalletTypeEOA
		}
		var encryptedPK any
		if walletType == chain.WalletTypeEOA {
			if strings.TrimSpace(wallet.PrivateKeyHex) == "" {
				return 0, nil, errors.New("failed to generate deposit wallet")
			}
			encrypted, err := cryptox.EncryptString(s.encryptKey, wallet.PrivateKeyHex)
			if err != nil {
				return 0, nil, err
			}
			encryptedPK = encrypted
		}

		custAddressRaw, _ := json.Marshal(req.CustomerAddress)

		tx, err := s.db.Begin(r.Context())
		if err != nil {
			return 0, nil, err
		}
		defer tx.Rollback(r.Context())

		_, err = tx.Exec(r.Context(), `
			INSERT INTO checkout_sessions(
				id, payment_link_id, merchant_id, status, customer_email, customer_name, customer_phone, customer_address,
				amount, currency, chain, token_symbol, token_address, expires_at, success_url, created_at, updated_at
			)
			VALUES($1, $2, $3, 'awaiting_funds', $4, $5, $6, $7::jsonb, $8, $9, $10, $11, $12, $13, $14, NOW(), NOW())
		`, sessionID, linkIDStr, reqAuth.MerchantID, req.CustomerEmail, req.CustomerName, req.CustomerPhone, string(custAddressRaw), amount, currency,
			strings.ToLower(req.Chain), strings.ToUpper(req.TokenSymbol), req.TokenAddress, sessionExpiresAt, req.SuccessURL)
		if err != nil {
			return 0, nil, err
		}

		_, err = tx.Exec(r.Context(), `
			INSERT INTO payment_intents(
				id, checkout_session_id, payment_link_id, merchant_id, status, chain, token_symbol, token_address,
				expected_amount, tolerance_percent, min_acceptable_amount, max_acceptable_amount, received_amount,
				confirmations, required_confirmations, expires_at, created_at, updated_at
			)
			VALUES(
				$1, $2, $3, $4, 'awaiting_funds', $5, $6, $7,
				$8, $9, $10, $11, 0,
				0, $12, $13, NOW(), NOW()
			)
		`, intentID, sessionID, linkIDStr, reqAuth.MerchantID, strings.ToLower(req.Chain), strings.ToUpper(req.TokenSymbol), req.TokenAddress,
			amount, s.cfg.DefaultTolerancePercent, minAccept, maxAccept, requiredConf, sessionExpiresAt)
		if err != nil {
			return 0, nil, err
		}

		_, err = tx.Exec(r.Context(), `
				INSERT INTO deposit_addresses(
					id, payment_intent_id, merchant_id, chain, address, encrypted_private_key,
					wallet_type, factory_address, wallet_salt, init_code_hash, status, created_at
				)
				VALUES($1, $2, $3, $4, $5, $6, $7, NULLIF($8, ''), NULLIF($9, ''), NULLIF($10, ''), 'active', NOW())
			`, addressID, intentID, reqAuth.MerchantID, strings.ToLower(req.Chain), wallet.Address, encryptedPK, walletType, wallet.FactoryAddress, wallet.WalletSalt, wallet.InitCodeHash)
		if err != nil {
			return 0, nil, err
		}

		if err = s.enqueueEventTx(r.Context(), tx, "payment_intent", intentID, reqAuth.MerchantID, "payment_intent.created", map[string]any{
			"payment_intent_id": intentID,
			"checkout_session":  sessionID,
			"payment_link_id":   linkIDStr,
			"chain":             strings.ToLower(req.Chain),
			"token_symbol":      strings.ToUpper(req.TokenSymbol),
			"expected_amount":   amount,
			"add_invoice_pdf":   addInvoice,
		}); err != nil {
			return 0, nil, err
		}

		if err = tx.Commit(r.Context()); err != nil {
			return 0, nil, err
		}

		return http.StatusCreated, map[string]any{
			"id":                sessionID,
			"payment_intent_id": intentID,
			"payment_link_code": code,
			"status":            "awaiting_funds",
			"amount":            amount,
			"currency":          currency,
			"chain":             strings.ToLower(req.Chain),
			"token_symbol":      strings.ToUpper(req.TokenSymbol),
			"deposit_address": map[string]any{
				"address": wallet.Address,
				"chain":   strings.ToLower(req.Chain),
			},
			"tolerance_percent":     s.cfg.DefaultTolerancePercent,
			"min_acceptable_amount": minAccept,
			"max_acceptable_amount": maxAccept,
			"expires_at":            sessionExpiresAt,
			"after_payment": map[string]any{
				"type":         afterType,
				"redirect_url": redirectURL,
			},
		}, nil
	})
}

func (s *Server) handleGetCheckoutSession(w http.ResponseWriter, r *http.Request) {
	reqAuth, ok := mustRequester(w, r)
	if !ok {
		return
	}
	sessionID, err := ids.Parse(strings.TrimSpace(chi.URLParam(r, "session_id")))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "invalid session id", middleware.GetRequestID(r.Context()))
		return
	}
	query := `
		SELECT c.id::text, c.payment_link_id::text, c.status, c.amount::text, c.currency, c.chain, c.token_symbol, c.token_address,
			c.customer_email, c.customer_name, c.customer_phone, c.customer_address,
			c.expires_at, c.success_url, c.created_at,
			i.id::text, i.status, i.expected_amount::text, i.received_amount::text, i.tolerance_percent::text,
			i.min_acceptable_amount::text, i.max_acceptable_amount::text, i.confirmations, i.required_confirmations, i.tx_hash,
			d.address
		FROM checkout_sessions c
		JOIN payment_intents i ON i.checkout_session_id=c.id
		JOIN deposit_addresses d ON d.payment_intent_id=i.id
		WHERE c.id=$1 AND c.merchant_id=$2
	`
	var sid, linkID, sessStatus, amountRaw, currency, chainName, tokenSymbol string
	var tokenAddress, customerEmail, customerName, customerPhone, successURL, txHash *string
	var customerAddress []byte
	var expiresAt, createdAt time.Time
	var intentID, intentStatus, expectedRaw, receivedRaw, tolRaw, minRaw, maxRaw string
	var confirmations, requiredConfs int
	var depositAddress string
	if err := s.db.QueryRow(r.Context(), query, sessionID, reqAuth.MerchantID).Scan(
		&sid, &linkID, &sessStatus, &amountRaw, &currency, &chainName, &tokenSymbol, &tokenAddress,
		&customerEmail, &customerName, &customerPhone, &customerAddress,
		&expiresAt, &successURL, &createdAt,
		&intentID, &intentStatus, &expectedRaw, &receivedRaw, &tolRaw,
		&minRaw, &maxRaw, &confirmations, &requiredConfs, &txHash,
		&depositAddress,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.WriteError(w, http.StatusNotFound, "not_found", "checkout session not found", middleware.GetRequestID(r.Context()))
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to fetch checkout session", middleware.GetRequestID(r.Context()))
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"id":            sid,
		"payment_link":  linkID,
		"status":        sessStatus,
		"amount":        parseFloatMaybe(amountRaw),
		"currency":      currency,
		"chain":         chainName,
		"token_symbol":  tokenSymbol,
		"token_address": tokenAddress,
		"customer": map[string]any{
			"email":   customerEmail,
			"name":    customerName,
			"phone":   customerPhone,
			"address": parseRawJSON(customerAddress),
		},
		"expires_at":  expiresAt,
		"success_url": successURL,
		"created_at":  createdAt,
		"payment_intent": map[string]any{
			"id":                     intentID,
			"status":                 intentStatus,
			"expected_amount":        parseFloatMaybe(expectedRaw),
			"received_amount":        parseFloatMaybe(receivedRaw),
			"tolerance_percent":      parseFloatMaybe(tolRaw),
			"min_acceptable_amount":  parseFloatMaybe(minRaw),
			"max_acceptable_amount":  parseFloatMaybe(maxRaw),
			"confirmations":          confirmations,
			"required_confirmations": requiredConfs,
			"tx_hash":                txHash,
			"deposit_address":        depositAddress,
		},
	})
}

func (s *Server) handleConfirmCheckoutSession(w http.ResponseWriter, r *http.Request) {
	reqAuth, ok := mustRequester(w, r)
	if !ok {
		return
	}
	sessionID, err := ids.Parse(strings.TrimSpace(chi.URLParam(r, "session_id")))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "invalid session id", middleware.GetRequestID(r.Context()))
		return
	}
	var req confirmCheckoutRequest
	if !s.parseJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.TxHash) == "" {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "tx_hash is required", middleware.GetRequestID(r.Context()))
		return
	}

	query := `
		SELECT i.id::text, i.payment_link_id::text, i.expected_amount::text, i.tolerance_percent::text, i.required_confirmations,
			i.chain, i.token_symbol, c.currency, p.title, COALESCE(l.add_invoice_pdf, false)
		FROM payment_intents i
		JOIN checkout_sessions c ON c.id=i.checkout_session_id
		JOIN payment_links p ON p.id=i.payment_link_id
		LEFT JOIN link_options l ON l.payment_link_id=p.id
		WHERE c.id=$1 AND i.merchant_id=$2
	`
	var intentIDStr, linkIDStr, expectedRaw, toleranceRaw string
	var requiredConfs int
	var chainName, tokenSymbol, currency, title string
	var addInvoice bool
	if err := s.db.QueryRow(r.Context(), query, sessionID, reqAuth.MerchantID).Scan(
		&intentIDStr, &linkIDStr, &expectedRaw, &toleranceRaw, &requiredConfs,
		&chainName, &tokenSymbol, &currency, &title, &addInvoice,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.WriteError(w, http.StatusNotFound, "not_found", "checkout session not found", middleware.GetRequestID(r.Context()))
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to load payment intent", middleware.GetRequestID(r.Context()))
		return
	}

	expected := parseFloatMaybe(expectedRaw).(float64)
	tolerance := parseFloatMaybe(toleranceRaw).(float64)
	newStatus := payment.ResolveIntentStatus(expected, req.ReceivedAmount, tolerance, req.Confirmations, requiredConfs)
	statusIsPaid := payment.IntentStatusIsPaid(newStatus)
	txStatus := "detected"
	if req.Confirmations >= requiredConfs {
		txStatus = "confirmed"
	}

	intentID, _ := ids.Parse(intentIDStr)
	rawPayload, _ := json.Marshal(req.RawPayload)

	tx, err := s.db.Begin(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to begin transaction", middleware.GetRequestID(r.Context()))
		return
	}
	defer tx.Rollback(r.Context())

	_, err = tx.Exec(r.Context(), `
		INSERT INTO chain_transactions(
			id, payment_intent_id, chain, tx_hash, block_number, from_address, to_address, amount,
			token_symbol, token_address, confirmations, status, raw_payload, observed_at, created_at
		)
		VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9, NULL, $10, $11, $12::jsonb, NOW(), NOW())
		ON CONFLICT (chain, tx_hash, payment_intent_id)
		DO UPDATE SET confirmations=EXCLUDED.confirmations, status=EXCLUDED.status, raw_payload=EXCLUDED.raw_payload
	`, ids.New(), intentID, chainName, req.TxHash, req.BlockNumber, req.FromAddress, req.ToAddress, req.ReceivedAmount,
		tokenSymbol, req.Confirmations, txStatus, string(rawPayload))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to write chain transaction", middleware.GetRequestID(r.Context()))
		return
	}

	var confirmedAt any
	if statusIsPaid {
		confirmedAt = time.Now().UTC()
	}
	_, err = tx.Exec(r.Context(), `
		UPDATE payment_intents
		SET status=$1, received_amount=$2, tx_hash=$3, confirmations=$4, confirmed_at=$5, updated_at=NOW()
		WHERE id=$6
	`, newStatus, req.ReceivedAmount, req.TxHash, req.Confirmations, confirmedAt, intentID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to update payment intent", middleware.GetRequestID(r.Context()))
		return
	}

	sessionStatus := "awaiting_funds"
	if statusIsPaid {
		sessionStatus = "paid"
	} else if newStatus == "expired" || newStatus == "failed" {
		sessionStatus = "failed"
	}
	_, err = tx.Exec(r.Context(), `UPDATE checkout_sessions SET status=$1, updated_at=NOW() WHERE id=$2`, sessionStatus, sessionID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to update checkout session", middleware.GetRequestID(r.Context()))
		return
	}

	_ = s.enqueueEventTx(r.Context(), tx, "payment_intent", intentIDStr, reqAuth.MerchantID, "payment.detected", map[string]any{
		"payment_intent_id": intentID,
		"tx_hash":           req.TxHash,
		"received_amount":   req.ReceivedAmount,
		"confirmations":     req.Confirmations,
		"status":            newStatus,
	})
	if statusIsPaid {
		_ = s.enqueueEventTx(r.Context(), tx, "payment_intent", intentIDStr, reqAuth.MerchantID, "payment.confirmed", map[string]any{
			"payment_intent_id": intentID,
			"tx_hash":           req.TxHash,
			"received_amount":   req.ReceivedAmount,
		})
	}

	if err = tx.Commit(r.Context()); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to commit", middleware.GetRequestID(r.Context()))
		return
	}

	if statusIsPaid && addInvoice {
		if objectKey, invErr := storage.StoreInvoicePDF(r.Context(), s.minio, reqAuth.MerchantID, intentID, req.ReceivedAmount, currency, title); invErr == nil && objectKey != "" {
			_, _ = s.db.Exec(r.Context(), `
				INSERT INTO invoices(id, payment_intent_id, merchant_id, object_key, amount, currency, created_at)
				VALUES($1, $2, $3, $4, $5, $6, NOW())
				ON CONFLICT (payment_intent_id) DO NOTHING
			`, ids.New(), intentID, reqAuth.MerchantID, objectKey, req.ReceivedAmount, currency)
		}
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"checkout_session_id":    sessionID,
		"payment_intent_id":      intentID,
		"status":                 newStatus,
		"confirmations":          req.Confirmations,
		"required_confirmations": requiredConfs,
	})
}

func (s *Server) handleGetPaymentIntent(w http.ResponseWriter, r *http.Request) {
	reqAuth, ok := mustRequester(w, r)
	if !ok {
		return
	}
	intentID := strings.TrimSpace(chi.URLParam(r, "id"))
	if intentID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "id is required", middleware.GetRequestID(r.Context()))
		return
	}
	query := `
		SELECT i.id::text, i.checkout_session_id::text, i.payment_link_id::text, i.status, i.chain, i.token_symbol, i.token_address,
			i.expected_amount::text, i.tolerance_percent::text, i.min_acceptable_amount::text, i.max_acceptable_amount::text,
			i.received_amount::text, i.tx_hash, i.confirmations, i.required_confirmations,
			i.confirmed_at, i.settled_at, i.expires_at, i.created_at, i.updated_at,
			d.address
		FROM payment_intents i
		LEFT JOIN deposit_addresses d ON d.payment_intent_id=i.id
		WHERE i.merchant_id=$1 AND i.id::text=$2
	`
	var id, sessionID, linkID, status, chainName, tokenSymbol string
	var tokenAddress, txHash, depositAddress *string
	var expectedRaw, tolRaw, minRaw, maxRaw, receivedRaw string
	var confs, requiredConfs int
	var confirmedAt, settledAt *time.Time
	var expiresAt, createdAt, updatedAt time.Time
	if err := s.db.QueryRow(r.Context(), query, reqAuth.MerchantID, intentID).Scan(
		&id, &sessionID, &linkID, &status, &chainName, &tokenSymbol, &tokenAddress,
		&expectedRaw, &tolRaw, &minRaw, &maxRaw, &receivedRaw, &txHash, &confs, &requiredConfs,
		&confirmedAt, &settledAt, &expiresAt, &createdAt, &updatedAt,
		&depositAddress,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.WriteError(w, http.StatusNotFound, "not_found", "payment intent not found", middleware.GetRequestID(r.Context()))
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to fetch payment intent", middleware.GetRequestID(r.Context()))
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"id":                     id,
		"checkout_session_id":    sessionID,
		"payment_link_id":        linkID,
		"status":                 status,
		"chain":                  chainName,
		"token_symbol":           tokenSymbol,
		"token_address":          tokenAddress,
		"expected_amount":        parseFloatMaybe(expectedRaw),
		"tolerance_percent":      parseFloatMaybe(tolRaw),
		"min_acceptable_amount":  parseFloatMaybe(minRaw),
		"max_acceptable_amount":  parseFloatMaybe(maxRaw),
		"received_amount":        parseFloatMaybe(receivedRaw),
		"tx_hash":                txHash,
		"confirmations":          confs,
		"required_confirmations": requiredConfs,
		"confirmed_at":           confirmedAt,
		"settled_at":             settledAt,
		"expires_at":             expiresAt,
		"deposit_address":        depositAddress,
		"created_at":             createdAt,
		"updated_at":             updatedAt,
	})
}
