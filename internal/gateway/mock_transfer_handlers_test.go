package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	cpayv1 "github.com/cpay-dev/cpay/internal/gen/cpay/v1"
	"github.com/cpay-dev/cpay/internal/shared/config"
	"github.com/google/uuid"
	"google.golang.org/grpc"
)

type fakeCheckoutClient struct {
	cpayv1.CheckoutServiceClient
	getCheckoutSessionFn func(ctx context.Context, in *cpayv1.GetCheckoutSessionRequest, opts ...grpc.CallOption) (*cpayv1.CheckoutSession, error)
}

func (f *fakeCheckoutClient) GetCheckoutSession(ctx context.Context, in *cpayv1.GetCheckoutSessionRequest, opts ...grpc.CallOption) (*cpayv1.CheckoutSession, error) {
	if f.getCheckoutSessionFn != nil {
		return f.getCheckoutSessionFn(ctx, in, opts...)
	}
	return &cpayv1.CheckoutSession{}, nil
}

type fakeGatewayOutbox struct {
	aggregateType string
	aggregateID   string
	merchantID    *uuid.UUID
	eventType     string
	payload       any
}

func (f *fakeGatewayOutbox) Enqueue(_ context.Context, aggregateType, aggregateID string, merchantID *uuid.UUID, eventType string, payload any) error {
	f.aggregateType = aggregateType
	f.aggregateID = aggregateID
	f.merchantID = merchantID
	f.eventType = eventType
	f.payload = payload
	return nil
}

func TestHandleCreateMockTransferRequiresUserToken(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/v1/mock_transfers", strings.NewReader(`{"session_id":"cs_1"}`))
	rr := httptest.NewRecorder()

	s.handleCreateMockTransfer(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestHandleCreateMockTransferRejectsAPIKeyPrincipal(t *testing.T) {
	merchantID := uuid.New()
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/v1/mock_transfers", strings.NewReader(`{"session_id":"cs_1"}`))
	req = req.WithContext(context.WithValue(req.Context(), requesterKey, requester{
		MerchantID: merchantID,
		IsUser:     false,
	}))
	rr := httptest.NewRecorder()

	s.handleCreateMockTransfer(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}

func TestHandleCreateMockTransferQueuesOutboxEventWithDefaults(t *testing.T) {
	merchantID := uuid.New()
	userID := uuid.New()
	fakeOutbox := &fakeGatewayOutbox{}
	fakeCheckout := &fakeCheckoutClient{
		getCheckoutSessionFn: func(_ context.Context, in *cpayv1.GetCheckoutSessionRequest, _ ...grpc.CallOption) (*cpayv1.CheckoutSession, error) {
			if in.GetMerchantId() != merchantID.String() {
				t.Fatalf("unexpected merchant id: %s", in.GetMerchantId())
			}
			if in.GetSessionId() != "cs_123" {
				t.Fatalf("unexpected session id: %s", in.GetSessionId())
			}
			return &cpayv1.CheckoutSession{
				Id: "cs_123",
				PaymentIntent: &cpayv1.PaymentIntent{
					Id:                    "pi_123",
					ExpectedAmount:        42.5,
					RequiredConfirmations: 12,
					DepositAddress:        "0xdeposit",
				},
			}, nil
		},
	}

	s := &Server{
		cfg:            config.Config{ServiceName: "api-gateway"},
		checkoutClient: fakeCheckout,
		outbox:         fakeOutbox,
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/mock_transfers", strings.NewReader(`{"session_id":"cs_123"}`))
	req = req.WithContext(context.WithValue(req.Context(), requesterKey, requester{
		MerchantID: merchantID,
		UserID:     &userID,
		IsUser:     true,
		Role:       "admin",
	}))
	rr := httptest.NewRecorder()

	s.handleCreateMockTransfer(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rr.Code)
	}
	if fakeOutbox.aggregateType != "mock_transfer" {
		t.Fatalf("unexpected aggregate type: %s", fakeOutbox.aggregateType)
	}
	if fakeOutbox.eventType != "mock_transfer.requested" {
		t.Fatalf("unexpected event type: %s", fakeOutbox.eventType)
	}
	payload, ok := fakeOutbox.payload.(mockTransferRequestedEvent)
	if !ok {
		t.Fatalf("unexpected payload type: %T", fakeOutbox.payload)
	}
	if payload.SessionID != "cs_123" || payload.PaymentIntentID != "pi_123" {
		t.Fatalf("unexpected payload identifiers: %+v", payload)
	}
	if payload.ReceivedAmount != 42.5 {
		t.Fatalf("unexpected received amount: %f", payload.ReceivedAmount)
	}
	if payload.Confirmations != 12 {
		t.Fatalf("unexpected confirmations: %d", payload.Confirmations)
	}
	if payload.ToAddress != "0xdeposit" {
		t.Fatalf("unexpected to address: %s", payload.ToAddress)
	}
	if payload.TxHash == "" {
		t.Fatalf("tx hash should be generated")
	}

	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["status"] != "queued" {
		t.Fatalf("unexpected status: %+v", body)
	}
}
