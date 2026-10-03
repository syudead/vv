//go:build windows && desktop

package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/syudead/vv/internal/desktop"
)

// Windows デスクトップ版（VVMDM.exe）の入口。`go build -tags desktop -ldflags
// "-H=windowsgui"` で組む。設定は環境変数（MDM_*）でなく自分で組み立て、run を同じ
// プロセスで呼び、待ち受けまで済んだらウィンドウを出す。ウィンドウを閉じたら
// run の停止手順を通して終わる（specs/037-windows-app/research.md R-1〜R-5、R-10、R-11）。
// 閉じる前の確認・二重起動・サインアウトの扱いは R-6〜R-8 による。
// このファイルは組み立てと配線だけを持ち、Win32 と WebView2 は internal/desktop が扱う。

// fatalStopGrace は WebView2 が続けられなくなったときに停止を待つ上限である。
// go-webview2 はそのあとプロセスを終わらせる。
const fatalStopGrace = 30 * time.Second

// instanceWait は、同じセッションの前のプロセスが停止の途中（ウィンドウを隠した
// あと）のときに、終わるのを待つ上限である（research.md R-6）。
const instanceWait = 30 * time.Second

// busyCheckTimeout は閉じる前の確認で「取り込みの途中か」を読む上限である。
// 読めなければ確認を出す。
const busyCheckTimeout = 3 * time.Second

func init() {
	// ウィンドウと WebView2 は作ったスレッドでしか扱えないので、main の goroutine を
	// 最初のスレッドに固定する。
	desktop.LockUIThread()
}

func main() {
	os.Exit(runDesktop())
}

