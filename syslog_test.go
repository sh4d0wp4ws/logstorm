package main

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"
)

func assertASCIIField(t *testing.T, value string, limit int) {
	t.Helper()
	if len(value) == 0 || len(value) > limit {
		t.Fatalf("invalid field length %d (limit %d): %q", len(value), limit, value)
	}
	for _, b := range []byte(value) {
		if b < 33 || b > 126 {
			t.Fatalf("non-PRINTUSASCII byte in %q", value)
		}
	}
}

func assertHostname(t *testing.T, host string) {
	t.Helper()
	assertASCIIField(t, host, 255)
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			t.Fatalf("invalid hostname label: %q", label)
		}
		for _, b := range []byte(label) {
			if !(b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '-') {
				t.Fatalf("invalid hostname character: %q", label)
			}
		}
	}
}

func assertRFC5424(t *testing.T, log string, priority int, timestamp string) []string {
	t.Helper()
	fields := strings.SplitN(log, " ", 8)
	if len(fields) != 8 {
		t.Fatalf("expected header, SD and MSG: %q", log)
	}
	if fields[0] != fmt.Sprintf("<%d>1", priority) || priority < 0 || priority > 191 {
		t.Fatalf("invalid PRI/VERSION: %q", fields[0])
	}
	if fields[1] != timestamp {
		t.Fatalf("timestamp = %q, want %q", fields[1], timestamp)
	}
	if _, err := time.Parse(time.RFC3339Nano, fields[1]); err != nil {
		t.Fatal(err)
	}
	assertHostname(t, fields[2])
	for i, limit := range []int{48, 128, 32} {
		assertASCIIField(t, fields[i+3], limit)
	}
	if fields[6] != "-" {
		t.Fatalf("STRUCTURED-DATA = %q, want NILVALUE", fields[6])
	}
	return fields
}

func TestRFC3164Constraints(t *testing.T) {
	created := time.Date(2026, 4, 7, 9, 30, 0, 0, time.FixedZone("UTC+07", 7*60*60))
	for _, priority := range []int{0, 191} {
		log := formatRFC3164(priority, created, "--my_host\t\n-.example.org", "a-b_\t\n9", 10000, "hello\x00\t\r\n世界")
		want := fmt.Sprintf("<%d>Apr  7 09:30:00 myhost ab9[10000]: hello      ", priority)
		if log != want {
			t.Fatalf("got %q, want %q", log, want)
		}
		t.Log(log)
		assertHostname(t, strings.Split(log[len(fmt.Sprintf("<%d>", priority))+16:], " ")[0])
		for _, b := range []byte(log) {
			if b < 32 || b > 126 {
				t.Fatalf("invalid visible ASCII in %q", log)
			}
		}
	}
}

func TestRFC3164ContentBudgetPreservesPrefix(t *testing.T) {
	created := time.Date(2026, 4, 7, 9, 30, 0, 0, time.UTC)
	host := strings.Repeat("a", 100) + ".example"
	tag := strings.Repeat("Z1-", 50)
	prefix := "<191>Apr  7 09:30:00 " + strings.Repeat("a", 63) + " " + strings.Repeat("Z1", 16) + "[10000]: "
	for _, length := range []int{0, 1, 1023 - len(prefix), 1024 - len(prefix), 2048} {
		log := formatRFC3164(191, created, host, tag, 10000, strings.Repeat("X", length))
		if !strings.HasPrefix(log, prefix) {
			t.Fatalf("header/TAG/PID changed: %q", log)
		}
		wantLength := len(prefix) + length
		if wantLength > 1023 {
			wantLength = 1023
		}
		if len(log) != wantLength || len(log)+1 > 1024 {
			t.Fatalf("invalid packet budget: body=%d", len(log))
		}
	}
	if log := formatRFC3164(0, created, "!", "!", 1, "ok"); log != "<0>Apr  7 09:30:00 logstorm logstorm[1]: ok" {
		t.Fatalf("invalid empty-name fallback: %q", log)
	}
}

func TestRFC5424TimezonesAndFieldOrder(t *testing.T) {
	for _, test := range []struct {
		zone *time.Location
		want string
	}{
		{time.UTC, "2026-01-15T10:00:00.123Z"},
		{time.FixedZone("UTC+07", 7*60*60), "2026-01-15T17:00:00.123+07:00"},
	} {
		created := time.Date(2026, 1, 15, 10, 0, 0, 123000000, time.UTC).In(test.zone)
		for _, priority := range []int{0, 191} {
			log := formatRFC5424(priority, created, "host.example", "app", "123", "ID1", "hello world")
			fields := assertRFC5424(t, log, priority, test.want)
			t.Log(log)
			if strings.Join(fields[2:], " ") != "host.example app 123 ID1 - hello world" {
				t.Fatalf("incorrect fields or spaces: %q", log)
			}
			parsed, err := time.Parse(time.RFC3339Nano, fields[1])
			if err != nil || !parsed.Equal(created) {
				t.Fatalf("timestamp changed the instant: %q", log)
			}
		}
	}
}

func TestRFC5424HeaderConstraints(t *testing.T) {
	for _, raw := range []string{"", "\x00\t\r\n\x7f界", strings.Repeat(" -a_!界. ", 100), strings.Repeat("z", 300)} {
		log := formatRFC5424(191, stopped, raw, raw, raw, raw, "message")
		fields := assertRFC5424(t, log, 191, "2018-04-22T09:30:00.000Z")
		if raw == "" && strings.Join(fields[3:6], " ") != "- - -" {
			t.Fatalf("missing header NILVALUEs: %q", log)
		}
	}
	for _, limit := range []int{48, 128, 32} {
		for _, length := range []int{limit - 1, limit, limit + 1} {
			field := syslogHeaderField(strings.Repeat("x", length), limit)
			want := length
			if want > limit {
				want = limit
			}
			if len(field) != want {
				t.Fatalf("header length=%d, want %d", len(field), want)
			}
		}
	}
	if got := syslogHeaderField("a b\t\r\n界", 48); got != "a_b____" {
		t.Fatalf("invalid replacement: %q", got)
	}
}

func TestNewRFC5424LogUsesConstrainedFormatter(t *testing.T) {
	log := NewLog("rfc5424", stopped)
	endPRI := strings.IndexByte(log, '>')
	if endPRI < 2 {
		t.Fatalf("invalid PRI: %q", log)
	}
	priority, err := strconv.Atoi(log[1:endPRI])
	if err != nil {
		t.Fatal(err)
	}
	assertRFC5424(t, log, priority, "2018-04-22T09:30:00.000Z")
}
