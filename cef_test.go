package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"bou.ke/monkey"
	"github.com/brianvoe/gofakeit"
)

func TestCEFEnvelopeAndSchema(t *testing.T) {
	monkey.Patch(gofakeit.IPv4Address, func() string { return "192.0.2.10" })
	defer monkey.Unpatch(gofakeit.IPv4Address)
	monkey.Patch(gofakeit.Number, func(min, max int) int { return max })
	defer monkey.Unpatch(gofakeit.Number)

	log := NewLog("cef", stopped)
	t.Log(log)
	fields := assertRFC5424(t, log, 164, "2018-04-22T09:30:00.000Z")
	if strings.Join(fields[2:7], " ") != "logstorm.example LogStorm - CEF -" {
		t.Fatalf("incorrect CEF envelope: %q", log)
	}
	if !strings.HasPrefix(fields[7], "CEF:0|") || strings.ContainsAny(log, "\r\n") {
		t.Fatalf("invalid CEF body position/framing: %q", log)
	}
	// The fixed generated header contains no escaped pipes; escaping is tested separately.
	header := strings.SplitN(fields[7], "|", 8)
	if len(header) != 8 || strings.Join(header[:7], "|") != "CEF:0|LogStorm|LogStorm|"+version+"|1001|Synthetic network connection allowed|5" {
		t.Fatalf("incorrect CEF header: %q", fields[7])
	}
	for i, limit := range []int{63, 63, 31, 1023, 512} {
		if len(header[i+1]) == 0 || len(header[i+1]) > limit {
			t.Fatalf("invalid CEF field size: %q", header[i+1])
		}
	}
	severity, err := strconv.Atoi(header[6])
	if err != nil || severity < 0 || severity > 10 || severity == 164%8 {
		t.Fatalf("invalid/incorrect CEF severity: %q", header[6])
	}
	extensions := strings.SplitN(header[7], " ", 7)
	keys := []string{"src", "dst", "spt", "dpt", "proto", "act", "msg"}
	if len(extensions) != len(keys) {
		t.Fatalf("invalid extensions: %q", header[7])
	}
	for i, key := range keys {
		name, value, ok := strings.Cut(extensions[i], "=")
		if !ok || name != key {
			t.Fatalf("incorrect extension order: %q", extensions)
		}
		switch key {
		case "src", "dst":
			if ip := net.ParseIP(value); ip == nil || ip.To4() == nil || strings.Contains(value, ":") {
				t.Fatalf("invalid IPv4: %q", value)
			}
		case "spt", "dpt":
			port, err := strconv.Atoi(value)
			if err != nil || port < 0 || port > 65535 {
				t.Fatalf("invalid port: %q", value)
			}
		case "proto":
			if value != "TCP" {
				t.Fatalf("invalid protocol: %q", value)
			}
		case "act":
			if value != "allowed" {
				t.Fatalf("invalid action: %q", value)
			}
		case "msg":
			if value != "Synthetic network connection allowed" {
				t.Fatalf("invalid message: %q", value)
			}
		}
	}
}

func TestCEFStreamsSendTCPAndUDP(t *testing.T) {
	tcp, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	defer tcp.Close()
	udp, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer udp.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	streams, err := LoadStreams(writeConfig(t, fmt.Sprintf(`streams:
  - name: cef-tcp
    format: cef
    type: tcp
    target: %s
    number: 1
  - name: cef-udp
    format: cef
    type: udp
    target: %s
    number: 1
`, tcp.Addr(), udp.LocalAddr())))
	if err != nil {
		t.Fatal(err)
	}
	if err := RunStreams(ctx, streams); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	if err := tcp.SetDeadline(deadline); err != nil {
		t.Fatal(err)
	}
	conn, err := tcp.AcceptTCP()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(deadline); err != nil {
		t.Fatal(err)
	}
	tcpBytes, err := io.ReadAll(conn)
	if err != nil {
		t.Fatal(err)
	}
	if err := udp.SetDeadline(deadline); err != nil {
		t.Fatal(err)
	}
	udpBytes := make([]byte, 4096)
	n, _, err := udp.ReadFrom(udpBytes)
	if err != nil {
		t.Fatal(err)
	}
	for _, packet := range []string{string(tcpBytes), string(udpBytes[:n])} {
		if !strings.HasSuffix(packet, "\n") || strings.ContainsAny(strings.TrimSuffix(packet, "\n"), "\r\n") {
			t.Fatalf("incorrect CEF transport framing: %q", packet)
		}
		if !strings.HasPrefix(packet, "<164>1 ") || !strings.Contains(packet, " logstorm.example LogStorm - CEF - CEF:0|") {
			t.Fatalf("invalid delivered CEF envelope: %q", packet)
		}
	}
}

func TestCEFEscaping(t *testing.T) {
	for _, test := range []struct {
		input, header, extension string
	}{
		{`\`, `\\`, `\\`},
		{"|", `\|`, "|"},
		{"=", "=", `\=`},
		{"two words", "two words", "two words"},
		{`a\|b=c`, `a\\\|b=c`, `a\\|b\=c`},
	} {
		if got := escapeCEFHeader(test.input); got != test.header {
			t.Errorf("header escaping %q: got %q, want %q", test.input, got, test.header)
		}
		if got := escapeCEFExtension(test.input); got != test.extension {
			t.Errorf("extension escaping %q: got %q, want %q", test.input, got, test.extension)
		}
	}
	logical := "first\r\nsecond\\n third=ok|yes"
	escaped := escapeCEFExtension(logical)
	if escaped != `first\r\nsecond\\n third\=ok|yes` || strings.ContainsAny(escaped, "\r\n") {
		t.Fatalf("invalid multiline escaping: %q", escaped)
	}
	log := formatRFC5424(164, stopped, "logstorm.example", "LogStorm", "-", "CEF",
		"CEF:0|LogStorm|LogStorm|"+version+"|1001|Synthetic network connection allowed|5|msg="+escaped)
	if strings.ContainsAny(log, "\r\n") || !strings.HasSuffix(log, "msg="+escaped) {
		t.Fatalf("logical newline escaped incorrectly in syslog body: %q", log)
	}
}
