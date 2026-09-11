package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/brianvoe/gofakeit"
)

const (
	// ApacheCommonLog : {host} {user-identifier} {auth-user-id} [{datetime}] "{method} {request} {protocol}" {response-code} {bytes}
	ApacheCommonLog = "%s - %s [%s] \"%s %s %s\" %d %d"
	// ApacheCombinedLog : {host} {user-identifier} {auth-user-id} [{datetime}] "{method} {request} {protocol}" {response-code} {bytes} "{referrer}" "{agent}"
	ApacheCombinedLog = "%s - %s [%s] \"%s %s %s\" %d %d \"%s\" \"%s\""
	// ApacheErrorLog : [{timestamp}] [{module}:{severity}] [pid {pid}:tid {thread-id}] [client %{client}:{port}] %{message}
	ApacheErrorLog = "[%s] [%s:%s] [pid %d:tid %d] [client %s:%d] %s"
	// RFC3164Log : <priority>{timestamp} {hostname} {application}[{pid}]: {message}
	RFC3164Log = "<%d>%s %s %s[%d]: %s"
	// RFC5424Log : <priority>{version} {iso-timestamp} {hostname} {application} {pid} {message-id} {structured-data} {message}
	RFC5424Log = "<%d>1 %s %s %s %s %s - %s"
	// CommonLogFormat : {host} {user-identifier} {auth-user-id} [{datetime}] "{method} {request} {protocol}" {response-code} {bytes}
	CommonLogFormat = "%s - %s [%s] \"%s %s %s\" %d %d"
	// JSONLogFormat : {"host": "{host}", "user-identifier": "{user-identifier}", "datetime": "{datetime}", "method": "{method}", "request": "{request}", "protocol": "{protocol}", "status", {status}, "bytes": {bytes}, "referer": "{referer}"}
	JSONLogFormat = `{"host":"%s", "user-identifier":"%s", "datetime":"%s", "method": "%s", "request": "%s", "protocol":"%s", "status":%d, "bytes":%d, "referer": "%s"}`
)

// NewApacheCommonLog creates a log string with apache common log format
func NewApacheCommonLog(t time.Time) string {
	return fmt.Sprintf(
		ApacheCommonLog,
		gofakeit.IPv4Address(),
		RandAuthUserID(),
		t.Format(Apache),
		gofakeit.HTTPMethod(),
		RandResourceURI(),
		RandHTTPVersion(),
		gofakeit.StatusCode(),
		gofakeit.Number(0, 30000),
	)
}

// NewApacheCombinedLog creates a log string with apache combined log format
func NewApacheCombinedLog(t time.Time) string {
	return fmt.Sprintf(
		ApacheCombinedLog,
		gofakeit.IPv4Address(),
		RandAuthUserID(),
		t.Format(Apache),
		gofakeit.HTTPMethod(),
		RandResourceURI(),
		RandHTTPVersion(),
		gofakeit.StatusCode(),
		gofakeit.Number(30, 100000),
		gofakeit.URL(),
		gofakeit.UserAgent(),
	)
}

// NewApacheErrorLog creates a log string with apache error log format
func NewApacheErrorLog(t time.Time) string {
	return fmt.Sprintf(
		ApacheErrorLog,
		t.Format(ApacheError),
		gofakeit.Word(),
		gofakeit.LogLevel("apache"),
		gofakeit.Number(1, 10000),
		gofakeit.Number(1, 10000),
		gofakeit.IPv4Address(),
		gofakeit.Number(1, 65535),
		gofakeit.HackerPhrase(),
	)
}

// NewRFC3164Log creates a log string with syslog (RFC3164) format
func NewRFC3164Log(t time.Time) string {
	return formatRFC3164(
		gofakeit.Number(0, 191),
		t,
		strings.ToLower(gofakeit.Username()),
		gofakeit.Word(),
		gofakeit.Number(1, 10000),
		gofakeit.HackerPhrase(),
	)
}

// NewRFC5424Log creates a log string with syslog (RFC5424) format
func NewRFC5424Log(t time.Time) string {
	return formatRFC5424(
		gofakeit.Number(0, 191),
		t,
		gofakeit.DomainName(),
		gofakeit.Word(),
		strconv.Itoa(gofakeit.Number(1, 10000)),
		fmt.Sprintf("ID%d", gofakeit.Number(1, 1000)),
		visibleASCII(gofakeit.HackerPhrase()),
	)
}

