package gateway

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"

	cpayv1 "github.com/cpay-dev/cpay/internal/gen/cpay/v1"
	"github.com/cpay-dev/cpay/internal/shared/chain"
	"github.com/cpay-dev/cpay/internal/shared/httpx"
	"github.com/cpay-dev/cpay/internal/shared/middleware"
	"github.com/ethereum/go-ethereum/common"
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

type createWebhookEndpointRequest struct {
	URL         string   `json:"url"`
	Description string   `json:"description,omitempty"`
	Events      []string `json:"events,omitempty"`
	MaxRetries  *int     `json:"max_retries,omitempty"`
}

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

	s.withIdempotency(w, r, reqAuth.MerchantID, "/v1/payment_links/:id/sessions", func() (int, any, error) {
		resp, err := s.checkoutClient.CreateCheckoutSession(s.rpcContext(r.Context()), &cpayv1.CreateCheckoutSessionRequest{
			MerchantId:          reqAuth.MerchantID,
			LinkIdentifier:      linkIdentifier,
			Amount:              req.Amount,
			Chain:               req.Chain,
			TokenSymbol:         req.TokenSymbol,
			TokenAddress:        req.TokenAddress,
			CustomerEmail:       req.CustomerEmail,
			CustomerName:        req.CustomerName,
			CustomerPhone:       req.CustomerPhone,
			CustomerAddressJson: mustJSON(req.CustomerAddress, "{}"),
			SuccessUrl:          req.SuccessURL,
			ExpiresInSec:        int32(req.ExpiresInSec),
			MetadataJson:        mustJSON(req.Metadata, "{}"),
		})
		if err != nil {
			return 0, nil, err
		}
		s.recordCheckoutConversionEventBySession(r.Context(), r, resp.GetId(), conversionEventPayClicked, map[string]any{"source": "merchant_api"})
		return http.StatusCreated, checkoutSessionCreateToResponse(resp), nil
	})
}

func (s *Server) handleCreatePublicCheckoutSession(w http.ResponseWriter, r *http.Request) {
	linkIdentifier := strings.TrimSpace(chi.URLParam(r, "id"))
	if linkIdentifier == "" {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "link id is required", middleware.GetRequestID(r.Context()))
		return
	}
	var req createCheckoutSessionRequest
	if !s.parseJSON(w, r, &req) {
		return
	}
	resp, err := s.checkoutClient.CreatePublicCheckoutSession(s.rpcContext(r.Context()), &cpayv1.CreatePublicCheckoutSessionRequest{
		LinkIdentifier:      linkIdentifier,
		Amount:              req.Amount,
		Chain:               req.Chain,
		TokenSymbol:         req.TokenSymbol,
		TokenAddress:        req.TokenAddress,
		CustomerEmail:       req.CustomerEmail,
		CustomerName:        req.CustomerName,
		CustomerPhone:       req.CustomerPhone,
		CustomerAddressJson: mustJSON(req.CustomerAddress, "{}"),
		SuccessUrl:          req.SuccessURL,
		ExpiresInSec:        int32(req.ExpiresInSec),
		MetadataJson:        mustJSON(req.Metadata, "{}"),
	})
	if err != nil {
		s.writeRPCError(w, r, err)
		return
	}
	payload := checkoutSessionCreateToResponse(resp)
	payload["client_secret"] = resp.GetClientSecret()
	s.recordCheckoutConversionEventBySession(r.Context(), r, resp.GetId(), conversionEventPayClicked, map[string]any{"source": "public_checkout"})
	httpx.WriteJSON(w, http.StatusCreated, payload)
}

