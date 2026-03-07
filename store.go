package billing

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/eleven-am/io-billing/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type postgresStore struct {
	db *gorm.DB
}

func newPostgresStore(db *gorm.DB) *postgresStore {
	return &postgresStore{db: db}
}

func (s *postgresStore) Migrate() error {
	return s.db.AutoMigrate(
		&models.BillingPlan{},
		&models.BillingPlanDimension{},
		&models.BillingTenantSubscription{},
		&models.BillingUsageLedger{},
	)
}

func (s *postgresStore) CreatePlan(ctx context.Context, plan Plan) error {
	id := plan.ID
	if id == "" {
		id = newID()
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		model := models.BillingPlan{
			ID:       id,
			Name:     plan.Name,
			PriceEUR: plan.PriceEUR,
			Active:   plan.Active,
		}

		if err := tx.Create(&model).Error; err != nil {
			return err
		}

		for metric, dim := range plan.Dimensions {
			dimModel := models.BillingPlanDimension{
				ID:          newID(),
				PlanID:      model.ID,
				Metric:      string(metric),
				Included:    dim.Included,
				OverageRate: dim.OverageRate,
				Unit:        dim.Unit,
				Enforcement: string(dim.Enforcement),
			}
			if err := tx.Create(&dimModel).Error; err != nil {
				return err
			}
		}

		return nil
	})
}

func (s *postgresStore) UpdatePlan(ctx context.Context, plan Plan) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.
			Model(&models.BillingPlan{}).
			Where("id = ?", plan.ID).
			Updates(map[string]any{
				"name":      plan.Name,
				"price_eur": plan.PriceEUR,
				"active":    plan.Active,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrPlanNotFound
		}

		if err := tx.Where("plan_id = ?", plan.ID).Delete(&models.BillingPlanDimension{}).Error; err != nil {
			return err
		}

		for metric, dim := range plan.Dimensions {
			dimModel := models.BillingPlanDimension{
				ID:          newID(),
				PlanID:      plan.ID,
				Metric:      string(metric),
				Included:    dim.Included,
				OverageRate: dim.OverageRate,
				Unit:        dim.Unit,
				Enforcement: string(dim.Enforcement),
			}
			if err := tx.Create(&dimModel).Error; err != nil {
				return err
			}
		}

		return nil
	})
}

func (s *postgresStore) GetPlan(ctx context.Context, planID string) (Plan, error) {
	var model models.BillingPlan
	if err := s.db.WithContext(ctx).Where("id = ?", planID).First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Plan{}, ErrPlanNotFound
		}
		return Plan{}, err
	}
	return s.loadPlanDimensions(ctx, model)
}

func (s *postgresStore) GetPlanByName(ctx context.Context, name string) (Plan, error) {
	var model models.BillingPlan
	if err := s.db.WithContext(ctx).Where("name = ?", name).First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Plan{}, ErrPlanNotFound
		}
		return Plan{}, err
	}
	return s.loadPlanDimensions(ctx, model)
}

func (s *postgresStore) ListPlans(ctx context.Context) ([]Plan, error) {
	var planModels []models.BillingPlan
	if err := s.db.WithContext(ctx).Where("active = ?", true).Find(&planModels).Error; err != nil {
		return nil, err
	}

	plans := make([]Plan, 0, len(planModels))
	for _, m := range planModels {
		p, err := s.loadPlanDimensions(ctx, m)
		if err != nil {
			return nil, err
		}
		plans = append(plans, p)
	}
	return plans, nil
}

func (s *postgresStore) CreateSubscription(ctx context.Context, sub TenantSubscription) error {
	id := sub.ID
	if id == "" {
		id = newID()
	}

	model := models.BillingTenantSubscription{
		ID:                 id,
		TenantID:           sub.TenantID,
		PlanID:             sub.PlanID,
		PolarCustomerID:    sub.PolarCustomerID,
		Status:             sub.Status,
		StartedAt:          sub.StartedAt,
		CurrentPeriodStart: sub.CurrentPeriodStart,
		CurrentPeriodEnd:   sub.CurrentPeriodEnd,
	}

	return s.db.WithContext(ctx).Create(&model).Error
}

