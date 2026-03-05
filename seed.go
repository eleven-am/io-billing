package billing

import "context"

func (c *Client) SeedDefaultPlans(ctx context.Context) error {
	plans := []Plan{
		{
			Name:     "free",
			PriceEUR: 0,
			Active:   true,
			Dimensions: map[Metric]Dimension{
				IngestTokens: {Included: 50_000, OverageRate: 0, Unit: "tokens"},
				QueryTokens:  {Included: 20_000, OverageRate: 0, Unit: "tokens"},
				VoiceMinutes: {Included: 5, OverageRate: 0, Unit: "minutes"},
				ComputeGBSec: {Included: 5_000, OverageRate: 0, Unit: "gb_seconds"},
				StorageGB:    {Included: 500, OverageRate: 0, Unit: "mb"},
				Events:       {Included: 10_000, OverageRate: 0, Unit: "events"},
			},
		},
		{
			Name:     "starter",
			PriceEUR: 2000,
			Active:   true,
			Dimensions: map[Metric]Dimension{
				IngestTokens: {Included: 500_000, OverageRate: 0.000005, Unit: "tokens"},
				QueryTokens:  {Included: 200_000, OverageRate: 0.000002, Unit: "tokens"},
				VoiceMinutes: {Included: 60, OverageRate: 0.05, Unit: "minutes"},
				ComputeGBSec: {Included: 50_000, OverageRate: 0.00002, Unit: "gb_seconds"},
				StorageGB:    {Included: 5_000, OverageRate: 0.0001, Unit: "mb"},
				Events:       {Included: 100_000, OverageRate: 0.00001, Unit: "events"},
			},
		},
		{
			Name:     "pro",
			PriceEUR: 10000,
			Active:   true,
			Dimensions: map[Metric]Dimension{
				IngestTokens: {Included: 5_000_000, OverageRate: 0.0000046, Unit: "tokens"},
				QueryTokens:  {Included: 2_000_000, OverageRate: 0.00000175, Unit: "tokens"},
				VoiceMinutes: {Included: 600, OverageRate: 0.03, Unit: "minutes"},
				ComputeGBSec: {Included: 500_000, OverageRate: 0.000015, Unit: "gb_seconds"},
				StorageGB:    {Included: 50_000, OverageRate: 0.00008, Unit: "mb"},
				Events:       {Included: 1_000_000, OverageRate: 0.000007, Unit: "events"},
			},
		},
	}

	for _, plan := range plans {
		_, err := c.store.GetPlanByName(ctx, plan.Name)
		if err == nil {
			continue
		}
		if err := c.store.CreatePlan(ctx, plan); err != nil {
			return err
		}
	}

	return nil
}
