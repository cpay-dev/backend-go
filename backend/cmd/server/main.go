package main

import (
	"context"
	"fmt"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/cpay-dev/backend/internal/api"
	"github.com/cpay-dev/backend/internal/auth"
	"github.com/cpay-dev/backend/internal/consumer"
	"github.com/cpay-dev/backend/internal/db"
	"github.com/cpay-dev/backend/internal/email"
	"github.com/cpay-dev/backend/internal/events"
	grpcclient "github.com/cpay-dev/backend/internal/grpc"
	"github.com/cpay-dev/backend/internal/scheduler"
	"github.com/cpay-dev/backend/internal/storage"
)

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs
	log.Logger = zerolog.New(os.Stdout).With().Timestamp().Logger()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Database
	dbURL := mustEnv("DATABASE_URL")
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to database")
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatal().Err(err).Msg("failed to ping database")
	}
	log.Info().Msg("connected to database")

	queries := db.New(pool)

	// Auth
	authService := auth.NewService(mustEnv("JWT_SECRET"))

	// MinIO storage
	minioUseSSL := envOr("MINIO_USE_SSL", "false") == "true"
	storageSvc, err := storage.NewMinIOService(
		mustEnv("MINIO_ENDPOINT"),
		mustEnv("MINIO_ACCESS_KEY"),
		mustEnv("MINIO_SECRET_KEY"),
		envOr("MINIO_BUCKET", "cpay-avatars"),
		envOr("MINIO_PUBLIC_URL", "http://localhost:9000"),
		minioUseSSL,
	)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create minio client")
	}
	log.Info().Msg("connected to minio")

	// NATS
	natsURL := envOr("NATS_URL", "nats://localhost:4222")
	eventPublisher, err := events.NewPublisher(natsURL)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create NATS publisher")
	}
	defer eventPublisher.Close()

	// Chain service (gRPC)
	chainServiceAddr := envOr("CHAIN_SERVICE_ADDR", "localhost:50051")
	chainClient, err := grpcclient.NewChainClient(chainServiceAddr)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to chain service")
	}
	defer chainClient.Close()
	log.Info().Str("addr", chainServiceAddr).Msg("connected to chain service")

	// Email service (optional — disabled if no API key)
	var emailSvc *email.Service
	if apiKey := os.Getenv("RESEND_API_KEY"); apiKey != "" {
		fromEmail := envOr("FROM_EMAIL", "receipt@cpay.dev")
		emailSvc = email.NewService(apiKey, fromEmail)
		log.Info().Str("from", fromEmail).Msg("email service enabled")
	} else {
		log.Warn().Msg("RESEND_API_KEY not set, email service disabled")
	}

	// NATS consumers
	cons := consumer.New(eventPublisher.JetStream(), queries, emailSvc, chainClient)
	if err := cons.Start(ctx); err != nil {
		log.Fatal().Err(err).Msg("failed to start NATS consumers")
	}

	// Scheduler
	sched := scheduler.New(queries, chainClient, eventPublisher)
	go sched.Start(ctx)

	// Router
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(corsMiddleware)

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"ok"}`)
	})

	api.RegisterRoutes(r, queries, authService, storageSvc, eventPublisher, chainClient)

	// Server
	port := envOr("PORT", "8080")
	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Info().Msg("shutting down server")
		cancel()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Error().Err(err).Msg("server shutdown error")
		}
	}()

	log.Info().Str("port", port).Msg("starting server")
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal().Err(err).Msg("server error")
	}
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatal().Str("key", key).Msg("required environment variable not set")
	}
	return v
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
