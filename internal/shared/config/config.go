package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/cpay-dev/cpay/internal/shared/chain"
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

	PublicWebOrigin    string
	WebAuthnRPID       string
	WebAuthnRPName     string
	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURI  string

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
	ChainRPCURLs            map[string]string
	ChainRPCFallbackURLs    map[string][]string
	PayoutMode              string
	PayoutHotWalletKey      string
	PayoutGasBufferPercent  float64
	CheckoutWalletFactories map[string]string

	OutboxPollInterval time.Duration
	WorkerInterval     time.Duration

	WebhookTimeout    time.Duration
	WebhookMaxRetries int

	BootstrapMerchantName string
	BootstrapAdminEmail   string
}

var defaultChainConfirmations = map[string]int{
	"ethereum":          64,
	"ethereum mainnet":  64,
	"mainnet":           64,
	"polygon":           6,
	"arbitrum":          4800,
	"arbitrum one":      4800,
	"base":              600,
	"avalanche":         12,
	"avalanche c-chain": 12,
	"avax":              12,
	"hyperevm":          3,
	"hyper evm":         3,
	"hyperliquid":       3,
	"hyperliquid evm":   3,
	"bnb":               6,
	"bsc":               6,
	"bnb smart chain":   6,
	"optimism":          600,
	"solana":            32,
	"tron":              21,
}

var defaultChainRPCURLs = map[string]string{
	"ethereum":  "https://ethereum-rpc.publicnode.com",
	"polygon":   "https://polygon-bor-rpc.publicnode.com",
	"arbitrum":  "https://arbitrum-one-rpc.publicnode.com",
	"base":      "https://base-rpc.publicnode.com",
	"avalanche": "https://avalanche-c-chain-rpc.publicnode.com",
	"hyperevm":  "https://rpc.hypurrscan.io",
	"bsc":       "https://bsc-rpc.publicnode.com",
	"optimism":  "https://optimism-rpc.publicnode.com",
	"solana":    "https://api.mainnet.solana.com",
	"ton":       "https://toncenter.com/api/v2",
	"tron":      "https://api.trongrid.io",
}

var defaultChainRPCFallbackURLs = map[string][]string{
	"hyperevm": {
		"https://hyperliquid.drpc.org",
		"https://rpc.hyperliquid.xyz/evm",
	},
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

		PublicWebOrigin:    strings.TrimRight(getEnv("PUBLIC_WEB_ORIGIN", "http://localhost:3000"), "/"),
		WebAuthnRPID:       getEnv("WEBAUTHN_RP_ID", "localhost"),
		WebAuthnRPName:     getEnv("WEBAUTHN_RP_NAME", "cpay.dev"),
		GoogleClientID:     getEnv("GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret: getEnv("GOOGLE_CLIENT_SECRET", ""),
		GoogleRedirectURI:  getEnv("GOOGLE_REDIRECT_URI", "http://localhost:3000/oauth/consume"),

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
		ChainConfirmations:      parseConfirmations(getEnv("CHAIN_CONFIRMATIONS", "")),
		ChainRPCURLs:            parseChainRPCURLs(getEnv("CHAIN_RPC_URLS", "")),
		ChainRPCFallbackURLs:    parseChainRPCFallbackURLs(getEnv("CHAIN_RPC_FALLBACK_URLS", "")),
		PayoutMode:              strings.ToLower(strings.TrimSpace(getEnv("PAYOUT_MODE", "production"))),
		PayoutHotWalletKey:      getEnv("PAYOUT_HOT_WALLET_PRIVATE_KEY", ""),
		PayoutGasBufferPercent:  getFloat("PAYOUT_GAS_BUFFER_PERCENT", 15),
		CheckoutWalletFactories: parseAddressMap(getEnv("CHECKOUT_WALLET_FACTORY_ADDRESSES", "")),

		OutboxPollInterval: getDuration("OUTBOX_POLL_INTERVAL", 2*time.Second),
		WorkerInterval:     getDuration("WORKER_INTERVAL", 10*time.Second),

		WebhookTimeout:    getDuration("WEBHOOK_TIMEOUT", 8*time.Second),
		WebhookMaxRetries: getInt("WEBHOOK_MAX_RETRIES", 8),

		BootstrapMerchantName: getEnv("BOOTSTRAP_MERCHANT_NAME", "Demo Merchant"),
		BootstrapAdminEmail:   getEnv("BOOTSTRAP_ADMIN_EMAIL", "admin@cpay.dev"),
	}
	return cfg
}

func parseConfirmations(raw string) map[string]int {
	out := cloneIntMap(defaultChainConfirmations)
	parseConfigPairs(raw, ":", func(chain, value string) {
		val, err := strconv.Atoi(value)
		if err != nil || val <= 0 {
			return
		}
		out[chain] = val
	})
	return out
}

func parseChainRPCURLs(raw string) map[string]string {
	out := cloneStringMap(defaultChainRPCURLs)
	parseConfigPairs(raw, "=", func(chain, rpcURL string) {
		out[chain] = rpcURL
	})
	return out
}

