package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"bou.ke/monkey"
	"github.com/brianvoe/gofakeit"
)

func TestNewSizedLogUsesGuaranteedMinimumWithLongestFixedFields(t *testing.T) {
	patchLongestSizedFields(t)
	longestTimestamp := time.Date(2018, time.April, 22, 9, 30, 0, 0, time.FixedZone("+07", 7*60*60))

	for _, test := range []struct {
		format string
		size   int
	}{
		{"apache_common", apacheCommonEventSizeMin},
		{"common_log", apacheCommonEventSizeMin},
		{"apache_combined", apacheCombinedEventSizeMin},
		{"apache_error", apacheErrorEventSizeMin},
		{"rfc3164", rfc3164EventSizeMin},
		{"rfc5424", rfc5424EventSizeMin},
		{"cef", 231},
		{"json", jsonEventSizeMin},
	} {
		t.Run(test.format, func(t *testing.T) {
			log, err := NewSizedLog(test.format, longestTimestamp, test.size)
			if err != nil {
				t.Fatal(err)
			}
			if len(log)+eventSizeLineFeed != test.size {
				t.Fatalf("size = %d, want %d", len(log)+eventSizeLineFeed, test.size)
			}
		})
	}
}

func TestEventSizeConstantsMatchDeterministicLongestFixedFields(t *testing.T) {
	timestamp := time.Date(2018, time.April, 22, 9, 30, 0, 0, time.FixedZone("+07", 7*60*60))
	address := "255.255.255.255"
	user := "runolfsdottir0000"
	method := "DELETE"
	protocol := "HTTP/1.1"
	domain := "internationalclicks-and-mortar.info"
	word := "exercitationem"
	referer := "https://www.internationalclicks-and-mortar.info/clicks-and-mortar/clicks-and-mortar/clicks-and-mortar/clicks-and-mortar"

	assertFixedSize(t, "apache common", len(fmt.Sprintf(ApacheCommonLog, address, user, timestamp.Format(Apache), method, "", protocol, 504, 30000)), 93)
	assertFixedSize(t, "apache combined", len(fmt.Sprintf(ApacheCombinedLog, address, user, timestamp.Format(Apache), method, "", protocol, 504, 100000, referer, strings.Repeat("A", 147))), 366)
	assertFixedSize(t, "apache error", len(fmt.Sprintf(ApacheErrorLog, timestamp.Format(ApacheError), word, "trace1-8", 10000, 10000, address, 65535, "")), 106)
	assertFixedSize(t, "RFC3164", len(formatRFC3164(191, timestamp, user, word, 10000, "")), 62)
	assertFixedSize(t, "RFC5424", len(formatRFC5424(191, timestamp, domain, word, "10000", "ID1000", "")), 103)
	assertFixedSize(t, "JSON", len(fmt.Sprintf(JSONLogFormat, address, user, timestamp.Format(CommonLog), method, "", protocol, 504, 30000, referer)), 327)

	header := []string{"CEF:0", "LogStorm", "LogStorm", version, "1001", "Synthetic network connection allowed", "5"}
	for i := 1; i < len(header); i++ {
		header[i] = escapeCEFHeader(header[i])
	}
	assertFixedSize(t, "CEF", len(formatCEFLog(timestamp, header, address, address, 65535, "")), 229)
	if cefFixedMin != 207 || cefFixedMax != 229 || cefEventSizeMin != 231 || cefEventSizeMax != 1231 {
		t.Fatalf("unexpected CEF bounds: fixed %d..%d, event size %d..%d", cefFixedMin, cefFixedMax, cefEventSizeMin, cefEventSizeMax)
	}
}

func assertFixedSize(t *testing.T, name string, got, want int) {
	t.Helper()
	if got != want {
		t.Fatalf("%s fixed size = %d, want %d", name, got, want)
	}
}

func TestNewSizedLogHonorsExactSizeAndFormatSyntax(t *testing.T) {
	for _, test := range []struct {
		format string
		size   int
	}{
		{"apache_common", 128},
		{"common_log", 128},
		{"apache_combined", 512},
		{"apache_error", 256},
		{"rfc3164", 1024},
		{"rfc5424", 512},
		{"cef", 1231},
		{"json", 512},
	} {
		t.Run(test.format, func(t *testing.T) {
			log, err := NewSizedLog(test.format, stopped, test.size)
			if err != nil {
				t.Fatal(err)
			}
			if len(log)+eventSizeLineFeed != test.size {
				t.Fatalf("size = %d, want %d", len(log)+eventSizeLineFeed, test.size)
			}
			switch test.format {
			case "json":
				var value map[string]any
				if err := json.Unmarshal([]byte(log), &value); err != nil {
					t.Fatalf("invalid JSON: %v", err)
				}
			case "rfc5424":
				fields := strings.SplitN(log, " ", 8)
				if len(fields) != 8 || !strings.HasPrefix(fields[0], "<") || fields[6] != "-" {
					t.Fatalf("invalid RFC5424 structure: %q", log)
				}
				if _, err := time.Parse(time.RFC3339Nano, fields[1]); err != nil {
					t.Fatalf("invalid RFC5424 timestamp: %v", err)
				}
			case "cef":
				message := log[strings.LastIndex(log, "msg=")+len("msg="):]
				if len(message) > 1023 {
					t.Fatalf("CEF msg size = %d, want <= 1023", len(message))
				}
			case "apache_common", "common_log", "apache_combined":
				if !strings.Contains(log, " /") {
					t.Fatalf("request URI was not retained in record: %q", log)
				}
			}
		})
	}
}

