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
