package model

type Chain string

const (
	ChainAny        Chain = "CHAIN_ANY"
	ChainAnyBitcoin Chain = "ANY_BTC"
	ChainAnyEVM     Chain = "ANY_EVM"
	ChainAnySVM     Chain = "ANY_SVM"

	ChainBitcoin  Chain = "BTC"
	ChainEthereum Chain = "ETH"
	ChainArbitrum Chain = "ARB"
	ChainPolygon  Chain = "POLYGON"
	ChainUnchain  Chain = "UNI"
	ChainSolana   Chain = "SOL"
)
