package config

import "testing"

func TestParseChainRPCURLs(t *testing.T) {
	got := parseChainRPCURLs("base=https://base.example, polygon=http://polygon.example")
	if got["ethereum"] != "https://ethereum-rpc.publicnode.com" {
		t.Fatalf("unexpected default ethereum rpc: %q", got["ethereum"])
	}
	if got["base"] != "https://base.example" {
		t.Fatalf("unexpected base rpc: %q", got["base"])
	}
	if got["hyperevm"] != "https://rpc.hypurrscan.io" {
		t.Fatalf("unexpected default hyperevm rpc: %q", got["hyperevm"])
	}
	if got["polygon"] != "http://polygon.example" {
		t.Fatalf("unexpected polygon rpc: %q", got["polygon"])
	}
	if got["tron"] != "https://api.trongrid.io" {
		t.Fatalf("unexpected default tron rpc: %q", got["tron"])
	}
	if got["ton"] != "https://toncenter.com/api/v2" {
		t.Fatalf("unexpected default ton rpc: %q", got["ton"])
	}
	if got["solana"] != "https://api.mainnet.solana.com" {
		t.Fatalf("unexpected default solana rpc: %q", got["solana"])
	}
}

func TestEVMChainRPCURLsIncludesDefaultsAndEnvOverrides(t *testing.T) {
	cfg := Config{ChainRPCURLs: parseChainRPCURLs("base=https://base.example,tron=https://tron.example")}

	got := cfg.EVMChainRPCURLs()
	if got["ethereum"] != "https://ethereum-rpc.publicnode.com" {
		t.Fatalf("expected ethereum default rpc, got %q", got["ethereum"])
	}
	if got["base"] != "https://base.example" {
		t.Fatalf("expected base rpc, got %q", got["base"])
	}
	if got["hyperevm"] != "https://rpc.hypurrscan.io" {
		t.Fatalf("expected hyperevm default rpc, got %q", got["hyperevm"])
	}
	if _, ok := got["tron"]; ok {
		t.Fatalf("expected tron rpc to be excluded from evm rpc map")
	}
	if _, ok := got["ton"]; ok {
		t.Fatalf("expected ton rpc to be excluded from evm rpc map")
	}
	if _, ok := got["solana"]; ok {
		t.Fatalf("expected solana rpc to be excluded from evm rpc map")
	}
}

func TestLoadUsesDefaultRPCURLsWhenEnvIsEmpty(t *testing.T) {
	t.Setenv("CHAIN_RPC_URLS", "")

	cfg := Load("test")
	got := cfg.EVMChainRPCURLs()
	if got["ethereum"] != "https://ethereum-rpc.publicnode.com" {
		t.Fatalf("expected ethereum default rpc, got %q", got["ethereum"])
	}
	if got["base"] != "https://base-rpc.publicnode.com" {
		t.Fatalf("expected base default rpc, got %q", got["base"])
	}
	if got["hyperevm"] != "https://rpc.hypurrscan.io" {
		t.Fatalf("expected hyperevm default rpc, got %q", got["hyperevm"])
	}
}

func TestEVMChainRPCFallbackURLsIncludesHyperEVMDefaults(t *testing.T) {
	cfg := Config{ChainRPCFallbackURLs: parseChainRPCFallbackURLs("tron=https://tron.example")}

	got := cfg.EVMChainRPCFallbackURLs()
	want := []string{"https://hyperliquid.drpc.org", "https://rpc.hyperliquid.xyz/evm"}
	for i, rpcURL := range want {
		if len(got["hyperevm"]) <= i || got["hyperevm"][i] != rpcURL {
			t.Fatalf("expected hyperevm fallback %d to be %q, got %#v", i, rpcURL, got["hyperevm"])
		}
	}
	if _, ok := got["tron"]; ok {
		t.Fatalf("expected tron fallback rpc to be excluded from evm fallback map")
	}
}

func TestValidateChainRPCURLsRequiresURLs(t *testing.T) {
	if err := ValidateChainRPCURLs(nil); err == nil {
		t.Fatalf("expected missing urls error")
	}
}

func TestValidateChainRPCURLsRejectsInvalidURL(t *testing.T) {
	if err := ValidateChainRPCURLs(map[string]string{"base": "not-a-url"}); err == nil {
		t.Fatalf("expected invalid url error")
	}
}

func TestParseCheckoutWalletFactories(t *testing.T) {
	got := parseAddressMap("base=0xde0B295669a9FD93d5F28D9Ec85E40f4cb697BAe,hyperevm=0x90a546a5fb533d4f168846400656b663F12578d6")
	if got["base"] != "0xde0B295669a9FD93d5F28D9Ec85E40f4cb697BAe" {
		t.Fatalf("unexpected base factory: %q", got["base"])
	}
	if got["hyperevm"] != "0x90a546a5fb533d4f168846400656b663F12578d6" {
		t.Fatalf("unexpected hyperevm factory: %q", got["hyperevm"])
	}
}

func TestValidateCheckoutWalletFactoriesRejectsInvalidAddress(t *testing.T) {
	err := ValidateCheckoutWalletFactories(map[string]string{"base": "not-a-wallet"})
	if err == nil {
		t.Fatalf("expected invalid factory error")
	}
}

func TestValidateProductionCreate2PayoutConfigRequiresHotWalletAndFactories(t *testing.T) {
	rpcURLs := map[string]string{"base": "https://base.example"}
	err := ValidateProductionCreate2PayoutConfig(rpcURLs, nil, "")
	if err == nil {
		t.Fatalf("expected missing factories error")
	}
	err = ValidateProductionCreate2PayoutConfig(rpcURLs, map[string]string{"base": "0xde0B295669a9FD93d5F28D9Ec85E40f4cb697BAe"}, "")
	if err == nil {
		t.Fatalf("expected missing hot wallet key error")
	}
	err = ValidateProductionCreate2PayoutConfig(rpcURLs, map[string]string{"polygon": "0xde0B295669a9FD93d5F28D9Ec85E40f4cb697BAe"}, "0xabc")
	if err == nil {
		t.Fatalf("expected missing base factory error")
	}
}

func TestConfirmationForChainUsesExplicitChainDefaultsAndOverrides(t *testing.T) {
	cfg := Config{ChainConfirmations: parseConfirmations("hyperevm:15")}

	if got := cfg.ConfirmationForChain("HyperEVM"); got != 15 {
		t.Fatalf("expected HyperEVM override 15, got %d", got)
	}
	if got := cfg.ConfirmationForChain("Arbitrum One"); got != 4800 {
		t.Fatalf("expected Arbitrum One default 4800, got %d", got)
	}
	if got := cfg.ConfirmationForChain("Base"); got != 600 {
		t.Fatalf("expected Base default 600, got %d", got)
	}
	if got := cfg.ConfirmationForChain("Ethereum Mainnet"); got != 64 {
		t.Fatalf("expected Ethereum Mainnet default 64, got %d", got)
	}
	if got := cfg.ConfirmationForChain("Polygon"); got != 6 {
		t.Fatalf("expected Polygon default 6, got %d", got)
	}
	if got := cfg.ConfirmationForChain("BSC"); got != 6 {
		t.Fatalf("expected BSC default 6, got %d", got)
	}
	if got := cfg.ConfirmationForChain("Tron"); got != 21 {
		t.Fatalf("expected Tron default 21, got %d", got)
	}
	if got := cfg.ConfirmationForChain("unknown-chain"); got != 12 {
		t.Fatalf("expected deterministic unknown-chain fallback 12, got %d", got)
	}
}
