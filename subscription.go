package billing

import (
	"context"
	"time"
)

func (c *Client) Subscribe(ctx context.Context, tenantID, planID, polarCustomerID string) error {
	if err := validateTenantID(tenantID); err != nil {
		return err
	}
	if err := validatePlanID(planID); err != nil {
		return err
	}

	plan, err := c.store.GetPlan(ctx, planID)
	if err != nil {
		return err
	}
	if !plan.Active {
		return ErrNoActivePlan
	}

	now := c.opts.Now()
	period := Period{
		Start: now,
		End:   now.AddDate(0, 1, 0).Add(-time.Nanosecond),
	}

	sub := TenantSubscription{
		ID:                 newSubID(),
		TenantID:           tenantID,
		PlanID:             planID,
		PolarCustomerID:    polarCustomerID,
		Status:             "active",
		StartedAt:          now,
		CurrentPeriodStart: period.Start,
		CurrentPeriodEnd:   period.End,
	}

	if err := c.store.CreateSubscription(ctx, sub); err != nil {
		return err
	}

	if err := c.syncQuotasFromPlan(ctx, tenantID, plan); err != nil {
		_ = c.store.DeleteSubscription(ctx, tenantID)
		return err
	}

	return nil
}

func (c *Client) GetSubscription(ctx context.Context, tenantID string) (TenantSubscription, error) {
	if err := validateTenantID(tenantID); err != nil {
		return TenantSubscription{}, err
	}
	return c.store.GetSubscription(ctx, tenantID)
}

func (c *Client) CancelSubscription(ctx context.Context, tenantID string) error {
	if err := validateTenantID(tenantID); err != nil {
		return err
	}

	sub, err := c.store.GetSubscription(ctx, tenantID)
	if err != nil {
		return err
	}

	sub.Status = "cancelled"
	return c.store.UpdateSubscription(ctx, sub)
}

func (c *Client) RenewPeriod(ctx context.Context, tenantID string) error {
	if err := validateTenantID(tenantID); err != nil {
		return err
	}

	sub, err := c.store.GetSubscription(ctx, tenantID)
	if err != nil {
		return err
	}

	plan, err := c.store.GetPlan(ctx, sub.PlanID)
	if err != nil {
		return err
	}

	now := c.opts.Now()
	newPeriod := CurrentPeriod(sub.StartedAt, now)

	sub.CurrentPeriodStart = newPeriod.Start
	sub.CurrentPeriodEnd = newPeriod.End

	if err := c.store.UpdateSubscription(ctx, sub); err != nil {
		return err
	}

	if err := c.resetUsageCounters(ctx, tenantID, newPeriod); err != nil {
		return err
	}

	return c.syncQuotasFromPlan(ctx, tenantID, plan)
}

func (c *Client) syncQuotasFromPlan(ctx context.Context, tenantID string, plan Plan) error {
	pipe := c.redis.Pipeline()
	for metric, dim := range plan.Dimensions {
		qKey := quotaKey(tenantID, metric)
		pipe.Set(ctx, qKey, dim.Included, 0)

		oKey := canOverageKey(tenantID, metric)
		canOverage := "0"
		if dim.Enforcement == EnforcementSoftCap {
			canOverage = "1"
		}
		pipe.Set(ctx, oKey, canOverage, 0)

		eKey := enforcementKey(tenantID, metric)
		pipe.Set(ctx, eKey, string(dim.Enforcement), 0)
	}
	_, err := pipe.Exec(ctx)
	return err
}

func (c *Client) resetUsageCounters(ctx context.Context, tenantID string, period Period) error {
	pipe := c.redis.Pipeline()
	for _, metric := range AllMetrics {
		usedKey := usageUsedKey(tenantID, period.Key(), metric)
		reservedKey := usageReservedKey(tenantID, period.Key(), metric)
		pipe.Set(ctx, usedKey, 0, 0)
		pipe.Set(ctx, reservedKey, 0, 0)
	}
	_, err := pipe.Exec(ctx)
	return err
}
