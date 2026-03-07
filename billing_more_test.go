package billing

import (
	"context"
	"errors"
	"testing"
)

func TestNewAndDefaults(t *testing.T) {
	c := New(nil, nil)
	if c == nil || c.store == nil {
		t.Fatal("client should be initialized")
	}
	if c.opts.Now == nil {
		t.Fatal("defaults should include clock")
	}
}

func TestQuotaHelpersAndSetters(t *testing.T) {
	c, _ := setupTestClient(t)
	ctx := context.Background()
	plan := createPlan(t, c, "plan-quota-helpers")
	subscribe(t, c, "tenant-q", plan)

	if err := c.SetQuota(ctx, "tenant-q", IngestTokens, 1234); err != nil {
		t.Fatal(err)
	}
	if err := c.SetQuotas(ctx, "tenant-q", map[Metric]int64{
		IngestTokens: 2000,
		Events:       20,
	}); err != nil {
		t.Fatal(err)
	}
	if err := c.setCanOverage(ctx, "tenant-q", IngestTokens, false); err != nil {
		t.Fatal(err)
	}

	statuses, err := c.CheckMultiple(ctx, "tenant-q", []Metric{IngestTokens, Events})
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 2 {
		t.Fatalf("expected 2 statuses, got %d", len(statuses))
	}
	if statuses[IngestTokens].CanOverage {
		t.Fatal("expected overage false after setCanOverage")
	}
	if int64ToStr(123) != "123" {
		t.Fatal("int64ToStr should convert")
	}
}

func TestMetricUnitsAllBranches(t *testing.T) {
	if IngestTokens.Unit() != "tokens" {
		t.Fatal("ingest unit mismatch")
	}
	if QueryTokens.Unit() != "tokens" {
		t.Fatal("query unit mismatch")
	}
	if VoiceMinutes.Unit() != "minutes" {
		t.Fatal("voice unit mismatch")
	}
	if ComputeGBSec.Unit() != "gb_seconds" {
		t.Fatal("compute unit mismatch")
	}
	if StorageGB.Unit() != "gb" {
		t.Fatal("storage unit mismatch")
	}
	if Events.Unit() != "events" {
		t.Fatal("events unit mismatch")
	}
	if Metric("x").Unit() != "" {
		t.Fatal("unknown metric should return empty unit")
	}
}

func TestQuotaExceededErrorBehavior(t *testing.T) {
	err := &QuotaExceededError{
		Metric:    IngestTokens,
		Used:      100,
		Limit:     50,
		Estimated: 10,
	}
	if err.Error() == "" {
		t.Fatal("error string must not be empty")
	}
	if !errors.Is(err, ErrQuotaExceeded) {
		t.Fatal("quota error should unwrap")
	}
}

func TestInternalParseAndValidationHelpers(t *testing.T) {
	if _, err := parsePeriodKey(""); err == nil {
		t.Fatal("empty period key should fail")
	}
	if _, _, err := decodeOpValue("bad"); !errors.Is(err, ErrOperationConflict) {
		t.Fatal("decodeOpValue should fail")
	}
	if _, _, err := decodeReserveOpValue("bad"); !errors.Is(err, ErrOperationConflict) {
		t.Fatal("decodeReserveOpValue should fail")
	}
	if err := validateOperationID(""); !errors.Is(err, ErrInvalidOperationID) {
		t.Fatal("validateOperationID should fail")
	}
	if !mustParseRFC3339Date("bad").IsZero() {
		t.Fatal("invalid date should return zero time")
	}
	if _, _, _, _, err := parseIncrementScriptResult("bad"); !errors.Is(err, ErrOperationConflict) {
		t.Fatal("parseIncrementScriptResult should fail on invalid type")
	}
	if _, _, _, _, _, err := parseReserveScriptResult("bad"); !errors.Is(err, ErrOperationConflict) {
		t.Fatal("parseReserveScriptResult should fail on invalid type")
	}
	if _, _, _, _, _, err := parseCommitScriptResult("bad"); !errors.Is(err, ErrOperationConflict) {
		t.Fatal("parseCommitScriptResult should fail on invalid type")
	}
	if _, _, _, err := parseReleaseScriptResult("bad"); !errors.Is(err, ErrOperationConflict) {
		t.Fatal("parseReleaseScriptResult should fail on invalid type")
	}
}

func TestUsageKeyCompatibility(t *testing.T) {
	p := Period{Start: mustParseRFC3339Date("2026-03-01T00:00:00Z")}
	got := usageKey("tenant-x", p, IngestTokens)
	if got == "" {
		t.Fatal("usageKey should be non-empty")
	}
}

func TestGetAllUsage(t *testing.T) {
	c, _ := setupTestClient(t)
	ctx := context.Background()
	plan := createPlan(t, c, "plan-all-usage")
	subscribe(t, c, "tenant-usage", plan)

	if err := c.Increment(ctx, IncrementRequest{
		TenantID:    "tenant-usage",
		Metric:      Events,
		Amount:      2,
		OperationID: "op-usage-1",
	}); err != nil {
		t.Fatal(err)
	}
	all, err := c.GetAllUsage(ctx, "tenant-usage")
	if err != nil {
		t.Fatal(err)
	}
	if all[Events] != 2 {
		t.Fatalf("expected events usage 2, got %d", all[Events])
	}
}