func checkoutSessionCreateToResponse(resp *cpayv1.CreateCheckoutSessionResponse) map[string]any {
	payload := map[string]any{
		"id":                    resp.GetId(),
		"payment_intent_id":     resp.GetPaymentIntentId(),
		"payment_link_code":     resp.GetPaymentLinkCode(),
		"status":                resp.GetStatus(),
		"amount":                resp.GetAmount(),
		"currency":              resp.GetCurrency(),
		"chain":                 resp.GetChain(),
		"token_symbol":          resp.GetTokenSymbol(),
		"expected_amount":       resp.GetExpectedAmount(),
		"deposit_address":       resp.GetDepositAddress(),
		"tolerance_percent":     resp.GetTolerancePercent(),
		"min_acceptable_amount": resp.GetMinAcceptableAmount(),
		"max_acceptable_amount": resp.GetMaxAcceptableAmount(),
		"expires_at":            resp.GetExpiresAt(),
		"after_payment": map[string]any{
			"type":         resp.GetAfterPaymentType(),
			"redirect_url": emptyToNil(resp.GetAfterPaymentRedirectUrl()),
		},
	}
	if walletTx, decimals, ok := browserWalletTransaction(resp); ok {
		payload["browser_wallet_transaction"] = walletTx
		payload["token_decimals"] = decimals
	}
	return payload
}

func browserWalletTransaction(resp *cpayv1.CreateCheckoutSessionResponse) (map[string]any, int, bool) {
	chainID, ok := evmChainID(resp.GetChain())
	if !ok || !common.IsHexAddress(resp.GetDepositAddress()) {
		return nil, 0, false
	}
	amount := resp.GetExpectedAmount()
	if amount <= 0 {
		amount = resp.GetAmount()
	}
	if decimals, ok := nativeTokenDecimals(resp.GetChain(), resp.GetTokenSymbol()); ok {
		value, err := decimalToBaseUnitHex(formatTokenAmount(amount, decimals), decimals)
		if err != nil {
			return nil, 0, false
		}
		return map[string]any{
			"chain_id": chainID,
			"to":       common.HexToAddress(resp.GetDepositAddress()).Hex(),
			"value":    value,
			"data":     "0x",
		}, decimals, true
	}

	contract, ok := chain.KnownEVMTokenContract(resp.GetChain(), resp.GetTokenSymbol())
	if !ok || !common.IsHexAddress(contract.Address) {
		return nil, 0, false
	}
	tokenAmount, err := decimalToBaseUnits(formatTokenAmount(amount, contract.Decimals), contract.Decimals)
	if err != nil {
		return nil, 0, false
	}
	return map[string]any{
		"chain_id": chainID,
		"to":       common.HexToAddress(contract.Address).Hex(),
		"value":    "0x0",
		"data":     erc20TransferData(resp.GetDepositAddress(), tokenAmount),
	}, contract.Decimals, true
}

func formatTokenAmount(amount float64, decimals int) string {
	formatted := strconv.FormatFloat(amount, 'f', decimals, 64)
	formatted = strings.TrimRight(formatted, "0")
	formatted = strings.TrimRight(formatted, ".")
	if formatted == "" {
		return "0"
	}
	return formatted
}

func evmChainID(chainName string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(chainName)) {
	case "ethereum", "ethereum mainnet", "mainnet":
		return "0x1", true
	case "optimism":
		return "0xa", true
	case "bsc", "bnb", "bnb smart chain":
		return "0x38", true
	case "polygon":
		return "0x89", true
	case "arbitrum", "arbitrum one":
		return "0xa4b1", true
	case "base":
		return "0x2105", true
	case "avalanche", "avalanche c-chain", "avax":
		return "0xa86a", true
	case "hyperevm", "hyper evm", "hyperliquid", "hyperliquid evm":
		return "0x3e7", true
	default:
		return "", false
	}
}

func nativeTokenDecimals(chainName, symbol string) (int, bool) {
	chainName = strings.ToLower(strings.TrimSpace(chainName))
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	switch chainName {
	case "ethereum", "ethereum mainnet", "mainnet", "optimism", "arbitrum", "arbitrum one", "base", "hyperevm", "hyper evm", "hyperliquid", "hyperliquid evm":
		return 18, symbol == "ETH" || symbol == "HYPE"
	case "bsc", "bnb", "bnb smart chain":
		return 18, symbol == "BNB"
	case "polygon":
		return 18, symbol == "MATIC" || symbol == "POL"
	case "avalanche", "avalanche c-chain", "avax":
		return 18, symbol == "AVAX"
	default:
		return 0, false
	}
}

