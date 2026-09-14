package main

import (
	"fmt"
	"strings"
)

const (
	// eventSizeProductMax is an isc4-flog allocation safety policy, not a
	// protocol or receiver message-size limit.
	eventSizeProductMax   = 1 << 20
	eventSizeUDPMax       = 65507
	eventSizeLineFeed     = 1
	eventSizeMinimumField = 1

	// These fixed-field maxima are derived from the pinned gofakeit v3.11.5
	// data and the current format strings. See event_size_test.go.
	apacheCommonFixedMax   = 93
	apacheCombinedFixedMax = 366
	apacheErrorFixedMax    = 106
	rfc3164FixedMax        = 62
	rfc5424FixedMax        = 103
	jsonFixedMax           = 327
	cefFixedMin            = 201
	cefFixedMax            = 223

	apacheCommonEventSizeMin   = apacheCommonFixedMax + eventSizeMinimumField + eventSizeLineFeed
	apacheCombinedEventSizeMin = apacheCombinedFixedMax + eventSizeMinimumField + eventSizeLineFeed
	apacheErrorEventSizeMin    = apacheErrorFixedMax + eventSizeMinimumField + eventSizeLineFeed
	rfc3164EventSizeMin        = rfc3164FixedMax + eventSizeMinimumField + eventSizeLineFeed
	rfc3164EventSizeMax        = 1024
	rfc5424EventSizeMin        = rfc5424FixedMax + eventSizeMinimumField + eventSizeLineFeed
	cefEventSizeMin            = cefFixedMax + eventSizeMinimumField + eventSizeLineFeed
	cefEventSizeMax            = cefFixedMin + 1023 + eventSizeLineFeed
	jsonEventSizeMin           = jsonFixedMax + eventSizeMinimumField + eventSizeLineFeed
)

type eventSizeRange struct {
	min int
	max int
}

// eventSizeRangeForFormat holds bounds derived from the pinned gofakeit data
// and the current formatters. Update the boundary tests when either changes.
func eventSizeRangeForFormat(format string) (eventSizeRange, error) {
	rangeForProductMax := eventSizeRange{max: eventSizeProductMax}
	switch format {
	case "apache_common", "common_log":
		rangeForProductMax.min = apacheCommonEventSizeMin
	case "apache_combined":
		rangeForProductMax.min = apacheCombinedEventSizeMin
	case "apache_error":
		rangeForProductMax.min = apacheErrorEventSizeMin
	case "rfc3164":
		rangeForProductMax.min = rfc3164EventSizeMin
		rangeForProductMax.max = rfc3164EventSizeMax
	case "rfc5424":
		rangeForProductMax.min = rfc5424EventSizeMin
	case "cef":
		rangeForProductMax.min = cefEventSizeMin
		rangeForProductMax.max = cefEventSizeMax
	case "json":
		rangeForProductMax.min = jsonEventSizeMin
	default:
		return eventSizeRange{}, fmt.Errorf("%s is not a valid format", format)
	}
	return rangeForProductMax, nil
}

func validateEventSize(format, outputType string, eventSize int) error {
	if eventSize <= 0 {
		return fmt.Errorf("event_size must be positive")
	}
	rangeForFormat, err := eventSizeRangeForFormat(format)
	if err != nil {
		return err
	}
	maximum := rangeForFormat.max
	if outputType == "udp" && maximum > eventSizeUDPMax {
		maximum = eventSizeUDPMax
	}
	if eventSize < rangeForFormat.min || eventSize > maximum {
		return fmt.Errorf("event_size must be between %d and %d bytes for %s %s output", rangeForFormat.min, maximum, format, outputType)
	}
	return nil
}

func validateSizedLogEventSize(format string, eventSize int) error {
	return validateEventSize(format, "", eventSize)
}

func sizedText(eventSize, fixedSize, minimumSize, maximumSize int) (string, error) {
	if fixedSize < 0 || fixedSize > eventSize-eventSizeLineFeed {
		return "", fmt.Errorf("event_size %d cannot fit the fixed record fields", eventSize)
	}
	textSize := eventSize - eventSizeLineFeed - fixedSize
	if textSize < minimumSize || (maximumSize > 0 && textSize > maximumSize) {
		return "", fmt.Errorf("event_size %d cannot fit the selected record field", eventSize)
	}
	return strings.Repeat("X", textSize), nil
}

func sizedRequest(eventSize, fixedSize int) (string, error) {
	requestSize := eventSize - eventSizeLineFeed - fixedSize
	if requestSize < 1 {
		return "", fmt.Errorf("event_size %d cannot fit a request URI", eventSize)
	}
	return "/" + strings.Repeat("X", requestSize-1), nil
}

func ensureEventSize(log string, eventSize int) (string, error) {
	if len(log)+eventSizeLineFeed != eventSize {
		return "", fmt.Errorf("event_size invariant violated: got %d bytes, want %d", len(log)+eventSizeLineFeed, eventSize)
	}
	return log, nil
}
