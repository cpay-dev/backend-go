package authn_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/imroc/req/v3"
	"github.com/stretchr/testify/require"

	"github.com/cpay-dev/backend-go/internal/authn"
)

func TestJWKSService_BootstrapSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"keys":[]}`))
	}))
	t.Cleanup(srv.Close)

	client := req.C()
	svc := authn.NewKeyfuncJWKSService([]string{srv.URL}, client)
	err := svc.Bootstrap(5 * time.Second)
	require.NoError(t, err, "bootstrap should succeed with empty JWKS")
	require.NotNil(t, svc.Keyfunc(), "keyfunc should be set after bootstrap")
}

func TestJWKSService_BootstrapHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	client := req.C()
	svc := authn.NewKeyfuncJWKSService([]string{srv.URL}, client)
	err := svc.Bootstrap(5 * time.Second)
	require.Error(t, err, "bootstrap should error on 500 response")
}
