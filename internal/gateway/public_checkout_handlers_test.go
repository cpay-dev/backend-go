package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	cpayv1 "github.com/cpay-dev/cpay/internal/gen/cpay/v1"
	"github.com/go-chi/chi/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

type checkoutHandlerTestServer struct {
	cpayv1.UnimplementedCheckoutServiceServer
	createPublicFn func(context.Context, *cpayv1.CreatePublicCheckoutSessionRequest) (*cpayv1.CreateCheckoutSessionResponse, error)
	getPublicFn    func(context.Context, *cpayv1.GetPublicCheckoutSessionRequest) (*cpayv1.PublicCheckoutSession, error)
}

func (s *checkoutHandlerTestServer) CreatePublicCheckoutSession(ctx context.Context, req *cpayv1.CreatePublicCheckoutSessionRequest) (*cpayv1.CreateCheckoutSessionResponse, error) {
	return s.createPublicFn(ctx, req)
}

func (s *checkoutHandlerTestServer) GetPublicCheckoutSession(ctx context.Context, req *cpayv1.GetPublicCheckoutSessionRequest) (*cpayv1.PublicCheckoutSession, error) {
	return s.getPublicFn(ctx, req)
}

func newCheckoutTestClient(t *testing.T, srv cpayv1.CheckoutServiceServer) cpayv1.CheckoutServiceClient {
	t.Helper()

	listener := bufconn.Listen(1024 * 1024)
	grpcServer := grpc.NewServer()
	cpayv1.RegisterCheckoutServiceServer(grpcServer, srv)
	go func() {
		_ = grpcServer.Serve(listener)
	}()
	t.Cleanup(func() {
		grpcServer.Stop()
		_ = listener.Close()
	})

	conn, err := grpc.DialContext(
		context.Background(),
		"bufnet",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
	)
	if err != nil {
		t.Fatalf("dial bufconn: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	return cpayv1.NewCheckoutServiceClient(conn)
}

func TestCreatePublicCheckoutSessionHandlerMapsRequest(t *testing.T) {
	var got *cpayv1.CreatePublicCheckoutSessionRequest
	client := newCheckoutTestClient(t, &checkoutHandlerTestServer{
		createPublicFn: func(_ context.Context, req *cpayv1.CreatePublicCheckoutSessionRequest) (*cpayv1.CreateCheckoutSessionResponse, error) {
			got = req
			return &cpayv1.CreateCheckoutSessionResponse{
				Id:                      "cs_test_123",
				PaymentIntentId:         "pi_test_123",
				PaymentLinkCode:         "plink_123",
				Status:                  "awaiting_funds",
				Amount:                  10.5,
				Currency:                "USD",
				Chain:                   "polygon",
				TokenSymbol:             "USDC",
				DepositAddress:          "0xabc",
				TolerancePercent:        0.25,
				MinAcceptableAmount:     10.47375,
				MaxAcceptableAmount:     10.52625,
				ExpiresAt:               "2026-04-05T12:00:00Z",
				AfterPaymentType:        "confirmation_page",
				AfterPaymentRedirectUrl: "",
				ClientSecret:            "secret_test",
			}, nil
		},
		getPublicFn: func(context.Context, *cpayv1.GetPublicCheckoutSessionRequest) (*cpayv1.PublicCheckoutSession, error) {
			t.Fatal("GetPublicCheckoutSession should not be called")
			return nil, nil
		},
	})
	s := &Server{checkoutClient: client}

	body := map[string]any{
		"amount":         10.5,
		"chain":          "polygon",
		"token_symbol":   "USDC",
		"token_address":  "0x1111111111111111111111111111111111111111",
		"customer_email": "buyer@example.com",
		"metadata": map[string]any{
			"order_id": "ord_123",
		},
	}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/public/payment_links/plink_123/sessions", bytes.NewReader(raw))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "plink_123")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	s.handleCreatePublicCheckoutSession(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rr.Code)
	}
	if got == nil {
		t.Fatalf("expected grpc request capture")
	}
	if got.GetLinkIdentifier() != "plink_123" {
		t.Fatalf("expected link identifier plink_123, got %q", got.GetLinkIdentifier())
	}
	if got.GetTokenSymbol() != "USDC" {
		t.Fatalf("expected token_symbol USDC, got %q", got.GetTokenSymbol())
	}
	if got.GetMetadataJson() != "{\"order_id\":\"ord_123\"}" {
		t.Fatalf("expected metadata json, got %q", got.GetMetadataJson())
	}

	var payload map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response json: %v", err)
	}
	if payload["id"] != "cs_test_123" {
		t.Fatalf("expected id cs_test_123, got %#v", payload["id"])
	}
	if payload["client_secret"] != "secret_test" {
		t.Fatalf("expected client_secret secret_test, got %#v", payload["client_secret"])
	}
}

