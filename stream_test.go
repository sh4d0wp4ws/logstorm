package main

import (
	"bufio"
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRunStreamsReusesTCPConnectionsAndIndependentUDPSockets(t *testing.T) {
	tcpListener, err := net.Listen("tcp", "127.0.0.1:0")
	if !assert.NoError(t, err) {
		return
	}
	defer tcpListener.Close()
	udpOne, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if !assert.NoError(t, err) {
		return
	}
	defer udpOne.Close()
	udpTwo, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if !assert.NoError(t, err) {
		return
	}
	defer udpTwo.Close()

	tcpLines := make(chan []string, 2)
	go func() {
		for i := 0; i < 2; i++ {
			connection, err := tcpListener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer connection.Close()
				reader := bufio.NewReader(connection)
				lines := make([]string, 0, 2)
				for i := 0; i < 2; i++ {
					line, err := reader.ReadString('\n')
					if err != nil {
						return
					}
					lines = append(lines, line)
				}
				tcpLines <- lines
			}()
		}
	}()

	streams := []Stream{
		{Name: "tcp-one", Option: &Option{Format: "apache_common", Type: "tcp", Target: tcpListener.Addr().String(), Number: 2}},
		{Name: "tcp-two", Option: &Option{Format: "apache_combined", Type: "tcp", Target: tcpListener.Addr().String(), Number: 2}},
		{Name: "udp-one", Option: &Option{Format: "apache_common", Type: "udp", Target: udpOne.LocalAddr().String(), Number: 2}},
		{Name: "udp-two", Option: &Option{Format: "apache_common", Type: "udp", Target: udpTwo.LocalAddr().String(), Number: 2}},
	}
	if !assert.NoError(t, RunStreams(context.Background(), streams)) {
		return
	}

	for i := 0; i < 2; i++ {
		select {
		case lines := <-tcpLines:
			assert.Len(t, lines, 2)
			for _, line := range lines {
				assert.Equal(t, byte('\n'), line[len(line)-1])
			}
		case <-time.After(time.Second):
			t.Fatal("TCP listener did not receive both stream connections")
		}
	}

	sourceOne := readUDPStream(t, udpOne, 2)
	sourceTwo := readUDPStream(t, udpTwo, 2)
	assert.NotEqual(t, sourceOne, sourceTwo)
}

func TestRunStreamsDoesNotStopOtherStreamsAfterFiniteCompletion(t *testing.T) {
	listener, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if !assert.NoError(t, err) {
		return
	}
	defer listener.Close()

	streams := []Stream{
		{Name: "fast", Option: &Option{Format: "apache_common", Type: "udp", Target: listener.LocalAddr().String(), Number: 1}},
		{Name: "slow", Option: &Option{Format: "apache_common", Type: "udp", Target: listener.LocalAddr().String(), Number: 2, Delay: 50 * time.Millisecond}},
	}
	if !assert.NoError(t, RunStreams(context.Background(), streams)) {
		return
	}
	readUDPDatagrams(t, listener, 3)
}

func TestRunStreamsCancelsPeersAndReturnsNamedFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if !assert.NoError(t, err) {
		return
	}
	target := listener.Addr().String()
	listener.Close()

	started := time.Now()
	err = RunStreams(context.Background(), []Stream{
		{Name: "broken", Option: &Option{Format: "apache_common", Type: "tcp", Target: target, Number: 1}},
		{Name: "waiting", Option: &Option{Format: "apache_common", Type: "udp", Target: "127.0.0.1:9", Number: 1, Delay: time.Hour}},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), `stream "broken"`)
	assert.True(t, time.Since(started) < time.Second)
}

func TestGenerateContextCancellationInterruptsLongDelay(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- GenerateContext(ctx, &Option{
			Format: "apache_common",
			Type:   "udp",
			Target: "127.0.0.1:9",
			Number: 1,
			Delay:  time.Hour,
		})
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		assert.True(t, errors.Is(err, context.Canceled))
	case <-time.After(time.Second):
		t.Fatal("cancellation did not interrupt delay")
	}
}

func TestCancellationCloseOnlyClosesWriterOnce(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	writer := &countingWriter{}
	ownedWriter := &closeOnceWriter{WriteCloser: writer}
	stop := closeOnCancellation(ctx, ownedWriter)
	cancel()
	stop()
	assert.NoError(t, ownedWriter.Close())
	assert.Equal(t, 1, writer.closeCount())
}

func readUDPStream(t *testing.T, listener *net.UDPConn, count int) string {
	t.Helper()
	var source string
	for i := 0; i < count; i++ {
		if err := listener.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
			t.Fatal(err)
		}
		buffer := make([]byte, 4096)
		n, address, err := listener.ReadFromUDP(buffer)
		if err != nil {
			t.Fatal(err)
		}
		assert.Equal(t, byte('\n'), buffer[n-1])
		if source == "" {
			source = address.String()
		} else {
			assert.Equal(t, source, address.String())
		}
	}
	return source
}

func readUDPDatagrams(t *testing.T, listener *net.UDPConn, count int) {
	t.Helper()
	for i := 0; i < count; i++ {
		if err := listener.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
			t.Fatal(err)
		}
		buffer := make([]byte, 4096)
		n, _, err := listener.ReadFromUDP(buffer)
		if err != nil {
			t.Fatal(err)
		}
		assert.Equal(t, byte('\n'), buffer[n-1])
	}
}

type countingWriter struct {
	mu     sync.Mutex
	closes int
}

func (writer *countingWriter) Write(data []byte) (int, error) {
	return len(data), nil
}

func (writer *countingWriter) Close() error {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	writer.closes++
	return nil
}

func (writer *countingWriter) closeCount() int {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	return writer.closes
}