func parseChainRPCFallbackURLs(raw string) map[string][]string {
	out := cloneStringSliceMap(defaultChainRPCFallbackURLs)
	parseConfigPairs(raw, "=", func(chain, value string) {
		urls := splitNonEmpty(value, "|")
		if len(urls) > 0 {
			out[chain] = urls
		}
	})
	return out
}

func (c Config) EVMChainRPCURLs() map[string]string {
	out := make(map[string]string, len(c.ChainRPCURLs))
	for chainName, rpcURL := range c.ChainRPCURLs {
		chainName = canonicalChainName(chainName)
		if !isEVMChain(chainName) {
			continue
		}
		out[chainName] = rpcURL
	}
	return out
}

func (c Config) EVMChainRPCFallbackURLs() map[string][]string {
	out := make(map[string][]string, len(c.ChainRPCFallbackURLs))
	for chainName, urls := range c.ChainRPCFallbackURLs {
		chainName = canonicalChainName(chainName)
		if !isEVMChain(chainName) {
			continue
		}
		if urls := trimNonEmpty(urls); len(urls) > 0 {
			out[chainName] = urls
		}
	}
	return out
}

func parseAddressMap(raw string) map[string]string {
	out := map[string]string{}
	parseConfigPairs(raw, "=", func(chain, address string) {
		out[chain] = address
	})
	return out
}

func (c Config) CheckoutWalletFactoryForChain(chain string) string {
	return c.CheckoutWalletFactories[canonicalChainName(chain)]
}

func ValidateChainRPCURLs(urls map[string]string) error {
	if len(urls) == 0 {
		return fmt.Errorf("CHAIN_RPC_URLS is required in production payout mode")
	}
	for chainName, rpcURL := range urls {
		u, err := url.Parse(rpcURL)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return fmt.Errorf("CHAIN_RPC_URLS has invalid url for %s", chainName)
		}
		if u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "ws" && u.Scheme != "wss" {
			return fmt.Errorf("CHAIN_RPC_URLS has unsupported scheme for %s", chainName)
		}
	}
	return nil
}

func ValidateCheckoutWalletFactories(factories map[string]string) error {
	for chainName, address := range factories {
		if !isHexAddress(address) {
			return fmt.Errorf("CHECKOUT_WALLET_FACTORY_ADDRESSES has invalid address for %s", chainName)
		}
	}
	return nil
}

func ValidateProductionCreate2PayoutConfig(rpcURLs, factories map[string]string, hotWalletKey string) error {
	if len(factories) == 0 {
		return fmt.Errorf("CHECKOUT_WALLET_FACTORY_ADDRESSES is required in production")
	}
	if strings.TrimSpace(hotWalletKey) == "" {
		return fmt.Errorf("PAYOUT_HOT_WALLET_PRIVATE_KEY is required in production")
	}
	if err := ValidateCheckoutWalletFactories(factories); err != nil {
		return err
	}
	for chainName := range rpcURLs {
		if _, ok := factories[canonicalChainName(chainName)]; !ok {
			return fmt.Errorf("CHECKOUT_WALLET_FACTORY_ADDRESSES is missing %s", chainName)
		}
	}
	return nil
}

func canonicalChainName(chainName string) string {
	return chain.NormalizeEVMChain(chainName)
}

func isEVMChain(chainName string) bool {
	return chain.IsEVMChain(chainName)
}

func parseConfigPairs(raw, sep string, visit func(chain, value string)) {
	for _, item := range strings.Split(raw, ",") {
		parts := strings.SplitN(strings.TrimSpace(item), sep, 2)
		if len(parts) != 2 {
			continue
		}
		chain, value := canonicalChainName(parts[0]), strings.TrimSpace(parts[1])
		if chain != "" && value != "" {
			visit(chain, value)
		}
	}
}

func cloneIntMap(in map[string]int) map[string]int {
	out := make(map[string]int, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneStringSliceMap(in map[string][]string) map[string][]string {
	out := make(map[string][]string, len(in))
	for k, v := range in {
		out[k] = append([]string(nil), v...)
	}
	return out
}

func splitNonEmpty(raw, sep string) []string {
	return trimNonEmpty(strings.Split(raw, sep))
}

func trimNonEmpty(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func isHexAddress(address string) bool {
	address = strings.TrimSpace(address)
	if len(address) != 42 || !strings.HasPrefix(address, "0x") {
		return false
	}
	for _, r := range address[2:] {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F') {
			continue
		}
		return false
	}
	return true
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
	k := canonicalChainName(chain)
	if v, ok := c.ChainConfirmations[k]; ok {
		return v
	}
	if v, ok := defaultChainConfirmations[k]; ok {
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
	if c.PayoutMode != "production" && c.PayoutMode != "mock" {
		return fmt.Errorf("PAYOUT_MODE must be production or mock")
	}
	if c.PayoutGasBufferPercent < 0 {
		return fmt.Errorf("PAYOUT_GAS_BUFFER_PERCENT must be non-negative")
	}
	if err := ValidateCheckoutWalletFactories(c.CheckoutWalletFactories); err != nil {
		return err
	}
	if c.Environment == "production" && c.PayoutMode == "production" {
		if err := ValidateProductionCreate2PayoutConfig(c.EVMChainRPCURLs(), c.CheckoutWalletFactories, c.PayoutHotWalletKey); err != nil {
			return err
		}
	}
	return nil
}
