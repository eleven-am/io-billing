package billing

import (
	"context"
	"strconv"
	"time"
)

const (
	actionReserve = "reserve"
	actionCommit  = "commit"
	actionRelease = "release"
)

type Reservation struct {
	ID          string `json:"id"`
	TenantID    string `json:"tenant_id"`
	Metric      Metric `json:"metric"`
	Amount      int64  `json:"amount"`
	PeriodKey   string `json:"period_key"`
	PeriodFrom  string `json:"period_from"`
	PeriodTo    string `json:"period_to"`
	OperationID string `json:"operation_id"`
	CreatedAt   string `json:"created_at"`
}

func (c *Client) Reserve(ctx context.Context, req ReserveRequest) (*Reservation, error) {
	if err := validateTenantID(req.TenantID); err != nil {
		return nil, err
	}
	if !req.Metric.Valid() {
		return nil, ErrInvalidMetric
	}
	if req.Amount <= 0 {
		return nil, ErrInvalidAmount
	}
	if err := validateOperationID(req.OperationID); err != nil {
		return nil, err
	}

	sub, plan, dim, period, err := c.loadContext(ctx, req.TenantID, req.Metric)
	if err != nil {
		return nil, err
	}

	now := c.opts.Now()
	reservationID := newReservationID()
	opValue := encodeReserveOpValue(reservationID, req.Amount)

	result, err := reserveScript.Run(ctx, c.redis, []string{
		usageUsedKey(req.TenantID, period.Key(), req.Metric),
		usageReservedKey(req.TenantID, period.Key(), req.Metric),
		quotaKey(req.TenantID, req.Metric),
		enforcementKey(req.TenantID, req.Metric),
		operationKey(req.TenantID, actionReserve, req.OperationID),
		reservationKey(reservationID),
	},
		req.Amount,
		reservationID,
		int(c.opts.ReservationTTL.Seconds()),
		int(c.opts.OperationTTL.Seconds()),
		opValue,
		req.TenantID,
		string(req.Metric),
		period.Start.Format(time.RFC3339),
		period.End.Format(time.RFC3339),
		period.Key(),
		req.OperationID,
		now.Format(time.RFC3339),
	).Result()
	if err != nil {
		return nil, err
	}

	status, payload, used, reserved, limit, err := parseReserveScriptResult(result)
	if err != nil {
		return nil, err
	}
	if status == 0 {
		return nil, &QuotaExceededError{
			Metric:    req.Metric,
			Used:      used + reserved,
			Limit:     limit,
			Estimated: req.Amount,
		}
	}
	if status == -1 {
		return nil, ErrInvalidAmount
	}

	var reservation *Reservation
	switch status {
	case 1:
		reservation = &Reservation{
			ID:          reservationID,
			TenantID:    req.TenantID,
			Metric:      req.Metric,
			Amount:      req.Amount,
			PeriodKey:   period.Key(),
			PeriodFrom:  period.Start.Format(time.RFC3339),
			PeriodTo:    period.End.Format(time.RFC3339),
			OperationID: req.OperationID,
			CreatedAt:   now.Format(time.RFC3339),
		}
	case 2:
		existingReservationID, amount, decodeErr := decodeReserveOpValue(payload)
		if decodeErr != nil || amount != req.Amount {
			return nil, ErrOperationConflict
		}
		reservation, err = c.loadReservation(ctx, existingReservationID)
		if err != nil {
			return nil, err
		}
	default:
		return nil, ErrOperationConflict
	}

	entry := LedgerEntry{
		TenantID:            req.TenantID,
		SubscriptionID:      sub.ID,
		PlanID:              plan.ID,
		Metric:              req.Metric,
		Action:              actionReserve,
		OperationID:         req.OperationID,
		PeriodStart:         reservation.PeriodKey,
		PeriodEnd:           period.End.Format("2006-01-02"),
		Units:               req.Amount,
		ReservedUnits:       req.Amount,
		IncludedSnapshot:    dim.Included,
		OverageRateSnapshot: dim.OverageRate,
		Unit:                dim.Unit,
		Metadata: map[string]any{
			"idempotent":     status == 2,
			"reservation_id": reservation.ID,
		},
	}
	if err := c.store.CreateLedgerEntry(ctx, entry); err != nil {
		return nil, err
	}

	return reservation, nil
}