// runDesktop は起動から終わりまでを行い、終了コードを返す。起動の確認は
// contracts/windows-app.md §2 の順に行い、最初の失敗だけをダイアログで示す。
func runDesktop() int {
	// 子プロセス（ffmpeg）も同じジョブに入るよう、最初に入る。失敗しても起動は
	// 続け、ログを開いてから記録する。
	jobErr := desktop.JoinKillOnCloseJob()

	port, err := desktop.ParseArgs(os.Args[1:])
	if err != nil {
		desktop.ShowError(desktop.MessageBadArgs(err))
		return 2
	}

	localAppData, err := desktop.LocalAppData()
	if err != nil {
		desktop.ShowError(desktop.MessageFailed(err, "(unavailable)"))
		return 1
	}
	paths, err := desktop.ResolvePaths(localAppData)
	if err != nil {
		desktop.ShowError(desktop.MessageFailed(err, "(unavailable)"))
		return 1
	}

	// 二重起動は DB を開く前、ログを開く前に判定する（research.md R-6）。ログを先に
	// 開くと、起動中のプロセスのログを前回分へ移そうとしてしまう。
	instance, err := desktop.AcquireInstance(paths.Root, instanceWait)
	if err != nil {
		desktop.ShowError(desktop.MessageFailed(err, "(unavailable)"))
		return 1
	}
	switch instance {
	case desktop.InstanceActivated:
		return 0
	case desktop.InstanceOtherSession:
		desktop.ShowError(desktop.MessageOtherSession())
		return 1
	case desktop.InstanceStillRunning:
		desktop.ShowError(desktop.MessageStillRunning())
		return 1
	}

	// データの置き場とログは先に用意し、失敗は確認の 2 番目として示す。1 番目の
	// 失敗もログに残せるようにするため。
	logOutput := io.Discard
	dirErr := desktop.PrepareDirs(paths)
	if dirErr == nil {
		logFile, err := desktop.OpenLog(paths)
		if err != nil {
			dirErr = &desktop.UnwritableDirError{Dir: paths.Logs, Err: err}
		} else {
			defer func() { _ = logFile.Close() }()
			logOutput = logFile
		}
	}
	logger := slog.New(slog.NewJSONHandler(logOutput, nil)).With(slog.String("component", "desktop"))
	if jobErr != nil {
		logger.Warn("could not join a job object; ffmpeg may outlive a killed process", slog.Any("error", jobErr))
	}

	// 1. zip から直接でなく、隣に同梱の ffmpeg がある。
	exe, err := os.Executable()
	if err != nil {
		logger.Error("cannot find the executable", slog.Any("error", err))
		desktop.ShowError(desktop.MessageFailed(err, paths.LogFile))
		return 1
	}
	if err := desktop.CheckBundle(exe, desktop.TempDir(), desktop.FileExists); err != nil {
		logger.Error("startup check failed", slog.Any("error", err))
		desktop.ShowError(desktop.MessageNotExtracted(paths.LogFile))
		return 1
	}
	// 2. データの置き場に書ける。
	if dirErr != nil {
		var unwritable *desktop.UnwritableDirError
		dir := paths.Root
		if errors.As(dirErr, &unwritable) {
			dir = unwritable.Dir
		}
		desktop.ShowError(desktop.MessageUnwritable(dir))
		return 1
	}
	// 3. WebView2 ランタイムがある。
	webView2Version, err := desktop.WebView2Version()
	if err != nil {
		logger.Error("startup check failed", slog.Any("error", err))
		desktop.ShowError(desktop.MessageNoWebView2(paths.LogFile))
		return 1
	}
	logger.Info("found the WebView2 Runtime", slog.String("version", webView2Version))

	// 同梱の ffmpeg を、利用者が入れた別の ffmpeg より先に使う。
	ffmpegDir := desktop.FFmpegDir(exe)
	if err := os.Setenv("PATH", desktop.PrependPath(ffmpegDir, os.Getenv("PATH"))); err != nil {
		logger.Error("cannot add ffmpeg to PATH", slog.Any("error", err))
		desktop.ShowError(desktop.MessageFailed(err, paths.LogFile))
		return 1
	}

	// 4・5. データベースと待ち受けは run が行い、失敗の段階で示す文を選ぶ。
	cfg := desktopConfig(paths, port)
	listening := make(chan struct{})
	stopRequested := make(chan struct{})
	var stopOnce sync.Once
	requestStop := func() { stopOnce.Do(func() { close(stopRequested) }) }
	// 閉じる前の確認が読む「取り込みの途中か」。run が走査を用意したら入る。
	var busyProbe atomic.Pointer[func(context.Context) (bool, error)]
	// 終わりは閉じる操作の goroutine と WebView2 の失敗の処理の両方が待つので、
	// 1 つの値を受け合う channel でなく、全員に届く知らせにする。
	server := startServer(func() error {
		return run(runOptions{
			Config:      cfg,
			LogOutput:   logOutput,
			Listener:    newReopenableListener(),
			OnListening: func() { close(listening) },
			OnBusyProbe: func(busy func(context.Context) (bool, error)) { busyProbe.Store(&busy) },
			NotifyStop: func() (<-chan struct{}, func()) {
				return stopRequested, nil
			},
		})
	})

	select {
	case <-listening:
	case <-server.Done():
		err := server.Err()
		logger.Error("could not start the server", slog.Any("error", err))
		desktop.ShowError(startupFailureMessage(err, port, paths.LogFile))
		return 1
	}

	// 待ち受けまで済んだので、ウィンドウが空白のまま残ることはない（R-10）。
	url := "http://localhost:" + strconv.Itoa(port) + "/"
	window, err := desktop.NewWindow(desktop.WindowOptions{
		URL:      url,
		DataPath: paths.WebView2,
		// 取り込みの途中なら確認を出す。ライブ変換の配信は含めない（R-7）。
		ShouldConfirmClose: func() bool { return closeNeedsConfirmation(busyProbe.Load(), logger) },
		// ウィンドウは先に隠れ、停止を終えてから壊す。
		OnClose: requestStop,
		// サインアウト・シャットダウンでは確認なしに同じ停止手順を通し、終わるまで
		// 戻らない（R-8）。
		OnEndSession: func() {
			requestStop()
			if !waitAtMost(func() { <-server.Done() }, fatalStopGrace) {
				logger.Warn("the server did not stop before the session ended")
			}
		},
		OnFatal: func(err error) {
			desktop.ShowError(desktop.MessageFailed(err, paths.LogFile))
			requestStop()
			select {
			case <-server.Done():
			case <-time.After(fatalStopGrace):
				logger.Warn("the server did not stop within the grace period")
			}
			os.Exit(1)
		},
		Logger: logger,
	})
	if err != nil {
		logger.Error("cannot open the window", slog.Any("error", err))
		requestStop()
		stopErr := server.Err()
		desktop.ShowError(desktop.MessageFailed(errors.Join(err, stopErr), paths.LogFile))
		return 1
	}

	// 停止を終えたら（閉じる操作でも、待ち受けを失ったときでも）ウィンドウを壊す。
	go func() {
		<-server.Done()
		window.Destroy()
	}()
	window.Run()

	if err := server.Err(); err != nil {
		logger.Error("the server stopped with an error", slog.Any("error", err))
		desktop.ShowError(desktop.MessageFailed(err, paths.LogFile))
		return 1
	}
	return 0
}

// closeNeedsConfirmation は閉じる前に確認を出すかを返す。取り込みの途中か読めない
// ときは、中断を知らせずに閉じないよう確認を出す。
func closeNeedsConfirmation(probe *func(context.Context) (bool, error), logger *slog.Logger) bool {
	if probe == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), busyCheckTimeout)
	defer cancel()
	busy, err := (*probe)(ctx)
	if err != nil {
		logger.Warn("could not tell whether an import is in progress; asking before closing", slog.Any("error", err))
		return true
	}
	return busy
}

// desktopConfig はデスクトップ版の設定を組み立てる。待ち受けはループバックだけで、
// 前に逆プロキシは無いので転送ヘッダを読まない（MDM_TRUSTED_PROXIES=none 相当、R-14）。
func desktopConfig(paths desktop.Paths, port int) Config {
	return Config{
		Addr:           net.JoinHostPort("127.0.0.1", strconv.Itoa(port)),
		DataDir:        paths.Data,
		LogLevel:       defaultLogLevel,
		TrustedProxies: nil,
	}
}

// startupFailureMessage は run が待ち受けの前に返した誤りを、示す文にする。
func startupFailureMessage(err error, port int, logFile string) string {
	if err == nil {
		err = errors.New("the server stopped before it started listening")
	}
	switch startupStageOf(err) {
	case stageDatabase:
		return desktop.MessageDatabase(err, logFile)
	case stageListen:
		return desktop.MessagePortInUse(port, logFile)
	default:
		return desktop.MessageFailed(err, logFile)
	}
}
