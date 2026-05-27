package chain

import "strings"

type TokenContract struct {
	Address  string
	Decimals int
}

var evmTokenContracts = map[string]map[string]TokenContract{
	"ethereum": {
		"USDC": {Address: "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48", Decimals: 6},
		"USDT": {Address: "0xdAC17F958D2ee523a2206206994597C13D831ec7", Decimals: 6},
		"DAI":  {Address: "0x6B175474E89094C44Da98b954EedeAC495271d0F", Decimals: 18},
		"WBTC": {Address: "0x2260FAC5E5542a773Aa44fBCfeDf7C193bc2C599", Decimals: 8},
	},
	"optimism": {
		"USDC": {Address: "0x0b2C639c533813f4Aa9D7837CAF62653d097Ff85", Decimals: 6},
		"USDT": {Address: "0x94b008aA00579c1307B0EF2c499ad98a8ce58e58", Decimals: 6},
		"DAI":  {Address: "0xDA10009cBd5D07dd0CeCc66161FC93D7c9000da1", Decimals: 18},
		"WBTC": {Address: "0x68f180fcCe6836688e9084f035309E29Bf0A2095", Decimals: 8},
		"OP":   {Address: "0x4200000000000000000000000000000000000042", Decimals: 18},
	},
	"bsc": {
		"USDC": {Address: "0x8AC76a51cc950d9822D68b83fE1Ad97B32Cd580d", Decimals: 18},
		"USDT": {Address: "0x55d398326f99059fF775485246999027B3197955", Decimals: 18},
		"DAI":  {Address: "0x1AF3F329e8BE154074D8769D1FFa4eE058B1DBc3", Decimals: 18},
		"ETH":  {Address: "0x2170Ed0880ac9A755fd29B2688956BD959F933F8", Decimals: 18},
		"WBTC": {Address: "0x7130d2A12B9BCbFAe4f2634d864A1Ee1Ce3Ead9c", Decimals: 18},
	},
	"polygon": {
		"USDC": {Address: "0x3c499c542cef5e3811e1192ce70d8cc03d5c3359", Decimals: 6},
		"USDT": {Address: "0xc2132D05D31c914a87C6611C10748AEb04B58e8F", Decimals: 6},
		"DAI":  {Address: "0x8f3Cf7ad23Cd3CaDbD9735AFf958023239c6A063", Decimals: 18},
		"WBTC": {Address: "0x1BFD67037B42Cf73acF2047067bd4F2C47D9BfD6", Decimals: 8},
	},
	"arbitrum": {
		"USDC": {Address: "0xaf88d065e77c8cC2239327C5EDb3A432268e5831", Decimals: 6},
		"USDT": {Address: "0xFd086bC7CD5C481DCC9C85ebE478A1C0b69FCbb9", Decimals: 6},
		"DAI":  {Address: "0xDA10009cBd5D07dd0CeCc66161FC93D7c9000da1", Decimals: 18},
		"WBTC": {Address: "0x2f2a2543B76A4166549F7aaB2e75Bef0aefC5B0f", Decimals: 8},
		"ARB":  {Address: "0x912CE59144191C1204E64559FE8253a0e49E6548", Decimals: 18},
	},
	"base": {
		"USDC": {Address: "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913", Decimals: 6},
		"USDT": {Address: "0xfde4C96c8593536E31f229EA8f37b2ADa2699bb2", Decimals: 6},
		"DAI":  {Address: "0x50c5725949A6F0c72E6C4a641F24049A917DB0Cb", Decimals: 18},
		"WBTC": {Address: "0x0555E30da8f98308EdB960aa94C0Db47230d2B9c", Decimals: 8},
	},
	"avalanche": {
		"USDC": {Address: "0xB97EF9Ef8734C71904D8002F8b6Bc66Dd9c48a6E", Decimals: 6},
		"USDT": {Address: "0xc7198437980c041c805A1EDcbA50c1Ce5db95118", Decimals: 6},
		"DAI":  {Address: "0xd586E7F844cEa2F87f50152665BCbc2C279D8d70", Decimals: 18},
		"WBTC": {Address: "0x50b7545627a5162F82A992c33b87aDc75187B218", Decimals: 8},
	},
	"hyperevm": {
		"USDT": {Address: "0xB8CE59FC3717ada4C02eaDF9682A9e934F625ebb", Decimals: 6},
	},
}

func KnownEVMTokenContract(chainName, symbol string) (TokenContract, bool) {
	bySymbol, ok := evmTokenContracts[NormalizeEVMChain(chainName)]
	if !ok {
		return TokenContract{}, false
	}
	contract, ok := bySymbol[strings.ToUpper(strings.TrimSpace(symbol))]
	return contract, ok
}

func EVMChainID(chainName string) (string, bool) {
	switch NormalizeEVMChain(chainName) {
	case "ethereum":
		return "0x1", true
	case "optimism":
		return "0xa", true
	case "bsc":
		return "0x38", true
	case "polygon":
		return "0x89", true
	case "arbitrum":
		return "0xa4b1", true
	case "base":
		return "0x2105", true
	case "avalanche":
		return "0xa86a", true
	case "hyperevm":
		return "0x3e7", true
	default:
		return "", false
	}
}

func NativeEVMTokenDecimals(chainName, symbol string) (int, bool) {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	switch NormalizeEVMChain(chainName) {
	case "ethereum", "optimism", "arbitrum", "base", "hyperevm":
		return 18, symbol == "ETH" || symbol == "HYPE"
	case "bsc":
		return 18, symbol == "BNB"
	case "polygon":
		return 18, symbol == "MATIC" || symbol == "POL"
	case "avalanche":
		return 18, symbol == "AVAX"
	default:
		return 0, false
	}
}
