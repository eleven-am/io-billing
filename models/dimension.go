package models

type BillingPlanDimension struct {
	ID          string  `gorm:"column:id;primaryKey"`
	PlanID      string  `gorm:"column:plan_id;not null;index"`
	Metric      string  `gorm:"column:metric;not null"`
	Included    int64   `gorm:"column:included;not null"`
	OverageRate float64 `gorm:"column:overage_rate;not null;default:0"`
	Unit        string  `gorm:"column:unit;not null"`
	Enforcement string  `gorm:"column:enforcement;not null;default:'hard_cap'"`
}

func (BillingPlanDimension) TableName() string {
	return "billing_plan_dimensions"
}
