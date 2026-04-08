package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	ServiceName string
	Environment string
	HTTPAddr    string
	GRPCAddr    string

	DatabaseURL string

	JWTSecret     string
	JWTAccessTTL  time.Duration
	JWTRefreshTTL time.Duration

	EncryptionKey string

	MinIOEndpoint  string
	MinIOAccessKey string
	MinIOSecretKey string
	MinIOBucket    string
	MinIOUseSSL    bool

	NATSURL string

	ResendAPIKey string
	EmailFrom    string
	EmailReplyTo string

	AuthGRPCAddr        string
	PaymentLinkGRPCAddr string
	CheckoutGRPCAddr    string

	DefaultTolerancePercent float64
	ChainConfirmations      map[string]int

	OutboxPollInterval time.Duration
	WorkerInterval     time.Duration

	WebhookTimeout    time.Duration
	WebhookMaxRetries int

	BootstrapMerchantName string
	BootstrapAdminEmail   string
	BootstrapAdminPass    string
}

func Load(serviceName string) Config {
	cfg := Config{
		ServiceName: serviceName,
		Environment: getEnv("APP_ENV", "development"),
		HTTPAddr:    getEnv("HTTP_ADDR", ":8080"),
		GRPCAddr:    getEnv("GRPC_ADDR", ":9090"),

		DatabaseURL: getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/cpay?sslmode=disable"),

		JWTSecret:     getEnv("JWT_SECRET", "cpay-dev-jwt-secret"),
		JWTAccessTTL:  getDuration("JWT_ACCESS_TTL", 15*time.Minute),
		JWTRefreshTTL: getDuration("JWT_REFRESH_TTL", 24*time.Hour*30),

		EncryptionKey: getEnv("KEY_ENCRYPTION_KEY", "cpay-dev-master-key-change-me"),

		MinIOEndpoint:  getEnv("MINIO_ENDPOINT", "localhost:9000"),
		MinIOAccessKey: getEnv("MINIO_ROOT_USER", "cpay-admin"),
		MinIOSecretKey: getEnv("MINIO_ROOT_PASSWORD", "cpay-secret-key"),
		MinIOBucket:    getEnv("MINIO_BUCKET", "cpay-invoices"),
		MinIOUseSSL:    getBool("MINIO_USE_SSL", false),

		NATSURL: getEnv("NATS_URL", "nats://localhost:4222"),

		ResendAPIKey: getEnv("RESEND_API_KEY", ""),
		EmailFrom:    getEnv("EMAIL_FROM", "CPay <onboarding@resend.dev>"),
		EmailReplyTo: getEnv("EMAIL_REPLY_TO", ""),

		AuthGRPCAddr:        getEnv("AUTH_GRPC_ADDR", "localhost:9091"),
		PaymentLinkGRPCAddr: getEnv("PAYMENT_LINK_GRPC_ADDR", "localhost:9092"),
		CheckoutGRPCAddr:    getEnv("CHECKOUT_GRPC_ADDR", "localhost:9093"),

		DefaultTolerancePercent: getFloat("DEFAULT_TOLERANCE_PERCENT", 0.25),
		ChainConfirmations:      parseConfirmations(getEnv("CHAIN_CONFIRMATIONS", "ethereum:12,polygon:12,arbitrum:20,base:12")),

		OutboxPollInterval: getDuration("OUTBOX_POLL_INTERVAL", 2*time.Second),
		WorkerInterval:     getDuration("WORKER_INTERVAL", 10*time.Second),

		WebhookTimeout:    getDuration("WEBHOOK_TIMEOUT", 8*time.Second),
		WebhookMaxRetries: getInt("WEBHOOK_MAX_RETRIES", 8),

		BootstrapMerchantName: getEnv("BOOTSTRAP_MERCHANT_NAME", "Demo Merchant"),
		BootstrapAdminEmail:   getEnv("BOOTSTRAP_ADMIN_EMAIL", "admin@cpay.dev"),
		BootstrapAdminPass:    getEnv("BOOTSTRAP_ADMIN_PASSWORD", "admin123"),
	}
	return cfg
}

func parseConfirmations(raw string) map[string]int {
	out := map[string]int{}
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		parts := strings.SplitN(item, ":", 2)
		if len(parts) != 2 {
			continue
		}
		val, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil || val <= 0 {
			continue
		}
		out[strings.ToLower(strings.TrimSpace(parts[0]))] = val
	}
	if len(out) == 0 {
		out["polygon"] = 12
	}
	return out
}

func getEnv(key, def string) string {
	v := os.Getenv(key)
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

func getInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func getBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func getFloat(key string, def float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return def
	}
	return f
}

func getDuration(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}

func (c Config) ConfirmationForChain(chain string) int {
	k := strings.ToLower(strings.TrimSpace(chain))
	if v, ok := c.ChainConfirmations[k]; ok {
		return v
	}
	for _, v := range c.ChainConfirmations {
		return v
	}
	return 12
}

func (c Config) Validate() error {
	if c.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	if c.JWTSecret == "" {
		return fmt.Errorf("JWT_SECRET is required")
	}
	return nil
}
