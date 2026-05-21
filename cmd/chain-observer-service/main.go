package main

import (
	"context"
	"os/signal"
	"syscall"
	"time"

	cpayv1 "github.com/cpay-dev/cpay/internal/gen/cpay/v1"
	"github.com/cpay-dev/cpay/internal/platform/bus"
	"github.com/cpay-dev/cpay/internal/platform/db"
	"github.com/cpay-dev/cpay/internal/platform/httpserver"
	"github.com/cpay-dev/cpay/internal/platform/migrate"
	"github.com/cpay-dev/cpay/internal/platform/workers"
	"github.com/cpay-dev/cpay/internal/shared/config"
	"github.com/cpay-dev/cpay/internal/shared/logx"
	"github.com/nats-io/nats.go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	cfg := config.Load("chain-observer-service")
	log := logx.New(cfg.ServiceName)
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

	var nc *nats.Conn
	if c, err := bus.Connect(cfg.NATSURL); err != nil {
		log.Warn().Err(err).Msg("nats unavailable")
	} else {
		nc = c
		defer nc.Close()
	}

	var checkoutClient cpayv1.CheckoutServiceClient
	checkoutConn, err := dialGRPC(ctx, cfg.CheckoutGRPCAddr)
	if err != nil {
		log.Warn().Err(err).Str("addr", cfg.CheckoutGRPCAddr).Msg("checkout grpc unavailable")
	} else {
		defer checkoutConn.Close()
		checkoutClient = cpayv1.NewCheckoutServiceClient(checkoutConn)
	}

	httpserver.StartHealthServer(ctx, cfg.HTTPAddr, cfg.ServiceName, log)
	observer := workers.ChainObserver{
		DB:             pool,
		CheckoutClient: checkoutClient,
		ChainRPCURLs:   cfg.EVMChainRPCURLs(),
		Log:            log,
		Interval:       cfg.WorkerInterval,
		Source:         cfg.ServiceName,
	}
	indexer := workers.MockTransferIndexer{NATS: nc, CheckoutClient: checkoutClient, Log: log}

	go observer.Run(ctx)
	go indexer.Run(ctx)

	<-ctx.Done()
}

func dialGRPC(ctx context.Context, addr string) (*grpc.ClientConn, error) {
	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return grpc.DialContext(
		dialCtx,
		addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
}
