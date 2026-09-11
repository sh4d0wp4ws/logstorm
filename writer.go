package main

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"sync"
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

func NewWriterContext(ctx context.Context, logType string, output string, target string) (io.WriteCloser, error) {
	if isNetworkOutput(logType) {
		return NewNetworkWriterContext(ctx, logType, target)
	}
	return NewWriter(logType, output, target)
}

type closeOnceWriter struct {
	io.WriteCloser
	once     sync.Once
	closeErr error
}

func (writer *closeOnceWriter) Close() error {
	writer.once.Do(func() {
		writer.closeErr = writer.WriteCloser.Close()
	})
	return writer.closeErr
}
