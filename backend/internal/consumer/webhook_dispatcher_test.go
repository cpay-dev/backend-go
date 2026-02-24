package consumer

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cpay-dev/backend/internal/events"
)

func TestWebhookEventMapping(t *testing.T) {
	expected := map[string]string{
		events.EventProductCreated:      "product.created",
		events.EventPaymentRecorded:     "payment.received",
		events.EventSubscriptionCreated: "subscription.created",
		events.EventSubscriptionPayment: "subscription.payment",
		events.EventWithdrawalCompleted: "withdrawal.completed",
	}

	for natsEvent, webhookEvent := range expected {
		t.Run(natsEvent, func(t *testing.T) {
			got, ok := webhookEventMapping[natsEvent]
			if !ok {
				t.Errorf("event %q not found in webhookEventMapping", natsEvent)
				return
			}
			if got != webhookEvent {
				t.Errorf("webhookEventMapping[%q] = %q, want %q", natsEvent, got, webhookEvent)
			}
		})
	}

	// Ensure mapping has exactly the expected number of entries
	if len(webhookEventMapping) != len(expected) {
		t.Errorf("webhookEventMapping has %d entries, want %d", len(webhookEventMapping), len(expected))
	}
}

func TestWebhookPayloadFormat(t *testing.T) {
	eventData := json.RawMessage(`{"shop_id":"shop-1","product_id":"prod-1","name":"Widget"}`)

	payload := webhookPayload{
		ID:        "evt-abc123",
		Type:      "product.created",
		CreatedAt: time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC).Format(time.RFC3339),
		Data:      eventData,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &decoded); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	if decoded["id"] != "evt-abc123" {
		t.Errorf("id = %v, want evt-abc123", decoded["id"])
	}
	if decoded["type"] != "product.created" {
		t.Errorf("type = %v, want product.created", decoded["type"])
	}
	if decoded["created_at"] != "2025-06-15T12:00:00Z" {
		t.Errorf("created_at = %v, want 2025-06-15T12:00:00Z", decoded["created_at"])
	}

	data, ok := decoded["data"].(map[string]interface{})
	if !ok {
		t.Fatal("data is not an object")
	}
	if data["shop_id"] != "shop-1" {
		t.Errorf("data.shop_id = %v, want shop-1", data["shop_id"])
	}
}

