package payment

import (
	"context"
	"os"
	"testing"

	apiasset "github.com/cpay-dev/backend-go/internal/api/asset"
	authn "github.com/cpay-dev/backend-go/internal/api/authn"
	merchmiddleware "github.com/cpay-dev/backend-go/internal/api/grpc/merchant/middleware"
	blockchainmodel "github.com/cpay-dev/backend-go/internal/api/repo/pg/blockchain/model"
	pgwallet "github.com/cpay-dev/backend-go/internal/api/repo/pg/wallet"
	apiwallet "github.com/cpay-dev/backend-go/internal/api/wallet"
	testingapi "github.com/cpay-dev/backend-go/internal/testing/api"
	pbpayment "github.com/cpay-dev/proto-go/api/v1/merchant/payment"
	pbwallet "github.com/cpay-dev/proto-go/api/v1/wallet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestCreateIntent(t *testing.T) {
	t.Parallel()

	endpoint := os.Getenv("WALLET_SERVICE_ENDPOINT")
	conn, err := grpc.NewClient(endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err, "connect to wallet service")
	t.Cleanup(func() {
		assert.NoError(t, conn.Close(), "close wallet service connection")
	})
	walletClient := pbwallet.NewWalletServiceClient(conn)
	conn.Connect()

	t.Run("ReusesAvailableWallet", func(t *testing.T) {
		repos := testingapi.SetupRepos(t,
			testingapi.WithAppRepo(), testingapi.WithSeedAppMerchant(),
			testingapi.WithBlockchainRepo(), testingapi.WithSeedBlockchain(),
			testingapi.WithPaymentRepo(),
			testingapi.WithWalletRepo(),
		)

		// Get any available asset from seeded blockchain
		assets, err := repos.Blockchain.ListAssets(t.Context(), blockchainmodel.ChainAny)
		require.NoError(t, err, "list assets")
		require.NotEmpty(t, assets, "seeded assets should not be empty")
		asset := assets[0]

		// Seed an AVAILABLE wallet for the asset's chain
		err = repos.Wallet.CreateWallet(t.Context(), pgwallet.Wallet{
			ChainID:             asset.ChainID,
			Status:              pgwallet.WalletStatusAvailable,
			KekVersion:          1,
			PublicKey:           "0xTESTPUBLICKEY",
			EncryptedPrivateKey: []byte("encrypted"),
		})
		require.NoError(t, err, "seed wallet")

		priceService := apiasset.NewPriceService(repos.Blockchain)
		walletService := apiwallet.NewService(repos.Wallet, walletClient)
		svc := NewService(repos.Blockchain, repos.Payment, walletService)
		svc.priceService = priceService

		ctx := testingapi.ContextWithApiKey(t.Context(), "automation")
		authnSvc := authn.NewService(repos.App)
		mw := merchmiddleware.NewMerchant(authnSvc, nil)

		var resp *pbpayment.CreateIntentResponse
		_, err = mw(ctx, &pbpayment.CreateIntentRequest{}, &grpc.UnaryServerInfo{FullMethod: "/cpay.api.v1.merchant.PaymentService/CreateIntent"}, func(ctx context.Context, _ interface{}) (interface{}, error) {
			var err error
			resp, err = svc.CreateIntent(ctx, &pbpayment.CreateIntentRequest{
				AssetId: asset.ID,
				Amount:  &pbpayment.CreateIntentRequest_AmountUsd{AmountUsd: "1.00"},
			})
			return resp, err
		})
		require.NoError(t, err, "create intent")
		require.NotNil(t, resp, "response should not be nil")
		require.NotNil(t, resp.Intent, "intent should not be nil")
		require.Equal(t, asset.ID, resp.Intent.AssetId, "asset id should match")
		require.Equal(t, pbpayment.IntentStatus_INTENT_STATUS_AWAITING_PAYMENT, resp.Intent.Status, "status should be awaiting payment")
	})

	t.Run("CreatesWalletViaClientWhenUnavailable", func(t *testing.T) {
		repos := testingapi.SetupRepos(t,
			testingapi.WithAppRepo(), testingapi.WithSeedAppMerchant(),
			testingapi.WithBlockchainRepo(), testingapi.WithSeedBlockchain(),
			testingapi.WithPaymentRepo(),
			testingapi.WithWalletRepo(),
		)

		assets, err := repos.Blockchain.ListAssets(t.Context(), blockchainmodel.ChainAny)
		require.NoError(t, err, "list assets")
		require.NotEmpty(t, assets, "seeded assets should not be empty")
		asset := assets[0]

		// Do NOT seed any AVAILABLE wallet, forcing client-based wallet creation path
		priceService := apiasset.NewPriceService(repos.Blockchain)
		walletService := apiwallet.NewService(repos.Wallet, walletClient)
		svc := NewService(repos.Blockchain, repos.Payment, walletService)
		svc.priceService = priceService

		ctx := testingapi.ContextWithApiKey(t.Context(), "automation")
		authnSvc := authn.NewService(repos.App)
		mw := merchmiddleware.NewMerchant(authnSvc, nil)

		var resp *pbpayment.CreateIntentResponse
		_, err = mw(ctx, &pbpayment.CreateIntentRequest{}, &grpc.UnaryServerInfo{FullMethod: "/cpay.api.v1.merchant.PaymentService/CreateIntent"}, func(ctx context.Context, _ interface{}) (interface{}, error) {
			var err error
			resp, err = svc.CreateIntent(ctx, &pbpayment.CreateIntentRequest{
				AssetId: asset.ID,
				Amount:  &pbpayment.CreateIntentRequest_AmountUsd{AmountUsd: "3.50"},
			})
			return resp, err
		})
		require.NoError(t, err, "create intent with client-created wallet")
		require.NotNil(t, resp, "response should not be nil")
		require.NotNil(t, resp.Intent, "intent should not be nil")
		require.Equal(t, asset.ID, resp.Intent.AssetId, "asset id should match")
		require.Equal(t, pbpayment.IntentStatus_INTENT_STATUS_AWAITING_PAYMENT, resp.Intent.Status, "status should be awaiting payment")
	})
}

