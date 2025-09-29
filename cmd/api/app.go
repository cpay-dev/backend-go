package main

import (
	"context"
	"fmt"
	"time"

	"github.com/cpay-dev/backend-go/internal/api/authn"
	"github.com/cpay-dev/backend-go/internal/api/grpc/merchant"
	"github.com/cpay-dev/backend-go/internal/api/repo/pg/app"
	"github.com/cpay-dev/backend-go/internal/api/repo/pg/blockchain"
	"github.com/cpay-dev/backend-go/pkg/db"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
)

type application struct {
	config         Config
	logger         zerolog.Logger
	dbPool         *pgxpool.Pool
	merchantServer *merchant.Server
	healthConn     *grpc.ClientConn
	healthClient   grpc_health_v1.HealthClient
}

func (a *application) startMerchantGrpc() error {
	a.logger.Info().Str("addr", a.config.ListenAddress).Msg("starting merchant grpc server...")
	dbPool := db.NewPgxPoolWrapper(a.dbPool)
	appRepo := app.NewPostgresRepo(dbPool)
	blockchainRepo := blockchain.NewPostgresRepo(dbPool)
	a.merchantServer = merchant.NewServer(
		a.logger,
		blockchainRepo,
		authn.NewService(appRepo),
		time.Second*time.Duration(a.config.HealthCheckInterval),
	)
	if err := a.merchantServer.Start(a.config.ListenAddress); err != nil {
		return fmt.Errorf("start merchant grpc server: %w", err)
	}
	return nil
}

func (a *application) checkMerchantGrpc(address string) error {
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("create health grpc connection: %w", err)
	}
	a.healthConn = conn
	a.healthClient = grpc_health_v1.NewHealthClient(conn)
	conn.Connect()
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	resp, err := a.healthClient.Check(ctx, &grpc_health_v1.HealthCheckRequest{Service: "merchant"})
	if err != nil {
		return fmt.Errorf("check merchant grpc server: %w", err)
	}

	if resp.Status != grpc_health_v1.HealthCheckResponse_SERVING {
		return fmt.Errorf("merchant grpc server is not serving")
	}

	return nil
}

func (a *application) stop() {
	a.logger.Info().Msg("stopping application...")
	if a.merchantServer != nil {
		a.merchantServer.Stop()
	}
	if a.healthConn != nil {
		a.healthConn.Close()
	}
	if a.dbPool != nil {
		a.dbPool.Close()
	}
	a.logger.Info().Msg("application stopped")
}
