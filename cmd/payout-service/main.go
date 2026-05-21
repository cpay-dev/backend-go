package main

import (
	"context"
	"os/signal"
	"syscall"
	"time"

	"github.com/cpay-dev/cpay/internal/platform/db"
	"github.com/cpay-dev/cpay/internal/platform/httpserver"
	"github.com/cpay-dev/cpay/internal/platform/migrate"
	"github.com/cpay-dev/cpay/internal/platform/workers"
	"github.com/cpay-dev/cpay/internal/shared/config"
	cryptox "github.com/cpay-dev/cpay/internal/shared/crypto"
	"github.com/cpay-dev/cpay/internal/shared/logx"
	"github.com/rs/zerolog"
)

func main() {
	cfg := config.Load("payout-service")
	log := logx.New(cfg.ServiceName)
	if err := cfg.Validate(); err != nil {
		log.Fatal().Err(err).Msg("invalid config")
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal().Err(err).Msg("db connect failed")
	}
	defer pool.Close()
	if err := migrate.RunUp(ctx, pool); err != nil {
		log.Fatal().Err(err).Msg("migrations failed")
	}

	httpserver.StartHealthServer(ctx, cfg.HTTPAddr, cfg.ServiceName, log)
	executor, cleanup := buildPayoutExecutor(ctx, cfg, log)
	defer cleanup()
	worker := workers.PayoutScheduler{
		DB:         pool,
		Log:        log,
		Interval:   cfg.WorkerInterval,
		Source:     cfg.ServiceName,
		Executor:   executor,
		EncryptKey: cryptox.NormalizeKey(cfg.EncryptionKey),
	}
	worker.Run(ctx)
}

func buildPayoutExecutor(ctx context.Context, cfg config.Config, log zerolog.Logger) (workers.PayoutExecutor, func()) {
	switch cfg.PayoutMode {
	case "mock":
		log.Warn().Msg("payout-service running with mocked payouts")
		return workers.MockPayoutExecutor{}, func() {}
	case "production":
		rpcURLs := cfg.EVMChainRPCURLs()
		if err := config.ValidateChainRPCURLs(rpcURLs); err != nil {
			log.Fatal().Err(err).Msg("invalid production payout config")
		}
		dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if cfg.Environment == "production" {
			if err := config.ValidateProductionCreate2PayoutConfig(rpcURLs, cfg.CheckoutWalletFactories, cfg.PayoutHotWalletKey); err != nil {
				log.Fatal().Err(err).Msg("invalid production checkout wallet config")
			}
		}
		executor, err := workers.NewEVMPayoutExecutor(dialCtx, rpcURLs, workers.EVMPayoutExecutorConfig{
			HotWalletPrivateKey: cfg.PayoutHotWalletKey,
			GasBufferPercent:    cfg.PayoutGasBufferPercent,
		})
		if err != nil {
			log.Fatal().Err(err).Msg("payout executor init failed")
		}
		return executor, executor.Close
	default:
		log.Fatal().Str("mode", cfg.PayoutMode).Msg("invalid payout mode")
		return workers.MockPayoutExecutor{}, func() {}
	}
}
