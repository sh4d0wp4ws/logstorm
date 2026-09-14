package main

import (
	"context"
	"errors"
	"math/bits"
	"net"
	"sync/atomic"
	"time"
)

const maxDuration = time.Duration(1<<63 - 1)

type generationPacer struct {
	parent           context.Context
	ctx              context.Context
	cancel           context.CancelFunc
	start            time.Time
	deadline         time.Time
	duration         time.Duration
	eps              int
	eventIndex       uint64
	closedByDuration atomic.Bool
}

func newGenerationPacer(parent context.Context, start time.Time, eps int, duration time.Duration) *generationPacer {
	pacer := &generationPacer{
		parent:   parent,
		ctx:      parent,
		cancel:   func() {},
		start:    start,
		duration: duration,
		eps:      eps,
	}
	if duration > 0 {
		pacer.deadline = start.Add(duration)
		pacer.ctx, pacer.cancel = context.WithDeadline(parent, pacer.deadline)
	}
	return pacer
}

func (pacer *generationPacer) Context() context.Context {
	return pacer.ctx
}

func (pacer *generationPacer) Cancel() {
	pacer.cancel()
}

func (pacer *generationPacer) Wait(delay time.Duration) (bool, error) {
	if stopped, err := pacer.Stopped(); stopped {
		return true, err
	}

	var err error
	if pacer.eps > 0 {
		err = waitUntil(pacer.ctx, pacer.scheduledAt())
	} else {
		err = waitForDelay(pacer.ctx, delay)
	}
	if err != nil {
		if stopped, stopErr := pacer.Stopped(); stopped {
			return true, stopErr
		}
		return false, err
	}
	return pacer.Stopped()
}

func (pacer *generationPacer) Stopped() (bool, error) {
	if pacer.eps == 0 && pacer.duration == 0 {
		return false, nil
	}
	if err := pacer.parent.Err(); err != nil {
		return true, err
	}
	if pacer.duration > 0 && !time.Now().Before(pacer.deadline) {
		return true, nil
	}
	if err := pacer.ctx.Err(); err != nil {
		return true, err
	}
	return false, nil
}

func (pacer *generationPacer) Timestamp(created time.Time) time.Time {
	if pacer.eps == 0 {
		return created
	}
	return pacer.scheduledAt()
}

func (pacer *generationPacer) Wrote() {
	if pacer.eps > 0 && pacer.eventIndex < ^uint64(0) {
		pacer.eventIndex++
	}
}

func (pacer *generationPacer) WriteError(err error) error {
	if parentErr := pacer.parent.Err(); parentErr != nil {
		return parentErr
	}
	if pacer.closedByDuration.Load() && errors.Is(err, net.ErrClosed) {
		return nil
	}
	return err
}

func (pacer *generationPacer) MarkDurationWriterClose() {
	if pacer.duration > 0 && !time.Now().Before(pacer.deadline) {
		pacer.closedByDuration.Store(true)
	}
}

func (pacer *generationPacer) scheduledAt() time.Time {
	eps := uint64(pacer.eps)
	seconds := pacer.eventIndex / eps
	if seconds > uint64(maxDuration/time.Second) {
		return pacer.start.Add(maxDuration)
	}
	productHigh, productLow := bits.Mul64(pacer.eventIndex%eps, uint64(time.Second))
	quotient, _ := bits.Div64(productHigh, productLow, eps)
	fraction := time.Duration(quotient)
	wholeSeconds := time.Duration(seconds) * time.Second
	if fraction > maxDuration-wholeSeconds {
		return pacer.start.Add(maxDuration)
	}
	return pacer.start.Add(wholeSeconds + fraction)
}

func waitUntil(ctx context.Context, scheduledAt time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	delay := time.Until(scheduledAt)
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