func (c *Client) Commit(ctx context.Context, req CommitRequest) error {
	if req.Reservation == nil {
		return ErrReservationNotFound
	}
	if err := validateTenantID(req.Reservation.TenantID); err != nil {
		return err
	}
	if req.Actual < 0 {
		return ErrInvalidAmount
	}
	if err := validateOperationID(req.OperationID); err != nil {
		return err
	}

	sub, plan, dim, period, err := c.loadContext(ctx, req.Reservation.TenantID, req.Reservation.Metric)
	if err != nil {
		return err
	}
	if req.Reservation.PeriodKey == "" {
		return ErrReservationNotFound
	}
	if req.Reservation.PeriodKey != "" {
		start := mustParseRFC3339Date(req.Reservation.PeriodFrom)
		end := mustParseRFC3339Date(req.Reservation.PeriodTo)
		if !start.IsZero() && !end.IsZero() {
			period = Period{
				Start: start,
				End:   end,
			}
		}
	}

	opValue := encodeOpValue(req.OperationID, req.Actual)
	result, err := commitScript.Run(ctx, c.redis, []string{
		usageUsedKey(req.Reservation.TenantID, req.Reservation.PeriodKey, req.Reservation.Metric),
		usageReservedKey(req.Reservation.TenantID, req.Reservation.PeriodKey, req.Reservation.Metric),
		operationKey(req.Reservation.TenantID, actionCommit, req.OperationID),
		reservationKey(req.Reservation.ID),
	}, req.Actual, opValue, int(c.opts.OperationTTL.Seconds())).Result()
	if err != nil {
		return err
	}

	status, payload, _, _, reservedAmount, err := parseCommitScriptResult(result)
	if err != nil {
		return err
	}
	if status == -1 {
		return ErrReservationNotFound
	}
	if status == -2 {
		return ErrInvalidAmount
	}
	if status == 2 {
		opID, amount, decodeErr := decodeOpValue(payload)
		if decodeErr != nil || opID != req.OperationID || amount != req.Actual {
			return ErrOperationConflict
		}
	}

	entry := LedgerEntry{
		TenantID:            req.Reservation.TenantID,
		SubscriptionID:      sub.ID,
		PlanID:              plan.ID,
		Metric:              req.Reservation.Metric,
		Action:              actionCommit,
		OperationID:         req.OperationID,
		PeriodStart:         req.Reservation.PeriodKey,
		PeriodEnd:           period.End.Format("2006-01-02"),
		Units:               req.Actual,
		ReservedUnits:       reservedAmount,
		IncludedSnapshot:    dim.Included,
		OverageRateSnapshot: dim.OverageRate,
		Unit:                dim.Unit,
		Metadata: map[string]any{
			"idempotent":     status == 2,
			"reservation_id": req.Reservation.ID,
		},
	}
	return c.store.CreateLedgerEntry(ctx, entry)
}

func (c *Client) Release(ctx context.Context, req ReleaseRequest) error {
	if req.Reservation == nil {
		return ErrReservationNotFound
	}
	if err := validateTenantID(req.Reservation.TenantID); err != nil {
		return err
	}
	if err := validateOperationID(req.OperationID); err != nil {
		return err
	}

	sub, plan, dim, period, err := c.loadContext(ctx, req.Reservation.TenantID, req.Reservation.Metric)
	if err != nil {
		return err
	}
	if req.Reservation.PeriodKey == "" {
		return ErrReservationNotFound
	}
	if req.Reservation.PeriodKey != "" {
		start := mustParseRFC3339Date(req.Reservation.PeriodFrom)
		end := mustParseRFC3339Date(req.Reservation.PeriodTo)
		if !start.IsZero() && !end.IsZero() {
			period = Period{
				Start: start,
				End:   end,
			}
		}
	}

	opValue := encodeOpValue(req.OperationID, req.Reservation.Amount)
	result, err := releaseScript.Run(ctx, c.redis, []string{
		usageReservedKey(req.Reservation.TenantID, req.Reservation.PeriodKey, req.Reservation.Metric),
		operationKey(req.Reservation.TenantID, actionRelease, req.OperationID),
		reservationKey(req.Reservation.ID),
	}, opValue, int(c.opts.OperationTTL.Seconds())).Result()
	if err != nil {
		return err
	}

	status, payload, released, err := parseReleaseScriptResult(result)
	if err != nil {
		return err
	}
	if status == -1 {
		return ErrReservationNotFound
	}
	if status == 2 {
		opID, amount, decodeErr := decodeOpValue(payload)
		if decodeErr != nil || opID != req.OperationID {
			return ErrOperationConflict
		}
		released = amount
	}

	entry := LedgerEntry{
		TenantID:            req.Reservation.TenantID,
		SubscriptionID:      sub.ID,
		PlanID:              plan.ID,
		Metric:              req.Reservation.Metric,
		Action:              actionRelease,
		OperationID:         req.OperationID,
		PeriodStart:         req.Reservation.PeriodKey,
		PeriodEnd:           period.End.Format("2006-01-02"),
		Units:               released,
		ReservedUnits:       released,
		IncludedSnapshot:    dim.Included,
		OverageRateSnapshot: dim.OverageRate,
		Unit:                dim.Unit,
		Metadata: map[string]any{
			"idempotent":     status == 2,
			"reservation_id": req.Reservation.ID,
		},
	}
	return c.store.CreateLedgerEntry(ctx, entry)
}

