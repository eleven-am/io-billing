package billing

import "strings"

func validatePlan(plan Plan) error {
	if strings.TrimSpace(plan.Name) == "" {
		return ErrInvalidPlan
	}
	if plan.PriceEUR < 0 {
		return ErrInvalidPlan
	}
	if len(plan.Dimensions) == 0 {
		return ErrInvalidPlan
	}

	for metric, dim := range plan.Dimensions {
		if !metric.Valid() {
			return ErrInvalidPlan
		}
		if dim.Included < 0 || dim.OverageRate < 0 {
			return ErrInvalidPlan
		}
		if strings.TrimSpace(dim.Unit) == "" {
			return ErrInvalidPlan
		}
		if !dim.Enforcement.Valid() {
			return ErrInvalidPlan
		}
		if dim.Enforcement == EnforcementHardCap && dim.OverageRate != 0 {
			return ErrInvalidPlan
		}
		if dim.Enforcement == EnforcementSoftCap && dim.OverageRate <= 0 {
			return ErrInvalidPlan
		}
	}
	return nil
}

func validateOperationID(id string) error {
	if strings.TrimSpace(id) == "" {
		return ErrInvalidOperationID
	}
	return nil
}
