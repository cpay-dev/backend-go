package middleware

import (
	"context"
	"fmt"
	"slices"

	"github.com/cpay-dev/backend-go/internal/api/authn"
	"github.com/cpay-dev/backend-go/pkg/grpc/middleware"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type MerchantContextKey struct{}

type MerchantInfo struct {
	ID     string
	APIKey string
}

func NewMerchant(authnService *authn.AuthnService, bypass []string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if slices.Contains(bypass, info.FullMethod) {
			return handler(ctx, req)
		}
		apiKey := middleware.MustGetApiKey(ctx)
		merchant, err := authnService.AuthenticateMerchant(ctx, apiKey)
		if err != nil {
			return nil, fmt.Errorf("authenticate merchant: %w", err)
		}
		if merchant == nil {
			return nil, status.Error(codes.PermissionDenied, "api key not found")
		}
		ctx = context.WithValue(ctx, MerchantContextKey{}, MerchantInfo{ID: merchant.ID, APIKey: apiKey})
		return handler(ctx, req)
	}
}

func MustGetMerchant(ctx context.Context) MerchantInfo {
	merchant, ok := ctx.Value(MerchantContextKey{}).(MerchantInfo)
	if !ok {
		panic(status.Error(codes.PermissionDenied, "merchant not found"))
	}
	return merchant
}

func GetMerchant(ctx context.Context) (MerchantInfo, bool) {
	merchant, ok := ctx.Value(MerchantContextKey{}).(MerchantInfo)
	if !ok {
		return MerchantInfo{}, false
	}
	return merchant, true
}
