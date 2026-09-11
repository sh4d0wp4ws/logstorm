package main

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Generate generates the logs with given options
func Generate(option *Option) error {
	return GenerateContext(context.Background(), option)
}

func GenerateContext(ctx context.Context, option *Option) (err error) {
	var (
		splitCount = 1
		created    = time.Now()

		interval time.Duration
		delay    time.Duration
	)

	if option.Delay > 0 {
		interval = option.Delay
		delay = interval
	}
	if option.Sleep > 0 {
		interval = option.Sleep
	}

	logFileName := option.Output
	writer, err := NewWriterContext(ctx, option.Type, logFileName, option.Target)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	if option.Type != "stdout" {
		writer = &closeOnceWriter{WriteCloser: writer}
		stopCancellationClose := func() {}
		if isNetworkOutput(option.Type) {
			stopCancellationClose = closeOnCancellation(ctx, writer)
		}
		defer func() {
			stopCancellationClose()
			if writer == nil {
				return
			}
			if closeErr := writer.Close(); err == nil && closeErr != nil {
				err = closeErr
			}
			if err == nil && isFileOutput(option.Type) {
				fmt.Println(logFileName, "is created.")
			}
		}()
	}

	if option.Forever {
		for {
			if err := waitForDelay(ctx, delay); err != nil {
				return err
			}
			log := NewLog(option.Format, created)
			if _, err := writer.Write([]byte(log + "\n")); err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return err
			}
			created = created.Add(interval)
		}
	}

	if option.Bytes == 0 {
		// Generates the logs until the certain number of lines is reached
		for line := 0; line < option.Number; line++ {
			if err := waitForDelay(ctx, delay); err != nil {
				return err
			}
			log := NewLog(option.Format, created)
			if _, err := writer.Write([]byte(log + "\n")); err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return err
			}

			if isFileOutput(option.Type) && (option.SplitBy > 0) && (line > option.SplitBy*splitCount) {
				writerToClose := writer
				writer = nil
				if err := writerToClose.Close(); err != nil {
					return err
				}
				fmt.Println(logFileName, "is created.")

				logFileName = NewSplitFileName(option.Output, splitCount)
				newWriter, err := NewWriterContext(ctx, option.Type, logFileName, option.Target)
				if err != nil {
					return err
				}
				writer = &closeOnceWriter{WriteCloser: newWriter}

				splitCount++
			}
			created = created.Add(interval)
		}
	} else {
		// Generates the logs until the certain size in bytes is reached
		bytes := 0
		for bytes < option.Bytes {
			if err := waitForDelay(ctx, delay); err != nil {
				return err
			}
			log := NewLog(option.Format, created)
			if _, err := writer.Write([]byte(log + "\n")); err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return err
			}

			bytes += len(log)
			if isFileOutput(option.Type) && (option.SplitBy > 0) && (bytes > option.SplitBy*splitCount+1) {
				writerToClose := writer
				writer = nil
				if err := writerToClose.Close(); err != nil {
					return err
				}
				fmt.Println(logFileName, "is created.")

				logFileName = NewSplitFileName(option.Output, splitCount)
				newWriter, err := NewWriterContext(ctx, option.Type, logFileName, option.Target)
				if err != nil {
					return err
				}
				writer = &closeOnceWriter{WriteCloser: newWriter}

				splitCount++
			}
			created = created.Add(interval)
		}
	}

	return nil
}

func waitForDelay(ctx context.Context, delay time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if delay == 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func closeOnCancellation(ctx context.Context, writer io.WriteCloser) func() {
	if ctx.Done() == nil {
		return func() {}
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		select {
		case <-ctx.Done():
			_ = writer.Close()
		case <-stop:
		}
	}()
	return func() {
		close(stop)
		<-done
	}
}

// NewLog creates a log for given format
func NewLog(format string, t time.Time) string {
	switch format {
	case "apache_common":
		return NewApacheCommonLog(t)
	case "apache_combined":
		return NewApacheCombinedLog(t)
	case "apache_error":
		return NewApacheErrorLog(t)
	case "rfc3164":
		return NewRFC3164Log(t)
	case "rfc5424":
		return NewRFC5424Log(t)
	case "common_log":
		return NewCommonLogFormat(t)
	case "json":
		return NewJSONLogFormat(t)
	default:
		return ""
	}
}

// NewSplitFileName creates a new file path with split count
func NewSplitFileName(path string, count int) string {
	logFileNameExt := filepath.Ext(path)
	pathWithoutExt := strings.TrimSuffix(path, logFileNameExt)
	return pathWithoutExt + strconv.Itoa(count) + logFileNameExt
}
