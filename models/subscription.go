package models

import "time"

type BillingTenantSubscription struct {
	ID                 string    `gorm:"column:id;primaryKey"`
	TenantID           string    `gorm:"column:tenant_id;uniqueIndex;not null"`
	PlanID             string    `gorm:"column:plan_id;not null;index"`
	PolarCustomerID    string    `gorm:"column:polar_customer_id"`
	Status             string    `gorm:"column:status;not null"`
	StartedAt          time.Time `gorm:"column:started_at;not null"`
	CurrentPeriodStart time.Time `gorm:"column:current_period_start;not null"`
	CurrentPeriodEnd   time.Time `gorm:"column:current_period_end;not null"`
	CreatedAt          time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt          time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (BillingTenantSubscription) TableName() string {
	return "billing_tenant_subscriptions"
}
