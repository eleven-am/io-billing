package billing

import (
	"context"
	"errors"
	"testing"
)

func TestReserveValidationBranches(t *testing.T) {
	c, _ := setupTestClient(t)
	ctx := context.Background()
	plan := createPlan(t, c, "plan-res-val")
	subscribe(t, c, "tenant-rv", plan)

	_, err := c.Reserve(ctx, ReserveRequest{
		TenantID:    "tenant-rv",
		Metric:      "bad",
		Amount:      1,
		OperationID: "op",
	})
	if !errors.Is(err, ErrInvalidMetric) {
		t.Fatalf("expected ErrInvalidMetric, got %v", err)
	}

	_, err = c.Reserve(ctx, ReserveRequest{
		TenantID:    "tenant-rv",
		Metric:      IngestTokens,
		Amount:      0,
		OperationID: "op",
	})
	if !errors.Is(err, ErrInvalidAmount) {
		t.Fatalf("expected ErrInvalidAmount, got %v", err)
	}

	_, err = c.Reserve(ctx, ReserveRequest{
		TenantID:    "tenant-rv",
		Metric:      IngestTokens,
		Amount:      1,
		OperationID: "",
	})
	if !errors.Is(err, ErrInvalidOperationID) {
		t.Fatalf("expected ErrInvalidOperationID, got %v", err)
	}
}

func TestCommitReleaseValidationBranches(t *testing.T) {
	c, _ := setupTestClient(t)
	ctx := context.Background()
	plan := createPlan(t, c, "plan-cr-val")
	subscribe(t, c, "tenant-cr", plan)

	res, err := c.Reserve(ctx, ReserveRequest{
		TenantID:    "tenant-cr",
		Metric:      IngestTokens,
		Amount:      100,
		OperationID: "op-res-cr",
	})
	if err != nil {
		t.Fatal(err)
	}

	err = c.Commit(ctx, CommitRequest{
		Reservation: res,
		Actual:      -1,
		OperationID: "op-commit-neg",
	})
	if !errors.Is(err, ErrInvalidAmount) {
		t.Fatalf("expected ErrInvalidAmount, got %v", err)
	}

	err = c.Commit(ctx, CommitRequest{
		Reservation: res,
		Actual:      1,
		OperationID: "",
	})
	if !errors.Is(err, ErrInvalidOperationID) {
		t.Fatalf("expected ErrInvalidOperationID, got %v", err)
	}

	err = c.Release(ctx, ReleaseRequest{
		Reservation: res,
		OperationID: "",
	})
	if !errors.Is(err, ErrInvalidOperationID) {
		t.Fatalf("expected ErrInvalidOperationID, got %v", err)
	}
}

func TestIncrementValidationBranches(t *testing.T) {
	c, _ := setupTestClient(t)
	ctx := context.Background()
	plan := createPlan(t, c, "plan-inc-val")
	subscribe(t, c, "tenant-iv", plan)

	err := c.Increment(ctx, IncrementRequest{
		TenantID:    "tenant-iv",
		Metric:      "bad",
		Amount:      1,
		OperationID: "op",
	})
	if !errors.Is(err, ErrInvalidMetric) {
		t.Fatalf("expected ErrInvalidMetric, got %v", err)
	}
	err = c.Increment(ctx, IncrementRequest{
		TenantID:    "tenant-iv",
		Metric:      IngestTokens,
		Amount:      0,
		OperationID: "op",
	})
	if !errors.Is(err, ErrInvalidAmount) {
		t.Fatalf("expected ErrInvalidAmount, got %v", err)
	}
	err = c.Increment(ctx, IncrementRequest{
		TenantID:    "tenant-iv",
		Metric:      IngestTokens,
		Amount:      1,
		OperationID: "",
	})
	if !errors.Is(err, ErrInvalidOperationID) {
		t.Fatalf("expected ErrInvalidOperationID, got %v", err)
	}
}

