package billing

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupTestClient(t *testing.T) (*Client, *miniredis.Miniredis) {
	t.Helper()

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}

	client := New(rdb, db)
	if err := client.Migrate(); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		mr.Close()
		rdb.Close()
	})

	return client, mr
}

func createTestPlan(t *testing.T, c *Client) Plan {
	t.Helper()
	plan := Plan{
		Name:     "test-plan",
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
	}
	ctx := context.Background()
	if err := c.CreatePlan(ctx, plan); err != nil {
		t.Fatal(err)
	}
	got, err := c.GetPlanByName(ctx, "test-plan")
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func subscribeTestTenant(t *testing.T, c *Client, tenantID string, plan Plan) {
	t.Helper()
	ctx := context.Background()
	if err := c.Subscribe(ctx, tenantID, plan.ID, "polar_123"); err != nil {
		t.Fatal(err)
	}
}

func TestPlanCRUD(t *testing.T) {
	c, _ := setupTestClient(t)
	ctx := context.Background()

	plan := Plan{
		Name:     "starter",
		PriceEUR: 2000,
		Active:   true,
		Dimensions: map[Metric]Dimension{
			IngestTokens: {Included: 500_000, OverageRate: 0.000005, Unit: "tokens"},
			Events:       {Included: 100_000, OverageRate: 0.00001, Unit: "events"},
		},
	}

	if err := c.CreatePlan(ctx, plan); err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}

	got, err := c.GetPlanByName(ctx, "starter")
	if err != nil {
		t.Fatalf("GetPlanByName: %v", err)
	}
	if got.Name != "starter" || got.PriceEUR != 2000 {
		t.Fatalf("unexpected plan: %+v", got)
	}
	if len(got.Dimensions) != 2 {
		t.Fatalf("expected 2 dimensions, got %d", len(got.Dimensions))
	}
	if got.Dimensions[IngestTokens].Included != 500_000 {
		t.Fatalf("unexpected ingest included: %d", got.Dimensions[IngestTokens].Included)
	}

	got2, err := c.GetPlan(ctx, got.ID)
	if err != nil {
		t.Fatalf("GetPlan: %v", err)
	}
	if got2.ID != got.ID {
		t.Fatal("GetPlan returned different plan")
	}

	got.Name = "starter-updated"
	got.PriceEUR = 2500
	got.Dimensions[Events] = Dimension{Included: 200_000, OverageRate: 0.00002, Unit: "events"}
	if err := c.UpdatePlan(ctx, got); err != nil {
		t.Fatalf("UpdatePlan: %v", err)
	}

	updated, err := c.GetPlan(ctx, got.ID)
	if err != nil {
		t.Fatalf("GetPlan after update: %v", err)
	}
	if updated.Name != "starter-updated" || updated.PriceEUR != 2500 {
		t.Fatalf("plan not updated: %+v", updated)
	}
	if updated.Dimensions[Events].Included != 200_000 {
		t.Fatalf("dimension not updated: %+v", updated.Dimensions[Events])
	}
}

func TestPlanNotFound(t *testing.T) {
	c, _ := setupTestClient(t)
	ctx := context.Background()

	_, err := c.GetPlan(ctx, "nonexistent")
	if !errors.Is(err, ErrPlanNotFound) {
		t.Fatalf("expected ErrPlanNotFound, got: %v", err)
	}

	_, err = c.GetPlanByName(ctx, "nonexistent")
	if !errors.Is(err, ErrPlanNotFound) {
		t.Fatalf("expected ErrPlanNotFound, got: %v", err)
	}
}