func erc20TransferData(to string, amount *big.Int) string {
	address := common.HexToAddress(to)
	return "0xa9059cbb" + strings.Repeat("0", 24) + strings.TrimPrefix(strings.ToLower(address.Hex()), "0x") + fmt.Sprintf("%064x", amount)
}

func decimalToBaseUnitHex(raw string, decimals int) (string, error) {
	amount, err := decimalToBaseUnits(raw, decimals)
	if err != nil {
		return "", err
	}
	return "0x" + amount.Text(16), nil
}

func decimalToBaseUnits(raw string, decimals int) (*big.Int, error) {
	if decimals < 0 {
		return nil, errors.New("decimals must be non-negative")
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("amount is required")
	}
	parts := strings.Split(raw, ".")
	if len(parts) > 2 {
		return nil, errors.New("amount is invalid")
	}
	whole := strings.TrimSpace(parts[0])
	frac := ""
	if len(parts) == 2 {
		frac = strings.TrimSpace(parts[1])
	}
	if strings.HasPrefix(whole, "+") {
		whole = strings.TrimPrefix(whole, "+")
	}
	if whole == "" {
		whole = "0"
	}
	if strings.HasPrefix(whole, "-") || len(frac) > decimals {
		return nil, errors.New("amount is invalid")
	}
	if frac != "" && strings.ContainsAny(frac, "+-") {
		return nil, errors.New("amount is invalid")
	}
	frac += strings.Repeat("0", decimals-len(frac))
	value := new(big.Int)
	if _, ok := value.SetString(whole+frac, 10); !ok {
		return nil, errors.New("amount is invalid")
	}
	return value, nil
}

func (s *Server) handleGetCheckoutSession(w http.ResponseWriter, r *http.Request) {
	reqAuth, ok := mustRequester(w, r)
	if !ok {
		return
	}
	sessionID := strings.TrimSpace(chi.URLParam(r, "session_id"))
	if sessionID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "session_id is required", middleware.GetRequestID(r.Context()))
		return
	}
	resp, err := s.checkoutClient.GetCheckoutSession(s.rpcContext(r.Context()), &cpayv1.GetCheckoutSessionRequest{MerchantId: reqAuth.MerchantID, SessionId: sessionID})
	if err != nil {
		s.writeRPCError(w, r, err)
		return
	}
	intent := resp.GetPaymentIntent()
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"id":            resp.GetId(),
		"payment_link":  resp.GetPaymentLinkId(),
		"status":        resp.GetStatus(),
		"amount":        resp.GetAmount(),
		"currency":      resp.GetCurrency(),
		"chain":         resp.GetChain(),
		"token_symbol":  resp.GetTokenSymbol(),
		"token_address": emptyToNil(resp.GetTokenAddress()),
		"customer": map[string]any{
			"email":   emptyToNil(resp.GetCustomerEmail()),
			"name":    emptyToNil(resp.GetCustomerName()),
			"phone":   emptyToNil(resp.GetCustomerPhone()),
			"address": parseJSONValue(resp.GetCustomerAddressJson(), map[string]any{}),
		},
		"expires_at":  resp.GetExpiresAt(),
		"success_url": emptyToNil(resp.GetSuccessUrl()),
		"created_at":  resp.GetCreatedAt(),
		"payment_intent": map[string]any{
			"id":                     intent.GetId(),
			"status":                 intent.GetStatus(),
			"expected_amount":        intent.GetExpectedAmount(),
			"received_amount":        intent.GetReceivedAmount(),
			"tolerance_percent":      intent.GetTolerancePercent(),
			"min_acceptable_amount":  intent.GetMinAcceptableAmount(),
			"max_acceptable_amount":  intent.GetMaxAcceptableAmount(),
			"confirmations":          intent.GetConfirmations(),
			"required_confirmations": intent.GetRequiredConfirmations(),
			"tx_hash":                emptyToNil(intent.GetTxHash()),
			"deposit_address":        emptyToNil(intent.GetDepositAddress()),
		},
	})
}

