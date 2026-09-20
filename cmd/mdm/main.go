// Command mdm は JSON API と埋め込み SPA を配信する単一バイナリである。
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi"
	"github.com/syudead/vv/internal/media"
	"github.com/syudead/vv/internal/store"
	"github.com/syudead/vv/web"
)

// version はリリース名である。ビルド時に
// -ldflags "-X main.version=…" で上書きできる（research.md R-007）。
var version = domain.DefaultVersion

// shutdownGrace は停止指示を受けてから処理中の要求を待つ猶予である
// （contracts/http-routes.md）。
const shutdownGrace = 10 * time.Second

// readHeaderTimeout は要求ヘッダの読み取りに与える上限である。
const readHeaderTimeout = 10 * time.Second

func main() {
	if err := run(); err != nil {
		// 記録の設定前に失敗する場合もあるため、利用者向けの説明は標準エラーへ出す。
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run は起動から停止までを行う。順序は
// 「設定読み込み → 記録の設定 → 起動前確認 → データベース接続とマイグレーション →
// HTTP サーバー起動」である。
func run() error {
	cfg, err := LoadConfig(os.Getenv)
	if err != nil {
		return err
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.Level()}))
	slog.SetDefault(logger)

	build := buildInfo()

	// どの設定で動いているかを後から追跡できるように、有効な設定値と
	// バージョン情報を1行で記録する（FR-007）。
	logger.LogAttrs(context.Background(), slog.LevelInfo, "起動します",
		append(cfg.LogAttrs(),
			slog.String("version", build.Version),
			slog.String("commit", build.Commit),
			slog.Time("builtAt", build.BuiltAt),
		)...)

	if err := checkPreconditions(cfg); err != nil {
		return err
	}

	db, err := store.Open(cfg.DataDir)
	if err != nil {
		return err
	}
	defer func() {
		if err := db.Close(); err != nil {
			logger.Warn("データベースを閉じられませんでした", slog.Any("error", err))
		}
	}()

	migrated, err := store.Migrate(context.Background(), db)
	if err != nil {
		return err
	}
	logger.Info("マイグレーションを適用しました",
		slog.String("database", db.Path()),
		slog.Int("applied", migrated.Applied),
		slog.Int64("version", migrated.Version),
	)

	// 走査とジョブは HTTP とは別の寿命で動く。停止指示でこの context を
	// 取り消すと、処理中のジョブは queued に残り、次の起動で再開できる。
	backgroundCtx, stopBackground := context.WithCancel(context.Background())
	defer stopBackground()

	lib := newLibrary(cfg, db, logger)
	lib.bindContext(backgroundCtx)

	// 前回の停止で running のまま残った走査を閉じる。閉じないと
	// 「実行中は1件だけ」の制約が働いたまま二度と取り込みを始められない。
	if err := lib.recoverInterrupted(backgroundCtx); err != nil {
		return err
	}

	worker := newWorker(cfg, db, logger)
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		worker.Run(backgroundCtx)
	}()

	handler := httpapi.NewRouter(httpapi.Options{
		Build:         build,
		Pinger:        db,
		Videos:        db,
		Playback:      db,
		Scans:         lib,
		ThumbnailsDir: cfg.ThumbnailsDir(),
		Assets:        web.Dist(),
		Logger:        logger,
	})

	if err := serve(cfg, handler, logger, nil); err != nil {
		return err
	}

	// HTTP の猶予待ちが終わってから、走査とワーカーを止める。処理中の
	// ジョブは running のまま残るが、次の起動で queued へ戻る（R-106）。
	stopBackground()
	<-workerDone
	logger.Info("取り込みとジョブを停止しました")

	return nil
}

// serve は HTTP サーバーを起動し、停止指示を待つ。
//
// SIGINT / SIGTERM を受けたら新規の接続受付を止め、処理中の要求を猶予時間まで
// 待ってから終了する。正常終了の終了コードは 0 である。
func serve(
	cfg Config,
	handler http.Handler,
	logger *slog.Logger,
	onListening func(),
) error {
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
	}

	listener, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return fmt.Errorf("待ち受けに失敗しました (%s): %w", cfg.Addr, err)
	}
	defer func() { _ = listener.Close() }()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	listenErr := make(chan error, 1)
	logger.Info("待ち受けを開始しました", slog.String("addr", cfg.Addr))
	if onListening != nil {
		onListening()
	}
	go func() {
		listenErr <- srv.Serve(listener)
	}()

	select {
	case err := <-listenErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("待ち受けに失敗しました (%s): %w", cfg.Addr, err)
		}
		return nil

	case <-ctx.Done():
		// 2 度目の指示で即座に終われるよう、通知の受け取りは戻しておく。
		stop()
		logger.Info("停止指示を受けました。処理中の要求を待ちます",
			slog.String("grace", shutdownGrace.String()))

		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("猶予 %s 以内に停止できませんでした: %w", shutdownGrace, err)
		}
		logger.Info("停止しました")
		return nil
	}
}

// checkPreconditions は起動前の前提を確認する。設定の不備と外部コマンドの不足を
// まとめて列挙し、設定を1回直すごとに再起動する往復を避ける
// （contracts/configuration.md）。
func checkPreconditions(cfg Config) error {
	problems := cfg.verifyProblems()

	if err := media.Preflight(); err != nil {
		problems = append(problems, err)
	}

	if len(problems) > 0 {
		return joinProblems("起動前の確認に失敗しました", problems)
	}
	return nil
}

// buildInfo は稼働中のバイナリを特定するための情報を集める。
// コミットと時刻はビルド時に Go が埋めるため、Makefile に git の呼び出しは要らない。
func buildInfo() domain.BuildInfo {
	info := domain.BuildInfo{Version: version}
	if info.Version == "" {
		info.Version = domain.DefaultVersion
	}

	read, ok := debug.ReadBuildInfo()
	if !ok {
		return info
	}

	for _, setting := range read.Settings {
		switch setting.Key {
		case "vcs.revision":
			info.Commit = setting.Value
		case "vcs.time":
			if at, err := time.Parse(time.RFC3339, setting.Value); err == nil {
				info.BuiltAt = at
			}
		}
	}
	return info
}
