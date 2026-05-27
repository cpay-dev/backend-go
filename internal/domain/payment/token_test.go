package payment

import "testing"

func TestTokenAllowedMatchesChainSymbolAndOptionalAddress(t *testing.T) {
	allowed := []AllowedToken{
		{Chain: "Base", Symbol: "usdc"},
		{Chain: "Polygon", Symbol: "USDT", Address: "0x1234000000000000000000000000000000000000"},
	}

	if !TokenAllowed(nil, "any", "anything", "") {
		t.Fatalf("empty allow list should allow any token")
	}
	if !TokenAllowed(allowed, " base ", "USDC", "") {
		t.Fatalf("expected chain/symbol match without address to be allowed")
	}
	if !TokenAllowed(allowed, "polygon", "usdt", "0x1234000000000000000000000000000000000000") {
		t.Fatalf("expected exact address match to be allowed")
	}
	if TokenAllowed(allowed, "polygon", "usdt", "0x9999000000000000000000000000000000000000") {
		t.Fatalf("expected different address to be rejected")
	}
	if TokenAllowed(allowed, "arbitrum", "USDC", "") {
		t.Fatalf("expected different chain to be rejected")
	}
}
