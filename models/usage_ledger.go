package models

import "time"

type BillingUsageLedger struct {
	ID                  string    `gorm:"column:id;primaryKey"`
	TenantID            string    `gorm:"column:tenant_id;not null;index:idx_billing_usage_tenant_period_metric,priority:1;index:ux_billing_usage_tenant_action_op,unique,priority:1"`
	SubscriptionID      string    `gorm:"column:subscription_id;index"`
	PlanID              string    `gorm:"column:plan_id;not null;index"`
	Metric              string    `gorm:"column:metric;not null;index:idx_billing_usage_tenant_period_metric,priority:3"`
	Action              string    `gorm:"column:action;not null;index:ux_billing_usage_tenant_action_op,unique,priority:2"`
	OperationID         string    `gorm:"column:operation_id;not null;index:ux_billing_usage_tenant_action_op,unique,priority:3"`
	PeriodStart         time.Time `gorm:"column:period_start;not null;index:idx_billing_usage_tenant_period_metric,priority:2"`
	PeriodEnd           time.Time `gorm:"column:period_end;not null"`
	Units               int64     `gorm:"column:units;not null"`
	ReservedUnits       int64     `gorm:"column:reserved_units;not null;default:0"`
	IncludedSnapshot    int64     `gorm:"column:included_snapshot;not null;default:0"`
	OverageRateSnapshot float64   `gorm:"column:overage_rate_snapshot;not null;default:0"`
	Unit                string    `gorm:"column:unit;not null"`
	MetadataJSON        string    `gorm:"column:metadata_json;type:text"`
	CreatedAt           time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (BillingUsageLedger) TableName() string {
	return "billing_usage_ledger"
}