func TestNewSizedCEFUsesShortestBaselineAtMaximum(t *testing.T) {
	header := []string{"CEF:0", "LogStorm", "LogStorm", version, "1001", "Synthetic network connection allowed", "5"}
	for i := 1; i < len(header); i++ {
		header[i] = escapeCEFHeader(header[i])
	}
	fixed := formatCEFLog(stopped, header, "2.2.2.2", "2.2.2.2", 1024, "")
	if len(fixed) != 207 {
		t.Fatalf("short CEF fixed size = %d, want 207", len(fixed))
	}
	message, err := sizedText(cefEventSizeMax, len(fixed), 1, 1023)
	if err != nil {
		t.Fatal(err)
	}
	log := formatCEFLog(stopped, header, "2.2.2.2", "2.2.2.2", 1024, message)
	if len(message) != 1023 {
		t.Fatalf("CEF msg size = %d, want 1023", len(message))
	}
	if len(log)+eventSizeLineFeed != cefEventSizeMax {
		t.Fatalf("event size = %d, want %d", len(log)+eventSizeLineFeed, cefEventSizeMax)
	}
}

func TestNewSizedLogRejectsFormatBoundaries(t *testing.T) {
	for _, test := range []struct {
		format string
		size   int
	}{
		{"apache_common", apacheCommonEventSizeMin - 1},
		{"common_log", apacheCommonEventSizeMin - 1},
		{"apache_combined", apacheCombinedEventSizeMin - 1},
		{"apache_error", apacheErrorEventSizeMin - 1},
		{"rfc3164", rfc3164EventSizeMin - 1},
		{"rfc3164", rfc3164EventSizeMax + 1},
		{"rfc5424", rfc5424EventSizeMin - 1},
		{"cef", 230},
		{"cef", 1232},
		{"json", jsonEventSizeMin - 1},
		{"rfc5424", eventSizeProductMax + 1},
	} {
		t.Run(fmt.Sprintf("%s-%d", test.format, test.size), func(t *testing.T) {
			if _, err := NewSizedLog(test.format, stopped, test.size); err == nil {
				t.Fatal("expected size validation error")
			}
		})
	}
}

func TestLoadStreamsValidatesEventSizeBeforeExecution(t *testing.T) {
	for _, test := range []struct {
		name   string
		format string
		type_  string
		size   int
		valid  bool
	}{
		{"minimum", "apache_common", "udp", apacheCommonEventSizeMin, true},
		{"zero", "apache_common", "udp", 0, false},
		{"negative", "apache_common", "udp", -1, false},
		{"rfc3164 maximum", "rfc3164", "udp", rfc3164EventSizeMax, true},
		{"rfc3164 overflow", "rfc3164", "tcp", rfc3164EventSizeMax + 1, false},
		{"cef maximum", "cef", "tcp", cefEventSizeMax, true},
		{"cef overflow", "cef", "udp", cefEventSizeMax + 1, false},
		{"udp maximum", "rfc5424", "udp", eventSizeUDPMax, true},
		{"udp overflow", "rfc5424", "udp", eventSizeUDPMax + 1, false},
		{"tcp product maximum", "rfc5424", "tcp", eventSizeProductMax, true},
		{"tcp product overflow", "rfc5424", "tcp", eventSizeProductMax + 1, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			content := fmt.Sprintf("streams:\n  - name: sized\n    format: %s\n    type: %s\n    target: 127.0.0.1:514\n    event_size: %d\n", test.format, test.type_, test.size)
			streams, err := LoadStreams(writeConfig(t, content))
			if test.valid {
				if err != nil {
					t.Fatal(err)
				}
				if streams[0].Option.EventSize != test.size {
					t.Fatalf("event size = %d, want %d", streams[0].Option.EventSize, test.size)
				}
			} else if err == nil {
				t.Fatal("expected configuration validation error")
			}
		})
	}
}

