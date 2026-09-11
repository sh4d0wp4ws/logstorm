package main

import (
	"context"
	"net"
)

func NewNetworkWriter(logType string, target string) (net.Conn, error) {
	return net.Dial(logType, target)
}

func NewNetworkWriterContext(ctx context.Context, logType string, target string) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, logType, target)
}
