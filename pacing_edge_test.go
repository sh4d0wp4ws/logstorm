package main

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGenerationPacerUsesNanosecondTimelineAtOneBillionEPS(t *testing.T) {
	start := time.Unix(0, 0)
	pacer := newGenerationPacer(context.Background(), start, int(time.Second), 0)
	pacer.eventIndex = uint64(int(time.Second) - 1)

	assert.Equal(t, start.Add(time.Second-time.Nanosecond), pacer.scheduledAt())
}

func TestGenerationPacerAvoidsOverflowAtMaximumEPS(t *testing.T) {
	start := time.Unix(0, 0)
	maximumEPS := int(^uint(0) >> 1)
	pacer := newGenerationPacer(context.Background(), start, maximumEPS, 0)
	pacer.eventIndex = uint64(maximumEPS - 1)

	assert.True(t, pacer.scheduledAt().After(start))
	assert.True(t, pacer.scheduledAt().Before(start.Add(time.Second)))
}

func TestGenerationPacerSaturatesTimelineInsteadOfOverflowing(t *testing.T) {
	start := time.Unix(0, 0)
	pacer := newGenerationPacer(context.Background(), start, 1, 0)
	pacer.eventIndex = ^uint64(0)
	scheduledAt := pacer.scheduledAt()
	pacer.Wrote()

	assert.Equal(t, ^uint64(0), pacer.eventIndex)
	assert.Equal(t, scheduledAt, pacer.scheduledAt())
	assert.True(t, scheduledAt.After(start))
}

func TestGenerationPacerSaturatesWholeAndFractionalTimelineOverflow(t *testing.T) {
	if strconv.IntSize < 64 {
		t.Skip("event index cannot reach the duration overflow boundary on 32-bit platforms")
	}
	start := time.Unix(0, 0)
	seconds := int64(maxDuration / time.Second)
	pacer := newGenerationPacer(context.Background(), start, 10, 0)
	pacer.eventIndex = uint64(seconds*10 + 9)

	assert.Equal(t, start.Add(maxDuration), pacer.scheduledAt())
}

func TestGenerationPacerPreservesWriteErrorAfterDurationWithoutWriterClose(t *testing.T) {
	pacer := newGenerationPacer(context.Background(), time.Now().Add(-time.Second), 1, time.Millisecond)
	defer pacer.Cancel()
	writeErr := errors.New("write failed")

	assert.Equal(t, writeErr, pacer.WriteError(writeErr))
}

func TestGenerationPacerPreservesNonCloseWriteErrorWhenDurationClosesWriter(t *testing.T) {
	pacer := newGenerationPacer(context.Background(), time.Now().Add(-time.Second), 1, time.Millisecond)
	defer pacer.Cancel()
	pacer.closedByDuration.Store(true)
	writeErr := errors.New("write failed")

	assert.Equal(t, writeErr, pacer.WriteError(writeErr))
}
