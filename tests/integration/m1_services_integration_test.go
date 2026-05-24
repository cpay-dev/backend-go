//go:build integration

package integration

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	cpayv1 "github.com/cpay-dev/cpay/internal/gen/cpay/v1"
	"github.com/cpay-dev/cpay/internal/platform/db"
	"github.com/cpay-dev/cpay/internal/platform/migrate"
	"github.com/cpay-dev/cpay/internal/services/authsvc"
	"github.com/cpay-dev/cpay/internal/services/checkoutsvc"
	"github.com/cpay-dev/cpay/internal/services/paymentlinksvc"
	"github.com/cpay-dev/cpay/internal/shared/chain"
	"github.com/cpay-dev/cpay/internal/shared/config"
	cryptox "github.com/cpay-dev/cpay/internal/shared/crypto"
	"github.com/cpay-dev/cpay/internal/shared/ids"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestMigrationsRunUpTwice(t *testing.T) {
	ctx := context.Background()
	pool, _ := openIntegrationDB(t)
	defer pool.Close()

	resetDatabase(t, ctx, pool)
	t.Setenv("MIGRATIONS_PATH", cfgMigrationsPath(t))

	if err := migrate.RunUp(ctx, pool); err != nil {
		t.Fatalf("first migrate up failed: %v", err)
	}
	if err := migrate.RunUp(ctx, pool); err != nil {
		t.Fatalf("second migrate up should be no-op, got: %v", err)
	}

	assertRelationExists(t, ctx, pool, "auth.users")
	assertRelationExists(t, ctx, pool, "catalog.payment_links")
	assertRelationExists(t, ctx, pool, "checkout.payment_intents")
	assertRelationExists(t, ctx, pool, "platform.outbox_events")
}

func TestPostgresULIDType_SQLAndGoRoundTrip(t *testing.T) {
	ctx := context.Background()
	pool, _ := openIntegrationDB(t)
	defer pool.Close()

	resetDatabase(t, ctx, pool)
	t.Setenv("MIGRATIONS_PATH", cfgMigrationsPath(t))
	if err := migrate.RunUp(ctx, pool); err != nil {
		t.Fatalf("migrate up failed: %v", err)
	}

	var generatedID, generatedType string
	if err := pool.QueryRow(ctx, `
		WITH generated AS (SELECT gen_ulid() AS id)
		SELECT id::text, pg_typeof(id)::text
		FROM generated
	`).Scan(&generatedID, &generatedType); err != nil {
		t.Fatalf("generate sql ulid failed: %v", err)
	}
	if generatedType != "ulid" {
		t.Fatalf("expected sql type ulid, got %q", generatedType)
	}
	if !ids.IsValid(generatedID) {
		t.Fatalf("expected valid sql-generated ULID, got %q", generatedID)
	}

	merchantID := ids.New()
	var insertedID, insertedType string
	if err := pool.QueryRow(ctx, `
		INSERT INTO auth.merchants(id, name, created_at, updated_at)
		VALUES($1, 'ULID Merchant', NOW(), NOW())
		RETURNING id, pg_typeof(id)::text
	`, merchantID).Scan(&insertedID, &insertedType); err != nil {
		t.Fatalf("insert go-generated ULID failed: %v", err)
	}
	if insertedID != merchantID {
		t.Fatalf("expected inserted id %s, got %s", merchantID, insertedID)
	}
	if insertedType != "ulid" {
		t.Fatalf("expected inserted column type ulid, got %q", insertedType)
	}

	var selectedID string
	if err := pool.QueryRow(ctx, `
		SELECT id
		FROM auth.merchants
		WHERE id=$1
	`, merchantID).Scan(&selectedID); err != nil {
		t.Fatalf("select native ulid into Go string failed: %v", err)
	}
	if selectedID != merchantID {
		t.Fatalf("expected selected id %s, got %s", merchantID, selectedID)
	}
}

