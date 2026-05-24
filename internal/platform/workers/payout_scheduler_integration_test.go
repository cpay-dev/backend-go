package workers

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/cpay-dev/cpay/internal/platform/db"
	"github.com/cpay-dev/cpay/internal/platform/migrate"
	cryptox "github.com/cpay-dev/cpay/internal/shared/crypto"
	"github.com/cpay-dev/cpay/internal/shared/ids"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

type recordingPayoutExecutor struct {
	calls []PayoutExecutionRequest
	err   error
}

func (e *recordingPayoutExecutor) ExecutePayout(_ context.Context, req PayoutExecutionRequest) (PayoutExecutionResult, error) {
	e.calls = append(e.calls, req)
	if e.err != nil {
		return PayoutExecutionResult{}, e.err
	}
	return PayoutExecutionResult{TxHash: "0xsettled"}, nil
}

func TestPayoutSchedulerLegacyEOAUnsupported(t *testing.T) {
	ctx, pool := openPayoutTestDB(t)
	encryptKey := cryptox.NormalizeKey("test-key")
	_, intentID := seedConfirmedPayoutIntent(t, ctx, pool, encryptKey, "0xde0B295669a9FD93d5F28D9Ec85E40f4cb697BAe")
	executor := &recordingPayoutExecutor{}

	worker := PayoutScheduler{DB: pool, Log: zerolog.Nop(), Source: "test-payout", Executor: executor, EncryptKey: encryptKey}
	worker.process(ctx)

	if len(executor.calls) != 0 {
		t.Fatalf("expected no executor calls, got %d", len(executor.calls))
	}
	var intentStatus, payoutStatus, itemStatus, itemError, payoutError string
	if err := pool.QueryRow(ctx, `
		SELECT i.status, p.status, pi.status, COALESCE(pi.last_error, ''), COALESCE(p.last_error, '')
		FROM checkout.payment_intents i
		JOIN checkout.payout_items pi ON pi.payment_intent_id=i.id
		JOIN checkout.payouts p ON p.id=pi.payout_id
		WHERE i.id=$1
	`, intentID).Scan(&intentStatus, &payoutStatus, &itemStatus, &itemError, &payoutError); err != nil {
		t.Fatalf("query statuses: %v", err)
	}
	if intentStatus != "confirmed" || payoutStatus != "failed" || itemStatus != "failed" {
		t.Fatalf("unexpected statuses: intent=%s payout=%s item=%s", intentStatus, payoutStatus, itemStatus)
	}
	if !strings.Contains(itemError, legacyEOAPayoutUnsupportedError) || !strings.Contains(payoutError, legacyEOAPayoutUnsupportedError) {
		t.Fatalf("expected legacy unsupported errors, item=%q payout=%q", itemError, payoutError)
	}
}

func TestPayoutSchedulerCreate2Success(t *testing.T) {
	ctx, pool := openPayoutTestDB(t)
	encryptKey := cryptox.NormalizeKey("test-key")
	_, intentID := seedConfirmedCreate2PayoutIntent(t, ctx, pool, "0xde0B295669a9FD93d5F28D9Ec85E40f4cb697BAe")
	executor := &recordingPayoutExecutor{}

	worker := PayoutScheduler{DB: pool, Log: zerolog.Nop(), Source: "test-payout", Executor: executor, EncryptKey: encryptKey}
	worker.process(ctx)

	if len(executor.calls) != 1 {
		t.Fatalf("expected one executor call, got %d", len(executor.calls))
	}
	if executor.calls[0].WalletType != "create2" {
		t.Fatalf("expected create2 wallet type, got %q", executor.calls[0].WalletType)
	}
	if executor.calls[0].DepositPrivateKeyHex != "" {
		t.Fatalf("expected no private key for create2 wallet, got %q", executor.calls[0].DepositPrivateKeyHex)
	}
	if executor.calls[0].FactoryAddress == "" || executor.calls[0].WalletSalt == "" {
		t.Fatalf("expected factory metadata in executor call: %+v", executor.calls[0])
	}
	var intentStatus, itemStatus, addressStatus string
	if err := pool.QueryRow(ctx, `
		SELECT i.status, pi.status, d.status
		FROM checkout.payment_intents i
		JOIN checkout.payout_items pi ON pi.payment_intent_id=i.id
		JOIN checkout.deposit_addresses d ON d.payment_intent_id=i.id
		WHERE i.id=$1
	`, intentID).Scan(&intentStatus, &itemStatus, &addressStatus); err != nil {
		t.Fatalf("query statuses: %v", err)
	}
	if intentStatus != "settled" || itemStatus != "completed" || addressStatus != "swept" {
		t.Fatalf("unexpected statuses: intent=%s item=%s address=%s", intentStatus, itemStatus, addressStatus)
	}
}

