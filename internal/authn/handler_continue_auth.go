package authn

import (
	"context"
	"errors"
	"fmt"

	authnpb "github.com/cpay-dev/proto-go/api/v1/authn"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) ContinueAuth(ctx context.Context, req *authnpb.ContinueAuthRequest) (*authnpb.ContinueAuthResponse, error) {
	method := req.GetProviderCallback()
	if method == nil {
		return nil, status.Error(codes.Unimplemented, "only provider callback supported")
	}

	res, err := s.auth.ContinueProviderAuth(ctx, method.GetState(), method.GetCode())
	if err != nil {
		switch {
		case errors.Is(err, ErrStateNotFound):
			return nil, status.Error(codes.InvalidArgument, "invalid state")
		case errors.Is(err, ErrIDTokenInvalid):
			return nil, status.Error(codes.Unauthenticated, "")
		default:
			return nil, fmt.Errorf("continue provider auth: %w", err)
		}
	}

	return &authnpb.ContinueAuthResponse{Data: &authnpb.ContinueAuthResponse_ProviderData{
		ProviderData: &authnpb.ProviderCallbackData{Email: res.Email, EmailVerified: res.EmailVerified},
	}}, nil
}
