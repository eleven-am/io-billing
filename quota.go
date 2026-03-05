package billing

import (
	"context"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

type QuotaStatus struct {
	Metric     Metric
	Used       int64
	Limit      int64
	Remaining  int64
	Exceeded   bool
	CanOverage bool
}

func (c *Client) Check(ctx context.Context, tenantID string, metric Metric) (QuotaStatus, error) {
	if !metric.Valid() {
		return QuotaStatus{}, ErrInvalidMetric
	}

	sub, err := c.store.GetSubscription(ctx, tenantID)
	if err != nil {
		return QuotaStatus{}, err
	}

	period := CurrentPeriod(sub.StartedAt, time.Now())
	uKey := usageKey(tenantID, period, metric)
	qKey := quotaKey(tenantID, metric)
	oKey := canOverageKey(tenantID, metric)

	pipe := c.redis.Pipeline()
	usageCmd := pipe.Get(ctx, uKey)
	quotaCmd := pipe.Get(ctx, qKey)
	overageCmd := pipe.Get(ctx, oKey)
	_, _ = pipe.Exec(ctx)

	used := getInt64OrZero(usageCmd)
	limit := getInt64OrZero(quotaCmd)
	canOverage := getBoolOrFalse(overageCmd)

	remaining := limit - used
	if remaining < 0 {
		remaining = 0
	}

	return QuotaStatus{
		Metric:     metric,
		Used:       used,
		Limit:      limit,
		Remaining:  remaining,
		Exceeded:   used >= limit,
		CanOverage: canOverage,
	}, nil
}

func (c *Client) CheckMultiple(ctx context.Context, tenantID string, metrics []Metric) (map[Metric]QuotaStatus, error) {
	result := make(map[Metric]QuotaStatus, len(metrics))
	for _, m := range metrics {
		status, err := c.Check(ctx, tenantID, m)
		if err != nil {
			return nil, err
		}
		result[m] = status
	}
	return result, nil
}

func (c *Client) SetQuota(ctx context.Context, tenantID string, metric Metric, limit int64) error {
	if !metric.Valid() {
		return ErrInvalidMetric
	}
	key := quotaKey(tenantID, metric)
	return c.redis.Set(ctx, key, limit, 0).Err()
}

func (c *Client) SetQuotas(ctx context.Context, tenantID string, quotas map[Metric]int64) error {
	pipe := c.redis.Pipeline()
	for metric, limit := range quotas {
		if !metric.Valid() {
			return ErrInvalidMetric
		}
		key := quotaKey(tenantID, metric)
		pipe.Set(ctx, key, limit, 0)
	}
	_, err := pipe.Exec(ctx)
	return err
}

func (c *Client) setCanOverage(ctx context.Context, tenantID string, metric Metric, canOverage bool) error {
	key := canOverageKey(tenantID, metric)
	val := "0"
	if canOverage {
		val = "1"
	}
	return c.redis.Set(ctx, key, val, 0).Err()
}

func getInt64OrZero(cmd *redis.StringCmd) int64 {
	val, err := cmd.Int64()
	if err != nil {
		return 0
	}
	return val
}

func getBoolOrFalse(cmd *redis.StringCmd) bool {
	val, err := cmd.Result()
	if err != nil {
		return false
	}
	return val == "1" || val == "true"
}

func int64ToStr(v int64) string {
	return strconv.FormatInt(v, 10)
}
