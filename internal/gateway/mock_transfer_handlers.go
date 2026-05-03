package gateway

import (
	"net/http"
	"strings"
	"time"

	cpayv1 "github.com/cpay-dev/cpay/internal/gen/cpay/v1"
	"github.com/cpay-dev/cpay/internal/shared/httpx"
	"github.com/cpay-dev/cpay/internal/shared/ids"
	"github.com/cpay-dev/cpay/internal/shared/middleware"
)

type createMockTransferRequest struct {
	SessionID      string         `json:"session_id"`
	ReceivedAmount *float64       `json:"received_amount,omitempty"`
	Confirmations  *int           `json:"confirmations,omitempty"`
	TxHash         string         `json:"tx_hash,omitempty"`
	BlockNumber    *int64         `json:"block_number,omitempty"`
	FromAddress    string         `json:"from_address,omitempty"`
	ToAddress      string         `json:"to_address,omitempty"`
	RawPayload     map[string]any `json:"raw_payload,omitempty"`
}

type mockTransferRequestedEvent struct {
	RequestID         string         `json:"request_id"`
	MerchantID        string         `json:"merchant_id"`
	SessionID         string         `json:"session_id"`
	PaymentIntentID   string         `json:"payment_intent_id"`
	TxHash            string         `json:"tx_hash"`
	ReceivedAmount    float64        `json:"received_amount"`
	Confirmations     int            `json:"confirmations"`
	BlockNumber       *int64         `json:"block_number,omitempty"`
	FromAddress       string         `json:"from_address,omitempty"`
	ToAddress         string         `json:"to_address,omitempty"`
	RawPayload        map[string]any `json:"raw_payload"`
	RequestedByUserID string         `json:"requested_by_user_id,omitempty"`
}

func (s *Server) handleCreateMockTransfer(w http.ResponseWriter, r *http.Request) {
	reqAuth, ok := s.requireUser(w, r)
	if !ok {
		return
	}

	var req createMockTransferRequest
	if !s.parseJSON(w, r, &req) {
		return
	}

	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "session_id is required", middleware.GetRequestID(r.Context()))
		return
	}
	if req.ReceivedAmount != nil && *req.ReceivedAmount <= 0 {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "received_amount must be greater than 0", middleware.GetRequestID(r.Context()))
		return
	}
	if req.Confirmations != nil && *req.Confirmations < 0 {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "confirmations must be greater than or equal to 0", middleware.GetRequestID(r.Context()))
		return
	}

	session, err := s.checkoutClient.GetCheckoutSession(s.rpcContext(r.Context()), &cpayv1.GetCheckoutSessionRequest{
		MerchantId: reqAuth.MerchantID,
		SessionId:  sessionID,
	})
	if err != nil {
		s.writeRPCError(w, r, err)
		return
	}

	intent := session.GetPaymentIntent()
	if intent == nil || strings.TrimSpace(intent.GetId()) == "" {
		httpx.WriteError(w, http.StatusUnprocessableEntity, "invalid_state", "checkout session has no payment intent", middleware.GetRequestID(r.Context()))
		return
	}

	receivedAmount := intent.GetExpectedAmount()
	if req.ReceivedAmount != nil {
		receivedAmount = *req.ReceivedAmount
	}

	confirmations := int(intent.GetRequiredConfirmations())
	if req.Confirmations != nil {
		confirmations = *req.Confirmations
	}

	txHash := strings.TrimSpace(req.TxHash)
	if txHash == "" {
		txHash = "mock_tx_" + strings.ReplaceAll(ids.New(), "-", "")
	}

	toAddress := strings.TrimSpace(req.ToAddress)
	if toAddress == "" {
		toAddress = strings.TrimSpace(intent.GetDepositAddress())
	}

	requestID := ids.New()
	eventPayload := req.RawPayload
	if eventPayload == nil {
		eventPayload = map[string]any{
			"source":       "admin_mock_transfer",
			"requested_at": time.Now().UTC().Format(time.RFC3339Nano),
			"request_id":   requestID,
		}
	}

	requestedByUserID := ""
	if reqAuth.UserID != nil {
		requestedByUserID = *reqAuth.UserID
	}

	if err := s.outbox.Enqueue(r.Context(), "mock_transfer", requestID, &reqAuth.MerchantID, "mock_transfer.requested", mockTransferRequestedEvent{
		RequestID:         requestID,
		MerchantID:        reqAuth.MerchantID,
		SessionID:         sessionID,
		PaymentIntentID:   intent.GetId(),
		TxHash:            txHash,
		ReceivedAmount:    receivedAmount,
		Confirmations:     confirmations,
		BlockNumber:       req.BlockNumber,
		FromAddress:       strings.TrimSpace(req.FromAddress),
		ToAddress:         toAddress,
		RawPayload:        eventPayload,
		RequestedByUserID: requestedByUserID,
	}); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to enqueue mock transfer", middleware.GetRequestID(r.Context()))
		return
	}

	httpx.WriteJSON(w, http.StatusAccepted, map[string]any{
		"request_id":        requestID,
		"session_id":        sessionID,
		"payment_intent_id": intent.GetId(),
		"tx_hash":           txHash,
		"received_amount":   receivedAmount,
		"confirmations":     confirmations,
		"block_number":      req.BlockNumber,
		"from_address":      strings.TrimSpace(req.FromAddress),
		"to_address":        toAddress,
		"status":            "queued",
	})
}