func TestSizedStreamWritesExactTCPAndUDPSizes(t *testing.T) {
	tcp, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	defer tcp.Close()
	udp, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	defer udp.Close()

	streams, err := LoadStreams(writeConfig(t, fmt.Sprintf(`streams:
  - name: tcp
    format: rfc5424
    type: tcp
    target: %s
    number: 1
    event_size: 512
  - name: udp
    format: json
    type: udp
    target: %s
    number: 1
    event_size: 512
`, tcp.Addr(), udp.LocalAddr())))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := RunStreams(ctx, streams); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(3 * time.Second)
	if err := tcp.SetDeadline(deadline); err != nil {
		t.Fatal(err)
	}
	connection, err := tcp.AcceptTCP()
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if err := connection.SetDeadline(deadline); err != nil {
		t.Fatal(err)
	}
	tcpRecord, err := io.ReadAll(connection)
	if err != nil {
		t.Fatal(err)
	}
	if len(tcpRecord) != 512 || tcpRecord[len(tcpRecord)-1] != '\n' {
		t.Fatalf("TCP record size/framing = %d/%q", len(tcpRecord), tcpRecord[len(tcpRecord)-1])
	}

	if err := udp.SetReadDeadline(deadline); err != nil {
		t.Fatal(err)
	}
	udpRecord := make([]byte, 1024)
	n, _, err := udp.ReadFromUDP(udpRecord)
	if err != nil {
		t.Fatal(err)
	}
	if n != 512 || udpRecord[n-1] != '\n' {
		t.Fatalf("UDP datagram size/framing = %d/%q", n, udpRecord[n-1])
	}
}

func patchLongestSizedFields(t *testing.T) {
	t.Helper()
	patches := []*monkey.PatchGuard{
		monkey.Patch(gofakeit.IPv4Address, func() string { return "255.255.255.255" }),
		monkey.Patch(RandAuthUserID, func() string { return "runolfsdottir0000" }),
		monkey.Patch(gofakeit.HTTPMethod, func() string { return "DELETE" }),
		monkey.Patch(RandHTTPVersion, func() string { return "HTTP/1.1" }),
		monkey.Patch(gofakeit.StatusCode, func() int { return 504 }),
		monkey.Patch(gofakeit.Word, func() string { return "exercitationem" }),
		monkey.Patch(gofakeit.LogLevel, func(string) string { return "trace1-8" }),
		monkey.Patch(gofakeit.DomainName, func() string { return "internationalclicks-and-mortar.info" }),
		monkey.Patch(gofakeit.URL, func() string {
			return "https://www.internationalclicks-and-mortar.info/clicks-and-mortar/clicks-and-mortar/clicks-and-mortar/clicks-and-mortar"
		}),
		monkey.Patch(gofakeit.UserAgent, func() string { return strings.Repeat("A", 147) }),
	}
	t.Cleanup(func() {
		for _, patch := range patches {
			patch.Unpatch()
		}
	})
}

func TestNewSizedLogJSONRequestRemainsValid(t *testing.T) {
	log, err := NewSizedLog("json", stopped, jsonEventSizeMin)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal([]byte(log), &value); err != nil {
		t.Fatal(err)
	}
	request, ok := value["request"].(string)
	if !ok || !strings.HasPrefix(request, "/") {
		t.Fatalf("sized JSON request is not a URI: %q", log)
	}
}

func TestNewSizedLogRepeatedBoundaryGeneration(t *testing.T) {
	for _, test := range []struct {
		format string
		size   int
	}{
		{"apache_common", apacheCommonEventSizeMin},
		{"apache_combined", apacheCombinedEventSizeMin},
		{"apache_error", apacheErrorEventSizeMin},
		{"rfc3164", rfc3164EventSizeMax},
		{"rfc5424", rfc5424EventSizeMin},
		{"cef", cefEventSizeMax},
		{"json", jsonEventSizeMin},
	} {
		t.Run(test.format, func(t *testing.T) {
			for i := 0; i < 10; i++ {
				log, err := NewSizedLog(test.format, stopped, test.size)
				if err != nil {
					t.Fatal(err)
				}
				if len(log)+eventSizeLineFeed != test.size {
					t.Fatalf("event size = %d, want %d", len(log)+eventSizeLineFeed, test.size)
				}
			}
		})
	}
}

func TestSizedStreamUsesExistingEPSAndDurationPath(t *testing.T) {
	listener, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- GenerateContext(ctx, &Option{Format: "apache_common", Type: "udp", Target: listener.LocalAddr().String(), Forever: true, EPS: 1, Duration: 100 * time.Millisecond, EventSize: 128})
	}()

	if err := listener.SetReadDeadline(time.Now().Add(300 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	record := make([]byte, 256)
	n, _, err := listener.ReadFromUDP(record)
	if err != nil {
		t.Fatal(err)
	}
	if n != 128 || record[n-1] != '\n' {
		t.Fatalf("sized EPS record size/framing = %d/%q", n, record[n-1])
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestSizedTCPRecordHasOneLine(t *testing.T) {
	log, err := NewSizedLog("rfc5424", stopped, 512)
	if err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(strings.NewReader(log + "\n")).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if len(line) != 512 || strings.Count(line, "\n") != 1 {
		t.Fatalf("line framing = %d bytes, %d newlines", len(line), strings.Count(line, "\n"))
	}
}
