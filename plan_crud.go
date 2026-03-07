package billing

import (
	"context"
	"strings"
)

func (c *Client) CreatePlan(ctx context.Context, plan Plan) error {
	if err := validatePlan(plan); err != nil {
		return err
	}
	return c.store.CreatePlan(ctx, plan)
}

func (c *Client) UpdatePlan(ctx context.Context, plan Plan) error {
	if plan.ID == "" {
		return ErrInvalidPlan
	}
	if err := validatePlan(plan); err != nil {
		return err
	}
	return c.store.UpdatePlan(ctx, plan)
}

func (c *Client) GetPlan(ctx context.Context, planID string) (Plan, error) {
	if err := validatePlanID(planID); err != nil {
		return Plan{}, err
	}
	return c.store.GetPlan(ctx, planID)
}

func (c *Client) GetPlanByName(ctx context.Context, name string) (Plan, error) {
	if strings.TrimSpace(name) == "" {
		return Plan{}, ErrInvalidPlan
	}
	return c.store.GetPlanByName(ctx, name)
}

func (c *Client) ListPlans(ctx context.Context) ([]Plan, error) {
	return c.store.ListPlans(ctx)
}
