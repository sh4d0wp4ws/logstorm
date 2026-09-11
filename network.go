package main

import "net"

func NewNetworkWriter(logType string, target string) (net.Conn, error) {
	return net.Dial(logType, target)
}
