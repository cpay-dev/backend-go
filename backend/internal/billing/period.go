package billing

import (
	"time"

	"github.com/cpay-dev/backend/internal/db"
)

// NextDueFrom calculates the next due date from a given time based on the billing period.
func NextDueFrom(from time.Time, period db.BillingPeriod) time.Time {
	switch period {
	case db.BillingPeriodDaily:
		return from.Add(24 * time.Hour)
	case db.BillingPeriodWeekly:
		return from.Add(7 * 24 * time.Hour)
	case db.BillingPeriodMonthly:
		return from.AddDate(0, 1, 0)
	case db.BillingPeriodYearly:
		return from.AddDate(1, 0, 0)
	default:
		return from.AddDate(0, 1, 0)
	}
}
