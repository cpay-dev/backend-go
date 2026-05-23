package checkoutsvc

import (
	"context"
	"testing"
)

func TestSettlementAmountForUSDStableTokenUsesFiatAmount(t *testing.T) {
	got, err := settlementAmountForToken(context.Background(), 0.01, "USD", "hyperevm", "USDT")
	if err != nil {
		t.Fatalf("settlement amount: %v", err)
	}
	if got != 0.01 {
		t.Fatalf("expected 0.01, got %v", got)
	}
}

func TestSettlementAmountForUnsupportedFiatCurrencyFails(t *testing.T) {
	if _, err := settlementAmountForToken(context.Background(), 0.01, "EUR", "hyperevm", "USDT"); err == nil {
		t.Fatalf("expected unsupported currency error")
	}
}
