package authsvc

import "testing"

func TestNormalizeSettlementAddress(t *testing.T) {
	got, err := normalizeSettlementAddress("0xde0B295669a9FD93d5F28D9Ec85E40f4cb697BAe")
	if err != nil {
		t.Fatalf("normalizeSettlementAddress returned error: %v", err)
	}
	if got != "0xde0B295669a9FD93d5F28D9Ec85E40f4cb697BAe" {
		t.Fatalf("unexpected checksum address: %s", got)
	}
}

func TestNormalizeSettlementAddressRejectsInvalidAddress(t *testing.T) {
	if _, err := normalizeSettlementAddress("not-a-wallet"); err == nil {
		t.Fatalf("expected invalid address error")
	}
}
