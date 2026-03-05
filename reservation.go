package billing

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"time"
)

type Reservation struct {
	ID       string `json:"id"`
	TenantID string `json:"tenant_id"`
	Metric   Metric `json:"metric"`
	Amount   int64  `json:"amount"`
}

func (c *Client) Reserve(ctx context.Context, tenantID string, metric Metric, estimated int64) (*Reservation, error) {
	if !metric.Valid() {
		return nil, ErrInvalidMetric
	}

	status, err := c.Check(ctx, tenantID, metric)
	if err != nil {
		return nil, err
	}

	if status.Exceeded && !status.CanOverage {
		return nil, &QuotaExceededError{
			Metric:    metric,
			Used:      status.Used,
			Limit:     status.Limit,
			Estimated: estimated,
		}
	}

	wouldUse := status.Used + estimated
	if wouldUse > status.Limit && !status.CanOverage {
		return nil, &QuotaExceededError{
			Metric:    metric,
			Used:      status.Used,
			Limit:     status.Limit,
			Estimated: estimated,
		}
	}

	sub, err := c.store.GetSubscription(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	period := CurrentPeriod(sub.StartedAt, time.Now())
	key := usageKey(tenantID, period, metric)
	if err := c.redis.IncrBy(ctx, key, estimated).Err(); err != nil {
		return nil, err
	}

	reservation := &Reservation{
		ID:       newReservationID(),
		TenantID: tenantID,
		Metric:   metric,
		Amount:   estimated,
	}

	data, err := json.Marshal(reservation)
	if err != nil {
		return nil, err
	}

	rKey := reservationKey(reservation.ID)
	if err := c.redis.Set(ctx, rKey, data, 24*time.Hour).Err(); err != nil {
		return nil, err
	}

	return reservation, nil
}

func (c *Client) Reconcile(ctx context.Context, reservation *Reservation, actual int64) error {
	if reservation == nil {
		return ErrReservationNotFound
	}

	diff := actual - reservation.Amount

	sub, err := c.store.GetSubscription(ctx, reservation.TenantID)
	if err != nil {
		return err
	}

	period := CurrentPeriod(sub.StartedAt, time.Now())
	key := usageKey(reservation.TenantID, period, reservation.Metric)

	if diff != 0 {
		if err := c.redis.IncrBy(ctx, key, diff).Err(); err != nil {
			return err
		}
	}

	rKey := reservationKey(reservation.ID)
	return c.redis.Del(ctx, rKey).Err()
}

func (c *Client) ReleaseReservation(ctx context.Context, reservation *Reservation) error {
	if reservation == nil {
		return ErrReservationNotFound
	}

	sub, err := c.store.GetSubscription(ctx, reservation.TenantID)
	if err != nil {
		return err
	}

	period := CurrentPeriod(sub.StartedAt, time.Now())
	key := usageKey(reservation.TenantID, period, reservation.Metric)

	if err := c.redis.DecrBy(ctx, key, reservation.Amount).Err(); err != nil {
		return err
	}

	rKey := reservationKey(reservation.ID)
	return c.redis.Del(ctx, rKey).Err()
}

func newReservationID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}
