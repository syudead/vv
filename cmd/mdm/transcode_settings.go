package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/syudead/vv/internal/app"
)

// startTranscodeSettings はライブ変換の映像エンコード方式の設定を作り、保存値を読み、
// 起動時の確認を背後で始めて待たずに戻る（specs/025-hardware-encoding/research.md R-2）。
// 確認が終わるまで、実際に使う方式は software である。確認は ctx の取り消しで止まる。
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
