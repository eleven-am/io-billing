package billing

import (
	"errors"
	"fmt"
)

var (
	ErrQuotaExceeded        = errors.New("billing: quota exceeded")
	ErrPlanNotFound         = errors.New("billing: plan not found")
	ErrSubscriptionNotFound = errors.New("billing: subscription not found")
	ErrNoActivePlan         = errors.New("billing: no active plan")
	ErrInvalidMetric        = errors.New("billing: invalid metric")
	ErrReservationNotFound  = errors.New("billing: reservation not found")
)

type QuotaExceededError struct {
	Metric    Metric
	Used      int64
	Limit     int64
	Estimated int64
}

func (e *QuotaExceededError) Error() string {
	if e.Estimated > 0 {
		return fmt.Sprintf("billing: quota exceeded for %s (used: %d, limit: %d, estimated: %d)", e.Metric, e.Used, e.Limit, e.Estimated)
	}
	return fmt.Sprintf("billing: quota exceeded for %s (used: %d, limit: %d)", e.Metric, e.Used, e.Limit)
}

func (e *QuotaExceededError) Unwrap() error {
	return ErrQuotaExceeded
}
