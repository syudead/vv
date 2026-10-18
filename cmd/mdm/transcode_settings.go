package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/syudead/vv/internal/app"
)

// startTranscodeSettings starts encoder probing asynchronously; software remains effective until
// completion, and ctx cancellation stops the probe.
func startTranscodeSettings(
	ctx context.Context,
	store app.TranscodeSettingsStore,
	checker app.EncoderChecker,
	goos string,
	logger *slog.Logger,
) (*app.TranscodeSettings, error) {
	settings, err := app.NewTranscodeSettings(ctx, app.TranscodeSettingsOptions{
		Store: store, Checker: checker, GOOS: goos, Logger: logger,
	})
	if err != nil {
		return nil, fmt.Errorf("cannot read the transcode video encoder setting: %w", err)
	}
	current := settings.Current()
	logger.Info("transcode video encoder loaded; checking hardware encoders in the background",
		slog.String("choice", string(current.Choice)),
		slog.String("effective", string(current.Effective)),
		slog.Bool("checking", current.Checking),
	)
	settings.StartChecks(ctx)
	return settings, nil
}
