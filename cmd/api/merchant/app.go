package main

import (
	"fmt"

	apiasset "github.com/cpay-dev/backend-go/internal/api/asset"
	"github.com/cpay-dev/backend-go/internal/api/authn"
	"github.com/cpay-dev/backend-go/internal/api/grpc/merchant"
	"github.com/cpay-dev/backend-go/internal/api/grpc/merchant/asset"
	"github.com/cpay-dev/backend-go/internal/api/grpc/merchant/chain"
	"github.com/cpay-dev/backend-go/internal/api/grpc/merchant/payment"
	"github.com/cpay-dev/backend-go/internal/api/repo/pg/app"
	"github.com/cpay-dev/backend-go/internal/api/repo/pg/blockchain"
	pgpayment "github.com/cpay-dev/backend-go/internal/api/repo/pg/payment"
	pgwallet "github.com/cpay-dev/backend-go/internal/api/repo/pg/wallet"
	apiwallet "github.com/cpay-dev/backend-go/internal/api/wallet"
	"github.com/cpay-dev/backend-go/pkg/db"
	pbwallet "github.com/cpay-dev/proto-go/api/v1/wallet"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
)

type application struct {
	config            Config
	logger            zerolog.Logger
	dbPool            *pgxpool.Pool
	walletServiceConn *grpc.ClientConn
	server            *merchant.Server
}

func (a *application) start() error {
	a.logger.Info().Str("addr", a.config.ListenAddress).Msg("starting merchant grpc server...")
	dbPool := db.NewPgxPoolWrapper(a.dbPool)
	appRepo := app.NewPostgresRepo(dbPool)
	blockchainRepo := blockchain.NewPostgresRepo(dbPool)
	paymentRepo := pgpayment.NewPostgresRepo(dbPool)
	walletRepo := pgwallet.NewPostgresRepo(dbPool)
	priceService := apiasset.NewPriceService(blockchainRepo)
	walletClient := pbwallet.NewWalletServiceClient(a.walletServiceConn)
	walletService := apiwallet.NewService(walletRepo, walletClient)
	a.server = merchant.NewServer(
		a.logger,
		authn.NewService(appRepo),
		asset.NewService(blockchainRepo, priceService),
		chain.NewService(blockchainRepo),
		payment.NewService(blockchainRepo, paymentRepo, walletService),
	)
	if err := a.server.Start(a.config.ListenAddress, a.config.HealthCheckIntervalDuration()); err != nil {
		return fmt.Errorf("start merchant grpc server: %w", err)
	}
	return nil
}

func (a *application) stop() {
	a.logger.Info().Msg("stopping application...")
	if a.server != nil {
		a.server.Stop()
	}
	if a.walletServiceConn != nil {
		a.walletServiceConn.Close()
	}
	if a.dbPool != nil {
		a.dbPool.Close()
	}
	a.logger.Info().Msg("application stopped")
}
