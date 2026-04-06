package payment

import "testing"

func TestComputeBounds(t *testing.T) {
	min, max := ComputeBounds(100, 0.25)
	if min != 99.75 || max != 100.25 {
		t.Fatalf("unexpected bounds: %.8f %.8f", min, max)
	}
}

func TestResolveIntentStatus(t *testing.T) {
	status := ResolveIntentStatus(100, 100.1, 0.25, 12, 12)
	if status != "confirmed" {
		t.Fatalf("expected confirmed, got %s", status)
	}
	status = ResolveIntentStatus(100, 98, 0.25, 12, 12)
	if status != "partial" {
		t.Fatalf("expected partial, got %s", status)
	}
}
