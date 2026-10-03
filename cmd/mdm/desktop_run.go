package main

import (
	"context"
	"io"
)

// desktopRunOptions はデスクトップ版の入口（desktop_windows.go）が run へ渡す引数を
// 組み立てる。Desktop を真にするので、待ち受けのホストは LAN からの接続の許可の保存値で
// 決まり、/api/settings/network が使える（specs/037-windows-app/research.md R-14）。
// desktop タグの無いビルドでも試験できるよう、タグなしのファイルに置く。
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
