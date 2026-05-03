package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestRouterMethodRegistration verifies that every expected HTTP method/path
// combination is registered in the gateway router. An unregistered route returns 405;
// a registered-but-unauthenticated route returns 401. This test catches the
// class of bug where a handler exists but its r.Patch / r.Get / etc. call is
// missing in Router().
func TestRouterMethodRegistration(t *testing.T) {
	s := &Server{}
	handler := s.Router()

	routes := []struct {
		method string
		path   string
	}{
		// payment links
		{http.MethodPost, "/v1/payment_links"},
		{http.MethodGet, "/v1/payment_links"},
		{http.MethodGet, "/v1/payment_links/some-id"},
		{http.MethodPatch, "/v1/payment_links/some-id"},
		{http.MethodPost, "/v1/payment_links/some-id/archive"},
		{http.MethodPost, "/v1/payment_links/some-id/sessions"},
		{http.MethodGet, "/v1/public/payment_links/some-code"},
		{http.MethodPost, "/v1/public/payment_links/some-code/sessions"},
		{http.MethodPost, "/v1/public/checkout/cs_test/confirm"},
		// products
		{http.MethodPost, "/v1/products"},
		{http.MethodGet, "/v1/products"},
		{http.MethodGet, "/v1/products/some-id"},
		{http.MethodGet, "/v1/public/products/some-id"},
		{http.MethodPost, "/v1/products/some-id"},
		{http.MethodPost, "/v1/products/some-id/image"},
		{http.MethodDelete, "/v1/products/some-id"},
		// checkout
		{http.MethodGet, "/v1/checkout/cs_test"},
		{http.MethodPost, "/v1/checkout/cs_test/confirm"},
		{http.MethodPost, "/v1/mock_transfers"},
		{http.MethodGet, "/v1/payments"},
		{http.MethodGet, "/v1/payments/pi_test"},
		// public media
		{http.MethodGet, "/v1/public/product_images/merchant/product/image.png"},
	}

	for _, tc := range routes {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code == http.StatusMethodNotAllowed {
				t.Errorf("route %s %s is not registered (got 405 — add it to Router())", tc.method, tc.path)
			}
		})
	}
}

func TestProductDetailRoutesArePublic(t *testing.T) {
	s := &Server{}
	handler := s.Router()

	for _, path := range []string{"/v1/products/some-id", "/v1/public/products/some-id"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code == http.StatusUnauthorized {
				t.Fatalf("product detail route should be public, got %d", rr.Code)
			}
		})
	}
}