func parseReserveScriptResult(result any) (status int64, payload string, used int64, reserved int64, limit int64, err error) {
	values, ok := result.([]any)
	if !ok || len(values) < 5 {
		return 0, "", 0, 0, 0, ErrOperationConflict
	}
	status, err = toInt64(values[0])
	if err != nil {
		return 0, "", 0, 0, 0, err
	}
	payload = toString(values[1])
	used, err = toInt64(values[2])
	if err != nil {
		return 0, "", 0, 0, 0, err
	}
	reserved, err = toInt64(values[3])
	if err != nil {
		return 0, "", 0, 0, 0, err
	}
	limit, err = toInt64(values[4])
	if err != nil {
		return 0, "", 0, 0, 0, err
	}
	return status, payload, used, reserved, limit, nil
}

func parseCommitScriptResult(result any) (status int64, payload string, used int64, reserved int64, released int64, err error) {
	values, ok := result.([]any)
	if !ok || len(values) < 5 {
		return 0, "", 0, 0, 0, ErrOperationConflict
	}
	status, err = toInt64(values[0])
	if err != nil {
		return 0, "", 0, 0, 0, err
	}
	payload = toString(values[1])
	used, err = toInt64(values[2])
	if err != nil {
		return 0, "", 0, 0, 0, err
	}
	reserved, err = toInt64(values[3])
	if err != nil {
		return 0, "", 0, 0, 0, err
	}
	released, err = toInt64(values[4])
	if err != nil {
		return 0, "", 0, 0, 0, err
	}
	return status, payload, used, reserved, released, nil
}

func parseReleaseScriptResult(result any) (status int64, payload string, released int64, err error) {
	values, ok := result.([]any)
	if !ok || len(values) < 4 {
		return 0, "", 0, ErrOperationConflict
	}
	status, err = toInt64(values[0])
	if err != nil {
		return 0, "", 0, err
	}
	payload = toString(values[1])
	released, err = toInt64(values[3])
	if err != nil {
		return 0, "", 0, err
	}
	return status, payload, released, nil
}

func (c *Client) loadReservation(ctx context.Context, reservationID string) (*Reservation, error) {
	fields, err := c.redis.HGetAll(ctx, reservationKey(reservationID)).Result()
	if err != nil || len(fields) == 0 {
		return nil, ErrReservationNotFound
	}
	amount, parseErr := strconv.ParseInt(fields["amount"], 10, 64)
	if parseErr != nil {
		return nil, ErrOperationConflict
	}
	return &Reservation{
		ID:          fields["id"],
		TenantID:    fields["tenant_id"],
		Metric:      Metric(fields["metric"]),
		Amount:      amount,
		PeriodKey:   fields["period_key"],
		PeriodFrom:  fields["period_start"],
		PeriodTo:    fields["period_end"],
		OperationID: fields["operation_id"],
		CreatedAt:   fields["created_at"],
	}, nil
}

func mustParseRFC3339Date(input string) time.Time {
	if input == "" {
		return time.Time{}
	}
	ts, err := time.Parse(time.RFC3339, input)
	if err != nil {
		return time.Time{}
	}
	return ts
}
