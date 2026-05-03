package middleware

import (
	"context"
	"net/http"

	"github.com/cpay-dev/cpay/internal/shared/ids"
)

type ctxKey string

const RequestIDKey ctxKey = "request_id"

func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rid := r.Header.Get("X-Request-ID")
		if rid == "" {
			rid = ids.New()
		}
		ctx := context.WithValue(r.Context(), RequestIDKey, rid)
		w.Header().Set("X-Request-ID", rid)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func GetRequestID(ctx context.Context) string {
	v := ctx.Value(RequestIDKey)
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
