package merchant

import (
	"context"
	"slices"

	"github.com/cpay-dev/backend-go/internal/api/authn"
	"github.com/cpay-dev/backend-go/pkg/grpc/middleware"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type MerchantContextKey struct{}

type MerchantInfo struct {
	ID string
}

func newMerchantMiddleware(authnService *authn.AuthnService, bypass []string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if slices.Contains(bypass, info.FullMethod) {
			return handler(ctx, req)
		}
		apiKey := middleware.MustGetApiKey(ctx)
		merchant, err := authnService.AuthenticateMerchant(ctx, apiKey)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "authenticate merchant: %v", err)
		}
		if merchant == nil {
			return nil, status.Errorf(codes.PermissionDenied, "api key not found")
		}
		ctx = context.WithValue(ctx, MerchantContextKey{}, MerchantInfo{ID: merchant.ID})
		return handler(ctx, req)
	}
}
