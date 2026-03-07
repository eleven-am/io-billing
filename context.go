package billing

import "context"

func (c *Client) loadContext(ctx context.Context, tenantID string, metric Metric) (TenantSubscription, Plan, Dimension, Period, error) {
	if err := validateTenantID(tenantID); err != nil {
		return TenantSubscription{}, Plan{}, Dimension{}, Period{}, err
	}

	sub, err := c.store.GetSubscription(ctx, tenantID)
	if err != nil {
		return TenantSubscription{}, Plan{}, Dimension{}, Period{}, err
	}

	plan, err := c.store.GetPlan(ctx, sub.PlanID)
	if err != nil {
		return TenantSubscription{}, Plan{}, Dimension{}, Period{}, err
	}

	dim, ok := plan.Dimensions[metric]
	if !ok {
		return TenantSubscription{}, Plan{}, Dimension{}, Period{}, ErrInvalidMetric
	}

	period := CurrentPeriod(sub.StartedAt, c.opts.Now())
	return sub, plan, dim, period, nil
}
