package billing

import "time"

type Options struct {
	ReservationTTL time.Duration
	OperationTTL   time.Duration
	Now            func() time.Time
}

func DefaultOptions() Options {
	return Options{
		ReservationTTL: 24 * time.Hour,
		OperationTTL:   45 * 24 * time.Hour,
		Now: func() time.Time {
			return time.Now().UTC()
		},
	}
}

func withDefaults(opts *Options) Options {
	base := DefaultOptions()
	if opts == nil {
		return base
	}
	if opts.ReservationTTL > 0 {
		base.ReservationTTL = opts.ReservationTTL
	}
	if opts.OperationTTL > 0 {
		base.OperationTTL = opts.OperationTTL
	}
	if opts.Now != nil {
		base.Now = opts.Now
	}
	return base
}
