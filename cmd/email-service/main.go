package main

import (
	"context"
	"os/signal"
	"syscall"

	"github.com/cpay-dev/cpay/internal/platform/bus"
	"github.com/cpay-dev/cpay/internal/platform/db"
	"github.com/cpay-dev/cpay/internal/platform/httpserver"
	"github.com/cpay-dev/cpay/internal/platform/migrate"
	"github.com/cpay-dev/cpay/internal/platform/storage"
	"github.com/cpay-dev/cpay/internal/platform/workers"
	"github.com/cpay-dev/cpay/internal/shared/config"
	"github.com/cpay-dev/cpay/internal/shared/logx"
	"github.com/nats-io/nats.go"
)

func main() {
	cfg := config.Load("email-service")
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

	var minioClient *storage.MinIO
	if m, err := storage.NewMinIO(ctx, cfg); err != nil {
		log.Warn().Err(err).Msg("minio init failed; receipt attachments disabled")
	} else {
		minioClient = m
	}

	var sender workers.EmailSender
	if s, err := workers.NewResendSender(cfg.ResendAPIKey); err != nil {
		log.Warn().Err(err).Msg("resend sender disabled")
	} else {
		sender = s
	}

	httpserver.StartHealthServer(ctx, cfg.HTTPAddr, cfg.ServiceName, log)
	worker := workers.EmailDispatcher{
		DB:      pool,
		Log:     log,
		Sender:  sender,
		Minio:   minioClient,
		From:    cfg.EmailFrom,
		ReplyTo: cfg.EmailReplyTo,
	}
	worker.Run(ctx, nc)
}
