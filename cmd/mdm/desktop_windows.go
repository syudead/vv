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

// Windows desktop entry point: build with -tags desktop -ldflags "-H=windowsgui". This file wires
// startup and shutdown; internal/desktop owns Win32 and WebView2.

// Bound shutdown before go-webview2 terminates the process after a fatal error.
const fatalStopGrace = 30 * time.Second

// Wait for a previous instance that has hidden its window but is still shutting down.
const instanceWait = 30 * time.Second

// Show confirmation if the busy-state query times out.
const busyCheckTimeout = 3 * time.Second

func init() {
	// Win32 and WebView2 must run on the thread that created them.
	desktop.LockUIThread()
}

func main() {
	os.Exit(runDesktop())
}

// runDesktop returns an exit code and shows only the first startup failure in a dialog.
func runDesktop() int {
	// Join the job first so child processes inherit it; log failures after opening the log.
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

	// Detect duplicate instances before opening the database or rotating the running instance's log.
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

	// Prepare logging first so even the first prerequisite failure is recorded.
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
	if dirErr != nil {
		var unwritable *desktop.UnwritableDirError
		dir := paths.Root
		if errors.As(dirErr, &unwritable) {
			dir = unwritable.Dir
		}
		desktop.ShowError(desktop.MessageUnwritable(dir))
		return 1
	}
	webView2Version, err := desktop.WebView2Version()
	if err != nil {
		logger.Error("startup check failed", slog.Any("error", err))
		desktop.ShowError(desktop.MessageNoWebView2(paths.LogFile))
		return 1
	}
	logger.Info("found the WebView2 Runtime", slog.String("version", webView2Version))

	// Prefer bundled ffmpeg over user-installed versions.
	ffmpegDir := desktop.FFmpegDir(exe)
	if err := os.Setenv("PATH", desktop.PrependPath(ffmpegDir, os.Getenv("PATH"))); err != nil {
		logger.Error("cannot add ffmpeg to PATH", slog.Any("error", err))
		desktop.ShowError(desktop.MessageFailed(err, paths.LogFile))
		return 1
	}

	// run reports database and listener failures by startup stage.
	cfg := desktopConfig(paths, port)
	listening := make(chan struct{})
	stopRequested := make(chan struct{})
	var stopOnce sync.Once
	requestStop := func() { stopOnce.Do(func() { close(stopRequested) }) }
	var busyProbe atomic.Pointer[func(context.Context) (bool, error)]
	// Broadcast completion because both window-close and WebView2 failure handlers wait for it.
	server := startServer(func() error {
		return run(desktopRunOptions(
			cfg,
			logOutput,
			func() { close(listening) },
			func(busy func(context.Context) (bool, error)) { busyProbe.Store(&busy) },
			stopRequested,
		))
	})

	select {
	case <-listening:
	case <-server.Done():
		err := server.Err()
		logger.Error("could not start the server", slog.Any("error", err))
		desktop.ShowError(startupFailureMessage(err, port, paths.LogFile))
		return 1
	}

	// Open the window only after the listener is ready, avoiding a blank window on startup failure.
	url := "http://localhost:" + strconv.Itoa(port) + "/"
	window, err := desktop.NewWindow(desktop.WindowOptions{
		URL:      url,
		DataPath: paths.WebView2,
		// Confirm active ingestion; live transcoding does not require confirmation.
		ShouldConfirmClose: func() bool { return closeNeedsConfirmation(busyProbe.Load(), logger) },
		// Hide first, then destroy the window after shutdown.
		OnClose: requestStop,
		// Sign-out and system shutdown wait for the same cleanup without confirmation.
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

// Confirm when busy-state lookup fails so ingestion cannot be silently interrupted.
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

// run chooses the listener host from saved LAN access; desktop mode ignores forwarded headers
// because there is no reverse proxy.
func desktopConfig(paths desktop.Paths, port int) Config {
	return Config{
		Addr:           net.JoinHostPort("127.0.0.1", strconv.Itoa(port)),
		DataDir:        paths.Data,
		LogLevel:       defaultLogLevel,
		TrustedProxies: nil,
	}
}

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
