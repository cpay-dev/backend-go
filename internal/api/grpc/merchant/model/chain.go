package model

import (
	"fmt"

	"github.com/cpay-dev/backend-go/internal/api/repo/pg/blockchain/model"
	pbblockchain "github.com/cpay-dev/proto-go/api/v1/blockchain"
)

func ChainIdToRepo(chain pbblockchain.ChainID) (model.Chain, error) {
	switch chain {
	case pbblockchain.ChainID_CHAIN_ID_ANY:
		return model.ChainAny, nil
	case pbblockchain.ChainID_CHAIN_ID_ANY_BTC:
		return model.ChainAnyBitcoin, nil
	case pbblockchain.ChainID_CHAIN_ID_ANY_EVM:
		return model.ChainAnyEVM, nil
	case pbblockchain.ChainID_CHAIN_ID_ANY_SVM:
		return model.ChainAnySVM, nil
	case pbblockchain.ChainID_CHAIN_ID_BTC_BITCOIN:
		return model.ChainBitcoin, nil
	case pbblockchain.ChainID_CHAIN_ID_EVM_ETHEREUM:
		return model.ChainEthereum, nil
	case pbblockchain.ChainID_CHAIN_ID_EVM_ARBITRUM:
		return model.ChainArbitrum, nil
	case pbblockchain.ChainID_CHAIN_ID_EVM_POLYGON:
		return model.ChainPolygon, nil
	case pbblockchain.ChainID_CHAIN_ID_EVM_UNICHAIN:
		return model.ChainUnchain, nil
	case pbblockchain.ChainID_CHAIN_ID_SVM_SOLANA:
		return model.ChainSolana, nil
	default:
		return "", fmt.Errorf("invalid chain: %s", chain)
	}
}

func ChainToProto(chain model.Chain) (pbblockchain.ChainID, error) {
	switch chain {
	case model.ChainAny:
		return pbblockchain.ChainID_CHAIN_ID_ANY, nil
	case model.ChainAnyBitcoin:
		return pbblockchain.ChainID_CHAIN_ID_ANY_BTC, nil
	case model.ChainAnyEVM:
		return pbblockchain.ChainID_CHAIN_ID_ANY_EVM, nil
	case model.ChainAnySVM:
		return pbblockchain.ChainID_CHAIN_ID_ANY_SVM, nil
	case model.ChainBitcoin:
		return pbblockchain.ChainID_CHAIN_ID_BTC_BITCOIN, nil
	case model.ChainEthereum:
		return pbblockchain.ChainID_CHAIN_ID_EVM_ETHEREUM, nil
	case model.ChainArbitrum:
		return pbblockchain.ChainID_CHAIN_ID_EVM_ARBITRUM, nil
	case model.ChainPolygon:
		return pbblockchain.ChainID_CHAIN_ID_EVM_POLYGON, nil
	case model.ChainUnchain:
		return pbblockchain.ChainID_CHAIN_ID_EVM_UNICHAIN, nil
	case model.ChainSolana:
		return pbblockchain.ChainID_CHAIN_ID_SVM_SOLANA, nil
	default:
		return pbblockchain.ChainID_CHAIN_ID_UNSPECIFIED, fmt.Errorf("invalid chain: %s", chain)
	}
}
