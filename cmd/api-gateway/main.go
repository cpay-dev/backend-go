package main

import (
	"context"
	"errors"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/cpay-dev/cpay/internal/gateway"
	cpayv1 "github.com/cpay-dev/cpay/internal/gen/cpay/v1"
	"github.com/cpay-dev/cpay/internal/platform/db"
	"github.com/cpay-dev/cpay/internal/platform/migrate"
	"github.com/cpay-dev/cpay/internal/shared/config"
	"github.com/cpay-dev/cpay/internal/shared/logx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	cfg := config.Load("api-gateway")
	log := logx.New(cfg.ServiceName)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	dbPool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal().Err(err).Msg("db connect failed")
	}
	defer dbPool.Close()

	if err := migrate.RunUp(ctx, dbPool); err != nil {
		log.Fatal().Err(err).Msg("migrations failed")
	}

	authConn, err := dialGRPC(ctx, cfg.AuthGRPCAddr)
	if err != nil {
		log.Fatal().Err(err).Str("addr", cfg.AuthGRPCAddr).Msg("dial auth-service failed")
	}
	defer authConn.Close()

	linkConn, err := dialGRPC(ctx, cfg.PaymentLinkGRPCAddr)
	if err != nil {
		log.Fatal().Err(err).Str("addr", cfg.PaymentLinkGRPCAddr).Msg("dial payment-link-service failed")
	}
	defer linkConn.Close()

	checkoutConn, err := dialGRPC(ctx, cfg.CheckoutGRPCAddr)
	if err != nil {
		log.Fatal().Err(err).Str("addr", cfg.CheckoutGRPCAddr).Msg("dial checkout-service failed")
	}
	defer checkoutConn.Close()

	app := gateway.NewServer(
		cfg,
		log,
		dbPool,
		cpayv1.NewAuthServiceClient(authConn),
		cpayv1.NewPaymentLinkServiceClient(linkConn),
		cpayv1.NewCheckoutServiceClient(checkoutConn),
	)

	httpSrv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           app.Router(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutdownCtx)
	}()

	log.Info().Str("addr", cfg.HTTPAddr).Msg("api-gateway started")
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal().Err(err).Msg("api-gateway failed")
	}
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
