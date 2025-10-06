package cleaner

import (
	"context"
	"time"

	"github.com/cpay-dev/backend-go/internal/indexer/repo/pg"
)

type Service struct {
	repo *pg.PostgresRepo
}

func NewService(repo *pg.PostgresRepo) *Service {
	return &Service{repo: repo}
}

func (s *Service) CleanupUnichain(ctx context.Context) (int, error) {
	return s.repo.DeleteTransfersByChain(ctx, pg.ChainUnichain, time.Minute*15)
}