func TestListPlans(t *testing.T) {
	c, _ := setupTestClient(t)
	ctx := context.Background()

	if err := c.CreatePlan(ctx, Plan{Name: "active1", PriceEUR: 100, Active: true, Dimensions: map[Metric]Dimension{}}); err != nil {
		t.Fatal(err)
	}
	if err := c.CreatePlan(ctx, Plan{Name: "active2", PriceEUR: 200, Active: true, Dimensions: map[Metric]Dimension{}}); err != nil {
		t.Fatal(err)
	}
	inactivePlan := Plan{Name: "inactive", PriceEUR: 300, Active: true, Dimensions: map[Metric]Dimension{}}
	if err := c.CreatePlan(ctx, inactivePlan); err != nil {
		t.Fatal(err)
	}
	got, _ := c.GetPlanByName(ctx, "inactive")
	got.Active = false
	if err := c.UpdatePlan(ctx, got); err != nil {
		t.Fatal(err)
	}

	plans, err := c.ListPlans(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 2 {
		t.Fatalf("expected 2 active plans, got %d", len(plans))
	}
}

func TestSubscriptionLifecycle(t *testing.T) {
	c, _ := setupTestClient(t)
	ctx := context.Background()
	plan := createTestPlan(t, c)

	if err := c.Subscribe(ctx, "tenant-1", plan.ID, "polar_abc"); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	sub, err := c.GetSubscription(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("GetSubscription: %v", err)
	}
	if sub.TenantID != "tenant-1" || sub.PlanID != plan.ID || sub.Status != "active" {
		t.Fatalf("unexpected subscription: %+v", sub)
	}
	if sub.PolarCustomerID != "polar_abc" {
		t.Fatalf("unexpected polar customer id: %s", sub.PolarCustomerID)
	}

	if err := c.CancelSubscription(ctx, "tenant-1"); err != nil {
		t.Fatalf("CancelSubscription: %v", err)
	}

	sub, err = c.GetSubscription(ctx, "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if sub.Status != "cancelled" {
		t.Fatalf("expected cancelled, got %s", sub.Status)
	}
}

func TestSubscriptionNotFound(t *testing.T) {
	c, _ := setupTestClient(t)
	ctx := context.Background()

	_, err := c.GetSubscription(ctx, "nonexistent")
	if !errors.Is(err, ErrSubscriptionNotFound) {
		t.Fatalf("expected ErrSubscriptionNotFound, got: %v", err)
	}
}

func TestUsageIncrement(t *testing.T) {
	c, _ := setupTestClient(t)
	ctx := context.Background()
	plan := createTestPlan(t, c)
	subscribeTestTenant(t, c, "tenant-1", plan)

	if err := c.Increment(ctx, "tenant-1", IngestTokens, 1000); err != nil {
		t.Fatal(err)
	}
	if err := c.Increment(ctx, "tenant-1", IngestTokens, 500); err != nil {
		t.Fatal(err)
	}

	used, err := c.GetUsage(ctx, "tenant-1", IngestTokens)
	if err != nil {
		t.Fatal(err)
	}
	if used != 1500 {
		t.Fatalf("expected 1500, got %d", used)
	}

	unused, err := c.GetUsage(ctx, "tenant-1", Events)
	if err != nil {
		t.Fatal(err)
	}
	if unused != 0 {
		t.Fatalf("expected 0, got %d", unused)
	}
}

func TestUsageInvalidMetric(t *testing.T) {
	c, _ := setupTestClient(t)
	ctx := context.Background()

	err := c.Increment(ctx, "tenant-1", "bogus", 100)
	if !errors.Is(err, ErrInvalidMetric) {
		t.Fatalf("expected ErrInvalidMetric, got: %v", err)
	}
}

func TestGetAllUsage(t *testing.T) {
	c, _ := setupTestClient(t)
	ctx := context.Background()
	plan := createTestPlan(t, c)
	subscribeTestTenant(t, c, "tenant-1", plan)

	c.Increment(ctx, "tenant-1", IngestTokens, 100)
	c.Increment(ctx, "tenant-1", Events, 50)

	all, err := c.GetAllUsage(ctx, "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if all[IngestTokens] != 100 {
		t.Fatalf("expected 100, got %d", all[IngestTokens])
	}
	if all[Events] != 50 {
		t.Fatalf("expected 50, got %d", all[Events])
	}
	if all[VoiceMinutes] != 0 {
		t.Fatalf("expected 0, got %d", all[VoiceMinutes])
	}
}

func TestQuotaCheck(t *testing.T) {
	c, _ := setupTestClient(t)
	ctx := context.Background()
	plan := createTestPlan(t, c)
	subscribeTestTenant(t, c, "tenant-1", plan)

	status, err := c.Check(ctx, "tenant-1", IngestTokens)
	if err != nil {
		t.Fatal(err)
	}
	if status.Used != 0 || status.Limit != 500_000 || status.Remaining != 500_000 {
		t.Fatalf("unexpected status: %+v", status)
	}
	if status.Exceeded {
		t.Fatal("should not be exceeded")
	}
	if !status.CanOverage {
		t.Fatal("starter plan should allow overage")
	}

	c.Increment(ctx, "tenant-1", IngestTokens, 500_000)

	status, err = c.Check(ctx, "tenant-1", IngestTokens)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Exceeded {
		t.Fatal("should be exceeded at limit")
	}
	if status.Remaining != 0 {
		t.Fatalf("expected 0 remaining, got %d", status.Remaining)
	}
}

func TestQuotaCheckFreeTierHardCap(t *testing.T) {
	c, _ := setupTestClient(t)
	ctx := context.Background()

	freePlan := Plan{
		Name:     "free",
		PriceEUR: 0,
		Active:   true,
		Dimensions: map[Metric]Dimension{
			IngestTokens: {Included: 50_000, OverageRate: 0, Unit: "tokens"},
		},
	}
	c.CreatePlan(ctx, freePlan)
	got, _ := c.GetPlanByName(ctx, "free")
	subscribeTestTenant(t, c, "tenant-free", got)

	status, err := c.Check(ctx, "tenant-free", IngestTokens)
	if err != nil {
		t.Fatal(err)
	}
	if status.CanOverage {
		t.Fatal("free tier should NOT allow overage")
	}
}

func TestQuotaCheckInvalidMetric(t *testing.T) {
	c, _ := setupTestClient(t)
	ctx := context.Background()

	_, err := c.Check(ctx, "tenant-1", "bogus")
	if !errors.Is(err, ErrInvalidMetric) {
		t.Fatalf("expected ErrInvalidMetric, got: %v", err)
	}
}

func TestSetQuota(t *testing.T) {
	c, _ := setupTestClient(t)
	ctx := context.Background()
	plan := createTestPlan(t, c)
	subscribeTestTenant(t, c, "tenant-1", plan)

	if err := c.SetQuota(ctx, "tenant-1", IngestTokens, 999); err != nil {
		t.Fatal(err)
	}

	status, err := c.Check(ctx, "tenant-1", IngestTokens)
	if err != nil {
		t.Fatal(err)
	}
	if status.Limit != 999 {
		t.Fatalf("expected limit 999, got %d", status.Limit)
	}
}

func TestSetQuotas(t *testing.T) {
	c, _ := setupTestClient(t)
	ctx := context.Background()
	plan := createTestPlan(t, c)
	subscribeTestTenant(t, c, "tenant-1", plan)

	quotas := map[Metric]int64{
		IngestTokens: 111,
		Events:       222,
	}
	if err := c.SetQuotas(ctx, "tenant-1", quotas); err != nil {
		t.Fatal(err)
	}

	s1, _ := c.Check(ctx, "tenant-1", IngestTokens)
	s2, _ := c.Check(ctx, "tenant-1", Events)
	if s1.Limit != 111 || s2.Limit != 222 {
		t.Fatalf("expected 111/222, got %d/%d", s1.Limit, s2.Limit)
	}
}

func TestCheckMultiple(t *testing.T) {
	c, _ := setupTestClient(t)
	ctx := context.Background()
	plan := createTestPlan(t, c)
	subscribeTestTenant(t, c, "tenant-1", plan)

	result, err := c.CheckMultiple(ctx, "tenant-1", []Metric{IngestTokens, Events})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 statuses, got %d", len(result))
	}
	if result[IngestTokens].Limit != 500_000 {
		t.Fatalf("unexpected limit: %d", result[IngestTokens].Limit)
	}
}

