package main

import (
	"context"

	"github.com/cpay-dev/backend-go/internal/indexer/transfers"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

type application struct {
	config   Config
	logger   zerolog.Logger
	dbPool   *pgxpool.Pool
	consumer *transfers.Consumer
}

func (a *application) run(ctx context.Context) error {
	a.logger.Info().Msg("starting application...")
	return a.consumer.Run(ctx)
}

func (a *application) stop() {
	a.logger.Info().Msg("stopping application...")
	if a.consumer != nil {
		a.logger.Debug().Msg("closing consumer")
		a.consumer.Stop()
	}
	if a.dbPool != nil {
		a.logger.Debug().Msg("closing db")
		a.dbPool.Close()
	}
	a.logger.Info().Msg("application stopped")
}
