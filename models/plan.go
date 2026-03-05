package models

import "time"

type BillingPlan struct {
	ID        string    `gorm:"column:id;primaryKey"`
	Name      string    `gorm:"column:name;uniqueIndex;not null"`
	PriceEUR  int64     `gorm:"column:price_eur;not null"`
	Active    bool      `gorm:"column:active;not null;default:true"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (BillingPlan) TableName() string {
	return "billing_plans"
}
