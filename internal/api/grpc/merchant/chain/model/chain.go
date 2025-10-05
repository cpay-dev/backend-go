package model

import (
	"fmt"

	blockchainmodel "github.com/cpay-dev/backend-go/internal/api/repo/pg/blockchain/model"
	pbblockchain "github.com/cpay-dev/proto-go/blockchain/v1"
)

func ChainToRepo(chain pbblockchain.Chain) (blockchainmodel.Chain, error) {
	switch chain {
	case pbblockchain.Chain_CHAIN_ANY:
		return blockchainmodel.ChainAny, nil
	case pbblockchain.Chain_CHAIN_ANY_BTC:
		return blockchainmodel.ChainAnyBitcoin, nil
	case pbblockchain.Chain_CHAIN_ANY_EVM:
		return blockchainmodel.ChainAnyEVM, nil
	case pbblockchain.Chain_CHAIN_ANY_SVM:
		return blockchainmodel.ChainAnySVM, nil
	case pbblockchain.Chain_CHAIN_BTC_BITCOIN:
		return blockchainmodel.ChainBitcoin, nil
	case pbblockchain.Chain_CHAIN_EVM_ETHEREUM:
		return blockchainmodel.ChainEthereum, nil
	case pbblockchain.Chain_CHAIN_EVM_ARBITRUM:
		return blockchainmodel.ChainArbitrum, nil
	case pbblockchain.Chain_CHAIN_EVM_POLYGON:
		return blockchainmodel.ChainPolygon, nil
	case pbblockchain.Chain_CHAIN_EVM_UNICHAIN:
		return blockchainmodel.ChainUnchain, nil
	case pbblockchain.Chain_CHAIN_SVM_SOLANA:
		return blockchainmodel.ChainSolana, nil
	default:
		return "", fmt.Errorf("invalid chain: %s", chain)
	}
}

func ChainToProto(chain blockchainmodel.Chain) (pbblockchain.Chain, error) {
	switch chain {
	case blockchainmodel.ChainAny:
		return pbblockchain.Chain_CHAIN_ANY, nil
	case blockchainmodel.ChainAnyBitcoin:
		return pbblockchain.Chain_CHAIN_ANY_BTC, nil
	case blockchainmodel.ChainAnyEVM:
		return pbblockchain.Chain_CHAIN_ANY_EVM, nil
	case blockchainmodel.ChainAnySVM:
		return pbblockchain.Chain_CHAIN_ANY_SVM, nil
	case blockchainmodel.ChainBitcoin:
		return pbblockchain.Chain_CHAIN_BTC_BITCOIN, nil
	case blockchainmodel.ChainEthereum:
		return pbblockchain.Chain_CHAIN_EVM_ETHEREUM, nil
	case blockchainmodel.ChainArbitrum:
		return pbblockchain.Chain_CHAIN_EVM_ARBITRUM, nil
	case blockchainmodel.ChainPolygon:
		return pbblockchain.Chain_CHAIN_EVM_POLYGON, nil
	case blockchainmodel.ChainUnchain:
		return pbblockchain.Chain_CHAIN_EVM_UNICHAIN, nil
	case blockchainmodel.ChainSolana:
		return pbblockchain.Chain_CHAIN_SVM_SOLANA, nil
	default:
		return pbblockchain.Chain_CHAIN_UNSPECIFIED, fmt.Errorf("invalid chain: %s", chain)
	}
}
