package subscription

func EvaluateCycle(balanceEnough bool, retryCount int, maxRetries int) (cycleStatus, subscriptionStatus, eventType string) {
	if balanceEnough {
		return "paid", "active", "subscription.debit_succeeded"
	}
	if retryCount >= maxRetries {
		return "failed", "past_due", "subscription.cycle_due"
	}
	return "failed", "active", "subscription.cycle_due"
}
