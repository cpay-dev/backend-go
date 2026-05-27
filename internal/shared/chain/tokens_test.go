package chain

import "testing"

func TestKnownEVMTokenContractUsesCanonicalChainAliases(t *testing.T) {
	contract, ok := KnownEVMTokenContract("BNB Smart Chain", "USDT")
	if !ok {
		t.Fatalf("expected BNB Smart Chain USDT contract")
	}
	if contract.Address != "0x55d398326f99059fF775485246999027B3197955" || contract.Decimals != 18 {
		t.Fatalf("unexpected BNB Smart Chain USDT contract: %+v", contract)
	}

	contract, ok = KnownEVMTokenContract("Avalanche C-Chain", "USDC")
	if !ok {
		t.Fatalf("expected Avalanche C-Chain USDC contract")
	}
	if contract.Address != "0xB97EF9Ef8734C71904D8002F8b6Bc66Dd9c48a6E" || contract.Decimals != 6 {
		t.Fatalf("unexpected Avalanche C-Chain USDC contract: %+v", contract)
	}
}

func TestEVMChainIDUsesCanonicalAliases(t *testing.T) {
	cases := map[string]string{
		"Ethereum Mainnet": "0x1",
		"BNB Smart Chain":  "0x38",
		"Arbitrum One":     "0xa4b1",
		"avax":             "0xa86a",
		"hyperliquid evm":  "0x3e7",
	}
	for raw, want := range cases {
		got, ok := EVMChainID(raw)
		if !ok || got != want {
			t.Fatalf("EVMChainID(%q) = %q, %v; want %q, true", raw, got, ok, want)
		}
	}
	if _, ok := EVMChainID("solana"); ok {
		t.Fatalf("expected solana to have no EVM chain ID")
	}
}

func TestNativeEVMTokenDecimalsUsesCanonicalAliases(t *testing.T) {
	cases := []struct {
		chain  string
		symbol string
	}{
		{chain: "Ethereum Mainnet", symbol: "ETH"},
		{chain: "BNB Smart Chain", symbol: "BNB"},
		{chain: "polygon", symbol: "pol"},
		{chain: "avax", symbol: "AVAX"},
		{chain: "hyperliquid evm", symbol: "HYPE"},
	}
	for _, tc := range cases {
		got, ok := NativeEVMTokenDecimals(tc.chain, tc.symbol)
		if !ok || got != 18 {
			t.Fatalf("NativeEVMTokenDecimals(%q, %q) = %d, %v; want 18, true", tc.chain, tc.symbol, got, ok)
		}
	}
	if _, ok := NativeEVMTokenDecimals("polygon", "ETH"); ok {
		t.Fatalf("expected polygon ETH to be non-native")
	}
}
