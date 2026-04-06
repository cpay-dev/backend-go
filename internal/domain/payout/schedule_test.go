package payout

import (
	"testing"
	"time"
)

func TestNextPayoutWindow(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 10, 0, 0, time.UTC)
	next := NextPayoutWindow(now, 12)
	if !next.After(now) {
		t.Fatalf("expected next after now")
	}
	if next.Hour() != 12 {
		t.Fatalf("expected hour 12, got %d", next.Hour())
	}
}
