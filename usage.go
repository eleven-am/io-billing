package billing

import (
	"context"
	"errors"
	"strconv"

	"github.com/redis/go-redis/v9"
)

const actionIncrement = "increment"

func (c *Client) Increment(ctx context.Context, req IncrementRequest) error {
	if err := validateTenantID(req.TenantID); err != nil {
		return err
	}
	if !req.Metric.Valid() {
		return ErrInvalidMetric
	}
	if req.Amount <= 0 {
		return ErrInvalidAmount
	}
	if err := validateOperationID(req.OperationID); err != nil {
		return err
	}

	sub, plan, dim, period, err := c.loadContext(ctx, req.TenantID, req.Metric)
	if err != nil {
		return err
	}

	opValue := encodeOpValue(req.OperationID, req.Amount)
	result, err := incrementScript.Run(ctx, c.redis, []string{
		usageUsedKey(req.TenantID, period.Key(), req.Metric),
		quotaKey(req.TenantID, req.Metric),
		enforcementKey(req.TenantID, req.Metric),
		operationKey(req.TenantID, actionIncrement, req.OperationID),
	}, req.Amount, opValue, int(c.opts.OperationTTL.Seconds())).Result()
	if err != nil {
		return err
	}

	status, payload, used, limit, err := parseIncrementScriptResult(result)
	if err != nil {
		return err
	}

	switch status {
	case 0:
		return &QuotaExceededError{
			Metric: req.Metric,
			Used:   used,
			Limit:  limit,
		}
	case -1:
		return ErrInvalidAmount
	case 1, 2:
		if payload != opValue {
			return ErrOperationConflict
		}
	default:
		return ErrOperationConflict
	}

	entry := LedgerEntry{
		TenantID:            req.TenantID,
		SubscriptionID:      sub.ID,
		PlanID:              plan.ID,
		Metric:              req.Metric,
		Action:              actionIncrement,
		OperationID:         req.OperationID,
		PeriodStart:         period.Key(),
		PeriodEnd:           period.End.Format("2006-01-02"),
		Units:               req.Amount,
		ReservedUnits:       0,
		IncludedSnapshot:    dim.Included,
		OverageRateSnapshot: dim.OverageRate,
		Unit:                dim.Unit,
		Metadata:            map[string]any{"idempotent": status == 2},
	}
	if err := c.store.CreateLedgerEntry(ctx, entry); err != nil {
		return err
	}
	return nil
}

func (c *Client) GetUsage(ctx context.Context, tenantID string, metric Metric) (int64, error) {
	if err := validateTenantID(tenantID); err != nil {
		return 0, err
	}
	if !metric.Valid() {
		return 0, ErrInvalidMetric
	}

	sub, err := c.store.GetSubscription(ctx, tenantID)
	if err != nil {
		return 0, err
	}

	period := CurrentPeriod(sub.StartedAt, c.opts.Now())
	key := usageUsedKey(tenantID, period.Key(), metric)

	val, err := c.redis.Get(ctx, key).Int64()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	return val, err
}

func (c *Client) GetAllUsage(ctx context.Context, tenantID string) (map[Metric]int64, error) {
	if err := validateTenantID(tenantID); err != nil {
		return nil, err
	}
	result := make(map[Metric]int64, len(AllMetrics))
	for _, m := range AllMetrics {
		val, err := c.GetUsage(ctx, tenantID, m)
		if err != nil {
			return nil, err
		}
		result[m] = val
	}
	return result, nil
}

func parseIncrementScriptResult(result any) (status int64, payload string, used int64, limit int64, err error) {
	values, ok := result.([]any)
	if !ok || len(values) < 4 {
		return 0, "", 0, 0, ErrOperationConflict
	}

	status, err = toInt64(values[0])
	if err != nil {
		return 0, "", 0, 0, err
	}
	payload = toString(values[1])
	used, err = toInt64(values[2])
	if err != nil {
		return 0, "", 0, 0, err
	}
	limit, err = toInt64(values[3])
	if err != nil {
		return 0, "", 0, 0, err
	}
	return status, payload, used, limit, nil
}

func toInt64(v any) (int64, error) {
	switch t := v.(type) {
	case int64:
		return t, nil
	case string:
		return strconv.ParseInt(t, 10, 64)
	case []byte:
		return strconv.ParseInt(string(t), 10, 64)
	default:
		return 0, ErrOperationConflict
	}
}

func toString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []byte:
		return string(t)
	default:
		return ""
	}
}
