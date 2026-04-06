package payment

import "testing"

func TestValidateLinkInput(t *testing.T) {
	amount := 10.0
	err := ValidateLinkInput(LinkInput{
		Title:            "Test",
		PricingMode:      "fixed",
		Amount:           &amount,
		Currency:         "USD",
		AllowedTokens:    1,
		AfterPaymentType: "confirmation_page",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}
