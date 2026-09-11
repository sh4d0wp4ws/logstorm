package main

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Generate generates the logs with given options
func Generate(option *Option) (err error) {
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
	writer, err := NewWriter(option.Type, logFileName, option.Target)
	if err != nil {
		return err
	}
	if option.Type != "stdout" {
		defer func() {
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
			time.Sleep(delay)
			log := NewLog(option.Format, created)
			if _, err := writer.Write([]byte(log + "\n")); err != nil {
				return err
			}
			created = created.Add(interval)
		}
	}

	if option.Bytes == 0 {
		// Generates the logs until the certain number of lines is reached
		for line := 0; line < option.Number; line++ {
			time.Sleep(delay)
			log := NewLog(option.Format, created)
			if _, err := writer.Write([]byte(log + "\n")); err != nil {
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
				newWriter, err := NewWriter(option.Type, logFileName, option.Target)
				if err != nil {
					return err
				}
				writer = newWriter

				splitCount++
			}
			created = created.Add(interval)
		}
	} else {
		// Generates the logs until the certain size in bytes is reached
		bytes := 0
		for bytes < option.Bytes {
			time.Sleep(delay)
			log := NewLog(option.Format, created)
			if _, err := writer.Write([]byte(log + "\n")); err != nil {
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
				newWriter, err := NewWriter(option.Type, logFileName, option.Target)
				if err != nil {
					return err
				}
				writer = newWriter

				splitCount++
			}
			created = created.Add(interval)
		}
	}

	return nil
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
