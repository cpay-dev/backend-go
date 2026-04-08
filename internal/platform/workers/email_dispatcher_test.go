package workers

import "testing"

func TestEmailEventSupported(t *testing.T) {
	if !emailEventSupported("payment.confirmed") {
		t.Fatalf("payment.confirmed should be supported")
	}
	if !emailEventSupported("invoice.created") {
		t.Fatalf("invoice.created should be supported")
	}
	if emailEventSupported("payment.detected") {
		t.Fatalf("payment.detected should not be supported")
	}
}

func TestUniqueEmails(t *testing.T) {
	in := []string{"  USER@example.com  ", "user@example.com", "admin@example.com", ""}
	got := uniqueEmails(in)
	if len(got) != 2 {
		t.Fatalf("expected 2 unique emails, got %d", len(got))
	}
	if got[0] != "user@example.com" {
		t.Fatalf("expected first email user@example.com, got %q", got[0])
	}
	if got[1] != "admin@example.com" {
		t.Fatalf("expected second email admin@example.com, got %q", got[1])
	}
}

func TestPaymentIntentIDFromEvent(t *testing.T) {
	raw := []byte(`{"payment_intent_id":"pi_123"}`)
	if got := paymentIntentIDFromEvent(raw); got != "pi_123" {
		t.Fatalf("expected pi_123, got %q", got)
	}
	if got := paymentIntentIDFromEvent([]byte(`{"x":1}`)); got != "" {
		t.Fatalf("expected empty id for payload without key, got %q", got)
	}
}
