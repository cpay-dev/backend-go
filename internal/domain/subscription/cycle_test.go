package subscription

import "testing"

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
