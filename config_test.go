package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "logstorm.yaml")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadStreamsUsesDefaultsForOmittedValues(t *testing.T) {
	streams, err := LoadStreams(writeConfig(t, `
streams:
  - name: apache
    format: apache_common
    type: udp
    target: 127.0.0.1:514
`))

	if !assert.NoError(t, err) {
		return
	}
	if !assert.Len(t, streams, 1) {
		return
	}
	assert.Equal(t, "apache", streams[0].Name)
	assert.Equal(t, "apache_common", streams[0].Option.Format)
	assert.Equal(t, "udp", streams[0].Option.Type)
	assert.Equal(t, "127.0.0.1:514", streams[0].Option.Target)
	assert.Equal(t, defaultOptions().Number, streams[0].Option.Number)
	assert.Equal(t, defaultOptions().Delay, streams[0].Option.Delay)
	assert.Equal(t, 0, streams[0].Option.EventSize)
}

func TestLoadStreamsAppliesExplicitNumberAndLoop(t *testing.T) {
	streams, err := LoadStreams(writeConfig(t, `
streams:
  - name: finite
    format: apache_common
    type: udp
    target: localhost:514
    number: 7
    delay: 20ms
  - name: forever
    format: apache_combined
    type: tcp
    target: "[::1]:515"
    loop: true
`))

	if !assert.NoError(t, err) {
		return
	}
	if !assert.Len(t, streams, 2) {
		return
	}
	assert.Equal(t, 7, streams[0].Option.Number)
	assert.Equal(t, 20*time.Millisecond, streams[0].Option.Delay)
	assert.False(t, streams[0].Option.Forever)
	assert.True(t, streams[1].Option.Forever)
}

func TestLoadStreamsAppliesEPSAndDuration(t *testing.T) {
	streams, err := LoadStreams(writeConfig(t, `
streams:
  - name: paced
    format: apache_common
    type: udp
    target: localhost:514
    number: 7
    eps: 25
    duration: " 30s "
`))

	if !assert.NoError(t, err) || !assert.Len(t, streams, 1) {
		return
	}
	assert.Equal(t, 25, streams[0].Option.EPS)
	assert.Equal(t, 30*time.Second, streams[0].Option.Duration)
	assert.False(t, streams[0].Option.Forever)
}

func TestLoadStreamsUsesDurationForContinuousStreamWhenNumberAndLoopAreOmitted(t *testing.T) {
	streams, err := LoadStreams(writeConfig(t, `
streams:
  - name: duration-only
    format: apache_common
    type: udp
    target: localhost:514
    duration: 10s
`))

	if !assert.NoError(t, err) || !assert.Len(t, streams, 1) {
		return
	}
	assert.True(t, streams[0].Option.Forever)
	assert.Equal(t, defaultOptions().Number, streams[0].Option.Number)
}

func TestLoadStreamsPreservesExplicitZeroNumberWithDuration(t *testing.T) {
	streams, err := LoadStreams(writeConfig(t, `
streams:
  - name: zero
    format: apache_common
    type: udp
    target: localhost:514
    number: 0
    duration: 10s
`))

	if !assert.NoError(t, err) || !assert.Len(t, streams, 1) {
		return
	}
	assert.False(t, streams[0].Option.Forever)
	assert.Equal(t, 0, streams[0].Option.Number)
}

func TestLoadStreamsRunsDurationOnlyStreamUntilExpiry(t *testing.T) {
	listener, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if !assert.NoError(t, err) {
		return
	}
	defer listener.Close()

	streams, err := LoadStreams(writeConfig(t, fmt.Sprintf(`
streams:
  - name: duration-only
    format: apache_common
    type: udp
    target: %s
    eps: 1
    duration: 100ms
`, listener.LocalAddr())))
	if !assert.NoError(t, err) {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- RunStreams(ctx, streams)
	}()

	if err := listener.SetReadDeadline(time.Now().Add(300 * time.Millisecond)); !assert.NoError(t, err) {
		return
	}
	buffer := make([]byte, 4096)
	n, _, err := listener.ReadFromUDP(buffer)
	assert.NoError(t, err)
	if err == nil {
		assert.Equal(t, byte('\n'), buffer[n-1])
	}

	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-timer.C:
		t.Fatal("duration-only stream did not complete")
	}
}

