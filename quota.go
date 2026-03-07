package billing

import (
	"context"
	"errors"
	"strconv"

	"github.com/redis/go-redis/v9"
)

type QuotaStatus struct {
	Metric      Metric
	Used        int64
	Reserved    int64
	Limit       int64
	Remaining   int64
	Exceeded    bool
	CanOverage  bool
	Enforcement EnforcementMode
}

func (c *Client) Check(ctx context.Context, tenantID string, metric Metric) (QuotaStatus, error) {
	if !metric.Valid() {
		return QuotaStatus{}, ErrInvalidMetric
	}

	sub, err := c.store.GetSubscription(ctx, tenantID)
	if err != nil {
		return QuotaStatus{}, err
	}

	period := CurrentPeriod(sub.StartedAt, c.opts.Now())
	uKey := usageUsedKey(tenantID, period.Key(), metric)
	rKey := usageReservedKey(tenantID, period.Key(), metric)
	qKey := quotaKey(tenantID, metric)
	oKey := canOverageKey(tenantID, metric)
	eKey := enforcementKey(tenantID, metric)

	pipe := c.redis.Pipeline()
	usageCmd := pipe.Get(ctx, uKey)
	reservedCmd := pipe.Get(ctx, rKey)
	quotaCmd := pipe.Get(ctx, qKey)
	overageCmd := pipe.Get(ctx, oKey)
	enforcementCmd := pipe.Get(ctx, eKey)
	_, _ = pipe.Exec(ctx)

	used := getInt64OrZero(usageCmd)
	reserved := getInt64OrZero(reservedCmd)
	limit := getInt64OrZero(quotaCmd)
	canOverage := getBoolOrFalse(overageCmd)
	enforcement := getEnforcementOrDefault(enforcementCmd)

	remaining := limit - used - reserved
	if remaining < 0 {
		remaining = 0
	}

	return QuotaStatus{
		Metric:      metric,
		Used:        used,
		Reserved:    reserved,
		Limit:       limit,
		Remaining:   remaining,
		Exceeded:    used+reserved >= limit,
		CanOverage:  canOverage,
		Enforcement: enforcement,
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
		if errors.Is(err, redis.Nil) {
			return 0
		}
		return 0
	}
	return val
}

func getBoolOrFalse(cmd *redis.StringCmd) bool {
	val, err := cmd.Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return false
		}
		return false
	}
	return val == "1" || val == "true"
}

func getEnforcementOrDefault(cmd *redis.StringCmd) EnforcementMode {
	val, err := cmd.Result()
	if err != nil {
		return EnforcementHardCap
	}
	mode := EnforcementMode(val)
	if !mode.Valid() {
		return EnforcementHardCap
	}
	return mode
}

func int64ToStr(v int64) string {
	return strconv.FormatInt(v, 10)
}

func (c *Client) CanConsume(ctx context.Context, tenantID string, metric Metric, amount int64) (QuotaAdmission, error) {
	if amount <= 0 {
		return QuotaAdmission{}, ErrInvalidAmount
	}
	status, err := c.Check(ctx, tenantID, metric)
	if err != nil {
		return QuotaAdmission{}, err
	}
	admission := QuotaAdmission{
		Metric:      metric,
		Requested:   amount,
		Used:        status.Used,
		Reserved:    status.Reserved,
		Limit:       status.Limit,
		Remaining:   status.Remaining,
		CanOverage:  status.CanOverage,
		Enforcement: status.Enforcement,
		CheckedAt:   c.opts.Now(),
	}

	if status.Enforcement == EnforcementSoftCap {
		admission.Allowed = true
		return admission, nil
	}

	if status.Used+status.Reserved+amount > status.Limit {
		admission.Allowed = false
		admission.Reason = "quota_exceeded"
		return admission, nil
	}
	admission.Allowed = true
	return admission, nil
}