func (s *Server) handleGetPublicCheckoutSession(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(chi.URLParam(r, "session_id"))
	if sessionID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "session_id is required", middleware.GetRequestID(r.Context()))
		return
	}
	resp, err := s.checkoutClient.GetPublicCheckoutSession(s.rpcContext(r.Context()), &cpayv1.GetPublicCheckoutSessionRequest{SessionId: sessionID})
	if err != nil {
		s.writeRPCError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"id":                     resp.GetId(),
		"status":                 resp.GetStatus(),
		"amount":                 resp.GetAmount(),
		"currency":               resp.GetCurrency(),
		"chain":                  resp.GetChain(),
		"token_symbol":           resp.GetTokenSymbol(),
		"expires_at":             resp.GetExpiresAt(),
		"payment_intent_id":      resp.GetPaymentIntentId(),
		"payment_intent_status":  resp.GetPaymentIntentStatus(),
		"received_amount":        resp.GetReceivedAmount(),
		"confirmations":          resp.GetConfirmations(),
		"required_confirmations": resp.GetRequiredConfirmations(),
		"transactions":           checkoutTransactionsJSON(resp.GetTransactions()),
		"deposit_address":        resp.GetDepositAddress(),
		"link": map[string]any{
			"title":       resp.GetLinkTitle(),
			"description": emptyToNil(resp.GetLinkDescription()),
			"image_url":   emptyToNil(resp.GetLinkImageUrl()),
			"cta_text":    resp.GetCtaText(),
		},
		"after_payment": map[string]any{
			"type":            resp.GetAfterPaymentType(),
			"redirect_url":    emptyToNil(resp.GetAfterPaymentRedirectUrl()),
			"success_message": emptyToNil(resp.GetAfterPaymentSuccessMessage()),
		},
	})
}

func checkoutTransactionsJSON(transactions []*cpayv1.CheckoutTransaction) []map[string]any {
	items := make([]map[string]any, 0, len(transactions))
	for _, tx := range transactions {
		if tx == nil {
			continue
		}
		item := map[string]any{
			"tx_hash":       tx.GetTxHash(),
			"amount":        tx.GetAmount(),
			"chain":         tx.GetChain(),
			"token_symbol":  tx.GetTokenSymbol(),
			"confirmations": tx.GetConfirmations(),
			"status":        tx.GetStatus(),
		}
		if tx.GetHasBlockNumber() {
			item["block_number"] = tx.GetBlockNumber()
		}
		items = append(items, item)
	}
	return items
}

func (s *Server) handleConfirmCheckoutSession(w http.ResponseWriter, r *http.Request) {
	reqAuth, ok := mustRequester(w, r)
	if !ok {
		return
	}
	sessionID := strings.TrimSpace(chi.URLParam(r, "session_id"))
	if sessionID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "session_id is required", middleware.GetRequestID(r.Context()))
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
	rawPayload, _ := json.Marshal(req.RawPayload)
	rpcReq := &cpayv1.ConfirmCheckoutSessionRequest{
		MerchantId:     reqAuth.MerchantID,
		SessionId:      sessionID,
		TxHash:         req.TxHash,
		ReceivedAmount: req.ReceivedAmount,
		Confirmations:  int32(req.Confirmations),
		FromAddress:    req.FromAddress,
		ToAddress:      req.ToAddress,
		RawPayloadJson: string(rawPayload),
	}
	if req.BlockNumber != nil {
		rpcReq.HasBlockNumber = true
		rpcReq.BlockNumber = *req.BlockNumber
	}

	resp, err := s.checkoutClient.ConfirmCheckoutSession(s.rpcContext(r.Context()), rpcReq)
	if err != nil {
		s.writeRPCError(w, r, err)
		return
	}
	s.recordCheckoutConversionEventBySession(r.Context(), r, sessionID, conversionEventPaymentMade, map[string]any{
		"tx_hash":          req.TxHash,
		"received_amount":  req.ReceivedAmount,
		"confirmations":    resp.GetConfirmations(),
		"payment_status":   resp.GetStatus(),
		"confirmation_src": "merchant_api",
	})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"id":                     resp.GetCheckoutSessionId(),
		"checkout_session_id":    resp.GetCheckoutSessionId(),
		"payment_intent_id":      resp.GetPaymentIntentId(),
		"status":                 resp.GetStatus(),
		"payment_intent_status":  resp.GetStatus(),
		"received_amount":        req.ReceivedAmount,
		"tx_hash":                req.TxHash,
		"confirmations":          resp.GetConfirmations(),
		"required_confirmations": resp.GetRequiredConfirmations(),
	})
}

