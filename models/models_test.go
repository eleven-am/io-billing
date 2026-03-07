package models

import "testing"

func TestTableNames(t *testing.T) {
	if (BillingPlan{}).TableName() != "billing_plans" {
		t.Fatal("unexpected billing_plans table name")
	}
	if (BillingPlanDimension{}).TableName() != "billing_plan_dimensions" {
		t.Fatal("unexpected billing_plan_dimensions table name")
	}
	if (BillingTenantSubscription{}).TableName() != "billing_tenant_subscriptions" {
		t.Fatal("unexpected billing_tenant_subscriptions table name")
	}
	if (BillingUsageLedger{}).TableName() != "billing_usage_ledger" {
		t.Fatal("unexpected billing_usage_ledger table name")
	}
}
