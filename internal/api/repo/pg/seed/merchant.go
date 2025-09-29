package seed

import (
	"context"
	"fmt"

	"github.com/cpay-dev/backend-go/internal/api/repo/pg/app"
)

type MerchantSeeder struct {
	repo *app.PostgresRepo
}

func NewMerchantSeeder(repo *app.PostgresRepo) *MerchantSeeder {
	return &MerchantSeeder{repo: repo}
}

func (s *MerchantSeeder) Seed(ctx context.Context) error {
	return s.repo.RunInTx(ctx, func(ctx context.Context) error {
		return s.seed(ctx)
	})
}

func (s *MerchantSeeder) seed(ctx context.Context) error {
	u := app.User{
		ID:     "01K40YW14CPAYUSER000000001",
		Status: app.UserStatusActive,
	}
	m := app.Merchant{
		ID:     "01K40YW14CPAYMERCHANT00001",
		UserID: "01K40YW14CPAYUSER000000001",
		Name:   "Automation Merchant",
		Status: app.MerchantStatusActive,
	}
	ak := app.MerchantAPIKey{
		ID:         "01K40YW14CPAYAPIKEY0000001",
		UserID:     "01K40YW14CPAYUSER000000001",
		MerchantID: "01K40YW14CPAYMERCHANT00001",
		Name:       "Automation API Key",
		Key:        "automation",
	}
	err := s.repo.RunInTx(ctx, func(ctx context.Context) error {
		if err := s.repo.CreateUser(ctx, u); err != nil {
			return fmt.Errorf("create user: %w", err)
		}
		if err := s.repo.CreateMerchant(ctx, m); err != nil {
			return fmt.Errorf("create merchant: %w", err)
		}
		if err := s.repo.CreateMerchantAPIKey(ctx, ak); err != nil {
			return fmt.Errorf("create merchant api key: %w", err)
		}
		return nil
	})
	return err
}