func (s *Server) handleConfirmPublicCheckoutSession(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(chi.URLParam(r, "session_id"))
	if sessionID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "session_id is required", middleware.GetRequestID(r.Context()))
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

	var merchantID string
	if err := s.db.QueryRow(r.Context(), `
		SELECT merchant_id::text
		FROM checkout.checkout_sessions
		WHERE id::text=$1
	`, sessionID).Scan(&merchantID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.WriteError(w, http.StatusNotFound, "not_found", "checkout session not found", middleware.GetRequestID(r.Context()))
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to load checkout session", middleware.GetRequestID(r.Context()))
		return
	}

	rawPayload, _ := json.Marshal(req.RawPayload)
	rpcReq := &cpayv1.ConfirmCheckoutSessionRequest{
		MerchantId:     merchantID,
		SessionId:      sessionID,
		TxHash:         req.TxHash,
		ReceivedAmount: req.ReceivedAmount,
		Confirmations:  int32(req.Confirmations),
		FromAddress:    req.FromAddress,
		ToAddress:      req.ToAddress,
		RawPayloadJson: string(rawPayload),
	}
	if req.BlockNumber != nil {
		rpcReq.HasBlockNumber = true
		rpcReq.BlockNumber = *req.BlockNumber
	}
	resp, err := s.checkoutClient.ConfirmCheckoutSession(s.rpcContext(r.Context()), rpcReq)
	if err != nil {
		s.writeRPCError(w, r, err)
		return
	}
	s.recordCheckoutConversionEventBySession(r.Context(), r, sessionID, conversionEventPaymentMade, map[string]any{
		"tx_hash":          req.TxHash,
		"received_amount":  req.ReceivedAmount,
		"confirmations":    resp.GetConfirmations(),
		"payment_status":   resp.GetStatus(),
		"confirmation_src": "public_checkout",
	})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"id":                     resp.GetCheckoutSessionId(),
		"checkout_session_id":    resp.GetCheckoutSessionId(),
		"payment_intent_id":      resp.GetPaymentIntentId(),
		"status":                 resp.GetStatus(),
		"confirmations":          resp.GetConfirmations(),
		"required_confirmations": resp.GetRequiredConfirmations(),
	})
}

func (s *Server) handleGetPaymentIntent(w http.ResponseWriter, r *http.Request) {
	reqAuth, ok := mustRequester(w, r)
	if !ok {
		return
	}
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "id is required", middleware.GetRequestID(r.Context()))
		return
	}
	resp, err := s.checkoutClient.GetPaymentIntent(s.rpcContext(r.Context()), &cpayv1.GetPaymentIntentRequest{MerchantId: reqAuth.MerchantID, Id: id})
	if err != nil {
		s.writeRPCError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"id":                     resp.GetId(),
		"checkout_session_id":    resp.GetCheckoutSessionId(),
		"payment_link_id":        resp.GetPaymentLinkId(),
		"status":                 resp.GetStatus(),
		"chain":                  resp.GetChain(),
		"token_symbol":           resp.GetTokenSymbol(),
		"token_address":          emptyToNil(resp.GetTokenAddress()),
		"expected_amount":        resp.GetExpectedAmount(),
		"tolerance_percent":      resp.GetTolerancePercent(),
		"min_acceptable_amount":  resp.GetMinAcceptableAmount(),
		"max_acceptable_amount":  resp.GetMaxAcceptableAmount(),
		"received_amount":        resp.GetReceivedAmount(),
		"tx_hash":                emptyToNil(resp.GetTxHash()),
		"confirmations":          resp.GetConfirmations(),
		"required_confirmations": resp.GetRequiredConfirmations(),
		"confirmed_at":           emptyToNil(resp.GetConfirmedAt()),
		"settled_at":             emptyToNil(resp.GetSettledAt()),
		"expires_at":             resp.GetExpiresAt(),
		"deposit_address":        emptyToNil(resp.GetDepositAddress()),
		"created_at":             resp.GetCreatedAt(),
		"updated_at":             resp.GetUpdatedAt(),
	})
}

