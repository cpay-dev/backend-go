package asset_test

import (
	"testing"

	apiasset "github.com/cpay-dev/backend-go/internal/api/asset"
	"github.com/cpay-dev/backend-go/internal/api/grpc/merchant/asset"
	testingapi "github.com/cpay-dev/backend-go/internal/testing/api"
	pbasset "github.com/cpay-dev/proto-go/api/v1/merchant/asset"
	pbblockchain "github.com/cpay-dev/proto-go/blockchain/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestListAssets(t *testing.T) {
	t.Parallel()

	repos := testingapi.SetupRepos(t,
		testingapi.WithBlockchainRepo(),
		testingapi.WithSeedBlockchain(),
	)
	service := asset.NewService(repos.Blockchain, apiasset.NewPriceService(repos.Blockchain))

	for _, expected := range []struct {
		Chain   pbblockchain.Chain
		Assets  []string
		ErrCode codes.Code
	}{
		{Chain: pbblockchain.Chain_CHAIN_ANY_BTC, Assets: nil, ErrCode: codes.NotFound},
		{Chain: pbblockchain.Chain_CHAIN_ANY_EVM, Assets: nil, ErrCode: codes.NotFound},
		{Chain: pbblockchain.Chain_CHAIN_ANY_SVM, Assets: nil, ErrCode: codes.NotFound},
		{Chain: pbblockchain.Chain_CHAIN_ANY, Assets: []string{"01K40YW14CPAY0N0CHA0N0SDT0"}},
		{Chain: pbblockchain.Chain_CHAIN_BTC_BITCOIN, Assets: nil},
		{Chain: pbblockchain.Chain_CHAIN_EVM_UNICHAIN, Assets: []string{"01K40YW14CPAY0N0CHA0N0SDT0"}},
	} {
		resp, err := service.ListAssets(t.Context(), &pbasset.ListAssetsRequest{ChainId: expected.Chain})
		if expected.ErrCode == codes.OK {
			require.NoError(t, err, "list assets")
			require.NotNil(t, resp, "response should not be nil")
			require.Equal(t, len(expected.Assets), len(resp.Assets), "assets length should match")
			for i, expectedAsset := range expected.Assets {
				if expected.Chain != pbblockchain.Chain_CHAIN_ANY {
					require.Equal(t, expected.Chain, resp.Assets[i].Chain, "chain should match")
				}
				require.Equal(t, expectedAsset, resp.Assets[i].Id, "asset id should match")
			}
		} else {
			require.Error(t, err, "list assets")
			require.Equal(t, expected.ErrCode, status.Code(err), "error code should match")
			require.Nil(t, resp, "response should be nil")
		}
	}
}
