package merchant_test

import (
	"fmt"
	"math/rand"
	"os"
	"testing"
	"time"

	apiasset "github.com/cpay-dev/backend-go/internal/api/asset"
	"github.com/cpay-dev/backend-go/internal/api/authn"
	"github.com/cpay-dev/backend-go/internal/api/grpc/merchant"
	"github.com/cpay-dev/backend-go/internal/api/grpc/merchant/asset"
	"github.com/cpay-dev/backend-go/internal/api/grpc/merchant/chain"
	"github.com/cpay-dev/backend-go/internal/api/grpc/merchant/payment"
	apiwallet "github.com/cpay-dev/backend-go/internal/api/wallet"
	testingapi "github.com/cpay-dev/backend-go/internal/testing/api"
	"github.com/cpay-dev/backend-go/pkg/log"
	pbasset "github.com/cpay-dev/proto-go/api/v1/merchant/asset"
	pbchain "github.com/cpay-dev/proto-go/api/v1/merchant/chain"
	pbwallet "github.com/cpay-dev/proto-go/api/v1/wallet"
	pbblockchain "github.com/cpay-dev/proto-go/blockchain/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

func TestGrpc(t *testing.T) {
	t.Parallel()

	logger := log.NewZerologPretty()
	repos := testingapi.SetupRepos(t,
		testingapi.WithBlockchainRepo(), testingapi.WithSeedBlockchain(),
		testingapi.WithAppRepo(), testingapi.WithSeedAppMerchant(),
		testingapi.WithPaymentRepo(),
		testingapi.WithWalletRepo(),
	)

	walletServiceConn, err := grpc.NewClient(os.Getenv("WALLET_SERVICE_ENDPOINT"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err, "connect to wallet service")
	t.Cleanup(func() {
		assert.NoError(t, walletServiceConn.Close(), "close wallet service connection")
	})

	walletClient := pbwallet.NewWalletServiceClient(walletServiceConn)
	priceService := apiasset.NewPriceService(repos.Blockchain)
	walletService := apiwallet.NewService(repos.Wallet, walletClient)
	server := merchant.NewServer(
		logger,
		authn.NewService(repos.App),
		asset.NewService(repos.Blockchain, priceService),
		chain.NewService(repos.Blockchain),
		payment.NewService(repos.Blockchain, repos.Payment, walletService),
	)
	randomListenAddress := fmt.Sprintf(":%d", rand.Intn(55_535)+10_000)

	go func() {
		err := server.Start(randomListenAddress, time.Second*30)
		assert.NoError(t, err, "start server")
		t.Cleanup(server.Stop)
	}()

	conn, err := grpc.NewClient(randomListenAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err, "connect to server")
	conn.Connect()

	t.Cleanup(func() {
		assert.NoError(t, conn.Close(), "close connection")
	})

	chainClient := pbchain.NewChainServiceClient(conn)
	assetClient := pbasset.NewAssetServiceClient(conn)

	t.Run("Unauthenticated", func(t *testing.T) {
		chains, err := chainClient.ListChains(t.Context(), &pbchain.ListChainsRequest{})
		require.Error(t, err, "list chains")
		require.Equal(t, codes.Unauthenticated, status.Code(err), "error code should be unauthenticated")
		require.Nil(t, chains, "chains should be nil")
	})

	t.Run("ListChains", func(t *testing.T) {
		ctx := testingapi.ContextWithApiKey(t.Context(), "automation")

		chains, err := chainClient.ListChains(ctx, &pbchain.ListChainsRequest{})
		require.NoError(t, err, "list chains")
		require.NotEmpty(t, chains, "chains")

		require.Equal(t, 10, len(chains.Chains), "should have 10 chains")
		require.Equal(t, pbblockchain.Chain_CHAIN_ANY_BTC, chains.Chains[0].Id, "chain id should match")
	})

	t.Run("ListAssets", func(t *testing.T) {
		ctx := testingapi.ContextWithApiKey(t.Context(), "automation")

		assets, err := assetClient.ListAssets(ctx, &pbasset.ListAssetsRequest{ChainId: pbblockchain.Chain_CHAIN_ANY})
		require.NoError(t, err, "list assets")
		require.NotEmpty(t, assets, "assets")
		require.Equal(t, 1, len(assets.Assets), "should have 1 asset")
		require.Equal(t, "01K40YW14CPAY0N0CHA0N0SDT0", assets.Assets[0].Id, "asset id should match")
	})

	t.Run("GetAssetPrice", func(t *testing.T) {
		ctx := testingapi.ContextWithApiKey(t.Context(), "automation")

		price, err := assetClient.GetAssetPrice(ctx, &pbasset.GetAssetPriceRequest{AssetId: "01K40YW14CPAY0N0CHA0N0SDT0"})
		require.NoError(t, err, "get asset price")
		require.Equal(t, "1", price.Price, "price should match")
	})
}