func TestGetPublicCheckoutSessionUsesSessionIDOnly(t *testing.T) {
	var got *cpayv1.GetPublicCheckoutSessionRequest
	client := newCheckoutTestClient(t, &checkoutHandlerTestServer{
		createPublicFn: func(context.Context, *cpayv1.CreatePublicCheckoutSessionRequest) (*cpayv1.CreateCheckoutSessionResponse, error) {
			t.Fatal("CreatePublicCheckoutSession should not be called")
			return nil, nil
		},
		getPublicFn: func(_ context.Context, req *cpayv1.GetPublicCheckoutSessionRequest) (*cpayv1.PublicCheckoutSession, error) {
			got = req
			return &cpayv1.PublicCheckoutSession{
				Id:                    "cs_test_456",
				Status:                "awaiting_funds",
				Amount:                20,
				Currency:              "USD",
				Chain:                 "polygon",
				TokenSymbol:           "USDC",
				ExpiresAt:             "2026-04-05T12:00:00Z",
				PaymentIntentId:       "pi_test_456",
				PaymentIntentStatus:   "awaiting_funds",
				ReceivedAmount:        19.95,
				Confirmations:         121,
				RequiredConfirmations: 20,
				Transactions: []*cpayv1.CheckoutTransaction{
					{
						TxHash:         "0xdef456",
						Amount:         7.5,
						Chain:          "polygon",
						TokenSymbol:    "USDC",
						Confirmations:  40,
						Status:         "confirmed",
						BlockNumber:    123456,
						HasBlockNumber: true,
					},
					{
						TxHash:        "0xabc123",
						Amount:        12.45,
						Chain:         "polygon",
						TokenSymbol:   "USDC",
						Confirmations: 121,
						Status:        "confirmed",
					},
				},
				DepositAddress:          "0xdef",
				LinkTitle:               "Support Us",
				CtaText:                 "Pay",
				AfterPaymentType:        "confirmation_page",
				AfterPaymentRedirectUrl: "",
			}, nil
		},
	})
	s := &Server{checkoutClient: client}

	req := httptest.NewRequest(http.MethodGet, "/v1/public/checkout/cs_test_456", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("session_id", "cs_test_456")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	s.handleGetPublicCheckoutSession(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if got == nil {
		t.Fatalf("expected grpc request capture")
	}
	if got.GetSessionId() != "cs_test_456" {
		t.Fatalf("expected session_id cs_test_456, got %q", got.GetSessionId())
	}

	var payload map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response json: %v", err)
	}
	if payload["id"] != "cs_test_456" {
		t.Fatalf("expected id cs_test_456, got %#v", payload["id"])
	}
	if payload["payment_intent_status"] != "awaiting_funds" {
		t.Fatalf("expected payment intent status awaiting_funds, got %#v", payload["payment_intent_status"])
	}
	if _, ok := payload["tx_hash"]; ok {
		t.Fatalf("public checkout session should not expose legacy tx_hash: %#v", payload["tx_hash"])
	}
	if payload["confirmations"] != float64(121) {
		t.Fatalf("expected confirmations 121, got %#v", payload["confirmations"])
	}
	if payload["required_confirmations"] != float64(20) {
		t.Fatalf("expected required confirmations 20, got %#v", payload["required_confirmations"])
	}
	transactions, ok := payload["transactions"].([]any)
	if !ok || len(transactions) != 2 {
		t.Fatalf("expected two transactions, got %#v", payload["transactions"])
	}
	first, ok := transactions[0].(map[string]any)
	if !ok {
		t.Fatalf("expected transaction object, got %#v", transactions[0])
	}
	if first["tx_hash"] != "0xdef456" || first["amount"] != float64(7.5) || first["block_number"] != float64(123456) {
		t.Fatalf("unexpected first transaction: %#v", first)
	}
}