func (s *Server) handleListPayments(w http.ResponseWriter, r *http.Request) {
	reqAuth, ok := mustRequester(w, r)
	if !ok {
		return
	}
	limit, offset := parsePagination(r, 50, 200)
	productID := strings.TrimSpace(r.URL.Query().Get("product_id"))
	paymentLinkID := strings.TrimSpace(r.URL.Query().Get("payment_link_id"))

	rows, err := s.db.Query(r.Context(), `
		SELECT
			i.id::text, i.checkout_session_id::text, i.payment_link_id::text,
			COALESCE(pl.product_id::text, ''), COALESCE(p.name, ''), pl.title, pl.code,
			c.amount::text, c.currency, COALESCE(c.customer_email, ''),
			i.status, i.chain, i.token_symbol,
			i.expected_amount::text, i.received_amount::text, i.tx_hash,
			i.confirmations, i.required_confirmations,
			i.expires_at, i.created_at, i.updated_at,
			COALESCE(d.wallet_type, ''), COALESCE(latest_payout.payout_status, ''),
			COALESCE(latest_payout.item_status, ''), COALESCE(latest_payout.tx_hash, ''),
			COALESCE(latest_payout.last_error, ''),
			COUNT(*) OVER() AS total
		FROM checkout.payment_intents i
		JOIN checkout.checkout_sessions c ON c.id=i.checkout_session_id
		JOIN catalog.payment_links pl ON pl.id=i.payment_link_id
		LEFT JOIN catalog.products p ON p.id=pl.product_id
		LEFT JOIN checkout.deposit_addresses d ON d.payment_intent_id=i.id
		LEFT JOIN LATERAL (
			SELECT po.status AS payout_status, pi.status AS item_status, pi.tx_hash, pi.last_error
			FROM checkout.payout_items pi
			JOIN checkout.payouts po ON po.id=pi.payout_id
			WHERE pi.payment_intent_id=i.id
			ORDER BY pi.created_at DESC
			LIMIT 1
		) latest_payout ON true
		WHERE i.merchant_id::text=$1
			AND ($2='' OR pl.product_id::text=$2)
			AND ($3='' OR i.payment_link_id::text=$3)
		ORDER BY i.created_at DESC
		LIMIT $4 OFFSET $5
	`, reqAuth.MerchantID, productID, paymentLinkID, limit, offset)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to list payments", middleware.GetRequestID(r.Context()))
		return
	}
	defer rows.Close()

	items := make([]map[string]any, 0)
	total := 0
	for rows.Next() {
		item, rowTotal, err := scanPaymentListRow(rows)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to scan payments", middleware.GetRequestID(r.Context()))
			return
		}
		total = rowTotal
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to list payments", middleware.GetRequestID(r.Context()))
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"data":   items,
		"limit":  limit,
		"offset": offset,
		"total":  total,
	})
}