func TestReservationFlow(t *testing.T) {
	c, _ := setupTestClient(t)
	ctx := context.Background()
	plan := createTestPlan(t, c)
	subscribeTestTenant(t, c, "tenant-1", plan)

	res, err := c.Reserve(ctx, "tenant-1", IngestTokens, 10_000)
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if res.Amount != 10_000 {
		t.Fatalf("unexpected amount: %d", res.Amount)
	}

	used, _ := c.GetUsage(ctx, "tenant-1", IngestTokens)
	if used != 10_000 {
		t.Fatalf("expected 10000 after reserve, got %d", used)
	}

	if err := c.Reconcile(ctx, res, 8_000); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	used, _ = c.GetUsage(ctx, "tenant-1", IngestTokens)
	if used != 8_000 {
		t.Fatalf("expected 8000 after reconcile, got %d", used)
	}
}

func TestReservationRelease(t *testing.T) {
	c, _ := setupTestClient(t)
	ctx := context.Background()
	plan := createTestPlan(t, c)
	subscribeTestTenant(t, c, "tenant-1", plan)

	res, err := c.Reserve(ctx, "tenant-1", IngestTokens, 5_000)
	if err != nil {
		t.Fatal(err)
	}

	if err := c.ReleaseReservation(ctx, res); err != nil {
		t.Fatalf("ReleaseReservation: %v", err)
	}

	used, _ := c.GetUsage(ctx, "tenant-1", IngestTokens)
	if used != 0 {
		t.Fatalf("expected 0 after release, got %d", used)
	}
}

