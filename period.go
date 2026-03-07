package billing

import (
	"fmt"
	"time"
)

type Period struct {
	Start time.Time
	End   time.Time
}

func (p Period) Key() string {
	return p.Start.UTC().Format("2006-01-02")
}

func CurrentPeriod(subscriptionStart time.Time, now time.Time) Period {
	start := subscriptionStart.UTC()
	now = now.UTC()

	day := start.Day()
	year := now.Year()
	month := now.Month()

	periodStart := adjustedDate(year, month, day)
	if periodStart.After(now) {
		periodStart = adjustedDate(year, month-1, day)
	}

	periodEnd := adjustedDate(periodStart.Year(), periodStart.Month()+1, day).Add(-time.Nanosecond)

	return Period{
		Start: periodStart,
		End:   periodEnd,
	}
}

func adjustedDate(year int, month time.Month, day int) time.Time {
	for day > 0 {
		t := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
		if t.Day() == day {
			return t
		}
		day--
	}
	return time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
}

func usageKey(tenantID string, period Period, metric Metric) string {
	return usageUsedKey(tenantID, period.Key(), metric)
}

func usageUsedKey(tenantID, periodKey string, metric Metric) string {
	return fmt.Sprintf("billing:usage:%s:%s:%s:used", tenantID, periodKey, metric)
}

func usageReservedKey(tenantID, periodKey string, metric Metric) string {
	return fmt.Sprintf("billing:usage:%s:%s:%s:reserved", tenantID, periodKey, metric)
}

func quotaKey(tenantID string, metric Metric) string {
	return fmt.Sprintf("billing:quota:%s:%s", tenantID, metric)
}

func reservationKey(reservationID string) string {
	return fmt.Sprintf("billing:reservation:%s", reservationID)
}

func canOverageKey(tenantID string, metric Metric) string {
	return fmt.Sprintf("billing:can_overage:%s:%s", tenantID, metric)
}

func enforcementKey(tenantID string, metric Metric) string {
	return fmt.Sprintf("billing:enforcement:%s:%s", tenantID, metric)
}

func operationKey(tenantID, action, operationID string) string {
	return fmt.Sprintf("billing:op:%s:%s:%s", tenantID, action, operationID)
}