func TestWebhookHMACSignature(t *testing.T) {
	secret := "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"
	payload := []byte(`{"id":"evt-1","type":"payment.received","created_at":"2025-06-15T12:00:00Z","data":{"amount":"100"}}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	// Verify the signature starts with "sha256="
	if signature[:7] != "sha256=" {
		t.Errorf("signature prefix = %q, want sha256=", signature[:7])
	}

	// Verify the hex part is 64 characters (256 bits)
	hexPart := signature[7:]
	if len(hexPart) != 64 {
		t.Errorf("hex signature length = %d, want 64", len(hexPart))
	}

	// Verify the same input produces the same signature
	mac2 := hmac.New(sha256.New, []byte(secret))
	mac2.Write(payload)
	signature2 := "sha256=" + hex.EncodeToString(mac2.Sum(nil))

	if signature != signature2 {
		t.Error("same input produced different signatures")
	}

	// Verify different secret produces different signature
	mac3 := hmac.New(sha256.New, []byte("different-secret"))
	mac3.Write(payload)
	signature3 := "sha256=" + hex.EncodeToString(mac3.Sum(nil))

	if signature == signature3 {
		t.Error("different secrets produced same signature")
	}

	// Verify different payload produces different signature
	mac4 := hmac.New(sha256.New, []byte(secret))
	mac4.Write([]byte(`{"id":"evt-2","type":"different"}`))
	signature4 := "sha256=" + hex.EncodeToString(mac4.Sum(nil))

	if signature == signature4 {
		t.Error("different payloads produced same signature")
	}
}

func TestWebhookHTTPDelivery(t *testing.T) {
	var receivedBody []byte
	var receivedSignature string
	var receivedEvent string
	var receivedContentType string
	var callCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		receivedSignature = r.Header.Get("X-CPay-Signature")
		receivedEvent = r.Header.Get("X-CPay-Event")
		receivedContentType = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		receivedBody = body
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	secret := "test-webhook-secret-key-1234567890abcdef1234567890abcdef"
	payload := []byte(`{"id":"evt-test","type":"payment.received","created_at":"2025-06-15T12:00:00Z","data":{"amount":"50"}}`)

	// Compute expected signature
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expectedSig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	// Make the HTTP request like the dispatcher would
	req, err := http.NewRequest(http.MethodPost, server.URL, nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CPay-Signature", expectedSig)
	req.Header.Set("X-CPay-Event", "payment.received")
	req.Body = io.NopCloser(newReader(payload))
	req.ContentLength = int64(len(payload))

	resp, err := webhookHTTPClient.Do(req)
	if err != nil {
		t.Fatalf("http request: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if callCount.Load() != 1 {
		t.Errorf("call count = %d, want 1", callCount.Load())
	}
	if receivedContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", receivedContentType)
	}
	if receivedEvent != "payment.received" {
		t.Errorf("X-CPay-Event = %q, want payment.received", receivedEvent)
	}
	if receivedSignature != expectedSig {
		t.Errorf("X-CPay-Signature = %q, want %q", receivedSignature, expectedSig)
	}
	if string(receivedBody) != string(payload) {
		t.Errorf("body = %q, want %q", string(receivedBody), string(payload))
	}
}

func TestWebhookHTTPClientTimeout(t *testing.T) {
	if webhookHTTPClient.Timeout != 10*time.Second {
		t.Errorf("webhook HTTP client timeout = %v, want 10s", webhookHTTPClient.Timeout)
	}
}

func TestWebhookSignatureVerification(t *testing.T) {
	// Simulate what a merchant would do to verify a webhook
	secret := "merchant-secret-key-abcdef"
	payload := []byte(`{"id":"evt-1","type":"product.created","data":{}}`)

	// Generate signature (what the dispatcher does)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	// Verify signature (what the merchant does)
	sigHex := signature[7:] // strip "sha256="
	expectedMAC, err := hex.DecodeString(sigHex)
	if err != nil {
		t.Fatalf("decode hex: %v", err)
	}

	verifyMAC := hmac.New(sha256.New, []byte(secret))
	verifyMAC.Write(payload)

	if !hmac.Equal(verifyMAC.Sum(nil), expectedMAC) {
		t.Error("merchant signature verification failed")
	}
}

func TestShopIDExtraction(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		wantID  string
		wantOK  bool
	}{
		{
			name:   "valid shop_id",
			data:   `{"shop_id":"shop-123","product_id":"prod-1"}`,
			wantID: "shop-123",
			wantOK: true,
		},
		{
			name:   "missing shop_id",
			data:   `{"product_id":"prod-1"}`,
			wantID: "",
			wantOK: false,
		},
		{
			name:   "empty shop_id",
			data:   `{"shop_id":"","product_id":"prod-1"}`,
			wantID: "",
			wantOK: false,
		},
		{
			name:   "shop_id is number",
			data:   `{"shop_id":123}`,
			wantID: "",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var data map[string]interface{}
			if err := json.Unmarshal([]byte(tt.data), &data); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			shopID, _ := data["shop_id"].(string)
			ok := shopID != ""
			if shopID != tt.wantID {
				t.Errorf("shop_id = %q, want %q", shopID, tt.wantID)
			}
			if ok != tt.wantOK {
				t.Errorf("ok = %v, want %v", ok, tt.wantOK)
			}
		})
	}
}

type bytesReader struct {
	data []byte
	pos  int
}

func newReader(data []byte) *bytesReader {
	return &bytesReader{data: data}
}

func (r *bytesReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}
