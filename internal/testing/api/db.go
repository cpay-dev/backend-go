package api_test

import (
	"context"
	"testing"
	"time"

	"github.com/cpay-dev/backend-go/internal/api/migrate"
	"github.com/cpay-dev/backend-go/internal/api/repo/pg/app"
	"github.com/cpay-dev/backend-go/internal/api/repo/pg/blockchain"
	"github.com/cpay-dev/backend-go/internal/api/repo/pg/payment"
	"github.com/cpay-dev/backend-go/internal/api/repo/pg/seed"
	"github.com/cpay-dev/backend-go/internal/api/repo/pg/wallet"
	"github.com/cpay-dev/backend-go/pkg/config"
	"github.com/cpay-dev/backend-go/pkg/db"
	"github.com/cpay-dev/backend-go/pkg/project"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// Option configures SetupRepos behavior.
type Option func(*setupConfig)

type setupConfig struct {
	wantAppRepo        bool
	wantBlockchainRepo bool
	wantPaymentRepo    bool
	wantWalletRepo     bool
	runSeedAppMerchant bool
	runSeedBlockchain  bool
	customSeeders      []func(ctx context.Context, r Repos) error
}

// WithAppRepo requests returning the app repo.
func WithAppRepo() Option { return func(c *setupConfig) { c.wantAppRepo = true } }

// WithBlockchainRepo requests returning the blockchain repo.
func WithBlockchainRepo() Option { return func(c *setupConfig) { c.wantBlockchainRepo = true } }

// WithPaymentRepo requests returning the payment repo.
func WithPaymentRepo() Option { return func(c *setupConfig) { c.wantPaymentRepo = true } }

// WithWalletRepo requests returning the wallet repo.
func WithWalletRepo() Option { return func(c *setupConfig) { c.wantWalletRepo = true } }

// WithSeedAppMerchant enables seeding default app data used in tests.
func WithSeedAppMerchant() Option { return func(c *setupConfig) { c.runSeedAppMerchant = true } }

// WithSeedBlockchain enables seeding default blockchain data used in tests.
func WithSeedBlockchain() Option { return func(c *setupConfig) { c.runSeedBlockchain = true } }

// WithSeeder registers a custom seeder to be executed after migrations.
func WithSeeder(fn func(ctx context.Context, r Repos) error) Option {
	return func(c *setupConfig) { c.customSeeders = append(c.customSeeders, fn) }
}

// Repos holds initialized repositories. Fields may be nil when not requested.
type Repos struct {
	App        *app.PostgresRepo
	Blockchain *blockchain.PostgresRepo
	Payment    *payment.PostgresRepo
	Wallet     *wallet.PostgresRepo
}

func SetupBlockchainRepo(t *testing.T) *blockchain.PostgresRepo {
	repos := SetupRepos(t, WithBlockchainRepo(), WithSeedBlockchain())
	return repos.Blockchain
}

func SetupAppRepo(t *testing.T) *app.PostgresRepo {
	repos := SetupRepos(t, WithAppRepo(), WithSeedAppMerchant())
	return repos.App
}

func SetupPaymentRepo(t *testing.T) *payment.PostgresRepo {
	repos := SetupRepos(t, WithPaymentRepo())
	return repos.Payment
}

// SetupRepos starts a temporary Postgres, migrates requested schemas, optionally seeds, and returns repos.
func SetupRepos(t *testing.T, opts ...Option) Repos {
	t.Helper()

	// Apply options
	cfg := &setupConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}

	container, err := postgres.Run(t.Context(), "ghcr.io/cpay-dev/postgres:18.0",
		testcontainers.WithWaitStrategyAndDeadline(time.Second*10, wait.ForExposedPort()))
	require.NoError(t, err, "run pg container")
	defer testcontainers.CleanupContainer(t, container)

	projRoot, err := project.FindProjectRoot(5)
	require.NoError(t, err, "find project root")

	connStr, err := container.ConnectionString(t.Context())
	require.NoError(t, err, "get pg conn str")

	pool, err := db.NewPgxPoolFromConn(t.Context(), connStr, nil, nil)
	require.NoError(t, err, "create pgx pool")
	t.Cleanup(pool.Close)

	_, err = pool.Exec(t.Context(), "CREATE EXTENSION ulid;")
	require.NoError(t, err, "create ulid extension")

	// Build DB config for migrators
	port, err := container.MappedPort(t.Context(), "5432")
	require.NoError(t, err, "get pg port")
	dbConf := config.Database{
		Host:     "localhost:" + port.Port(),
		Username: "postgres",
		Password: "postgres",
		Database: "postgres",
		SSLMode:  "disable",
	}

	ctx := t.Context()

	// Determine which schemas are needed (by repos requested or seeders required)

	// Migrate required schemas
	if cfg.wantAppRepo {
		sqlSchemaDir := "file://" + projRoot + "/internal/api/repo/pg/app/sql"
		m := migrate.NewMigrator(migrate.Config{
			Database:     dbConf,
			ForceVersion: 1,
			SqlSchemaDir: sqlSchemaDir,
			Schema:       "app",
		})
		require.NoError(t, m.Migrate(ctx), "migrate app schema")
	}
	if cfg.wantBlockchainRepo {
		sqlSchemaDir := "file://" + projRoot + "/internal/api/repo/pg/blockchain/sql"
		m := migrate.NewMigrator(migrate.Config{
			Database:     dbConf,
			ForceVersion: 1,
			SqlSchemaDir: sqlSchemaDir,
			Schema:       "blockchain",
		})
		require.NoError(t, m.Migrate(ctx), "migrate blockchain schema")
	}
	if cfg.wantWalletRepo {
		sqlSchemaDir := "file://" + projRoot + "/internal/api/repo/pg/wallet/sql"
		m := migrate.NewMigrator(migrate.Config{
			Database:     dbConf,
			ForceVersion: 1,
			SqlSchemaDir: sqlSchemaDir,
			Schema:       "wallet",
		})
		require.NoError(t, m.Migrate(ctx), "migrate wallet schema")
	}
	if cfg.wantPaymentRepo {
		sqlSchemaDir := "file://" + projRoot + "/internal/api/repo/pg/payment/sql"
		m := migrate.NewMigrator(migrate.Config{
			Database:     dbConf,
			ForceVersion: 1,
			SqlSchemaDir: sqlSchemaDir,
			Schema:       "payment",
		})
		require.NoError(t, m.Migrate(ctx), "migrate payment schema")
	}

	// Construct repos as needed
	wrapped := db.NewPgxPoolWrapper(pool)
	var appRepo *app.PostgresRepo
	var blockchainRepo *blockchain.PostgresRepo
	var paymentRepo *payment.PostgresRepo
	var walletRepo *wallet.PostgresRepo
	if cfg.wantAppRepo {
		appRepo = app.NewPostgresRepo(wrapped)
	}
	if cfg.wantBlockchainRepo {
		blockchainRepo = blockchain.NewPostgresRepo(wrapped)
	}
	if cfg.wantPaymentRepo {
		paymentRepo = payment.NewPostgresRepo(wrapped)
	}
	if cfg.wantWalletRepo {
		walletRepo = wallet.NewPostgresRepo(wrapped)
	}

	// Run seeders if requested
	if cfg.runSeedAppMerchant {
		seeder := seed.NewMerchantSeeder(appRepo)
		require.NoError(t, seeder.Seed(ctx), "seed app merchant data")
	}
	if cfg.runSeedBlockchain {
		seeder := seed.NewBlockchainSeeder(blockchainRepo)
		require.NoError(t, seeder.Seed(ctx), "seed blockchain data")
	}

	// Run custom seeders, if any
	if len(cfg.customSeeders) > 0 {
		allRepos := Repos{App: appRepo, Blockchain: blockchainRepo}
		for _, s := range cfg.customSeeders {
			require.NoError(t, s(ctx, allRepos), "run custom seeder")
		}
	}

	// Return only the repos that were requested by caller
	var out Repos
	if cfg.wantAppRepo {
		out.App = appRepo
	}
	if cfg.wantBlockchainRepo {
		out.Blockchain = blockchainRepo
	}
	if cfg.wantPaymentRepo {
		out.Payment = paymentRepo
	}
	if cfg.wantWalletRepo {
		out.Wallet = walletRepo
	}
	return out
}