func scanPaymentListRow(scanner interface{ Scan(dest ...any) error }) (map[string]any, int, error) {
	var id, checkoutSessionID, linkID, productID, productName, linkTitle, linkCode, currency, customerEmail, status, chainName, tokenSymbol string
	var walletType, payoutStatus, payoutItemStatus, payoutTxHash, payoutLastError string
	var fiatAmountRaw, expectedRaw, receivedRaw string
	var txHash *string
	var confirmations, requiredConfirmations int
	var expiresAt, createdAt, updatedAt time.Time
	var total int
	if err := scanner.Scan(
		&id, &checkoutSessionID, &linkID,
		&productID, &productName, &linkTitle, &linkCode,
		&fiatAmountRaw, &currency, &customerEmail,
		&status, &chainName, &tokenSymbol,
		&expectedRaw, &receivedRaw, &txHash,
		&confirmations, &requiredConfirmations,
		&expiresAt, &createdAt, &updatedAt,
		&walletType, &payoutStatus, &payoutItemStatus, &payoutTxHash, &payoutLastError,
		&total,
	); err != nil {
		return nil, 0, err
	}

	return map[string]any{
		"id":                     id,
		"checkout_session_id":    checkoutSessionID,
		"payment_link_id":        linkID,
		"product_id":             emptyToNil(productID),
		"product_name":           emptyToNil(productName),
		"payment_link_title":     linkTitle,
		"payment_link_code":      linkCode,
		"amount":                 decimalToFloat(fiatAmountRaw),
		"currency":               currency,
		"customer_email":         emptyToNil(customerEmail),
		"status":                 status,
		"chain":                  chainName,
		"token_symbol":           tokenSymbol,
		"expected_amount":        decimalToFloat(expectedRaw),
		"received_amount":        decimalToFloat(receivedRaw),
		"tx_hash":                strPtrToAny(txHash),
		"confirmations":          confirmations,
		"required_confirmations": requiredConfirmations,
		"expires_at":             expiresAt.UTC().Format(time.RFC3339Nano),
		"created_at":             createdAt.UTC().Format(time.RFC3339Nano),
		"updated_at":             updatedAt.UTC().Format(time.RFC3339Nano),
		"wallet_type":            emptyToNil(walletType),
		"payout_status":          emptyToNil(payoutStatus),
		"payout_item_status":     emptyToNil(payoutItemStatus),
		"payout_tx_hash":         emptyToNil(payoutTxHash),
		"payout_last_error":      emptyToNil(payoutLastError),
	}, total, nil
}

func decimalToFloat(raw string) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return 0
	}
	return v
}

func (s *Server) handleCreateWebhookEndpoint(w http.ResponseWriter, r *http.Request) {
	reqAuth, ok := mustRequester(w, r)
	if !ok {
		return
	}
	var req createWebhookEndpointRequest
	if !s.parseJSON(w, r, &req) {
		return
	}
	s.withIdempotency(w, r, reqAuth.MerchantID, "/v1/webhook_endpoints", func() (int, any, error) {
		rpcReq := &cpayv1.CreateWebhookEndpointRequest{
			MerchantId:  reqAuth.MerchantID,
			Url:         req.URL,
			Description: req.Description,
			Events:      req.Events,
			MaxRetries:  0,
		}
		if req.MaxRetries != nil {
			rpcReq.MaxRetries = int32(*req.MaxRetries)
		}
		resp, err := s.checkoutClient.CreateWebhookEndpoint(s.rpcContext(r.Context()), rpcReq)
		if err != nil {
			return 0, nil, err
		}
		return http.StatusCreated, map[string]any{
			"id":          resp.GetId(),
			"url":         resp.GetUrl(),
			"description": resp.GetDescription(),
			"events":      resp.GetEvents(),
			"max_retries": resp.GetMaxRetries(),
			"secret":      resp.GetSecret(),
		}, nil
	})
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

	s.withIdempotency(w, r, reqAuth.MerchantID, "/v1/subscriptions", func() (int, any, error) {
		rpcReq := &cpayv1.CreateSubscriptionRequest{
			MerchantId:           reqAuth.MerchantID,
			CustomerRef:          req.CustomerRef,
			Chain:                req.Chain,
			TokenSymbol:          req.TokenSymbol,
			TokenAddress:         req.TokenAddress,
			Amount:               req.Amount,
			Currency:             req.Currency,
			IntervalUnit:         req.IntervalUnit,
			IntervalCount:        int32(req.IntervalCount),
			VaultContractAddress: req.Vault.ContractAddress,
			VaultCustomerWallet:  req.Vault.CustomerWallet,
			VaultMaxTotalAmount:  req.Vault.MaxTotalAmount,
			VaultRemainingAmount: req.Vault.RemainingAmount,
			MetadataJson:         mustJSON(req.Metadata, "{}"),
		}
		if req.PaymentLinkID != nil {
			rpcReq.PaymentLinkId = strings.TrimSpace(*req.PaymentLinkID)
		}
		if req.FirstBillingAt != nil {
			rpcReq.FirstBillingAt = req.FirstBillingAt.UTC().Format(time.RFC3339)
		}
		if req.Vault.ExpiresAt != nil {
			rpcReq.VaultExpiresAt = req.Vault.ExpiresAt.UTC().Format(time.RFC3339)
		}
		resp, err := s.checkoutClient.CreateSubscription(s.rpcContext(r.Context()), rpcReq)
		if err != nil {
			return 0, nil, err
		}
		return http.StatusCreated, map[string]any{
			"id":              resp.GetId(),
			"status":          resp.GetStatus(),
			"chain":           resp.GetChain(),
			"token_symbol":    resp.GetTokenSymbol(),
			"amount":          resp.GetAmount(),
			"currency":        resp.GetCurrency(),
			"interval_unit":   resp.GetIntervalUnit(),
			"interval_count":  resp.GetIntervalCount(),
			"next_billing_at": resp.GetNextBillingAt(),
			"vault_authorization": map[string]any{
				"id":               resp.GetVaultAuthorizationId(),
				"remaining_amount": resp.GetVaultRemainingAmount(),
			},
			"first_cycle_id": resp.GetFirstCycleId(),
		}, nil
	})
}