func TestReservationRejectsFreeTierOverQuota(t *testing.T) {
	c, _ := setupTestClient(t)
	ctx := context.Background()

	freePlan := Plan{
		Name:     "free-res",
		PriceEUR: 0,
		Active:   true,
		Dimensions: map[Metric]Dimension{
			IngestTokens: {Included: 1_000, OverageRate: 0, Unit: "tokens"},
		},
	}
	c.CreatePlan(ctx, freePlan)
	got, _ := c.GetPlanByName(ctx, "free-res")
	subscribeTestTenant(t, c, "tenant-free", got)

	_, err := c.Reserve(ctx, "tenant-free", IngestTokens, 2_000)
	if err == nil {
		t.Fatal("expected quota exceeded error")
	}

	var qErr *QuotaExceededError
	if !errors.As(err, &qErr) {
		t.Fatalf("expected QuotaExceededError, got: %v", err)
	}
	if qErr.Estimated != 2_000 {
		t.Fatalf("expected estimated 2000, got %d", qErr.Estimated)
	}
}

func TestReservationReconcileHigher(t *testing.T) {
	c, _ := setupTestClient(t)
	ctx := context.Background()
	plan := createTestPlan(t, c)
	subscribeTestTenant(t, c, "tenant-1", plan)

	res, _ := c.Reserve(ctx, "tenant-1", IngestTokens, 5_000)
	c.Reconcile(ctx, res, 7_000)

	used, _ := c.GetUsage(ctx, "tenant-1", IngestTokens)
	if used != 7_000 {
		t.Fatalf("expected 7000, got %d", used)
	}
}

func TestReservationNilErrors(t *testing.T) {
	c, _ := setupTestClient(t)
	ctx := context.Background()

	if err := c.Reconcile(ctx, nil, 100); !errors.Is(err, ErrReservationNotFound) {
		t.Fatalf("expected ErrReservationNotFound, got: %v", err)
	}
	if err := c.ReleaseReservation(ctx, nil); !errors.Is(err, ErrReservationNotFound) {
		t.Fatalf("expected ErrReservationNotFound, got: %v", err)
	}
}

func TestPeriodCalculation(t *testing.T) {
	tests := []struct {
		name      string
		startedAt time.Time
		now       time.Time
		wantStart time.Time
		wantEnd   time.Time
	}{
		{
			name:      "same month",
			startedAt: time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC),
			now:       time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC),
			wantStart: time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2026, 4, 4, 0, 0, 0, 0, time.UTC).Add(-time.Nanosecond),
		},
		{
			name:      "next month",
			startedAt: time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC),
			now:       time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
			wantStart: time.Date(2026, 4, 4, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC).Add(-time.Nanosecond),
		},
		{
			name:      "started on 31st, in february",
			startedAt: time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC),
			now:       time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC),
			wantStart: time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC),
		},
		{
			name:      "on anniversary day",
			startedAt: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
			now:       time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC),
			wantStart: time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := CurrentPeriod(tt.startedAt, tt.now)
			if !tt.wantStart.IsZero() && !p.Start.Equal(tt.wantStart) {
				t.Errorf("Start: got %v, want %v", p.Start, tt.wantStart)
			}
			if !tt.wantEnd.IsZero() && !p.End.Equal(tt.wantEnd) {
				t.Errorf("End: got %v, want %v", p.End, tt.wantEnd)
			}
			if p.Start.After(tt.now) {
				t.Error("period start should not be after now")
			}
			if p.End.Before(tt.now) {
				t.Error("period end should not be before now")
			}
		})
	}
}

func TestPeriodKey(t *testing.T) {
	p := Period{
		Start: time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC),
		End:   time.Date(2026, 4, 3, 23, 59, 59, 0, time.UTC),
	}
	if p.Key() != "2026-03-04" {
		t.Fatalf("expected 2026-03-04, got %s", p.Key())
	}
}

func TestOverageReport(t *testing.T) {
	c, _ := setupTestClient(t)
	ctx := context.Background()
	plan := createTestPlan(t, c)
	subscribeTestTenant(t, c, "tenant-1", plan)

	c.Increment(ctx, "tenant-1", IngestTokens, 600_000)
	c.Increment(ctx, "tenant-1", Events, 50_000)

	report, err := c.GetOverageReport(ctx, "tenant-1")
	if err != nil {
		t.Fatal(err)
	}

	if len(report.Items) != 1 {
		t.Fatalf("expected 1 overage item (only ingest), got %d", len(report.Items))
	}

	item := report.Items[0]
	if item.Metric != IngestTokens {
		t.Fatalf("expected ingest_tokens, got %s", item.Metric)
	}
	if item.Overage != 100_000 {
		t.Fatalf("expected overage 100000, got %d", item.Overage)
	}
	if item.AmountCents <= 0 {
		t.Fatalf("expected positive amount, got %d", item.AmountCents)
	}
	if report.TotalCents != item.AmountCents {
		t.Fatalf("total mismatch: %d vs %d", report.TotalCents, item.AmountCents)
	}
}

