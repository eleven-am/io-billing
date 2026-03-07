package billing

import "time"

type ReserveRequest struct {
	TenantID    string
	Metric      Metric
	Amount      int64
	OperationID string
}

type CommitRequest struct {
	Reservation *Reservation
	Actual      int64
	OperationID string
}

type ReleaseRequest struct {
	Reservation *Reservation
	OperationID string
}

type IncrementRequest struct {
	TenantID    string
	Metric      Metric
	Amount      int64
	OperationID string
}

type QuotaAdmission struct {
	Allowed     bool
	Reason      string
	Metric      Metric
	Requested   int64
	Used        int64
	Reserved    int64
	Limit       int64
	Remaining   int64
	CanOverage  bool
	Enforcement EnforcementMode
	CheckedAt   time.Time
}