func TestPayoutSchedulerCreate2FailureIsRetryable(t *testing.T) {
	ctx, pool := openPayoutTestDB(t)
	encryptKey := cryptox.NormalizeKey("test-key")
	_, intentID := seedConfirmedCreate2PayoutIntent(t, ctx, pool, "0xde0B295669a9FD93d5F28D9Ec85E40f4cb697BAe")
	executor := &recordingPayoutExecutor{err: errors.New("estimate deployAndSweep gas: insufficient funds")}

	worker := PayoutScheduler{DB: pool, Log: zerolog.Nop(), Source: "test-payout", Executor: executor, EncryptKey: encryptKey}
	worker.process(ctx)

	var intentStatus, payoutStatus, itemStatus, lastError string
	if err := pool.QueryRow(ctx, `
		SELECT i.status, p.status, pi.status, COALESCE(pi.last_error, '')
		FROM checkout.payment_intents i
		JOIN checkout.payout_items pi ON pi.payment_intent_id=i.id
		JOIN checkout.payouts p ON p.id=pi.payout_id
		WHERE i.id=$1
	`, intentID).Scan(&intentStatus, &payoutStatus, &itemStatus, &lastError); err != nil {
		t.Fatalf("query statuses: %v", err)
	}
	if intentStatus != "confirmed" || payoutStatus != "scheduled" || itemStatus != "failed" {
		t.Fatalf("unexpected statuses: intent=%s payout=%s item=%s", intentStatus, payoutStatus, itemStatus)
	}
	if !strings.Contains(lastError, "deployAndSweep") {
		t.Fatalf("expected clear last_error, got %q", lastError)
	}
}

func TestPayoutSchedulerMissingSettlementAddressRetries(t *testing.T) {
	ctx, pool := openPayoutTestDB(t)
	encryptKey := cryptox.NormalizeKey("test-key")
	_, intentID := seedConfirmedPayoutIntent(t, ctx, pool, encryptKey, "")
	executor := &recordingPayoutExecutor{}

	worker := PayoutScheduler{DB: pool, Log: zerolog.Nop(), Source: "test-payout", Executor: executor, EncryptKey: encryptKey}
	worker.process(ctx)

	if len(executor.calls) != 0 {
		t.Fatalf("expected no executor calls, got %d", len(executor.calls))
	}
	var intentStatus, payoutStatus, itemStatus string
	if err := pool.QueryRow(ctx, `
		SELECT i.status, p.status, pi.status
		FROM checkout.payment_intents i
		JOIN checkout.payout_items pi ON pi.payment_intent_id=i.id
		JOIN checkout.payouts p ON p.id=pi.payout_id
		WHERE i.id=$1
	`, intentID).Scan(&intentStatus, &payoutStatus, &itemStatus); err != nil {
		t.Fatalf("query statuses: %v", err)
	}
	if intentStatus != "confirmed" || payoutStatus != "scheduled" || itemStatus != "failed" {
		t.Fatalf("unexpected statuses: intent=%s payout=%s item=%s", intentStatus, payoutStatus, itemStatus)
	}
}

func TestPayoutSchedulerMockModeDoesNotRequireSettlementAddress(t *testing.T) {
	ctx, pool := openPayoutTestDB(t)
	encryptKey := cryptox.NormalizeKey("test-key")
	_, intentID := seedConfirmedPayoutIntent(t, ctx, pool, encryptKey, "")

	worker := PayoutScheduler{DB: pool, Log: zerolog.Nop(), Source: "test-payout", Executor: MockPayoutExecutor{}, EncryptKey: encryptKey}
	worker.process(ctx)

	var intentStatus, itemStatus string
	if err := pool.QueryRow(ctx, `
		SELECT i.status, pi.status
		FROM checkout.payment_intents i
		JOIN checkout.payout_items pi ON pi.payment_intent_id=i.id
		WHERE i.id=$1
	`, intentID).Scan(&intentStatus, &itemStatus); err != nil {
		t.Fatalf("query statuses: %v", err)
	}
	if intentStatus != "settled" || itemStatus != "completed" {
		t.Fatalf("unexpected statuses: intent=%s item=%s", intentStatus, itemStatus)
	}
}

func openPayoutTestDB(t *testing.T) (context.Context, *pgxpool.Pool) {
	t.Helper()
	dbURL := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if dbURL == "" {
		t.Skip("set TEST_DATABASE_URL to run payout scheduler integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	pool, err := db.Connect(ctx, dbURL)
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	t.Cleanup(pool.Close)
	resetPayoutTestDB(t, ctx, pool)
	t.Setenv("MIGRATIONS_PATH", payoutTestMigrationsPath(t))
	if err := migrate.RunUp(ctx, pool); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	return ctx, pool
}

func resetPayoutTestDB(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	for _, stmt := range []string{
		`DROP SCHEMA IF EXISTS platform CASCADE`,
		`DROP SCHEMA IF EXISTS checkout CASCADE`,
		`DROP SCHEMA IF EXISTS catalog CASCADE`,
		`DROP SCHEMA IF EXISTS auth CASCADE`,
		`DROP TABLE IF EXISTS schema_migrations`,
	} {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			t.Fatalf("reset db failed: %v", err)
		}
	}
}

func payoutTestMigrationsPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", "migrations"))
}

