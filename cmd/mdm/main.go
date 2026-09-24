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
	"sync"
	"syscall"
	"time"

	"github.com/syudead/vv/internal/app"
	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi"
	"github.com/syudead/vv/internal/jobs"
	"github.com/syudead/vv/internal/media"
	"github.com/syudead/vv/internal/opener"
	"github.com/syudead/vv/internal/scanner"
	"github.com/syudead/vv/internal/store"
	"github.com/syudead/vv/web"
)

// version はリリース名である。ビルド時に
// -ldflags "-X main.version=…" で上書きできる。
var version = domain.DefaultVersion

// shutdownGrace は停止指示を受けてから処理中の要求を待つ猶予である。
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
	// バージョン情報を1行で記録する。
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

	// 照合用の鍵が古い規則のままの所在を、ジョブや HTTP を動かす前に作り直す。
	// 失敗したら起動を止める（古い鍵のまま検索を出さない）。途中まで書いた分は
	// 版が行ごとに残るので、次の起動で続きから埋まる。
	refreshed, err := db.RefreshSearchKeys(context.Background())
	if err != nil {
		return fmt.Errorf("照合用の鍵を作り直せません: %w", err)
	}
	logger.Info("照合用の鍵を作り直しました", slog.Int("locations", refreshed))

	// 走査とジョブは HTTP とは別の寿命で動く。停止指示でこの context を
	// 取り消すと、処理中のジョブは queued に残り、次の起動で再開できる。
	backgroundCtx, stopBackground := context.WithCancel(context.Background())
	defer stopBackground()

	// 画面へ送る変化の知らせ。走査とワーカーが知らせ、/api/events が配る。
	events := httpapi.NewEvents()

	scans := app.NewScans(app.ScansOptions{
		Store: db,
		NewScanner: func(reporter app.ScanReporter) app.Scanner {
			return scanner.New(scanner.Options{Index: db, Queue: db, Reporter: reporter, Logger: logger})
		},
		Context:  backgroundCtx,
		Notifier: events,
		Logger:   logger,
	})

	// 前回の停止で running のまま残った走査を閉じ、処理中だった仕事を戻す。
	if err := scans.RecoverInterrupted(backgroundCtx); err != nil {
		return err
	}

	// 前回の停止で残った生成途中の成果物を消す。ワーカーを動かす前なので、
	// 生成中のものを消すことはない。
	if err := media.RemoveTemporary(cfg.ThumbnailsDir()); err != nil {
		logger.Warn("生成途中の成果物を削除できませんでした", slog.Any("error", err))
	}
	assets := media.NewAssets(cfg.ThumbnailsDir())
	ingest := app.NewIngest(app.IngestOptions{
		Store: db, Generator: assets, Notifier: events, Logger: logger,
	})
	// 取り込みの段階ごとにワーカーを置く。仕事を積んだ取引が確定したら、その
	// 段階のワーカーを起こす。ワーカーは待ち行列を一定間隔で問い合わせない。
	workers := make([]*jobs.Worker, 0, len(domain.JobKinds))
	for _, kind := range domain.JobKinds {
		worker := jobs.New(jobs.Options{
			Kind:     kind,
			Queue:    db,
			Handler:  ingest.Handler(kind),
			Finished: ingest.JobFinished,
			Logger:   logger,
		})
		ingest.AttachWorker(kind, worker)
		workers = append(workers, worker)
	}
	db.OnJobsChanged(ingest.JobsChanged)
	db.OnVideosDeleted(ingest.VideosDeleted)
	var workersDone sync.WaitGroup
	for _, worker := range workers {
		workersDone.Go(func() { worker.Run(backgroundCtx) })
	}

	// request単位のtranscode processはHTTP requestより長生きさせない。Shutdownは
	// 実行中requestのcontextを取り消さないため、server寿命を別に持って先にcancelする。
	requestMediaCtx, stopRequestMedia := context.WithCancel(context.Background())
	defer stopRequestMedia()

	// 既定アプリを起動できる環境かどうかは、ここで1度だけ決める。要求のたびには
	// コマンドを探さない。
	fileOpener := opener.New()
	logger.Info("ファイルを開く機能の状態", slog.Bool("available", fileOpener.Available()),
		slog.String("command", fileOpener.Command()))

	// 動画の応答に要る判断（消えたプレビューの作り直し、シーク用プレビューの
	// 状態）と関連動画の組み立ては、アプリケーション層が行う。
	catalog := app.NewCatalog(app.CatalogOptions{Store: db, Files: assets, Logger: logger})
	// 設定画面のメディアフォルダは、パスをファイルシステムで確かめてから保存する。
	mediaFolders := app.NewMediaFolders(app.MediaFoldersOptions{Store: db, Checker: scanner.NewFolderChecker()})

	handler := httpapi.NewRouter(httpapi.Options{
		Build:          build,
		Pinger:         db,
		Videos:         db,
		Playback:       db,
		Scans:          scans,
		MediaFolders:   mediaFolders,
		Folders:        db,
		ThumbnailsDir:  cfg.ThumbnailsDir(),
		Transcoder:     media.NewLiveTranscoder(requestMediaCtx.Done()),
		SeekThumbnails: media.NewSeekThumbnailCache(cfg.ThumbnailsDir()),
		Catalog:        catalog,
		Opener:         fileOpener,
		Processing:     db,
		Events:         events,
		Assets:         web.Dist(),
		Logger:         logger,
	})

	// 変化の知らせの接続は終わりが無いので、停止の猶予待ちより先に閉じる。
	beforeShutdown := func() {
		stopRequestMedia()
		events.Close()
	}
	if err := serve(cfg, handler, logger, nil, beforeShutdown); err != nil {
		return err
	}

	// HTTP の猶予待ちが終わってから、走査とワーカーを止める。処理中の
	// ジョブは running のまま残るが、次の起動で queued へ戻る。
	stopBackground()
	workersDone.Wait()
	// 背後で動いている生成物の削除を、データベースを閉じる前に終える。
	// 途中で閉じると、消すはずの生成物が残り続ける。
	ingest.Wait()
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
	beforeShutdown func(),
) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return serveUntil(ctx.Done(), stop, cfg, handler, logger, onListening, beforeShutdown)
}

func serveUntil(
	stopRequested <-chan struct{},
	restoreSignals func(),
	cfg Config,
	handler http.Handler,
	logger *slog.Logger,
	onListening func(),
	beforeShutdown func(),
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

	case <-stopRequested:
		// 2 度目の指示で即座に終われるよう、通知の受け取りは戻しておく。
		if restoreSignals != nil {
			restoreSignals()
		}
		logger.Info("停止指示を受けました。処理中の要求を待ちます",
			slog.String("grace", shutdownGrace.String()))
		if beforeShutdown != nil {
			beforeShutdown()
		}

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
// まとめて列挙し、設定を1回直すごとに再起動する往復を避ける。
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
// コミットと時刻はビルド時に Go が埋めるため、Taskfile に git の呼び出しは要らない。
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