func TestGetIntent(t *testing.T) {
	t.Parallel()

	repos := testingapi.SetupRepos(t,
		testingapi.WithAppRepo(), testingapi.WithSeedAppMerchant(),
		testingapi.WithBlockchainRepo(), testingapi.WithSeedBlockchain(),
		testingapi.WithPaymentRepo(),
		testingapi.WithWalletRepo(),
	)

	assets, err := repos.Blockchain.ListAssets(t.Context(), blockchainmodel.ChainAny)
	require.NoError(t, err, "list assets")
	require.NotEmpty(t, assets, "seeded assets should not be empty")
	asset := assets[0]

	// Seed available wallet
	err = repos.Wallet.CreateWallet(t.Context(), pgwallet.Wallet{
		ChainID:             asset.ChainID,
		Status:              pgwallet.WalletStatusAvailable,
		KekVersion:          1,
		PublicKey:           "0xTESTPUBLICKEY",
		EncryptedPrivateKey: []byte("encrypted"),
	})
	require.NoError(t, err, "seed wallet")

	endpoint := os.Getenv("WALLET_SERVICE_ENDPOINT")
	if endpoint == "" {
		t.Skip("WALLET_SERVICE_ENDPOINT not set; skipping")
	}
	conn, err := grpc.NewClient(endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err, "connect to wallet service")
	t.Cleanup(func() { _ = conn.Close() })
	walletClient := pbwallet.NewWalletServiceClient(conn)

	priceService := apiasset.NewPriceService(repos.Blockchain)
	walletService := apiwallet.NewService(repos.Wallet, walletClient)
	svc := NewService(repos.Blockchain, repos.Payment, walletService)
	svc.priceService = priceService

	ctx := testingapi.ContextWithApiKey(t.Context(), "automation")
	authnSvc := authn.NewService(repos.App)
	mw := merchmiddleware.NewMerchant(authnSvc, nil)

	var created *pbpayment.CreateIntentResponse
	_, err = mw(ctx, &pbpayment.CreateIntentRequest{}, &grpc.UnaryServerInfo{FullMethod: "/cpay.api.v1.merchant.PaymentService/CreateIntent"}, func(ctx context.Context, _ interface{}) (interface{}, error) {
		var err error
		created, err = svc.CreateIntent(ctx, &pbpayment.CreateIntentRequest{
			AssetId: asset.ID,
			Amount:  &pbpayment.CreateIntentRequest_AmountUsd{AmountUsd: "2.50"},
		})
		return created, err
	})
	require.NoError(t, err, "create intent")
	require.NotNil(t, created.Intent, "intent should not be nil")

	var fetched *pbpayment.GetIntentResponse
	_, err = mw(ctx, &pbpayment.GetIntentRequest{}, &grpc.UnaryServerInfo{FullMethod: "/cpay.api.v1.merchant.PaymentService/GetIntent"}, func(ctx context.Context, _ interface{}) (interface{}, error) {
		var err error
		fetched, err = svc.GetIntent(ctx, &pbpayment.GetIntentRequest{Id: created.Intent.Id})
		return fetched, err
	})
	require.NoError(t, err, "get intent")
	require.NotNil(t, fetched, "response should not be nil")
	require.NotNil(t, fetched.Intent, "intent should not be nil")
	require.Equal(t, created.Intent.Id, fetched.Intent.Id, "intent id should match")
}