func TestStoreCreateLedgerEntryValidation(t *testing.T) {
	c, _ := setupTestClient(t)
	ctx := context.Background()
	err := c.store.CreateLedgerEntry(ctx, LedgerEntry{})
	if err == nil {
		t.Fatal("expected invalid ledger entry error")
	}
}

func TestCheckPropagatesRedisFailure(t *testing.T) {
	c, _ := setupTestClient(t)
	ctx := context.Background()
	plan := createPlan(t, c, "plan-redis-down")
	subscribe(t, c, "tenant-down", plan)

	if err := c.redis.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Check(ctx, "tenant-down", IngestTokens); err == nil {
		t.Fatal("expected redis error")
	}
}

func TestPlanLookupDoesNotMaskDBFailures(t *testing.T) {
	c, _ := setupTestClient(t)
	ctx := context.Background()

	sqlDB, err := c.store.db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}

	_, err = c.GetPlanByName(ctx, "anything")
	if err == nil {
		t.Fatal("expected db error")
	}
	if errors.Is(err, ErrPlanNotFound) {
		t.Fatalf("unexpected ErrPlanNotFound masking db error: %v", err)
	}
}

func TestSeedDefaultPlansPropagatesDBFailures(t *testing.T) {
	c, _ := setupTestClient(t)
	ctx := context.Background()

	sqlDB, err := c.store.db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}

	err = c.SeedDefaultPlans(ctx)
	if err == nil {
		t.Fatal("expected db error")
	}
	if errors.Is(err, ErrPlanNotFound) {
		t.Fatalf("unexpected ErrPlanNotFound masking db error: %v", err)
	}
}

func TestPlanAndSubscriptionNotFoundPaths(t *testing.T) {
	c, _ := setupTestClient(t)
	ctx := context.Background()

	if _, err := c.GetPlan(ctx, "missing-plan"); !errors.Is(err, ErrPlanNotFound) {
		t.Fatalf("expected ErrPlanNotFound, got %v", err)
	}
	if _, err := c.GetSubscription(ctx, "missing-tenant"); !errors.Is(err, ErrSubscriptionNotFound) {
		t.Fatalf("expected ErrSubscriptionNotFound, got %v", err)
	}
}

func TestGetPlanAndNameValidation(t *testing.T) {
	c, _ := setupTestClient(t)
	ctx := context.Background()

	if _, err := c.GetPlan(ctx, ""); !errors.Is(err, ErrInvalidPlanID) {
		t.Fatalf("expected ErrInvalidPlanID, got %v", err)
	}
	if _, err := c.GetPlanByName(ctx, "   "); !errors.Is(err, ErrInvalidPlan) {
		t.Fatalf("expected ErrInvalidPlan, got %v", err)
	}
}

func TestDefaultOptionsAndModeNormalization(t *testing.T) {
	opts := DefaultOptions()
	if opts.OperationTTL <= 0 || opts.ReservationTTL <= 0 || opts.Now == nil {
		t.Fatal("default options should be fully initialized")
	}

	c, _ := setupTestClient(t)
	ctx := context.Background()
	plan := createPlan(t, c, "plan-mode-normalization")
	subscribe(t, c, "tenant-mode", plan)

	period := CurrentPeriod(c.opts.Now(), c.opts.Now())
	enforcement := enforcementKey("tenant-mode", IngestTokens)
	if err := c.redis.Set(ctx, enforcement, "unknown-mode", 0).Err(); err != nil {
		t.Fatal(err)
	}
	usage := usageUsedKey("tenant-mode", period.Key(), IngestTokens)
	if err := c.redis.Set(ctx, usage, "not-an-int", 0).Err(); err != nil {
		t.Fatal(err)
	}

	if _, err := c.Check(ctx, "tenant-mode", IngestTokens); err == nil {
		t.Fatal("expected invalid redis integer parse error")
	}
}

func TestSetCanOverageValidation(t *testing.T) {
	c, _ := setupTestClient(t)
	ctx := context.Background()

	if err := c.setCanOverage(ctx, "", IngestTokens, true); !errors.Is(err, ErrInvalidTenantID) {
		t.Fatalf("expected ErrInvalidTenantID, got %v", err)
	}
	if err := c.setCanOverage(ctx, "tenant", Metric("bad"), true); !errors.Is(err, ErrInvalidMetric) {
		t.Fatalf("expected ErrInvalidMetric, got %v", err)
	}
}

func TestSubscribeRollsBackWhenQuotaSyncFails(t *testing.T) {
	c, _ := setupTestClient(t)
	ctx := context.Background()
	plan := createPlan(t, c, "plan-sub-rollback")

	if err := c.redis.Close(); err != nil {
		t.Fatal(err)
	}

	err := c.Subscribe(ctx, "tenant-sub-rollback", plan.ID, "polar")
	if err == nil {
		t.Fatal("expected subscribe error when redis is down")
	}

	_, err = c.store.GetSubscription(ctx, "tenant-sub-rollback")
	if !errors.Is(err, ErrSubscriptionNotFound) {
		t.Fatalf("expected subscription rollback, got %v", err)
	}
}
