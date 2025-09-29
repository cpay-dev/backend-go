package model

import (
	"fmt"

	"github.com/cpay-dev/backend-go/internal/api/repo/pg/blockchain/model"
	pbblockchain "github.com/cpay-dev/proto-go/blockchain/v1"
)

func ChainToRepo(chain pbblockchain.Chain) (model.Chain, error) {
	switch chain {
	case pbblockchain.Chain_CHAIN_ANY:
		return model.ChainAny, nil
	case pbblockchain.Chain_CHAIN_ANY_BTC:
		return model.ChainAnyBitcoin, nil
	case pbblockchain.Chain_CHAIN_ANY_EVM:
		return model.ChainAnyEVM, nil
	case pbblockchain.Chain_CHAIN_ANY_SVM:
		return model.ChainAnySVM, nil
	case pbblockchain.Chain_CHAIN_BTC_BITCOIN:
		return model.ChainBitcoin, nil
	case pbblockchain.Chain_CHAIN_EVM_ETHEREUM:
		return model.ChainEthereum, nil
	case pbblockchain.Chain_CHAIN_EVM_ARBITRUM:
		return model.ChainArbitrum, nil
	case pbblockchain.Chain_CHAIN_EVM_POLYGON:
		return model.ChainPolygon, nil
	case pbblockchain.Chain_CHAIN_EVM_UNICHAIN:
		return model.ChainUnchain, nil
	case pbblockchain.Chain_CHAIN_SVM_SOLANA:
		return model.ChainSolana, nil
	default:
		return "", fmt.Errorf("invalid chain: %s", chain)
	}
}

func ChainToProto(chain model.Chain) (pbblockchain.Chain, error) {
	switch chain {
	case model.ChainAny:
		return pbblockchain.Chain_CHAIN_ANY, nil
	case model.ChainAnyBitcoin:
		return pbblockchain.Chain_CHAIN_ANY_BTC, nil
	case model.ChainAnyEVM:
		return pbblockchain.Chain_CHAIN_ANY_EVM, nil
	case model.ChainAnySVM:
		return pbblockchain.Chain_CHAIN_ANY_SVM, nil
	case model.ChainBitcoin:
		return pbblockchain.Chain_CHAIN_BTC_BITCOIN, nil
	case model.ChainEthereum:
		return pbblockchain.Chain_CHAIN_EVM_ETHEREUM, nil
	case model.ChainArbitrum:
		return pbblockchain.Chain_CHAIN_EVM_ARBITRUM, nil
	case model.ChainPolygon:
		return pbblockchain.Chain_CHAIN_EVM_POLYGON, nil
	case model.ChainUnchain:
		return pbblockchain.Chain_CHAIN_EVM_UNICHAIN, nil
	case model.ChainSolana:
		return pbblockchain.Chain_CHAIN_SVM_SOLANA, nil
	default:
		return pbblockchain.Chain_CHAIN_UNSPECIFIED, fmt.Errorf("invalid chain: %s", chain)
	}
}