func seedConfirmedPayoutIntent(t *testing.T, ctx context.Context, pool *pgxpool.Pool, encryptKey []byte, settlementAddress string) (string, string) {
	t.Helper()
	merchantID := ids.New()
	linkID := ids.New()
	sessionID := ids.New()
	intentID := ids.New()
	addressID := ids.New()
	encryptedPK, err := cryptox.EncryptString(encryptKey, "0xabc123")
	if err != nil {
		t.Fatalf("encrypt private key: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO auth.merchants(id, name, settlement_address, created_at, updated_at)
		VALUES($1, 'Merchant', NULLIF($2, ''), NOW(), NOW())
	`, merchantID, settlementAddress); err != nil {
		t.Fatalf("insert merchant: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO catalog.payment_links(id, merchant_id, code, title, pricing_mode, amount, currency, cta_text, after_payment_type, created_at, updated_at)
		VALUES($1, $2, $3, 'Link', 'fixed', 1, 'USD', 'Pay', 'confirmation_page', NOW(), NOW())
	`, linkID, merchantID, ids.New()); err != nil {
		t.Fatalf("insert link: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO checkout.checkout_sessions(id, payment_link_id, merchant_id, status, amount, currency, chain, token_symbol, expires_at, created_at, updated_at)
		VALUES($1, $2, $3, 'paid', 1, 'USD', 'base', 'ETH', NOW() + INTERVAL '1 hour', NOW(), NOW())
	`, sessionID, linkID, merchantID); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO checkout.payment_intents(
			id, checkout_session_id, payment_link_id, merchant_id, status, chain, token_symbol,
			expected_amount, tolerance_percent, min_acceptable_amount, max_acceptable_amount, received_amount,
			confirmations, required_confirmations, expires_at, created_at, updated_at
		)
		VALUES($1, $2, $3, $4, 'confirmed', 'base', 'ETH', 1, 0.25, 0.99, 1.01, 1, 12, 12, NOW() + INTERVAL '1 hour', NOW(), NOW())
	`, intentID, sessionID, linkID, merchantID); err != nil {
		t.Fatalf("insert intent: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO checkout.deposit_addresses(id, payment_intent_id, merchant_id, chain, address, encrypted_private_key, status, created_at)
		VALUES($1, $2, $3, 'base', '0xde0B295669a9FD93d5F28D9Ec85E40f4cb697BAe', $4, 'active', NOW())
	`, addressID, intentID, merchantID, encryptedPK); err != nil {
		t.Fatalf("insert address: %v", err)
	}
	return merchantID, intentID
}

func seedConfirmedCreate2PayoutIntent(t *testing.T, ctx context.Context, pool *pgxpool.Pool, settlementAddress string) (string, string) {
	t.Helper()
	merchantID := ids.New()
	linkID := ids.New()
	sessionID := ids.New()
	intentID := ids.New()
	addressID := ids.New()

	if _, err := pool.Exec(ctx, `
		INSERT INTO auth.merchants(id, name, settlement_address, created_at, updated_at)
		VALUES($1, 'Merchant', NULLIF($2, ''), NOW(), NOW())
	`, merchantID, settlementAddress); err != nil {
		t.Fatalf("insert merchant: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO catalog.payment_links(id, merchant_id, code, title, pricing_mode, amount, currency, cta_text, after_payment_type, created_at, updated_at)
		VALUES($1, $2, $3, 'Link', 'fixed', 1, 'USD', 'Pay', 'confirmation_page', NOW(), NOW())
	`, linkID, merchantID, ids.New()); err != nil {
		t.Fatalf("insert link: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO checkout.checkout_sessions(id, payment_link_id, merchant_id, status, amount, currency, chain, token_symbol, expires_at, created_at, updated_at)
		VALUES($1, $2, $3, 'paid', 1, 'USD', 'base', 'ETH', NOW() + INTERVAL '1 hour', NOW(), NOW())
	`, sessionID, linkID, merchantID); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO checkout.payment_intents(
			id, checkout_session_id, payment_link_id, merchant_id, status, chain, token_symbol,
			expected_amount, tolerance_percent, min_acceptable_amount, max_acceptable_amount, received_amount,
			confirmations, required_confirmations, expires_at, created_at, updated_at
		)
		VALUES($1, $2, $3, $4, 'confirmed', 'base', 'ETH', 1, 0.25, 0.99, 1.01, 1, 12, 12, NOW() + INTERVAL '1 hour', NOW(), NOW())
	`, intentID, sessionID, linkID, merchantID); err != nil {
		t.Fatalf("insert intent: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO checkout.deposit_addresses(
			id, payment_intent_id, merchant_id, chain, address, encrypted_private_key,
			wallet_type, factory_address, wallet_salt, status, created_at
		)
		VALUES(
			$1, $2, $3, 'base', '0xde0B295669a9FD93d5F28D9Ec85E40f4cb697BAe', NULL,
			'create2', '0x90a546a5fb533d4f168846400656b663F12578d6',
			'0x000000000000000000000000000000000000000000000000000000000000007b', 'active', NOW()
		)
	`, addressID, intentID, merchantID); err != nil {
		t.Fatalf("insert address: %v", err)
	}
	return merchantID, intentID
}
