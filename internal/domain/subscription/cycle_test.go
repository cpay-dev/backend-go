package subscription

import (
	"testing"
	"time"
)

func TestEvaluateCycle(t *testing.T) {
	status, subStatus, evt := EvaluateCycle(true, 0, 3)
	if status != "paid" || subStatus != "active" || evt != "subscription.debit_succeeded" {
		t.Fatalf("unexpected paid result")
	}
	status, subStatus, evt = EvaluateCycle(false, 5, 3)
	if status != "failed" || subStatus != "past_due" || evt != "subscription.cycle_due" {
		t.Fatalf("unexpected failed result")
	}
}

func TestAddInterval(t *testing.T) {
	start := time.Date(2026, 1, 31, 12, 0, 0, 0, time.UTC)

	if got := AddInterval(start, "day", 2); !got.Equal(start.Add(48 * time.Hour)) {
		t.Fatalf("unexpected day interval: %s", got)
	}
	if got := AddInterval(start, " week ", 2); !got.Equal(start.Add(14 * 24 * time.Hour)) {
		t.Fatalf("unexpected week interval: %s", got)
	}
	if got := AddInterval(start, "month", 1); !got.Equal(start.AddDate(0, 1, 0)) {
		t.Fatalf("unexpected month interval: %s", got)
	}
	if got := AddInterval(start, "month", 0); !got.Equal(start.AddDate(0, 1, 0)) {
		t.Fatalf("expected non-positive count to default to one month: %s", got)
	}
}