func TestLoadStreamsTrimsFormatAndType(t *testing.T) {
	streams, err := LoadStreams(writeConfig(t, `
streams:
  - name: apache
    format: " apache_common "
    type: " udp "
    target: localhost:514
`))

	if !assert.NoError(t, err) {
		return
	}
	if !assert.Len(t, streams, 1) {
		return
	}
	assert.Equal(t, "apache_common", streams[0].Option.Format)
	assert.Equal(t, "udp", streams[0].Option.Type)
}

func TestLoadStreamsAcceptsCEF(t *testing.T) {
	streams, err := LoadStreams(writeConfig(t, `
streams:
  - name: cef-udp
    format: cef
    type: udp
    target: localhost:514
  - name: cef-tcp
    format: cef
    type: tcp
    target: localhost:515
`))
	if !assert.NoError(t, err) || !assert.Len(t, streams, 2) {
		return
	}
	for _, stream := range streams {
		assert.Equal(t, "cef", stream.Option.Format)
		assert.NotEmpty(t, NewLog(stream.Option.Format, stopped))
	}
}

func TestLoadStreamsRejectsInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"number and loop", "streams:\n  - name: apache\n    format: apache_common\n    type: udp\n    target: localhost:514\n    number: 1\n    loop: true\n"},
		{"malformed yaml", "streams: [\n"},
		{"unknown field", "streams:\n  - name: apache\n    format: apache_common\n    type: udp\n    target: localhost:514\n    extra: true\n"},
		{"duplicate name", "streams:\n  - name: apache\n    format: apache_common\n    type: udp\n    target: localhost:514\n  - name: apache\n    format: apache_common\n    type: tcp\n    target: localhost:515\n"},
		{"empty streams", "streams: []\n"},
		{"blank name", "streams:\n  - name: ' '\n    format: apache_common\n    type: udp\n    target: localhost:514\n"},
		{"format", "streams:\n  - name: apache\n    format: invalid\n    type: udp\n    target: localhost:514\n"},
		{"type", "streams:\n  - name: apache\n    format: apache_common\n    type: stdout\n    target: localhost:514\n"},
		{"missing target", "streams:\n  - name: apache\n    format: apache_common\n    type: udp\n"},
		{"invalid target", "streams:\n  - name: apache\n    format: apache_common\n    type: udp\n    target: localhost\n"},
		{"invalid delay", "streams:\n  - name: apache\n    format: apache_common\n    type: udp\n    target: localhost:514\n    delay: nope\n"},
		{"zero eps", "streams:\n  - name: apache\n    format: apache_common\n    type: udp\n    target: localhost:514\n    eps: 0\n"},
		{"negative eps", "streams:\n  - name: apache\n    format: apache_common\n    type: udp\n    target: localhost:514\n    eps: -1\n"},
		{"malformed duration", "streams:\n  - name: apache\n    format: apache_common\n    type: udp\n    target: localhost:514\n    duration: nope\n"},
		{"zero duration", "streams:\n  - name: apache\n    format: apache_common\n    type: udp\n    target: localhost:514\n    duration: 0s\n"},
		{"negative duration", "streams:\n  - name: apache\n    format: apache_common\n    type: udp\n    target: localhost:514\n    duration: -1s\n"},
		{"eps and delay", "streams:\n  - name: apache\n    format: apache_common\n    type: udp\n    target: localhost:514\n    eps: 1\n    delay: 1s\n"},
		{"eps and zero delay", "streams:\n  - name: apache\n    format: apache_common\n    type: udp\n    target: localhost:514\n    eps: 1\n    delay: 0s\n"},
		{"event size type", "streams:\n  - name: apache\n    format: apache_common\n    type: udp\n    target: localhost:514\n    event_size: nope\n"},
		{"malformed scalar", "streams:\n  - name: apache\n    format: 123\n    type: udp\n    target: localhost:514\n"},
		{"multiple documents", "streams:\n  - name: apache\n    format: apache_common\n    type: udp\n    target: localhost:514\n---\nstreams: []\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := LoadStreams(writeConfig(t, test.content))
			assert.Error(t, err)
		})
	}
}

func TestLoadStreamsRequiresExplicitFormatAndType(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"format", "streams:\n  - name: apache\n    type: udp\n    target: localhost:514\n", "format is required"},
		{"type", "streams:\n  - name: apache\n    format: apache_common\n    target: localhost:514\n", "type is required"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := LoadStreams(writeConfig(t, test.content))
			assert.Error(t, err)
			assert.Contains(t, err.Error(), test.want)
		})
	}
}
