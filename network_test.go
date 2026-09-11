package main

import (
	"bufio"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGenerateTCPReusesOneConnectionAndDelimitsLogs(t *testing.T) {
	a := assert.New(t)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if !a.NoError(err) {
		return
	}
	defer listener.Close()

	lines := make(chan []string, 1)
	errs := make(chan error, 1)
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			errs <- err
			return
		}
		defer connection.Close()

		reader := bufio.NewReader(connection)
		received := make([]string, 0, 3)
		for i := 0; i < 3; i++ {
			line, err := reader.ReadString('\n')
			if err != nil {
				errs <- err
				return
			}
			received = append(received, line)
		}
		lines <- received
	}()

	err = Generate(&Option{
		Format: "apache_common",
		Type:   "tcp",
		Target: listener.Addr().String(),
		Number: 3,
	})
	if !a.NoError(err) {
		return
	}

	select {
	case err := <-errs:
		a.NoError(err)
		return
	case received := <-lines:
		a.Len(received, 3)
		for _, line := range received {
			a.NotEmpty(line)
			a.Equal(byte('\n'), line[len(line)-1])
		}
	case <-time.After(time.Second):
		t.Fatal("TCP listener did not receive generated logs")
	}

	tcpListener := listener.(*net.TCPListener)
	if !a.NoError(tcpListener.SetDeadline(time.Now().Add(100 * time.Millisecond))) {
		return
	}
	_, err = listener.Accept()
	if networkError, ok := err.(net.Error); a.True(ok) {
		a.True(networkError.Timeout())
	}
}

func TestGenerateTCPReturnsConnectionError(t *testing.T) {
	err := Generate(&Option{
		Format: "apache_common",
		Type:   "tcp",
		Target: "127.0.0.1:0",
		Number: 1,
	})

	assert.Error(t, err)
}

func TestGenerateUDPSendsNewlineDelimitedDatagramsWithoutSplitting(t *testing.T) {
	a := assert.New(t)

	listener, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if !a.NoError(err) {
		return
	}
	defer listener.Close()

	err = Generate(&Option{
		Format:  "apache_common",
		Type:    "udp",
		Target:  listener.LocalAddr().String(),
		Number:  3,
		SplitBy: 1,
	})
	if !a.NoError(err) {
		return
	}

	var sender string
	for i := 0; i < 3; i++ {
		buffer := make([]byte, 4096)
		if !a.NoError(listener.SetReadDeadline(time.Now().Add(time.Second))) {
			return
		}
		count, address, err := listener.ReadFromUDP(buffer)
		if !a.NoError(err) {
			return
		}

		a.True(count > 0)
		a.Equal(byte('\n'), buffer[count-1])
		if sender == "" {
			sender = address.String()
		} else {
			a.Equal(sender, address.String())
		}
	}
}
