package merchant_test

import (
	"testing"

	"github.com/cpay-dev/backend-go/internal/api/grpc/merchant"
	testingapi "github.com/cpay-dev/backend-go/internal/testing/api"
	pbmerchant "github.com/cpay-dev/proto-go/api/v1/merchant"
	pbblockchain "github.com/cpay-dev/proto-go/blockchain/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestListChains(t *testing.T) {
	t.Parallel()

	blockchainRepo := testingapi.SetupBlockchainRepo(t)
	service := merchant.NewService(blockchainRepo)

	resp, err := service.ListChains(t.Context(), &pbmerchant.ListChainsRequest{})
	require.NoError(t, err, "list chains")
	require.NotNil(t, resp, "response should not be nil")
	require.NotEmpty(t, resp.Chains, "response should not be empty")

	for i, expected := range []struct {
		ID   pbblockchain.Chain
		Name string
	}{
		{ID: pbblockchain.Chain_CHAIN_ANY_BTC, Name: "Any Bitcoin chain"},
		{ID: pbblockchain.Chain_CHAIN_ANY, Name: "Any chain"},
		{ID: pbblockchain.Chain_CHAIN_ANY_EVM, Name: "Any EVM chain"},
		{ID: pbblockchain.Chain_CHAIN_ANY_SVM, Name: "Any SVM chain"},

		{ID: pbblockchain.Chain_CHAIN_EVM_ARBITRUM, Name: "Arbitrum"},
		{ID: pbblockchain.Chain_CHAIN_BTC_BITCOIN, Name: "Bitcoin"},
		{ID: pbblockchain.Chain_CHAIN_EVM_ETHEREUM, Name: "Ethereum"},

		{ID: pbblockchain.Chain_CHAIN_EVM_POLYGON, Name: "Polygon"},
		{ID: pbblockchain.Chain_CHAIN_SVM_SOLANA, Name: "Solana"},
		{ID: pbblockchain.Chain_CHAIN_EVM_UNICHAIN, Name: "Unichain"},
	} {
		require.Equal(t, expected.ID, resp.Chains[i].Id, "chain id should match")
		require.Equal(t, expected.Name, resp.Chains[i].Name, "chain name should match")
	}
}

func TestListAssets(t *testing.T) {
	t.Parallel()

	blockchainRepo := testingapi.SetupBlockchainRepo(t)
	service := merchant.NewService(blockchainRepo)

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
		resp, err := service.ListAssets(t.Context(), &pbmerchant.ListAssetsRequest{ChainId: expected.Chain})
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
