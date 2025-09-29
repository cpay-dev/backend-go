package merchant_test

import (
	"testing"

	"github.com/cpay-dev/backend-go/internal/api/grpc/merchant"
	testingapi "github.com/cpay-dev/backend-go/internal/testing/api"
	pbmerchant "github.com/cpay-dev/proto-go/api/v1/merchant"
	pbblockchain "github.com/cpay-dev/proto-go/blockchain/v1"
	"github.com/stretchr/testify/require"
)

func TestListChains(t *testing.T) {
	t.Parallel()

	repos := testingapi.SetupRepos(t,
		testingapi.WithBlockchainRepo(), testingapi.WithSeedBlockchain(),
		testingapi.WithPaymentRepo(),
	)

	service := merchant.NewService(repos.Blockchain, repos.Payment)

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
