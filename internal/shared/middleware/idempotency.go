package middleware

import (
	"context"
	"net/http"
	"strings"
)

const IdempotencyKeyHeader = "Idempotency-Key"

type idemKeyType string

const idemCtxKey idemKeyType = "idempotency_key"

func Idempotency(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost && r.Method != http.MethodPut && r.Method != http.MethodPatch {
			next.ServeHTTP(w, r)
			return
		}
		key := strings.TrimSpace(r.Header.Get(IdempotencyKeyHeader))
		if len(key) > 128 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"code":"invalid_idempotency_key","message":"Idempotency-Key is too long"}}`))
			return
		}
		ctx := context.WithValue(r.Context(), idemCtxKey, key)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func GetIdempotencyKey(ctx context.Context) string {
	v := ctx.Value(idemCtxKey)
	s, _ := v.(string)
	return strings.TrimSpace(s)
}
