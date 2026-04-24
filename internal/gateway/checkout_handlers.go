package gateway

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	cpayv1 "github.com/cpay-dev/cpay/internal/gen/cpay/v1"
	"github.com/cpay-dev/cpay/internal/shared/httpx"
	"github.com/cpay-dev/cpay/internal/shared/middleware"
	"github.com/go-chi/chi/v5"
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
			MerchantId:          reqAuth.MerchantID.String(),
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
	httpx.WriteJSON(w, http.StatusCreated, payload)
}

func checkoutSessionCreateToResponse(resp *cpayv1.CreateCheckoutSessionResponse) map[string]any {
	return map[string]any{
		"id":                resp.GetId(),
		"payment_intent_id": resp.GetPaymentIntentId(),
		"payment_link_code": resp.GetPaymentLinkCode(),
		"status":            resp.GetStatus(),
		"amount":            resp.GetAmount(),
		"currency":          resp.GetCurrency(),
		"chain":             resp.GetChain(),
		"token_symbol":      resp.GetTokenSymbol(),
		"deposit_address": resp.GetDepositAddress(),
		"tolerance_percent":     resp.GetTolerancePercent(),
		"min_acceptable_amount": resp.GetMinAcceptableAmount(),
		"max_acceptable_amount": resp.GetMaxAcceptableAmount(),
		"expires_at":            resp.GetExpiresAt(),
		"after_payment": map[string]any{
			"type":         resp.GetAfterPaymentType(),
			"redirect_url": emptyToNil(resp.GetAfterPaymentRedirectUrl()),
		},
	}
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
	resp, err := s.checkoutClient.GetCheckoutSession(s.rpcContext(r.Context()), &cpayv1.GetCheckoutSessionRequest{MerchantId: reqAuth.MerchantID.String(), SessionId: sessionID})
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
	clientSecret := strings.TrimSpace(r.URL.Query().Get("client_secret"))
	if clientSecret == "" {
		clientSecret = strings.TrimSpace(r.Header.Get("X-Client-Secret"))
	}
	if clientSecret == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "client_secret is required", middleware.GetRequestID(r.Context()))
		return
	}
	resp, err := s.checkoutClient.GetPublicCheckoutSession(s.rpcContext(r.Context()), &cpayv1.GetPublicCheckoutSessionRequest{SessionId: sessionID, ClientSecret: clientSecret})
	if err != nil {
		s.writeRPCError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"id":                resp.GetId(),
		"status":            resp.GetStatus(),
		"amount":            resp.GetAmount(),
		"currency":          resp.GetCurrency(),
		"chain":             resp.GetChain(),
		"token_symbol":      resp.GetTokenSymbol(),
		"expires_at":        resp.GetExpiresAt(),
		"payment_intent_id": resp.GetPaymentIntentId(),
		"deposit_address":   resp.GetDepositAddress(),
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
		MerchantId:     reqAuth.MerchantID.String(),
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
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
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
	resp, err := s.checkoutClient.GetPaymentIntent(s.rpcContext(r.Context()), &cpayv1.GetPaymentIntentRequest{MerchantId: reqAuth.MerchantID.String(), Id: id})
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
			MerchantId:  reqAuth.MerchantID.String(),
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
			MerchantId:           reqAuth.MerchantID.String(),
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
	resp, err := s.checkoutClient.PauseSubscription(s.rpcContext(r.Context()), &cpayv1.PauseSubscriptionRequest{MerchantId: reqAuth.MerchantID.String(), Id: id})
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
	resp, err := s.checkoutClient.ResumeSubscription(s.rpcContext(r.Context()), &cpayv1.ResumeSubscriptionRequest{MerchantId: reqAuth.MerchantID.String(), Id: id})
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
	resp, err := s.checkoutClient.GetSubscriptionCycles(s.rpcContext(r.Context()), &cpayv1.GetSubscriptionCyclesRequest{MerchantId: reqAuth.MerchantID.String(), Id: id, Limit: int32(limit)})
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
