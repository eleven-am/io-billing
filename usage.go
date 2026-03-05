package billing

import (
	"context"
	"time"
)

func (c *Client) Increment(ctx context.Context, tenantID string, metric Metric, amount int64) error {
	if !metric.Valid() {
		return ErrInvalidMetric
	}

	sub, err := c.store.GetSubscription(ctx, tenantID)
	if err != nil {
		return err
	}

	period := CurrentPeriod(sub.StartedAt, time.Now())
	key := usageKey(tenantID, period, metric)

	return c.redis.IncrBy(ctx, key, amount).Err()
}

func (c *Client) GetUsage(ctx context.Context, tenantID string, metric Metric) (int64, error) {
	if !metric.Valid() {
		return 0, ErrInvalidMetric
	}

	sub, err := c.store.GetSubscription(ctx, tenantID)
	if err != nil {
		return 0, err
	}

	period := CurrentPeriod(sub.StartedAt, time.Now())
	key := usageKey(tenantID, period, metric)

	val, err := c.redis.Get(ctx, key).Int64()
	if err != nil && err.Error() == "redis: nil" {
		return 0, nil
	}
	return val, err
}

func (c *Client) GetAllUsage(ctx context.Context, tenantID string) (map[Metric]int64, error) {
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
