package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "flog.yaml")
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