func (s *postgresStore) GetSubscription(ctx context.Context, tenantID string) (TenantSubscription, error) {
	var model models.BillingTenantSubscription
	if err := s.db.WithContext(ctx).Where("tenant_id = ?", tenantID).First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return TenantSubscription{}, ErrSubscriptionNotFound
		}
		return TenantSubscription{}, err
	}

	return TenantSubscription{
		ID:                 model.ID,
		TenantID:           model.TenantID,
		PlanID:             model.PlanID,
		PolarCustomerID:    model.PolarCustomerID,
		Status:             model.Status,
		StartedAt:          model.StartedAt,
		CurrentPeriodStart: model.CurrentPeriodStart,
		CurrentPeriodEnd:   model.CurrentPeriodEnd,
		CreatedAt:          model.CreatedAt,
		UpdatedAt:          model.UpdatedAt,
	}, nil
}

func (s *postgresStore) UpdateSubscription(ctx context.Context, sub TenantSubscription) error {
	res := s.db.WithContext(ctx).
		Model(&models.BillingTenantSubscription{}).
		Where("id = ?", sub.ID).
		Updates(map[string]any{
			"plan_id":              sub.PlanID,
			"polar_customer_id":    sub.PolarCustomerID,
			"status":               sub.Status,
			"current_period_start": sub.CurrentPeriodStart,
			"current_period_end":   sub.CurrentPeriodEnd,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrSubscriptionNotFound
	}
	return nil
}

func (s *postgresStore) DeleteSubscription(ctx context.Context, tenantID string) error {
	if err := s.db.WithContext(ctx).Where("tenant_id = ?", tenantID).Delete(&models.BillingTenantSubscription{}).Error; err != nil {
		return err
	}
	return nil
}

func (s *postgresStore) loadPlanDimensions(ctx context.Context, model models.BillingPlan) (Plan, error) {
	var dims []models.BillingPlanDimension
	if err := s.db.WithContext(ctx).Where("plan_id = ?", model.ID).Find(&dims).Error; err != nil {
		return Plan{}, err
	}

	dimensions := make(map[Metric]Dimension, len(dims))
	for _, d := range dims {
		dimensions[Metric(d.Metric)] = Dimension{
			Included:    d.Included,
			OverageRate: d.OverageRate,
			Unit:        d.Unit,
			Enforcement: EnforcementMode(d.Enforcement),
		}
	}

	return Plan{
		ID:         model.ID,
		Name:       model.Name,
		PriceEUR:   model.PriceEUR,
		Dimensions: dimensions,
		Active:     model.Active,
		CreatedAt:  model.CreatedAt,
		UpdatedAt:  model.UpdatedAt,
	}, nil
}

type LedgerEntry struct {
	TenantID            string
	SubscriptionID      string
	PlanID              string
	Metric              Metric
	Action              string
	OperationID         string
	PeriodStart         string
	PeriodEnd           string
	Units               int64
	ReservedUnits       int64
	IncludedSnapshot    int64
	OverageRateSnapshot float64
	Unit                string
	Metadata            map[string]any
}

func (s *postgresStore) CreateLedgerEntry(ctx context.Context, entry LedgerEntry) error {
	if entry.TenantID == "" || entry.OperationID == "" || entry.Action == "" {
		return ErrInvalidLedgerEntry
	}

	metadata := "{}"
	if len(entry.Metadata) > 0 {
		b, err := json.Marshal(entry.Metadata)
		if err != nil {
			return err
		}
		metadata = string(b)
	}

	periodStart, err := parsePeriodKey(entry.PeriodStart)
	if err != nil {
		return err
	}
	periodEnd, err := parsePeriodKey(entry.PeriodEnd)
	if err != nil {
		return err
	}
	periodEnd = periodEnd.Add(24*time.Hour - time.Nanosecond)

	model := models.BillingUsageLedger{
		ID:                  newID(),
		TenantID:            entry.TenantID,
		SubscriptionID:      entry.SubscriptionID,
		PlanID:              entry.PlanID,
		Metric:              string(entry.Metric),
		Action:              entry.Action,
		OperationID:         entry.OperationID,
		PeriodStart:         periodStart,
		PeriodEnd:           periodEnd,
		Units:               entry.Units,
		ReservedUnits:       entry.ReservedUnits,
		IncludedSnapshot:    entry.IncludedSnapshot,
		OverageRateSnapshot: entry.OverageRateSnapshot,
		Unit:                entry.Unit,
		MetadataJSON:        metadata,
	}

	err = s.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "tenant_id"},
				{Name: "action"},
				{Name: "operation_id"},
			},
			DoNothing: true,
		}).
		Create(&model).Error
	if err != nil {
		return err
	}
	return nil
}

func parsePeriodKey(key string) (time.Time, error) {
	if key == "" {
		return time.Time{}, ErrInvalidPeriodKey
	}
	t, err := time.Parse("2006-01-02", key)
	if err != nil {
		return time.Time{}, ErrInvalidPeriodKey
	}
	return t, nil
}
