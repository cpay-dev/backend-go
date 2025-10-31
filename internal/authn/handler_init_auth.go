package authn

import (
	"context"
	"fmt"

	authnpb "github.com/cpay-dev/proto-go/api/v1/authn"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) InitAuth(ctx context.Context, req *authnpb.InitAuthRequest) (*authnpb.InitAuthResponse, error) {
	method := req.GetProvider()
	if method == nil {
		return nil, status.Error(codes.Unimplemented, "only provider method supported")
	}
	res, err := s.auth.InitProviderAuth(ctx, method.Provider)
	if err != nil {
		switch err {
		case ErrProviderUnsupported:
			return nil, status.Error(codes.Unimplemented, "provider not supported")
		default:
			return nil, fmt.Errorf("init provider auth: %w", err)
		}
	}
	return &authnpb.InitAuthResponse{Continuation: &authnpb.InitAuthResponse_Provider{
		Provider: &authnpb.ProviderContinuation{State: res.State, RedirectUrl: res.RedirectURL},
	}}, nil
}
