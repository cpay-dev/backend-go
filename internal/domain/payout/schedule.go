package payout

import "time"

func NextPayoutWindow(now time.Time, hourUTC int) time.Time {
	if hourUTC < 0 || hourUTC > 23 {
		hourUTC = 0
	}
	t := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), hourUTC, 0, 0, 0, time.UTC)
	if !t.After(now.UTC()) {
		t = t.Add(24 * time.Hour)
	}
	return t
}
