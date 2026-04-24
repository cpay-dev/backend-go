package workers

import (
	"context"
	"encoding/json"
	"testing"

	cpayv1 "github.com/cpay-dev/cpay/internal/gen/cpay/v1"
	"github.com/cpay-dev/cpay/internal/shared/events"
	"google.golang.org/grpc"
)

type fakeIndexerCheckoutClient struct {
	cpayv1.CheckoutServiceClient
	confirmReq *cpayv1.ConfirmCheckoutSessionRequest
}

func (f *fakeIndexerCheckoutClient) ConfirmCheckoutSession(_ context.Context, in *cpayv1.ConfirmCheckoutSessionRequest, _ ...grpc.CallOption) (*cpayv1.ConfirmCheckoutSessionResponse, error) {
	f.confirmReq = in
	return &cpayv1.ConfirmCheckoutSessionResponse{
		CheckoutSessionId: in.GetSessionId(),
		PaymentIntentId:   "pi_123",
		Status:            "confirmed",
	}, nil
}

func TestMockTransferIndexerProcessMessageConfirmsCheckout(t *testing.T) {
	client := &fakeIndexerCheckoutClient{}
	worker := &MockTransferIndexer{
		CheckoutClient: client,
	}

	block := int64(12345)
	env, err := events.NewEnvelope("mock_transfer.requested", "test", "merchant_1", "req_1", map[string]any{
		"request_id":        "req_1",
		"merchant_id":       "merchant_1",
		"session_id":        "cs_1",
		"payment_intent_id": "pi_1",
		"tx_hash":           "mock_tx_1",
		"received_amount":   12.34,
		"confirmations":     9,
		"block_number":      block,
		"from_address":      "0xfrom",
		"to_address":        "0xto",
		"raw_payload": map[string]any{
			"kind": "mock",
		},
	})
	if err != nil {
		t.Fatalf("new envelope: %v", err)
	}
	body, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}

	worker.processMessage(context.Background(), body)

	if client.confirmReq == nil {
		t.Fatalf("confirm request was not sent")
	}
	if client.confirmReq.GetMerchantId() != "merchant_1" {
		t.Fatalf("unexpected merchant id: %s", client.confirmReq.GetMerchantId())
	}
	if client.confirmReq.GetSessionId() != "cs_1" {
		t.Fatalf("unexpected session id: %s", client.confirmReq.GetSessionId())
	}
	if !client.confirmReq.GetHasBlockNumber() || client.confirmReq.GetBlockNumber() != block {
		t.Fatalf("unexpected block number in confirm request")
	}
	if client.confirmReq.GetTxHash() != "mock_tx_1" {
		t.Fatalf("unexpected tx hash: %s", client.confirmReq.GetTxHash())
	}
	if client.confirmReq.GetReceivedAmount() != 12.34 {
		t.Fatalf("unexpected amount: %f", client.confirmReq.GetReceivedAmount())
	}
	if client.confirmReq.GetConfirmations() != 9 {
		t.Fatalf("unexpected confirmations: %d", client.confirmReq.GetConfirmations())
	}
	if client.confirmReq.GetRawPayloadJson() == "" || client.confirmReq.GetRawPayloadJson() == "{}" {
		t.Fatalf("raw payload should be forwarded")
	}
}

func TestMockTransferIndexerProcessMessageSkipsInvalidPayload(t *testing.T) {
	client := &fakeIndexerCheckoutClient{}
	worker := &MockTransferIndexer{
		CheckoutClient: client,
	}

	worker.processMessage(context.Background(), []byte(`{"invalid":"payload"}`))

	if client.confirmReq != nil {
		t.Fatalf("confirm request should not be sent for invalid payload")
	}
}
