package middleware

import (
	"context"
	"slices"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const ApiKeyHeaderKey = "x-api-key"

type ApiKeyContextKey struct{}

var ErrApiKeyNotFound = status.Error(codes.Unauthenticated, "no api key provided")

func NewApiKey(bypass []string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if slices.Contains(bypass, info.FullMethod) {
			return handler(ctx, req)
		}
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "no authentication metadata")
		}
		apiKeys := md[ApiKeyHeaderKey]
		if len(apiKeys) == 0 {
			return nil, ErrApiKeyNotFound
		}
		if len(apiKeys) != 1 {
			return nil, status.Error(codes.Unauthenticated, "multiple api keys provided")
		}
		apiKey := apiKeys[0]
		ctx = context.WithValue(ctx, ApiKeyContextKey{}, apiKey)
		return handler(ctx, req)
	}
}

func MustGetApiKey(ctx context.Context) string {
	apiKey, ok := ctx.Value(ApiKeyContextKey{}).(string)
	if !ok {
		panic(ErrApiKeyNotFound)
	}
	return apiKey
}

func GetApiKey(ctx context.Context) (string, bool) {
	apiKey, ok := ctx.Value(ApiKeyContextKey{}).(string)
	if !ok {
		return "", false
	}
	return apiKey, true
}
