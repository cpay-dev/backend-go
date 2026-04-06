package rpcx

import (
	"context"
	"strings"

	"google.golang.org/grpc/metadata"
)

const (
	RequestIDKey   = "x-request-id"
	IdempotencyKey = "idempotency-key"
)

func WithOutboundMetadata(ctx context.Context, requestID, idempotency string) context.Context {
	pairs := []string{}
	if strings.TrimSpace(requestID) != "" {
		pairs = append(pairs, RequestIDKey, strings.TrimSpace(requestID))
	}
	if strings.TrimSpace(idempotency) != "" {
		pairs = append(pairs, IdempotencyKey, strings.TrimSpace(idempotency))
	}
	if len(pairs) == 0 {
		return ctx
	}
	return metadata.NewOutgoingContext(ctx, metadata.Pairs(pairs...))
}

func RequestIDFromContext(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	values := md.Get(RequestIDKey)
	if len(values) == 0 {
		return ""
	}
	return strings.TrimSpace(values[0])
}
