package main

import (
	"context"
	"io"
)

// Keep desktop option assembly untagged so ordinary builds can test it.
func desktopRunOptions(
	cfg Config,
	logOutput io.Writer,
	onListening func(),
	onBusyProbe func(busy func(context.Context) (bool, error)),
	stopRequested <-chan struct{},
) runOptions {
	return runOptions{
		Config:      cfg,
		LogOutput:   logOutput,
		Listener:    newReopenableListener(),
		OnListening: onListening,
		OnBusyProbe: onBusyProbe,
		NotifyStop: func() (<-chan struct{}, func()) {
			return stopRequested, nil
		},
		Desktop: true,
	}
}
