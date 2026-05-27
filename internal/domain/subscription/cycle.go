package subscription

import (
	"strings"
	"time"
)

func EvaluateCycle(balanceEnough bool, retryCount int, maxRetries int) (cycleStatus, subscriptionStatus, eventType string) {
	if balanceEnough {
		return "paid", "active", "subscription.debit_succeeded"
	}
	if retryCount >= maxRetries {
		return "failed", "past_due", "subscription.cycle_due"
	}
	return "failed", "active", "subscription.cycle_due"
}

func AddInterval(t time.Time, unit string, count int) time.Time {
	if count <= 0 {
		count = 1
	}
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "day":
		return t.Add(time.Duration(count) * 24 * time.Hour)
	case "week":
		return t.Add(time.Duration(count*7) * 24 * time.Hour)
	default:
		return t.AddDate(0, count, 0)
	}
}
