package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/cpay-dev/backend-go/internal/indexer/cleaner"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

type application struct {
	config  Config
	logger  zerolog.Logger
	dbPool  *pgxpool.Pool
	cleaner *cleaner.Service
}

func (a *application) run(ctx context.Context, interval time.Duration) error {
	a.logger.Info().Msg("starting application...")
	a.logger.Info().Msg("running initial clean...")
	if err := a.clean(ctx); err != nil {
		return fmt.Errorf("initial clean: %w", err)
	}
	t := time.NewTicker(interval)
	for {
		select {
		case <-ctx.Done():
			err := ctx.Err()
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		case <-t.C:
			if err := a.clean(ctx); err != nil {
				return err
			}
		}
	}
}

func (a *application) stop() {
	a.logger.Info().Msg("stopping application...")
	if a.dbPool != nil {
		a.logger.Debug().Msg("closing db")
		a.dbPool.Close()
	}
	a.logger.Info().Msg("application stopped")
}

func (a *application) clean(ctx context.Context) error {
	type cleaner struct {
		name string
		fn   func(context.Context) (int, error)
	}
	cleaners := []cleaner{
		{name: "unichain", fn: a.cleaner.CleanupUnichain},
	}

	var errs []error
	errCh := make(chan error, len(cleaners))
	wg := sync.WaitGroup{}
	wg.Add(len(cleaners))

	for _, cleaner := range cleaners {
		go func() {
			defer wg.Done()
			num, err := cleaner.fn(ctx)
			if err != nil {
				errCh <- err
			}
			a.logger.Err(err).Str("chain", cleaner.name).Int("num", num).Msg("cleaner done")
		}()
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}
