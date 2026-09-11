package main

import (
	"context"
	"errors"
	"math/rand"
	"os"
	"os/signal"
	"time"

	"github.com/mingrammer/cfmt"
)

func main() {
	rand.Seed(time.Now().UnixNano())
	opts := ParseOptions()
	if opts.Config != "" {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		streams, err := LoadStreams(opts.Config)
		if err == nil {
			err = RunStreams(ctx, streams)
		}
		if errors.Is(err, context.Canceled) {
			return
		}
		if err != nil {
			cfmt.Warningln(err.Error())
			os.Exit(1)
		}
		return
	}
	if err := Run(opts); err != nil {
		cfmt.Warningln(err.Error())
	}
}