func TestM1ServiceFlow_AuthPaymentLinkCheckout(t *testing.T) {
	ctx := context.Background()
	pool, cfg := openIntegrationDB(t)
	defer pool.Close()

	resetDatabase(t, ctx, pool)
	t.Setenv("MIGRATIONS_PATH", cfgMigrationsPath(t))
	if err := migrate.RunUp(ctx, pool); err != nil {
		t.Fatalf("migrate up failed: %v", err)
	}

	log := zerolog.New(io.Discard)
	authService := authsvc.New(cfg, log, pool)
	if err := authService.EnsureBootstrap(ctx); err != nil {
		t.Fatalf("bootstrap failed: %v", err)
	}

	var merchantID string
	if err := pool.QueryRow(ctx, `SELECT merchant_id::text FROM auth.users WHERE lower(email)=lower($1)`, cfg.BootstrapAdminEmail).Scan(&merchantID); err != nil {
		t.Fatalf("load bootstrap merchant failed: %v", err)
	}
	if merchantID == "" {
		t.Fatalf("expected merchant_id from bootstrap user")
	}

	createdKey, err := authService.CreateApiKey(ctx, &cpayv1.CreateApiKeyRequest{
		MerchantId: merchantID,
		Name:       "integration-key",
		Scopes:     []string{"payments:write"},
	})
	if err != nil {
		t.Fatalf("create api key failed: %v", err)
	}
	if strings.TrimSpace(createdKey.GetPlainKey()) == "" {
		t.Fatalf("expected plaintext api key")
	}

	validated, err := authService.ValidateCredential(ctx, &cpayv1.ValidateCredentialRequest{
		ApiKey: createdKey.GetPlainKey(),
	})
	if err != nil {
		t.Fatalf("validate api key failed: %v", err)
	}
	if validated.GetPrincipal().GetMerchantId() != merchantID {
		t.Fatalf("expected merchant_id %s, got %s", merchantID, validated.GetPrincipal().GetMerchantId())
	}

	_, err = authService.RevokeApiKey(ctx, &cpayv1.RevokeApiKeyRequest{
		MerchantId: merchantID,
		Id:         createdKey.GetApiKey().GetId(),
	})
	if err != nil {
		t.Fatalf("revoke api key failed: %v", err)
	}
	_, err = authService.ValidateCredential(ctx, &cpayv1.ValidateCredentialRequest{
		ApiKey: createdKey.GetPlainKey(),
	})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("expected unauthenticated after revoke, got %v", err)
	}

	linkService := paymentlinksvc.New(cfg, log, pool)
	amount := 10.0
	reusable := true
	linkResp, err := linkService.CreatePaymentLink(ctx, &cpayv1.CreatePaymentLinkRequest{
		MerchantId:  merchantID,
		Title:       "Integration Link",
		PricingMode: "fixed",
		Amount:      &amount,
		Currency:    "USD",
		AllowedTokens: []*cpayv1.AllowedToken{
			{
				Chain:   "polygon",
				Symbol:  "USDC",
				Address: "0x1111111111111111111111111111111111111111",
			},
		},
		Reusable: &reusable,
		Options: &cpayv1.LinkOptions{
			AddInvoicePdf: false,
		},
	})
	if err != nil {
		t.Fatalf("create payment link failed: %v", err)
	}

	checkoutService := checkoutsvc.New(
		cfg,
		log,
		pool,
		chain.NewEVMAdapter(cfg.ChainConfirmations),
		cryptox.NormalizeKey(cfg.EncryptionKey),
		nil,
	)
	sessionResp, err := checkoutService.CreatePublicCheckoutSession(ctx, &cpayv1.CreatePublicCheckoutSessionRequest{
		LinkIdentifier: linkResp.GetCode(),
		Chain:          "polygon",
		TokenSymbol:    "USDC",
		TokenAddress:   "0x1111111111111111111111111111111111111111",
	})
	if err != nil {
		t.Fatalf("create public checkout session failed: %v", err)
	}
	if sessionResp.GetClientSecret() == "" {
		t.Fatalf("expected client_secret from public session")
	}
	if sessionResp.GetStatus() != "awaiting_funds" {
		t.Fatalf("expected awaiting_funds status, got %s", sessionResp.GetStatus())
	}

	publicSession, err := checkoutService.GetPublicCheckoutSession(ctx, &cpayv1.GetPublicCheckoutSessionRequest{
		SessionId: sessionResp.GetId(),
	})
	if err != nil {
		t.Fatalf("get public checkout session failed: %v", err)
	}
	if publicSession.GetId() != sessionResp.GetId() {
		t.Fatalf("expected session id %s, got %s", sessionResp.GetId(), publicSession.GetId())
	}
	if publicSession.GetDepositAddress() == "" {
		t.Fatalf("expected deposit address")
	}

	firstConfirmResp, err := checkoutService.ConfirmCheckoutSession(ctx, &cpayv1.ConfirmCheckoutSessionRequest{
		MerchantId:     merchantID,
		SessionId:      sessionResp.GetId(),
		TxHash:         "0xabc123",
		ReceivedAmount: 4,
		Confirmations:  int32(cfg.ConfirmationForChain("polygon")),
		RawPayloadJson: `{"source":"integration"}`,
	})
	if err != nil {
		t.Fatalf("confirm first checkout transaction failed: %v", err)
	}
	if firstConfirmResp.GetStatus() != "partial" {
		t.Fatalf("expected partial status after first transaction, got %s", firstConfirmResp.GetStatus())
	}

	confirmResp, err := checkoutService.ConfirmCheckoutSession(ctx, &cpayv1.ConfirmCheckoutSessionRequest{
		MerchantId:     merchantID,
		SessionId:      sessionResp.GetId(),
		TxHash:         "0xdef456",
		ReceivedAmount: 6,
		Confirmations:  int32(cfg.ConfirmationForChain("polygon")),
		RawPayloadJson: `{"source":"integration"}`,
	})
	if err != nil {
		t.Fatalf("confirm second checkout transaction failed: %v", err)
	}
	if confirmResp.GetStatus() != "confirmed" {
		t.Fatalf("expected confirmed status, got %s", confirmResp.GetStatus())
	}

	intent, err := checkoutService.GetPaymentIntent(ctx, &cpayv1.GetPaymentIntentRequest{
		MerchantId: merchantID,
		Id:         sessionResp.GetPaymentIntentId(),
	})
	if err != nil {
		t.Fatalf("get payment intent failed: %v", err)
	}
	if intent.GetStatus() != "confirmed" {
		t.Fatalf("expected intent confirmed, got %s", intent.GetStatus())
	}
	if intent.GetTxHash() != "0xdef456" {
		t.Fatalf("expected latest tx hash 0xdef456, got %s", intent.GetTxHash())
	}
	if intent.GetReceivedAmount() != amount {
		t.Fatalf("expected aggregated received amount %.2f, got %.2f", amount, intent.GetReceivedAmount())
	}

	publicSession, err = checkoutService.GetPublicCheckoutSession(ctx, &cpayv1.GetPublicCheckoutSessionRequest{
		SessionId: sessionResp.GetId(),
	})
	if err != nil {
		t.Fatalf("get confirmed public checkout session failed: %v", err)
	}
	if publicSession.GetReceivedAmount() != amount {
		t.Fatalf("expected public session received amount %.2f, got %.2f", amount, publicSession.GetReceivedAmount())
	}
	if len(publicSession.GetTransactions()) != 2 {
		t.Fatalf("expected two public checkout transactions, got %d", len(publicSession.GetTransactions()))
	}
	if publicSession.GetTransactions()[0].GetTxHash() != "0xdef456" || publicSession.GetTransactions()[0].GetAmount() != 6 {
		t.Fatalf("expected latest transaction first, got %#v", publicSession.GetTransactions()[0])
	}

	var outboxCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM platform.outbox_events`).Scan(&outboxCount); err != nil {
		t.Fatalf("count outbox events failed: %v", err)
	}
	if outboxCount < 4 {
		t.Fatalf("expected at least 4 outbox events, got %d", outboxCount)
	}
}

func openIntegrationDB(t *testing.T) (*pgxpool.Pool, config.Config) {
	t.Helper()

	dbURL := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if dbURL == "" {
		dbURL = strings.TrimSpace(os.Getenv("DATABASE_URL"))
	}
	if dbURL == "" {
		t.Skip("set TEST_DATABASE_URL (or DATABASE_URL) to run integration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := db.Connect(ctx, dbURL)
	if err != nil {
		t.Skipf("postgres unavailable for integration tests: %v", err)
	}

	cfg := config.Config{
		ServiceName:             "integration-test",
		DatabaseURL:             dbURL,
		JWTSecret:               "integration-jwt-secret",
		JWTAccessTTL:            15 * time.Minute,
		JWTRefreshTTL:           24 * time.Hour,
		EncryptionKey:           "integration-encryption-key",
		DefaultTolerancePercent: 0.25,
		ChainConfirmations: map[string]int{
			"polygon": 2,
		},
		WebhookMaxRetries:     3,
		BootstrapMerchantName: "Integration Merchant",
		BootstrapAdminEmail:   "admin@cpay.dev",
	}
	return pool, cfg
}

func resetDatabase(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	stmts := []string{
		`DROP SCHEMA IF EXISTS platform CASCADE`,
		`DROP SCHEMA IF EXISTS checkout CASCADE`,
		`DROP SCHEMA IF EXISTS catalog CASCADE`,
		`DROP SCHEMA IF EXISTS auth CASCADE`,
		`DROP TABLE IF EXISTS schema_migrations`,
	}
	for _, stmt := range stmts {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			t.Fatalf("reset db with %q failed: %v", stmt, err)
		}
	}
}

func assertRelationExists(t *testing.T, ctx context.Context, pool *pgxpool.Pool, relation string) {
	t.Helper()
	var exists bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, relation).Scan(&exists); err != nil {
		t.Fatalf("check relation %s failed: %v", relation, err)
	}
	if !exists {
		t.Fatalf("expected relation %s to exist", relation)
	}
}

func cfgMigrationsPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "migrations"))
}
