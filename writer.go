package main

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
)

func NewWriter(logType string, output string, target string) (io.WriteCloser, error) {
	switch logType {
	case "stdout":
		return os.Stdout, nil
	case "log":
		return os.Create(output)
	case "gz":
		logFile, err := os.Create(output)
		if err != nil {
			return nil, err
		}
		return gzip.NewWriter(logFile), nil
	case "tcp", "udp":
		return NewNetworkWriter(logType, target)
	default:
		return nil, fmt.Errorf("%s is not a valid log type", logType)
	}
}