func (s *Server) handlePauseSubscription(w http.ResponseWriter, r *http.Request) {
	reqAuth, ok := mustRequester(w, r)
	if !ok {
		return
	}
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "id is required", middleware.GetRequestID(r.Context()))
		return
	}
	resp, err := s.checkoutClient.PauseSubscription(s.rpcContext(r.Context()), &cpayv1.PauseSubscriptionRequest{MerchantId: reqAuth.MerchantID, Id: id})
	if err != nil {
		s.writeRPCError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"id": resp.GetId(), "status": resp.GetStatus()})
}

func (s *Server) handleResumeSubscription(w http.ResponseWriter, r *http.Request) {
	reqAuth, ok := mustRequester(w, r)
	if !ok {
		return
	}
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "id is required", middleware.GetRequestID(r.Context()))
		return
	}
	resp, err := s.checkoutClient.ResumeSubscription(s.rpcContext(r.Context()), &cpayv1.ResumeSubscriptionRequest{MerchantId: reqAuth.MerchantID, Id: id})
	if err != nil {
		s.writeRPCError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"id": resp.GetId(), "status": resp.GetStatus()})
}

func (s *Server) handleGetSubscriptionCycles(w http.ResponseWriter, r *http.Request) {
	reqAuth, ok := mustRequester(w, r)
	if !ok {
		return
	}
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "id is required", middleware.GetRequestID(r.Context()))
		return
	}
	limit := 20
	if q := r.URL.Query().Get("limit"); q != "" {
		if v, err := strconv.Atoi(q); err == nil && v > 0 && v <= 200 {
			limit = v
		}
	}
	resp, err := s.checkoutClient.GetSubscriptionCycles(s.rpcContext(r.Context()), &cpayv1.GetSubscriptionCyclesRequest{MerchantId: reqAuth.MerchantID, Id: id, Limit: int32(limit)})
	if err != nil {
		s.writeRPCError(w, r, err)
		return
	}
	items := make([]map[string]any, 0, len(resp.GetData()))
	for _, c := range resp.GetData() {
		items = append(items, map[string]any{
			"id":                c.GetId(),
			"cycle_index":       c.GetCycleIndex(),
			"period_start":      c.GetPeriodStart(),
			"period_end":        c.GetPeriodEnd(),
			"due_at":            c.GetDueAt(),
			"status":            c.GetStatus(),
			"amount":            c.GetAmount(),
			"payment_intent_id": emptyToNil(c.GetPaymentIntentId()),
			"retry_count":       c.GetRetryCount(),
			"last_error":        emptyToNil(c.GetLastError()),
			"created_at":        c.GetCreatedAt(),
			"updated_at":        c.GetUpdatedAt(),
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"subscription_id": resp.GetSubscriptionId(), "data": items})
}
