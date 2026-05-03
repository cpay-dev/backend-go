package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cpay-dev/cpay/internal/shared/ids"
	"github.com/go-chi/chi/v5"
)

type fakeMediaStore struct {
	payload     []byte
	contentType string
	objectKey   string
}

func (f *fakeMediaStore) PutObjectBytes(_ context.Context, objectKey, contentType string, payload []byte) error {
	f.objectKey = objectKey
	f.contentType = contentType
	f.payload = append([]byte(nil), payload...)
	return nil
}

func (f *fakeMediaStore) GetObjectBytes(_ context.Context, objectKey string) ([]byte, string, error) {
	f.objectKey = objectKey
	return append([]byte(nil), f.payload...), f.contentType, nil
}

func TestHandleUploadProductImageRequiresMediaStore(t *testing.T) {
	merchantID := ids.New()
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/v1/products/prod_123/image", nil)
	req = req.WithContext(context.WithValue(req.Context(), requesterKey, requester{
		MerchantID: merchantID,
		IsUser:     true,
	}))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "prod_123")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	s.handleUploadProductImage(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rr.Code)
	}
}

func TestHandleGetProductImageServesStoredBytes(t *testing.T) {
	store := &fakeMediaStore{
		payload:     []byte("image-bytes"),
		contentType: "image/png",
	}
	s := &Server{mediaStore: store}
	req := httptest.NewRequest(http.MethodGet, "/v1/public/product_images/merchant/product/image.png", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("merchant_id", "merchant")
	rctx.URLParams.Add("product_id", "product")
	rctx.URLParams.Add("image_id", "image.png")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	s.handleGetProductImage(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if rr.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("unexpected content type: %q", rr.Header().Get("Content-Type"))
	}
	if rr.Body.String() != "image-bytes" {
		t.Fatalf("unexpected body: %q", rr.Body.String())
	}
	if store.objectKey != "product-images/merchant/product/image.png" {
		t.Fatalf("unexpected object key: %q", store.objectKey)
	}
}

func TestProductImageContentType(t *testing.T) {
	png := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 0}
	contentType, ext, ok := productImageContentType(png, "")
	if !ok {
		t.Fatalf("expected png to be accepted")
	}
	if contentType != "image/png" || ext != ".png" {
		t.Fatalf("unexpected png mapping: %s %s", contentType, ext)
	}

	_, _, ok = productImageContentType([]byte("not an image"), "text/plain")
	if ok {
		t.Fatalf("expected text/plain to be rejected")
	}

	_, _, ok = productImageContentType([]byte("not an image"), "image/png")
	if ok {
		t.Fatalf("expected spoofed image/png to be rejected")
	}
}
