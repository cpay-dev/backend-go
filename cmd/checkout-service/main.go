package main

import (
	"context"
	"net"
	"os/signal"
	"syscall"

	cpayv1 "github.com/cpay-dev/cpay/internal/gen/cpay/v1"
	"github.com/cpay-dev/cpay/internal/platform/db"
	"github.com/cpay-dev/cpay/internal/platform/httpserver"
	"github.com/cpay-dev/cpay/internal/platform/migrate"
	"github.com/cpay-dev/cpay/internal/platform/storage"
	"github.com/cpay-dev/cpay/internal/services/checkoutsvc"
	"github.com/cpay-dev/cpay/internal/shared/chain"
	"github.com/cpay-dev/cpay/internal/shared/config"
	cryptox "github.com/cpay-dev/cpay/internal/shared/crypto"
	"github.com/cpay-dev/cpay/internal/shared/logx"
	"google.golang.org/grpc"
)

func main() {
	cfg := config.Load("checkout-service")
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

	var minioClient *storage.MinIO
	if m, err := storage.NewMinIO(ctx, cfg); err != nil {
		log.Warn().Err(err).Msg("minio init failed; invoice upload disabled")
	} else {
		minioClient = m
	}

	svc := checkoutsvc.New(cfg, log, pool, chain.NewEVMAdapter(cfg.ChainConfirmations), cryptox.NormalizeKey(cfg.EncryptionKey), minioClient)
	lis, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		log.Fatal().Err(err).Str("addr", cfg.GRPCAddr).Msg("grpc listen failed")
	}
	grpcServer := grpc.NewServer()
	cpayv1.RegisterCheckoutServiceServer(grpcServer, svc)

	httpserver.StartHealthServer(ctx, cfg.HTTPAddr, cfg.ServiceName, log)

	go func() {
		<-ctx.Done()
		grpcServer.GracefulStop()
	}()

	log.Info().Str("grpc_addr", cfg.GRPCAddr).Str("http_addr", cfg.HTTPAddr).Msg("checkout-service started")
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatal().Err(err).Msg("checkout-service stopped")
	}
}