func TestOverageReportNoOverage(t *testing.T) {
	c, _ := setupTestClient(t)
	ctx := context.Background()
	plan := createTestPlan(t, c)
	subscribeTestTenant(t, c, "tenant-1", plan)

	c.Increment(ctx, "tenant-1", IngestTokens, 100_000)

	report, err := c.GetOverageReport(ctx, "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Items) != 0 {
		t.Fatalf("expected no overage items, got %d", len(report.Items))
	}
	if report.TotalCents != 0 {
		t.Fatalf("expected 0 total, got %d", report.TotalCents)
	}
}

func TestSeedDefaultPlans(t *testing.T) {
	c, _ := setupTestClient(t)
	ctx := context.Background()

	if err := c.SeedDefaultPlans(ctx); err != nil {
		t.Fatalf("first seed: %v", err)
	}

	plans, err := c.ListPlans(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 3 {
		t.Fatalf("expected 3 plans, got %d", len(plans))
	}

	if err := c.SeedDefaultPlans(ctx); err != nil {
		t.Fatalf("second seed (idempotent): %v", err)
	}

	plans, err = c.ListPlans(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 3 {
		t.Fatalf("expected 3 plans after re-seed, got %d", len(plans))
	}
}

func TestSeedPlanDimensions(t *testing.T) {
	c, _ := setupTestClient(t)
	ctx := context.Background()

	c.SeedDefaultPlans(ctx)

	free, err := c.GetPlanByName(ctx, "free")
	if err != nil {
		t.Fatal(err)
	}
	if len(free.Dimensions) != 6 {
		t.Fatalf("expected 6 dimensions on free plan, got %d", len(free.Dimensions))
	}
	if free.Dimensions[IngestTokens].OverageRate != 0 {
		t.Fatal("free plan should have 0 overage rate")
	}

	starter, err := c.GetPlanByName(ctx, "starter")
	if err != nil {
		t.Fatal(err)
	}
	if starter.PriceEUR != 2000 {
		t.Fatalf("expected starter 2000 cents, got %d", starter.PriceEUR)
	}
	if starter.Dimensions[IngestTokens].OverageRate == 0 {
		t.Fatal("starter should have non-zero overage rate")
	}

	pro, err := c.GetPlanByName(ctx, "pro")
	if err != nil {
		t.Fatal(err)
	}
	if pro.PriceEUR != 10000 {
		t.Fatalf("expected pro 10000 cents, got %d", pro.PriceEUR)
	}
}

func TestMetricValid(t *testing.T) {
	if !IngestTokens.Valid() {
		t.Fatal("IngestTokens should be valid")
	}
	if Metric("bogus").Valid() {
		t.Fatal("bogus should not be valid")
	}
}

func TestMetricUnit(t *testing.T) {
	if IngestTokens.Unit() != "tokens" {
		t.Fatalf("expected tokens, got %s", IngestTokens.Unit())
	}
	if VoiceMinutes.Unit() != "minutes" {
		t.Fatalf("expected minutes, got %s", VoiceMinutes.Unit())
	}
}

func TestRenewPeriod(t *testing.T) {
	c, _ := setupTestClient(t)
	ctx := context.Background()
	plan := createTestPlan(t, c)
	subscribeTestTenant(t, c, "tenant-1", plan)

	c.Increment(ctx, "tenant-1", IngestTokens, 100_000)

	if err := c.RenewPeriod(ctx, "tenant-1"); err != nil {
		t.Fatalf("RenewPeriod: %v", err)
	}

	sub, _ := c.GetSubscription(ctx, "tenant-1")
	if sub.CurrentPeriodStart.IsZero() {
		t.Fatal("period start should not be zero")
	}
}

func TestQuotaExceededError(t *testing.T) {
	qErr := &QuotaExceededError{
		Metric:    IngestTokens,
		Used:      500_000,
		Limit:     500_000,
		Estimated: 10_000,
	}

	if !errors.Is(qErr, ErrQuotaExceeded) {
		t.Fatal("QuotaExceededError should unwrap to ErrQuotaExceeded")
	}

	msg := qErr.Error()
	if msg == "" {
		t.Fatal("error message should not be empty")
	}
}
