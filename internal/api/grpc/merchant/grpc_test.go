package merchant_test

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/cpay-dev/backend-go/internal/api/authn"
	"github.com/cpay-dev/backend-go/internal/api/grpc/merchant"
	testingapi "github.com/cpay-dev/backend-go/internal/testing/api"
	"github.com/cpay-dev/backend-go/pkg/log"
	pbmerchant "github.com/cpay-dev/proto-go/api/v1/merchant"
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
	)
	server := merchant.NewServer(logger, repos.Blockchain, authn.NewService(repos.App), time.Second*30)
	randomListenAddress := fmt.Sprintf(":%d", rand.Intn(55_535)+10_000)

	go func() {
		err := server.Start(randomListenAddress)
		assert.NoError(t, err, "start server")
		t.Cleanup(server.Stop)
	}()

	conn, err := grpc.NewClient(randomListenAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err, "connect to server")

	conn.Connect()
	t.Cleanup(func() {
		assert.NoError(t, conn.Close(), "close connection")
	})

	merchantClient := pbmerchant.NewMerchantServiceClient(conn)

	t.Run("Unauthenticated", func(t *testing.T) {
		chains, err := merchantClient.ListChains(t.Context(), &pbmerchant.ListChainsRequest{})
		require.Error(t, err, "list chains")
		require.Equal(t, codes.Unauthenticated, status.Code(err), "error code should be unauthenticated")
		require.Nil(t, chains, "chains should be nil")
	})

	t.Run("ListChains", func(t *testing.T) {
		ctx := testingapi.ContextWithApiKey(t.Context(), "automation")

		chains, err := merchantClient.ListChains(ctx, &pbmerchant.ListChainsRequest{})
		require.NoError(t, err, "list chains")
		require.NotEmpty(t, chains, "chains")

		require.Equal(t, 10, len(chains.Chains), "should have 10 chains")
		require.Equal(t, pbblockchain.Chain_CHAIN_ANY_BTC, chains.Chains[0].Id, "chain id should match")
	})

	t.Run("ListAssets", func(t *testing.T) {
		ctx := testingapi.ContextWithApiKey(t.Context(), "automation")

		assets, err := merchantClient.ListAssets(ctx, &pbmerchant.ListAssetsRequest{ChainId: pbblockchain.Chain_CHAIN_ANY})
		require.NoError(t, err, "list assets")
		require.NotEmpty(t, assets, "assets")
		require.Equal(t, 1, len(assets.Assets), "should have 1 asset")
		require.Equal(t, "01K40YW14CPAY0N0CHA0N0SDT0", assets.Assets[0].Id, "asset id should match")
	})
}
