package billing

import "time"

type Plan struct {
	ID         string
	Name       string
	PriceEUR   int64
	Dimensions map[Metric]Dimension
	Active     bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type Dimension struct {
	Included    int64
	OverageRate float64
	Unit        string
}

type TenantSubscription struct {
	ID                 string
	TenantID           string
	PlanID             string
	PolarCustomerID    string
	Status             string
	StartedAt          time.Time
	CurrentPeriodStart time.Time
	CurrentPeriodEnd   time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}
