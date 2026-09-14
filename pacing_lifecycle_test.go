package main

import (
	"context"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"bou.ke/monkey"
	"github.com/stretchr/testify/assert"
)

type blockingWriteConn struct {
	net.Conn
	writeStarted chan struct{}
	closed       chan struct{}
	once         sync.Once
	mu           sync.Mutex
	closes       int
}

func (conn *blockingWriteConn) Write([]byte) (int, error) {
	close(conn.writeStarted)
	<-conn.closed
	return 0, net.ErrClosed
}

func (conn *blockingWriteConn) Close() error {
	conn.mu.Lock()
	conn.closes++
	conn.mu.Unlock()
	conn.once.Do(func() {
		close(conn.closed)
	})
	return conn.Conn.Close()
}

func (conn *blockingWriteConn) closeCount() int {
	conn.mu.Lock()
	defer conn.mu.Unlock()
	return conn.closes
}

func TestGenerateContextSchedulesFirstEPSEventImmediately(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()
	conn := &observedConn{Conn: client}

	writerPatch := monkey.Patch(NewWriterContext, func(context.Context, string, string, string) (io.WriteCloser, error) {
		return conn, nil
	})
	defer writerPatch.Unpatch()

	lines := collectLineTimes(server)
	started := time.Now()
	err := GenerateContext(context.Background(), &Option{Format: "apache_common", Type: "tcp", Target: "ignored", Number: 1, EPS: 1})
	got := waitForLineTimes(t, lines)

	assert.NoError(t, err)
	if assert.Len(t, got.times, 1) {
		assert.True(t, got.times[0].Sub(started) < 200*time.Millisecond)
	}
}

func TestGenerateContextStopsAtNumberBeforeDuration(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()
	conn := &observedConn{Conn: client}

	writerPatch := monkey.Patch(NewWriterContext, func(context.Context, string, string, string) (io.WriteCloser, error) {
		return conn, nil
	})
	defer writerPatch.Unpatch()

	lines := collectLineTimes(server)
	started := time.Now()
	err := GenerateContext(context.Background(), &Option{Format: "apache_common", Type: "tcp", Target: "ignored", Number: 2, Duration: time.Second})
	got := waitForLineTimes(t, lines)

	assert.NoError(t, err)
	assert.True(t, time.Since(started) < 200*time.Millisecond)
	assert.Len(t, got.times, 2)
}

func TestGenerateContextNormalizesDurationClosingBlockedNetworkWrite(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()
	conn := &blockingWriteConn{
		Conn:         client,
		writeStarted: make(chan struct{}),
		closed:       make(chan struct{}),
	}

	writerPatch := monkey.Patch(NewWriterContext, func(context.Context, string, string, string) (io.WriteCloser, error) {
		return conn, nil
	})
	defer writerPatch.Unpatch()

	done := make(chan error, 1)
	go func() {
		done <- GenerateContext(context.Background(), &Option{Format: "apache_common", Type: "tcp", Target: "ignored", Number: 1, EPS: 1, Duration: 50 * time.Millisecond})
	}()

	timer := time.NewTimer(time.Second)
	select {
	case <-conn.writeStarted:
	case <-timer.C:
		t.Fatal("timed out waiting for the blocked write")
	}

	assert.NoError(t, waitForGeneration(t, done))
	assert.Equal(t, 1, conn.closeCount())
}
