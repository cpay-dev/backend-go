package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIdempotencyMiddlewareStoresKey(t *testing.T) {
	var captured string
	h := Idempotency(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		captured = GetIdempotencyKey(r.Context())
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/test", nil)
	req.Header.Set(IdempotencyKeyHeader, "abc-123")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if captured != "abc-123" {
		t.Fatalf("expected key to be captured, got %q", captured)
	}
}

func TestIdempotencyMiddlewareRejectsLongKey(t *testing.T) {
	h := Idempotency(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatalf("handler should not be called for invalid key")
	}))

	long := make([]byte, 129)
	for i := range long {
		long[i] = 'a'
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/test", nil)
	req.Header.Set(IdempotencyKeyHeader, string(long))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}