func formatRFC3164(priority int, t time.Time, host, tag string, pid int, message string) string {
	host, _, _ = strings.Cut(host, ".")
	tag = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, tag)
	if tag == "" {
		tag = "flog"
	}
	if len(tag) > 32 {
		tag = tag[:32]
	}
	prefix := fmt.Sprintf(RFC3164Log, priority, t.Format(RFC3164), syslogHostname(host), tag, pid, "")
	message = visibleASCII(message)
	// Reserve the terminal LF added by GenerateContext within the 1024-byte packet.
	if budget := 1023 - len(prefix); len(message) > budget {
		message = message[:budget]
	}
	return prefix + message
}

func formatRFC5424(priority int, t time.Time, host, app, proc, msgID, message string) string {
	return fmt.Sprintf(RFC5424Log, priority, t.Format(RFC5424), syslogHostname(host),
		syslogHeaderField(app, 48), syslogHeaderField(proc, 128), syslogHeaderField(msgID, 32), message)
}

func syslogHostname(value string) string {
	labels := strings.Split(value, ".")
	for i, label := range labels {
		label = strings.Map(func(r rune) rune {
			if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' {
				return r
			}
			return -1
		}, label)
		if len(label) > 63 {
			label = label[:63]
		}
		label = strings.Trim(label, "-")
		if label == "" {
			label = "flog"
		}
		labels[i] = label
	}
	host := strings.Join(labels, ".")
	if len(host) > 253 {
		host = strings.TrimRight(host[:253], ".-")
	}
	return host
}

func syslogHeaderField(value string, limit int) string {
	value = strings.Map(func(r rune) rune {
		if r >= 33 && r <= 126 {
			return r
		}
		return '_'
	}, value)
	if value == "" {
		return "-"
	}
	if len(value) > limit {
		value = value[:limit]
	}
	return value
}

func visibleASCII(value string) string {
	return strings.Map(func(r rune) rune {
		if r >= 32 && r <= 126 {
			return r
		}
		return ' '
	}, value)
}

// NewCEFLog creates a synthetic CEF:0 event in the MSG of an RFC5424 envelope.
func NewCEFLog(t time.Time) string {
	header := []string{"CEF:0", "isc4", "flog", version, "1001", "Synthetic network connection allowed", "5"}
	for i := 1; i < len(header); i++ {
		header[i] = escapeCEFHeader(header[i])
	}
	extension := fmt.Sprintf("src=%s dst=%s spt=%d dpt=443 proto=TCP act=allowed msg=%s",
		gofakeit.IPv4Address(), gofakeit.IPv4Address(), gofakeit.Number(1024, 65535),
		escapeCEFExtension("Synthetic network connection allowed"))
	// local4.warning (20*8+4) is independent of the CEF Severity header value 5.
	return formatRFC5424(164, t, "isc4-flog.example", "isc4-flog", "-", "CEF", strings.Join(header, "|")+"|"+extension)
}

func escapeCEFHeader(value string) string {
	return strings.NewReplacer("\\", "\\\\", "|", "\\|").Replace(value)
}

func escapeCEFExtension(value string) string {
	return strings.NewReplacer("\\", "\\\\", "=", "\\=", "\r", "\\r", "\n", "\\n").Replace(value)
}

// NewCommonLogFormat creates a log string with common log format
func NewCommonLogFormat(t time.Time) string {
	return fmt.Sprintf(
		CommonLogFormat,
		gofakeit.IPv4Address(),
		RandAuthUserID(),
		t.Format(CommonLog),
		gofakeit.HTTPMethod(),
		RandResourceURI(),
		RandHTTPVersion(),
		gofakeit.StatusCode(),
		gofakeit.Number(0, 30000),
	)
}

// NewJSONLogFormat creates a log string with json log format
func NewJSONLogFormat(t time.Time) string {
	return fmt.Sprintf(
		JSONLogFormat,
		gofakeit.IPv4Address(),
		RandAuthUserID(),
		t.Format(CommonLog),
		gofakeit.HTTPMethod(),
		RandResourceURI(),
		RandHTTPVersion(),
		gofakeit.StatusCode(),
		gofakeit.Number(0, 30000),
		gofakeit.URL(),
	)
}
