package billing

import "context"

func (c *Client) CreatePlan(ctx context.Context, plan Plan) error {
	return c.store.CreatePlan(ctx, plan)
}

func (c *Client) UpdatePlan(ctx context.Context, plan Plan) error {
	return c.store.UpdatePlan(ctx, plan)
}

func (c *Client) GetPlan(ctx context.Context, planID string) (Plan, error) {
	return c.store.GetPlan(ctx, planID)
}

func (c *Client) GetPlanByName(ctx context.Context, name string) (Plan, error) {
	return c.store.GetPlanByName(ctx, name)
}

func (c *Client) ListPlans(ctx context.Context) ([]Plan, error) {
	return c.store.ListPlans(ctx)
}