func TestUpdatePlanValidationBranches(t *testing.T) {
	c, _ := setupTestClient(t)
	ctx := context.Background()
	err := c.UpdatePlan(ctx, Plan{})
	if !errors.Is(err, ErrInvalidPlan) {
		t.Fatalf("expected ErrInvalidPlan, got %v", err)
	}

	plan := createPlan(t, c, "plan-update-val")
	plan.Dimensions[IngestTokens] = Dimension{
		Included:    1,
		OverageRate: 0,
		Unit:        "tokens",
		Enforcement: EnforcementSoftCap,
	}
	err = c.UpdatePlan(ctx, plan)
	if !errors.Is(err, ErrInvalidPlan) {
		t.Fatalf("expected ErrInvalidPlan for soft cap without overage, got %v", err)
	}
}

func TestOptionsWithDefaultsBranches(t *testing.T) {
	opts := withDefaults(nil)
	if opts.Now == nil || opts.OperationTTL <= 0 || opts.ReservationTTL <= 0 {
		t.Fatal("expected non-zero defaults")
	}

	override := withDefaults(&Options{})
	if override.Now == nil {
		t.Fatal("expected default clock when nil override")
	}
}

func TestToInt64AndToStringBranches(t *testing.T) {
	if _, err := toInt64(1.2); !errors.Is(err, ErrOperationConflict) {
		t.Fatalf("expected ErrOperationConflict, got %v", err)
	}
	if s := toString(123); s != "" {
		t.Fatalf("expected empty string, got %q", s)
	}
}

func TestInvalidTenantValidationAcrossSurface(t *testing.T) {
	c, _ := setupTestClient(t)
	ctx := context.Background()

	if _, err := c.Check(ctx, "", IngestTokens); !errors.Is(err, ErrInvalidTenantID) {
		t.Fatalf("expected ErrInvalidTenantID, got %v", err)
	}
	if _, err := c.CheckMultiple(ctx, "", []Metric{IngestTokens}); !errors.Is(err, ErrInvalidTenantID) {
		t.Fatalf("expected ErrInvalidTenantID, got %v", err)
	}
	if err := c.SetQuota(ctx, "", IngestTokens, 1); !errors.Is(err, ErrInvalidTenantID) {
		t.Fatalf("expected ErrInvalidTenantID, got %v", err)
	}
	if err := c.Increment(ctx, IncrementRequest{
		TenantID:    "",
		Metric:      IngestTokens,
		Amount:      1,
		OperationID: "op",
	}); !errors.Is(err, ErrInvalidTenantID) {
		t.Fatalf("expected ErrInvalidTenantID, got %v", err)
	}
	if _, err := c.GetUsage(ctx, "", IngestTokens); !errors.Is(err, ErrInvalidTenantID) {
		t.Fatalf("expected ErrInvalidTenantID, got %v", err)
	}
	if _, err := c.GetOverageReport(ctx, ""); !errors.Is(err, ErrInvalidTenantID) {
		t.Fatalf("expected ErrInvalidTenantID, got %v", err)
	}
	if err := c.Subscribe(ctx, "", "plan", "polar"); !errors.Is(err, ErrInvalidTenantID) {
		t.Fatalf("expected ErrInvalidTenantID, got %v", err)
	}
}

func TestSetQuotaRejectsNegative(t *testing.T) {
	c, _ := setupTestClient(t)
	ctx := context.Background()
	err := c.SetQuota(ctx, "tenant", IngestTokens, -1)
	if !errors.Is(err, ErrInvalidAmount) {
		t.Fatalf("expected ErrInvalidAmount, got %v", err)
	}
}

func TestUpdatePlanNotFound(t *testing.T) {
	c, _ := setupTestClient(t)
	ctx := context.Background()

	err := c.UpdatePlan(ctx, Plan{
		ID:       "missing-plan",
		Name:     "ghost",
		PriceEUR: 1000,
		Active:   true,
		Dimensions: map[Metric]Dimension{
			IngestTokens: {Included: 1, OverageRate: 1, Unit: "tokens", Enforcement: EnforcementSoftCap},
		},
	})
	if !errors.Is(err, ErrPlanNotFound) {
		t.Fatalf("expected ErrPlanNotFound, got %v", err)
	}
}
