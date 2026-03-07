package billing

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/eleven-am/io-billing/models"
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

	now := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)
	client := NewWithOptions(rdb, db, &Options{
		Now: func() time.Time { return now },
	})
	if err := client.Migrate(); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		_ = rdb.Close()
		mr.Close()
	})
	return client, mr
}

func createPlan(t *testing.T, c *Client, name string) Plan {
	t.Helper()
	ctx := context.Background()
	plan := Plan{
		Name:     name,
		PriceEUR: 2000,
		Active:   true,
		Dimensions: map[Metric]Dimension{
			IngestTokens: {Included: 500_000, OverageRate: 0.000005, Unit: "tokens", Enforcement: EnforcementSoftCap},
			QueryTokens:  {Included: 200_000, OverageRate: 0.000002, Unit: "tokens", Enforcement: EnforcementSoftCap},
			VoiceMinutes: {Included: 60, OverageRate: 0.05, Unit: "minutes", Enforcement: EnforcementSoftCap},
			ComputeGBSec: {Included: 50_000, OverageRate: 0.00002, Unit: "gb_seconds", Enforcement: EnforcementSoftCap},
			StorageGB:    {Included: 5_000, OverageRate: 0.0001, Unit: "mb", Enforcement: EnforcementSoftCap},
			Events:       {Included: 100_000, OverageRate: 0.00001, Unit: "events", Enforcement: EnforcementSoftCap},
		},
	}
	if err := c.CreatePlan(ctx, plan); err != nil {
		t.Fatal(err)
	}
	got, err := c.GetPlanByName(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func createHardCapPlan(t *testing.T, c *Client, name string, limit int64) Plan {
	t.Helper()
	ctx := context.Background()
	plan := Plan{
		Name:     name,
		PriceEUR: 0,
		Active:   true,
		Dimensions: map[Metric]Dimension{
			IngestTokens: {Included: limit, OverageRate: 0, Unit: "tokens", Enforcement: EnforcementHardCap},
		},
	}
	if err := c.CreatePlan(ctx, plan); err != nil {
		t.Fatal(err)
	}
	got, err := c.GetPlanByName(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func subscribe(t *testing.T, c *Client, tenantID string, plan Plan) {
	t.Helper()
	ctx := context.Background()
	if err := c.Subscribe(ctx, tenantID, plan.ID, "polar_test"); err != nil {
		t.Fatal(err)
	}
}

func TestPlanCRUDAndValidation(t *testing.T) {
	c, _ := setupTestClient(t)
	ctx := context.Background()

	plan := createPlan(t, c, "starter")
	if plan.Dimensions[IngestTokens].Enforcement != EnforcementSoftCap {
		t.Fatal("expected enforcement to persist")
	}

	plan.Name = "starter-updated"
	plan.Dimensions[Events] = Dimension{
		Included:    250_000,
		OverageRate: 0.00001,
		Unit:        "events",
		Enforcement: EnforcementSoftCap,
	}
	if err := c.UpdatePlan(ctx, plan); err != nil {
		t.Fatal(err)
	}

	updated, err := c.GetPlan(ctx, plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "starter-updated" {
		t.Fatalf("unexpected name: %s", updated.Name)
	}
	if updated.Dimensions[Events].Included != 250_000 {
		t.Fatalf("unexpected events included: %d", updated.Dimensions[Events].Included)
	}

	err = c.CreatePlan(ctx, Plan{
		Name:     "invalid",
		PriceEUR: 100,
		Active:   true,
		Dimensions: map[Metric]Dimension{
			IngestTokens: {Included: 10, OverageRate: 1, Unit: "tokens", Enforcement: EnforcementHardCap},
		},
	})
	if !errors.Is(err, ErrInvalidPlan) {
		t.Fatalf("expected ErrInvalidPlan, got %v", err)
	}
}

func TestSubscriptionLifecycle(t *testing.T) {
	c, _ := setupTestClient(t)
	ctx := context.Background()

	plan := createPlan(t, c, "plan-sub")
	subscribe(t, c, "tenant-1", plan)

	sub, err := c.GetSubscription(ctx, "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if sub.PlanID != plan.ID || sub.Status != "active" {
		t.Fatalf("unexpected sub: %+v", sub)
	}

	if err := c.CancelSubscription(ctx, "tenant-1"); err != nil {
		t.Fatal(err)
	}
	cancelled, err := c.GetSubscription(ctx, "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != "cancelled" {
		t.Fatalf("expected cancelled, got %s", cancelled.Status)
	}
}

func TestQuotaCheckShowsReserved(t *testing.T) {
	c, _ := setupTestClient(t)
	ctx := context.Background()
	plan := createPlan(t, c, "plan-reserved")
	subscribe(t, c, "tenant-1", plan)

	_, err := c.Reserve(ctx, ReserveRequest{
		TenantID:    "tenant-1",
		Metric:      IngestTokens,
		Amount:      20_000,
		OperationID: "op-reserve-1",
	})
	if err != nil {
		t.Fatal(err)
	}

	status, err := c.Check(ctx, "tenant-1", IngestTokens)
	if err != nil {
		t.Fatal(err)
	}
	if status.Used != 0 || status.Reserved != 20_000 {
		t.Fatalf("unexpected usage status: %+v", status)
	}
	if status.Remaining != 480_000 {
		t.Fatalf("unexpected remaining: %d", status.Remaining)
	}
}

func TestCanConsumeHardCap(t *testing.T) {
	c, _ := setupTestClient(t)
	ctx := context.Background()
	plan := createHardCapPlan(t, c, "free", 1000)
	subscribe(t, c, "tenant-hard", plan)

	allowed, err := c.CanConsume(ctx, "tenant-hard", IngestTokens, 500)
	if err != nil {
		t.Fatal(err)
	}
	if !allowed.Allowed {
		t.Fatal("expected allowed")
	}

	denied, err := c.CanConsume(ctx, "tenant-hard", IngestTokens, 1500)
	if err != nil {
		t.Fatal(err)
	}
	if denied.Allowed || denied.Reason != "quota_exceeded" {
		t.Fatalf("expected denied quota_exceeded, got %+v", denied)
	}
}

func TestIncrementIdempotentAndConflict(t *testing.T) {
	c, _ := setupTestClient(t)
	ctx := context.Background()
	plan := createPlan(t, c, "plan-increment")
	subscribe(t, c, "tenant-1", plan)

	req := IncrementRequest{
		TenantID:    "tenant-1",
		Metric:      IngestTokens,
		Amount:      1000,
		OperationID: "op-inc-1",
	}
	if err := c.Increment(ctx, req); err != nil {
		t.Fatal(err)
	}
	if err := c.Increment(ctx, req); err != nil {
		t.Fatal(err)
	}
	used, err := c.GetUsage(ctx, "tenant-1", IngestTokens)
	if err != nil {
		t.Fatal(err)
	}
	if used != 1000 {
		t.Fatalf("expected 1000, got %d", used)
	}

	err = c.Increment(ctx, IncrementRequest{
		TenantID:    "tenant-1",
		Metric:      IngestTokens,
		Amount:      999,
		OperationID: "op-inc-1",
	})
	if !errors.Is(err, ErrOperationConflict) {
		t.Fatalf("expected ErrOperationConflict, got %v", err)
	}
}

func TestIncrementHardCapRejects(t *testing.T) {
	c, _ := setupTestClient(t)
	ctx := context.Background()
	plan := createHardCapPlan(t, c, "free-inc", 1000)
	subscribe(t, c, "tenant-free", plan)

	err := c.Increment(ctx, IncrementRequest{
		TenantID:    "tenant-free",
		Metric:      IngestTokens,
		Amount:      1200,
		OperationID: "op-inc-hard",
	})
	var qErr *QuotaExceededError
	if !errors.As(err, &qErr) {
		t.Fatalf("expected QuotaExceededError, got %v", err)
	}
}

func TestReserveCommitFlow(t *testing.T) {
	c, _ := setupTestClient(t)
	ctx := context.Background()
	plan := createPlan(t, c, "plan-commit")
	subscribe(t, c, "tenant-1", plan)

	res, err := c.Reserve(ctx, ReserveRequest{
		TenantID:    "tenant-1",
		Metric:      IngestTokens,
		Amount:      10_000,
		OperationID: "op-res-1",
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := c.Commit(ctx, CommitRequest{
		Reservation: res,
		Actual:      8_000,
		OperationID: "op-commit-1",
	}); err != nil {
		t.Fatal(err)
	}

	used, err := c.GetUsage(ctx, "tenant-1", IngestTokens)
	if err != nil {
		t.Fatal(err)
	}
	if used != 8_000 {
		t.Fatalf("expected 8000, got %d", used)
	}
	status, err := c.Check(ctx, "tenant-1", IngestTokens)
	if err != nil {
		t.Fatal(err)
	}
	if status.Reserved != 0 {
		t.Fatalf("expected reserved 0, got %d", status.Reserved)
	}
}

func TestReserveReleaseFlow(t *testing.T) {
	c, _ := setupTestClient(t)
	ctx := context.Background()
	plan := createPlan(t, c, "plan-release")
	subscribe(t, c, "tenant-1", plan)

	res, err := c.Reserve(ctx, ReserveRequest{
		TenantID:    "tenant-1",
		Metric:      IngestTokens,
		Amount:      5000,
		OperationID: "op-res-2",
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := c.Release(ctx, ReleaseRequest{
		Reservation: res,
		OperationID: "op-rel-1",
	}); err != nil {
		t.Fatal(err)
	}
	status, err := c.Check(ctx, "tenant-1", IngestTokens)
	if err != nil {
		t.Fatal(err)
	}
	if status.Used != 0 || status.Reserved != 0 {
		t.Fatalf("expected zero usage/reserved, got %+v", status)
	}
}

func TestReserveIdempotentAndConflict(t *testing.T) {
	c, _ := setupTestClient(t)
	ctx := context.Background()
	plan := createPlan(t, c, "plan-res-idemp")
	subscribe(t, c, "tenant-1", plan)

	req := ReserveRequest{
		TenantID:    "tenant-1",
		Metric:      IngestTokens,
		Amount:      777,
		OperationID: "op-res-idemp",
	}
	first, err := c.Reserve(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	second, err := c.Reserve(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatalf("expected same reservation id, got %s vs %s", first.ID, second.ID)
	}

	_, err = c.Reserve(ctx, ReserveRequest{
		TenantID:    "tenant-1",
		Metric:      IngestTokens,
		Amount:      778,
		OperationID: "op-res-idemp",
	})
	if !errors.Is(err, ErrOperationConflict) {
		t.Fatalf("expected ErrOperationConflict, got %v", err)
	}
}

func TestCommitAndReleaseIdempotent(t *testing.T) {
	c, _ := setupTestClient(t)
	ctx := context.Background()
	plan := createPlan(t, c, "plan-op-idemp")
	subscribe(t, c, "tenant-1", plan)

	res, err := c.Reserve(ctx, ReserveRequest{
		TenantID:    "tenant-1",
		Metric:      IngestTokens,
		Amount:      1000,
		OperationID: "op-r-base",
	})
	if err != nil {
		t.Fatal(err)
	}

	commit := CommitRequest{
		Reservation: res,
		Actual:      900,
		OperationID: "op-c-idemp",
	}
	if err := c.Commit(ctx, commit); err != nil {
		t.Fatal(err)
	}
	if err := c.Commit(ctx, commit); err != nil {
		t.Fatal(err)
	}
	used, err := c.GetUsage(ctx, "tenant-1", IngestTokens)
	if err != nil {
		t.Fatal(err)
	}
	if used != 900 {
		t.Fatalf("expected 900, got %d", used)
	}

	res2, err := c.Reserve(ctx, ReserveRequest{
		TenantID:    "tenant-1",
		Metric:      IngestTokens,
		Amount:      333,
		OperationID: "op-r-2",
	})
	if err != nil {
		t.Fatal(err)
	}
	release := ReleaseRequest{
		Reservation: res2,
		OperationID: "op-rel-idemp",
	}
	if err := c.Release(ctx, release); err != nil {
		t.Fatal(err)
	}
	if err := c.Release(ctx, release); err != nil {
		t.Fatal(err)
	}
	status, err := c.Check(ctx, "tenant-1", IngestTokens)
	if err != nil {
		t.Fatal(err)
	}
	if status.Reserved != 0 {
		t.Fatalf("expected 0 reserved, got %d", status.Reserved)
	}
}

func TestReservationNilErrors(t *testing.T) {
	c, _ := setupTestClient(t)
	ctx := context.Background()

	err := c.Commit(ctx, CommitRequest{Reservation: nil, Actual: 1, OperationID: "x"})
	if !errors.Is(err, ErrReservationNotFound) {
		t.Fatalf("expected ErrReservationNotFound, got %v", err)
	}
	err = c.Release(ctx, ReleaseRequest{Reservation: nil, OperationID: "x"})
	if !errors.Is(err, ErrReservationNotFound) {
		t.Fatalf("expected ErrReservationNotFound, got %v", err)
	}
}

func TestOverageReport(t *testing.T) {
	c, _ := setupTestClient(t)
	ctx := context.Background()
	plan := createPlan(t, c, "plan-overage")
	subscribe(t, c, "tenant-1", plan)

	if err := c.Increment(ctx, IncrementRequest{
		TenantID:    "tenant-1",
		Metric:      IngestTokens,
		Amount:      600_000,
		OperationID: "op-inc-over-1",
	}); err != nil {
		t.Fatal(err)
	}
	if err := c.Increment(ctx, IncrementRequest{
		TenantID:    "tenant-1",
		Metric:      Events,
		Amount:      50_000,
		OperationID: "op-inc-over-2",
	}); err != nil {
		t.Fatal(err)
	}

	report, err := c.GetOverageReport(ctx, "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Items) != 1 {
		t.Fatalf("expected 1 overage item, got %d", len(report.Items))
	}
	if report.Items[0].Metric != IngestTokens {
		t.Fatalf("expected ingest overage, got %s", report.Items[0].Metric)
	}
	if report.TotalCents <= 0 {
		t.Fatalf("expected positive overage cents, got %d", report.TotalCents)
	}
}

func TestLedgerEntriesAreRecorded(t *testing.T) {
	c, _ := setupTestClient(t)
	ctx := context.Background()
	plan := createPlan(t, c, "plan-ledger")
	subscribe(t, c, "tenant-1", plan)

	res, err := c.Reserve(ctx, ReserveRequest{
		TenantID:    "tenant-1",
		Metric:      IngestTokens,
		Amount:      1234,
		OperationID: "op-ledger-res",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Commit(ctx, CommitRequest{
		Reservation: res,
		Actual:      1200,
		OperationID: "op-ledger-commit",
	}); err != nil {
		t.Fatal(err)
	}
	if err := c.Increment(ctx, IncrementRequest{
		TenantID:    "tenant-1",
		Metric:      Events,
		Amount:      5,
		OperationID: "op-ledger-inc",
	}); err != nil {
		t.Fatal(err)
	}

	var count int64
	if err := c.store.db.WithContext(ctx).Model(&models.BillingUsageLedger{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count < 3 {
		t.Fatalf("expected at least 3 ledger rows, got %d", count)
	}
}

func TestSeedDefaultPlans(t *testing.T) {
	c, _ := setupTestClient(t)
	ctx := context.Background()

	if err := c.SeedDefaultPlans(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.SeedDefaultPlans(ctx); err != nil {
		t.Fatal(err)
	}
	plans, err := c.ListPlans(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 3 {
		t.Fatalf("expected 3 plans, got %d", len(plans))
	}
	free, err := c.GetPlanByName(ctx, "free")
	if err != nil {
		t.Fatal(err)
	}
	if free.Dimensions[IngestTokens].Enforcement != EnforcementHardCap {
		t.Fatal("free plan ingest must be hard-cap")
	}
}

func TestPeriodCalculationAndKey(t *testing.T) {
	start := time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 2, 15, 10, 0, 0, 0, time.UTC)
	p := CurrentPeriod(start, now)

	if p.Start.After(now) || p.End.Before(now) {
		t.Fatalf("invalid period: %+v", p)
	}
	if p.Key() == "" {
		t.Fatal("expected non-empty period key")
	}
}

func TestMetricHelpers(t *testing.T) {
	if !IngestTokens.Valid() {
		t.Fatal("expected ingest metric to be valid")
	}
	if Metric("bogus").Valid() {
		t.Fatal("bogus metric should be invalid")
	}
	if VoiceMinutes.Unit() != "minutes" {
		t.Fatalf("unexpected unit: %s", VoiceMinutes.Unit())
	}
}

func TestRenewPeriod(t *testing.T) {
	c, _ := setupTestClient(t)
	ctx := context.Background()
	plan := createPlan(t, c, "plan-renew")
	subscribe(t, c, "tenant-1", plan)

	if err := c.RenewPeriod(ctx, "tenant-1"); err != nil {
		t.Fatal(err)
	}
	sub, err := c.GetSubscription(ctx, "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if sub.CurrentPeriodStart.IsZero() || sub.CurrentPeriodEnd.IsZero() {
		t.Fatal("period fields should not be zero")
	}
}
