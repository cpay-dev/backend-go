package payment

import (
	"math"
	"strings"
)

func ComputeBounds(expectedAmount, tolerancePercent float64) (minAmount, maxAmount float64) {
	if expectedAmount < 0 {
		expectedAmount = 0
	}
	if tolerancePercent < 0 {
		tolerancePercent = 0
	}
	multiplier := tolerancePercent / 100
	minAmount = expectedAmount * (1 - multiplier)
	maxAmount = expectedAmount * (1 + multiplier)
	if minAmount < 0 {
		minAmount = 0
	}
	return round8(minAmount), round8(maxAmount)
}

func ResolveIntentStatus(expectedAmount, receivedAmount, tolerancePercent float64, confirmations, requiredConfirmations int) string {
	if receivedAmount <= 0 {
		return "awaiting_funds"
	}
	minAmount, maxAmount := ComputeBounds(expectedAmount, tolerancePercent)
	if confirmations < requiredConfirmations {
		if receivedAmount < minAmount {
			return "partial"
		}
		if receivedAmount > maxAmount {
			return "overpaid"
		}
		return "awaiting_funds"
	}
	if receivedAmount < minAmount {
		return "partial"
	}
	if receivedAmount > maxAmount {
		return "overpaid"
	}
	return "confirmed"
}

func IntentStatusIsPaid(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "confirmed", "overpaid", "settled":
		return true
	default:
		return false
	}
}

func round8(v float64) float64 {
	return math.Round(v*1e8) / 1e8
}
