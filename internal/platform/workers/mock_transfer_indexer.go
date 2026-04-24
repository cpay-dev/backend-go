package workers

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	cpayv1 "github.com/cpay-dev/cpay/internal/gen/cpay/v1"
	"github.com/cpay-dev/cpay/internal/shared/events"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"
)

type MockTransferIndexer struct {
	NATS           *nats.Conn
	CheckoutClient cpayv1.CheckoutServiceClient
	Log            zerolog.Logger
}

type mockTransferRequestedPayload struct {
	RequestID       string         `json:"request_id"`
	MerchantID      string         `json:"merchant_id"`
	SessionID       string         `json:"session_id"`
	PaymentIntentID string         `json:"payment_intent_id"`
	TxHash          string         `json:"tx_hash"`
	ReceivedAmount  float64        `json:"received_amount"`
	Confirmations   int            `json:"confirmations"`
	BlockNumber     *int64         `json:"block_number,omitempty"`
	FromAddress     string         `json:"from_address,omitempty"`
	ToAddress       string         `json:"to_address,omitempty"`
	RawPayload      map[string]any `json:"raw_payload"`
}

func (w *MockTransferIndexer) Run(ctx context.Context) {
	if w.NATS == nil {
		w.Log.Warn().Msg("mock transfer indexer disabled: nats is not configured")
		<-ctx.Done()
		return
	}
	if w.CheckoutClient == nil {
		w.Log.Warn().Msg("mock transfer indexer disabled: checkout client is not configured")
		<-ctx.Done()
		return
	}

	sub, err := w.NATS.Subscribe("events.mock_transfer.requested", func(msg *nats.Msg) {
		w.processMessage(ctx, msg.Data)
	})
	if err != nil {
		w.Log.Error().Err(err).Msg("mock transfer indexer: subscribe failed")
		<-ctx.Done()
		return
	}
	if err := w.NATS.Flush(); err != nil {
		w.Log.Error().Err(err).Msg("mock transfer indexer: flush failed")
		<-ctx.Done()
		return
	}
	defer sub.Drain()

	<-ctx.Done()
}

func (w *MockTransferIndexer) processMessage(ctx context.Context, payload []byte) {
	var env events.Envelope
	if err := json.Unmarshal(payload, &env); err != nil {
		w.Log.Error().Err(err).Msg("mock transfer indexer: decode envelope failed")
		return
	}

	var reqData mockTransferRequestedPayload
	if err := json.Unmarshal(env.Data, &reqData); err != nil {
		w.Log.Error().Err(err).Str("event_id", env.ID).Msg("mock transfer indexer: decode payload failed")
		return
	}

	if strings.TrimSpace(reqData.MerchantID) == "" || strings.TrimSpace(reqData.SessionID) == "" || strings.TrimSpace(reqData.TxHash) == "" {
		w.Log.Error().
			Str("event_id", env.ID).
			Str("merchant_id", reqData.MerchantID).
			Str("session_id", reqData.SessionID).
			Msg("mock transfer indexer: required fields are missing")
		return
	}

	rawPayload := "{}"
	if reqData.RawPayload != nil {
		if b, err := json.Marshal(reqData.RawPayload); err == nil {
			rawPayload = string(b)
		}
	}

	confirmReq := &cpayv1.ConfirmCheckoutSessionRequest{
		MerchantId:     reqData.MerchantID,
		SessionId:      reqData.SessionID,
		TxHash:         reqData.TxHash,
		ReceivedAmount: reqData.ReceivedAmount,
		Confirmations:  int32(reqData.Confirmations),
		FromAddress:    strings.TrimSpace(reqData.FromAddress),
		ToAddress:      strings.TrimSpace(reqData.ToAddress),
		RawPayloadJson: rawPayload,
	}
	if reqData.BlockNumber != nil {
		confirmReq.HasBlockNumber = true
		confirmReq.BlockNumber = *reqData.BlockNumber
	}

	rpcCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	resp, err := w.CheckoutClient.ConfirmCheckoutSession(rpcCtx, confirmReq)
	if err != nil {
		w.Log.Error().
			Err(err).
			Str("event_id", env.ID).
			Str("request_id", reqData.RequestID).
			Str("session_id", reqData.SessionID).
			Msg("mock transfer indexer: confirm checkout failed")
		return
	}

	w.Log.Info().
		Str("event_id", env.ID).
		Str("request_id", reqData.RequestID).
		Str("session_id", reqData.SessionID).
		Str("payment_intent_id", resp.GetPaymentIntentId()).
		Str("status", resp.GetStatus()).
		Msg("mock transfer indexer: checkout confirmed")
}
