package workers

import "testing"

func TestEventAllowed(t *testing.T) {
	if !eventAllowed([]byte(`["*"]`), "payment.confirmed") {
		t.Fatalf("wildcard should allow")
	}
	if !eventAllowed([]byte(`["payment.confirmed"]`), "payment.confirmed") {
		t.Fatalf("exact match should allow")
	}
	if eventAllowed([]byte(`["payment.detected"]`), "payment.confirmed") {
		t.Fatalf("different event should not allow")
	}
}
