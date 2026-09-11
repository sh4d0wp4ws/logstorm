package main

import (
	"context"
	"errors"
	"fmt"
)

type streamResult struct {
	name string
	err  error
}

func RunStreams(ctx context.Context, streams []Stream) error {
	if len(streams) == 0 {
		return fmt.Errorf("at least one stream is required")
	}

	streamContext, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make(chan streamResult, len(streams))
	for _, stream := range streams {
		stream := stream
		go func() {
			results <- streamResult{name: stream.Name, err: GenerateContext(streamContext, stream.Option)}
		}()
	}

	var primaryErr error
	for range streams {
		result := <-results
		if result.err == nil || errors.Is(result.err, context.Canceled) {
			continue
		}
		if primaryErr == nil {
			primaryErr = fmt.Errorf("stream %q: %w", result.name, result.err)
			cancel()
		}
	}
	if primaryErr != nil {
		return primaryErr
	}
	return ctx.Err()
}
