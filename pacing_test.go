package main

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"bou.ke/monkey"
	"github.com/stretchr/testify/assert"
)

type lineTimesResult struct {
	times []time.Time
	err   error
}

type observedConn struct {
	net.Conn
	writeDelay time.Duration
	writeErr   error
	mu         sync.Mutex
	closes     int
}

func (conn *observedConn) Write(data []byte) (int, error) {
	if conn.writeDelay > 0 {
		timer := time.NewTimer(conn.writeDelay)
		<-timer.C
	}
	if conn.writeErr != nil {
		return 0, conn.writeErr
	}
	return conn.Conn.Write(data)
}

func (conn *observedConn) Close() error {
	conn.mu.Lock()
	conn.closes++
	conn.mu.Unlock()
	return conn.Conn.Close()
}

func (conn *observedConn) closeCount() int {
	conn.mu.Lock()
	defer conn.mu.Unlock()
	return conn.closes
}

func collectLineTimes(conn net.Conn) <-chan lineTimesResult {
	result := make(chan lineTimesResult, 1)
	go func() {
		reader := bufio.NewReader(conn)
		var times []time.Time
		for {
			_, err := reader.ReadString('\n')
			if err != nil {
				result <- lineTimesResult{times: times, err: err}
				return
			}
			times = append(times, time.Now())
		}
	}()
	return result
}

func waitForLineTimes(t *testing.T, result <-chan lineTimesResult) lineTimesResult {
	t.Helper()
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case got := <-result:
		return got
	case <-timer.C:
		t.Fatal("timed out waiting for generated logs")
		return lineTimesResult{}
	}
}

func TestGenerateContextUsesPlannedEPSTimestamps(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()
	conn := &observedConn{Conn: client}

	writerPatch := monkey.Patch(NewWriterContext, func(context.Context, string, string, string) (io.WriteCloser, error) {
		return conn, nil
	})
	defer writerPatch.Unpatch()

	var timestamps []time.Time
	logPatch := monkey.Patch(NewLog, func(_ string, timestamp time.Time) string {
		timestamps = append(timestamps, timestamp)
		return "event"
	})
	defer logPatch.Unpatch()

	lines := collectLineTimes(server)
	err := GenerateContext(context.Background(), &Option{Format: "apache_common", Type: "tcp", Target: "ignored", Number: 3, EPS: 100})
	got := waitForLineTimes(t, lines)

	assert.NoError(t, err)
	assert.Len(t, got.times, 3)
	assert.Len(t, timestamps, 3)
	assert.Equal(t, 10*time.Millisecond, timestamps[1].Sub(timestamps[0]))
	assert.Equal(t, 10*time.Millisecond, timestamps[2].Sub(timestamps[1]))
}

func TestGenerateContextPacesEPSWithoutAccumulatingWriteTime(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()
	conn := &observedConn{Conn: client, writeDelay: 120 * time.Millisecond}

	writerPatch := monkey.Patch(NewWriterContext, func(context.Context, string, string, string) (io.WriteCloser, error) {
		return conn, nil
	})
	defer writerPatch.Unpatch()

	lines := collectLineTimes(server)
	err := GenerateContext(context.Background(), &Option{Format: "apache_common", Type: "tcp", Target: "ignored", Number: 3, EPS: 5})
	got := waitForLineTimes(t, lines)

	if !assert.NoError(t, err) || !assert.Len(t, got.times, 3) {
		return
	}
	for index := 1; index < len(got.times); index++ {
		interval := got.times[index].Sub(got.times[index-1])
		assert.True(t, interval >= 170*time.Millisecond)
		assert.True(t, interval <= 270*time.Millisecond)
	}
}

func TestGenerateContextStartsDurationAfterWriterInitialization(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()
	conn := &observedConn{Conn: client}
	initialized := make(chan struct{})
	release := make(chan struct{})

	writerPatch := monkey.Patch(NewWriterContext, func(context.Context, string, string, string) (io.WriteCloser, error) {
		close(initialized)
		<-release
		return conn, nil
	})
	defer writerPatch.Unpatch()

	lines := collectLineTimes(server)
	done := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		done <- GenerateContext(ctx, &Option{Format: "apache_common", Type: "tcp", Target: "ignored", Forever: true, EPS: 10, Duration: 50 * time.Millisecond})
	}()

	<-initialized
	timer := time.NewTimer(100 * time.Millisecond)
	<-timer.C
	close(release)

	assert.NoError(t, waitForGeneration(t, done))
	got := waitForLineTimes(t, lines)
	assert.NotEmpty(t, got.times)
}

func TestGenerateContextStopsAtDurationBeforeSecondEPSEvent(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()
	conn := &observedConn{Conn: client}

	writerPatch := monkey.Patch(NewWriterContext, func(context.Context, string, string, string) (io.WriteCloser, error) {
		return conn, nil
	})
	defer writerPatch.Unpatch()

	lines := collectLineTimes(server)
	started := time.Now()
	err := GenerateContext(context.Background(), &Option{Format: "apache_common", Type: "tcp", Target: "ignored", Number: 2, EPS: 1, Duration: 50 * time.Millisecond})
	got := waitForLineTimes(t, lines)

	assert.NoError(t, err)
	assert.True(t, time.Since(started) >= 40*time.Millisecond)
	assert.Len(t, got.times, 1)
	assert.Equal(t, 1, conn.closeCount())
}

func TestGenerateContextReturnsParentCancellationDuringEPSWait(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()
	conn := &observedConn{Conn: client}

	writerPatch := monkey.Patch(NewWriterContext, func(context.Context, string, string, string) (io.WriteCloser, error) {
		return conn, nil
	})
	defer writerPatch.Unpatch()

	firstLine := readFirstLine(server)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- GenerateContext(ctx, &Option{Format: "apache_common", Type: "tcp", Target: "ignored", Forever: true, EPS: 1})
	}()

	timer := time.NewTimer(time.Second)
	select {
	case <-timer.C:
		t.Fatal("timed out waiting for the first event")
	case err := <-firstLine:
		assert.NoError(t, err)
	}
	cancel()

	assert.True(t, errors.Is(waitForGeneration(t, done), context.Canceled))
}

func TestGenerateContextReturnsGenuineWriteErrorBeforeDurationExpiry(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()
	writeErr := errors.New("write failed")
	conn := &observedConn{Conn: client, writeErr: writeErr}

	writerPatch := monkey.Patch(NewWriterContext, func(context.Context, string, string, string) (io.WriteCloser, error) {
		return conn, nil
	})
	defer writerPatch.Unpatch()

	err := GenerateContext(context.Background(), &Option{Format: "apache_common", Type: "tcp", Target: "ignored", Number: 1, Duration: time.Second})
	assert.True(t, errors.Is(err, writeErr))
}

func waitForGeneration(t *testing.T, done <-chan error) error {
	t.Helper()
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case err := <-done:
		return err
	case <-timer.C:
		t.Fatal("timed out waiting for generation to finish")
		return nil
	}
}

func readFirstLine(conn net.Conn) <-chan error {
	result := make(chan error, 1)
	go func() {
		_, err := bufio.NewReader(conn).ReadString('\n')
		result <- err
	}()
	return result
}
