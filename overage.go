package billing

import (
	"context"
	"math"
	"time"
)

type OverageReport struct {
	TenantID    string
	PeriodStart time.Time
	PeriodEnd   time.Time
	Items       []OverageItem
	TotalCents  int64
}

type OverageItem struct {
	Metric      Metric
	Used        int64
	Included    int64
	Overage     int64
	Rate        float64
	AmountCents int64
}

func (c *Client) GetOverageReport(ctx context.Context, tenantID string) (*OverageReport, error) {
	sub, err := c.store.GetSubscription(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	plan, err := c.store.GetPlan(ctx, sub.PlanID)
	if err != nil {
		return nil, err
	}

	period := CurrentPeriod(sub.StartedAt, time.Now())

	report := &OverageReport{
		TenantID:    tenantID,
		PeriodStart: period.Start,
		PeriodEnd:   period.End,
	}

	var totalCents int64

	for _, metric := range AllMetrics {
		dim, ok := plan.Dimensions[metric]
		if !ok {
			continue
		}

		if dim.OverageRate == 0 {
			continue
		}

		key := usageKey(tenantID, period, metric)
		used, err := c.redis.Get(ctx, key).Int64()
		if err != nil && err.Error() == "redis: nil" {
			used = 0
		} else if err != nil {
			return nil, err
		}

		if used <= dim.Included {
			continue
		}

		overage := used - dim.Included
		amountCents := int64(math.Ceil(float64(overage) * dim.OverageRate * 100))

		report.Items = append(report.Items, OverageItem{
			Metric:      metric,
			Used:        used,
			Included:    dim.Included,
			Overage:     overage,
			Rate:        dim.OverageRate,
			AmountCents: amountCents,
		})

		totalCents += amountCents
	}

	report.TotalCents = totalCents
	return report, nil
}
