package authn

import (
	"context"
	"errors"
	"fmt"

	"github.com/cpay-dev/backend-go/internal/api/repo/pg/app"
)

var ErrMerchantNotFound = errors.New("merchant not found")
var ErrMerchantIsNotActive = errors.New("merchant is not active")

func (s *AuthnService) AuthenticateMerchant(ctx context.Context, key string) (*app.Merchant, error) {
	merchant, err := s.repo.GetMerchantByAPIKey(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("get merchant by api key: %w", err)
	}
	if merchant == nil {
		return nil, ErrMerchantNotFound
	}
	if merchant.Status != app.MerchantStatusActive {
		return nil, ErrMerchantIsNotActive
	}
	return merchant, nil
}
