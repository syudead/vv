// Command mdm は JSON API と埋め込み SPA を配信する単一バイナリである。
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os/signal"
	"runtime"
	"runtime/debug"
	"sync"
	"syscall"
	"time"
	// 視聴履歴の日付は画面が送る IANA の地域名で数える（specs/043-watch-history/research.md R-11）。
	// 地域の情報を持たないホスト（Windows や最小のコンテナ）でも time.LoadLocation が読めるよう埋め込む。
	_ "time/tzdata"

	"github.com/syudead/vv/internal/app"
	"github.com/syudead/vv/internal/artifacts"
	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/eventbus"
	"github.com/syudead/vv/internal/httpapi"
	"github.com/syudead/vv/internal/jobs"
	"github.com/syudead/vv/internal/media"
	"github.com/syudead/vv/internal/mediafs"
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

// scanStopGrace は停止時に走査の終わりを待つ上限である。走査は取り消しを見て
// 止まるが、応答しない置き場（切れた NAS など）の読み取りは取り消しでは戻らない。
const scanStopGrace = 10 * time.Second

// readHeaderTimeout は要求ヘッダの読み取りに与える上限である。
const readHeaderTimeout = 10 * time.Second

// runOptions は起動・停止の手順に外から渡すものである。タグなしの main は環境変数の
// 設定・標準出力・MDM_ADDR・SIGINT/SIGTERM を渡す。デスクトップ版は設定を自分で
// 組み立て、同じ手順を同じプロセスで呼ぶ（specs/037-windows-app/research.md R-1）。
type runOptions struct {
	Config Config
	// LogOutput は JSON の記録の書き先である。
	LogOutput io.Writer
	// Listener は Config.Addr で待ち受けを作る部品で、アドレスを変えて開き直せる。
	Listener *reopenableListener
	// OnListening は待ち受けを開いた直後に呼ぶ。nil なら呼ばない。
	OnListening func()
	// NotifyStop は停止の指示の受け取りを始める。HTTP サーバーを起動する直前に呼ぶ。
	NotifyStop stopNotifier
	// Desktop はデスクトップ版の起動である。真なら待ち受けのホストを LAN からの接続の
	// 許可の保存値で決め（Config.Addr のポートを使う）、/api/settings/network で切り替え
	// られるようにする。偽なら Config.Addr のまま待ち受け、その経路は 404 を返す
	// （specs/037-windows-app/research.md R-14）。
	Desktop bool
	// OnBusyProbe は走査を用意したあと、待ち受けを開く前に、取り込みの途中かを問う
	// 関数（app.Scans.Busy）を渡す。デスクトップ版の閉じる前の確認が使う
	// （specs/037-windows-app/research.md R-7）。nil なら呼ばない。
	OnBusyProbe func(busy func(context.Context) (bool, error))
}

// stopNotifier は停止の指示の受け取りを始め、指示で閉じる channel と、受け取りを
// やめる関数（nil でよい）を返す。
type stopNotifier func() (stopRequested <-chan struct{}, release func())

// notifySignals は SIGINT / SIGTERM を停止の指示として受け取る。
func notifySignals() (<-chan struct{}, func()) {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	return ctx.Done(), stop
}

// run は起動から停止までを行う。順序は
// 「記録の設定 → 起動前確認 → データベース接続とマイグレーション →
// HTTP サーバー起動」である。
func run(opts runOptions) error {
	cfg := opts.Config
	logger := slog.New(slog.NewJSONHandler(opts.LogOutput, &slog.HandlerOptions{Level: cfg.Level()}))
	slog.SetDefault(logger)

	build := buildInfo()

	// どの設定で動いているかを後から追跡できるように、有効な設定値と
	// バージョン情報を1行で記録する。
	logger.LogAttrs(context.Background(), slog.LevelInfo, "starting",
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
		return &startupError{stage: stageDatabase, err: err}
	}
	defer func() {
		if err := db.Close(); err != nil {
			logger.Warn("could not close the database", slog.Any("error", err))
		}
	}()

	migrated, err := store.Migrate(context.Background(), db)
	if err != nil {
		return &startupError{stage: stageDatabase, err: err}
	}
	logger.Info("applied migrations",
		slog.String("database", db.Path()),
		slog.Int("applied", migrated.Applied),
		slog.Int64("version", migrated.Version),
	)
	libraryStore := db.Library()

	// 照合用の鍵が古い規則のままの所在を、ジョブや HTTP を動かす前に作り直す。
	// 失敗したら起動を止める（古い鍵のまま検索を出さない）。途中まで書いた分は
	// 版が行ごとに残るので、次の起動で続きから埋まる。
	refreshed, err := libraryStore.RefreshSearchKeys(context.Background())
	if err != nil {
		return fmt.Errorf("cannot rebuild the search keys: %w", err)
	}
	logger.Info("rebuilt the search keys", slog.Int("locations", refreshed))

	// タグ名の照合用の鍵も同じ時点で作り直す。メディアフォルダに依らないので
	// folderMu は取らない（specs/014-video-tags/data-model.md §7）。数はタグの名前と
	// 却下した名前の行の合計なので、属性は names と呼ぶ。
	tagsRefreshed, err := db.Tags().RefreshSearchKeys(context.Background())
	if err != nil {
		return fmt.Errorf("cannot rebuild the tag search keys: %w", err)
	}
	logger.Info("rebuilt the tag search keys", slog.Int("names", tagsRefreshed))

	// 移行の前に書かれた視聴履歴の行に、題名の照合形（title_key）を埋める。動画の無い件の検索が
	// 読むので、受け付けより前に行い、失敗したら起動を止める
	// （specs/043-watch-history/data-model.md「Migration」）。
	titleKeysFilled, err := db.Playback().RefreshWatchHistoryTitleKeys(context.Background())
	if err != nil {
		return fmt.Errorf("cannot fill the watch history title keys: %w", err)
	}
	logger.Info("filled the watch history title keys", slog.Int("entries", titleKeysFilled))

	// フォルダの索引（グループとフォルダ名）を、規則の版が古いか前回の作り直しが
	// 失敗していたときだけ作り直す。title_key は照合用の鍵の規則で作るので、その
	// 作り直しの後に行う。失敗しても前の索引のまま起動し、次のスキャン・例外の
	// 変更・メディアフォルダの変更か次の起動で作り直す
	// （specs/017-folder-groups/data-model.md §3）。
	if rebuilt, err := db.ScanIndex().RefreshFolderIndex(context.Background()); err != nil {
		logger.Warn("could not rebuild the folder index", slog.Any("error", err))
	} else if rebuilt {
		logger.Info("rebuilt the folder index")
	}

	// 認証の準備。期限切れのセッションを消し、未設定なら初回設定を促す。
	authStore := db.Auth()
	prepareAuth(context.Background(), authStore, time.Now(), logger)

	// LAN からの接続の許可はデスクトップ版だけが持つ。DB を開いたあと、ジョブや待ち受けを
	// 動かす前に保存値を読み、待ち受けるアドレスを決める。nil のままなら経路は 404 を返す。
	var networkSettings httpapi.NetworkSettings
	if opts.Desktop {
		addr, settings, err := startNetworkSettings(context.Background(), db.Settings(), opts.Listener, cfg.Addr, logger)
		if err != nil {
			return &startupError{stage: stageDatabase, err: err}
		}
		cfg.Addr = addr
		networkSettings = settings
	}

	// 走査とジョブは HTTP とは別の寿命で動く。停止指示でこの context を
	// 取り消すと、処理中のジョブは queued に残り、次の起動で再開できる。
	backgroundCtx, stopBackground := context.WithCancel(context.Background())
	defer stopBackground()

	// 状態の変化の配り先。保存層・取り込み・走査はここへ発行するだけで、
	// 誰が受け取るかを知らない。受け取る側は subscribeEvents で登録する。
	bus := eventbus.New(logger)
	db.PublishTo(bus)
	// 画面へ送る変化の知らせ。/api/events が配る。
	events := httpapi.NewEvents()

	ingestStore := db.Ingest()
	scanStore := db.Scans()
	scanIndexStore := db.ScanIndex()
	settingsStore := db.Settings()
	playbackStore := db.Playback()

	scans := app.NewScans(app.ScansOptions{
		Store: scanStore,
		Jobs:  ingestStore,
		// 閉じる確認が読む「取り込みの途中か」（Busy）の未完了の仕事の有無。
		UnfinishedJobs: ingestStore,
		FolderIndex:    scanIndexStore,
		NewScanner: func(reporter app.ScanReporter) app.Scanner {
			return scanner.New(scanner.Options{
				Index: scanIndexStore, Queue: ingestStore, Reporter: reporter, Logger: logger,
			})
		},
		Folders:   scanIndexStore,
		Context:   backgroundCtx,
		Publisher: bus,
		Logger:    logger,
	})

	// メディアフォルダの変更を検知して、変わったディレクトリだけを取り込む。始めるのは、
	// 中断した走査を閉じて始め直したあとである。
	autoImport := newAutoImport(settingsStore, scanIndexStore, scans, logger)

	// 前回の停止で running のまま残った走査を閉じ、処理中だった仕事を戻す。
	if err := scans.RecoverInterrupted(backgroundCtx); err != nil {
		return err
	}
	if opts.OnBusyProbe != nil {
		opts.OnBusyProbe(scans.Busy)
	}

	// 生成物の置き場。パスの規則・公開・確認・読み出し・削除はここだけが持つ。
	artifactStore := artifacts.New(cfg.ThumbnailsDir())
	// 前回の停止で残った生成途中の成果物を消す。ワーカーを動かす前なので、
	// 生成中のものを消すことはない。
	if err := artifactStore.RemoveTemporary(); err != nil {
		logger.Warn("could not delete unfinished artifacts", slog.Any("error", err))
	}
	ingest := app.NewIngest(app.IngestOptions{
		Store: ingestStore, Generator: media.NewAssets(), Artifacts: artifactStore, Publisher: bus, Logger: logger,
	})
	// 取り込みの段階ごとにワーカーを置く。仕事を積んだ取引が確定したら、その
	// 段階のワーカーを起こす（subscribeEvents）。ワーカーは待ち行列を一定間隔で
	// 問い合わせない。
	workers := make([]*jobs.Worker, 0, len(domain.JobKinds))
	wakers := make(map[domain.JobKind]waker, len(domain.JobKinds))
	for _, kind := range domain.JobKinds {
		worker := jobs.New(jobs.Options{
			Kind:    kind,
			Queue:   ingestStore,
			Handler: ingest.Handler(kind),
			// 仕事の開始と終了を、取り込み中の今の処理として知らせる
			// （specs/024-import-progress/research.md R-8）。
			Started: scans.JobStarted,
			Finished: func(job domain.Job) {
				scans.JobFinished(job)
				ingest.JobFinished(job)
			},
			Logger: logger,
		})
		workers = append(workers, worker)
		wakers[kind] = worker
	}
	subscriptions := subscribeEvents(bus, eventSubscribers{
		Screen:           events.Handle,
		Workers:          wakers,
		ReleaseArtifacts: ingest.ReleaseArtifacts,
	})
	// ライブ変換の映像エンコード方式の起動時の確認の寿命。停止の指示で止める。
	checksCtx, stopChecks := context.WithCancel(backgroundCtx)
	defer stopChecks()
	// 方式の設定。作る前に戻ったときは nil のままである。
	var transcodeSettings *app.TranscodeSettings

	var workersDone sync.WaitGroup
	for _, worker := range workers {
		workersDone.Go(func() { worker.Run(backgroundCtx) })
	}
	// ここから先は、どこで戻っても（待ち受けを開けない・待ち受けを失った・停止の
	// 猶予を越えたときも）、データベースを閉じる前に走査とワーカーを止める。止めずに
	// 戻ると、始め直した走査やワーカーが閉じたデータベースへ書く。データベースを
	// 閉じる defer より後に登録するので、その前に走る。
	defer func() {
		// HTTP の猶予待ちが終わってから、走査とワーカーを止める。処理中の
		// ジョブは running のまま残るが、次の起動で queued へ戻る。止めたワーカーを
		// 起こさないよう、先に起こす購読をやめる。
		// 取り消した確認の ffmpeg が終わるのを待つ。確認は取り消しで戻るので長くは待たない。
		stopChecks()
		if transcodeSettings != nil && !waitAtMost(func() { <-transcodeSettings.Done() }, scanStopGrace) {
			logger.Warn("the hardware encoder checks did not stop within the grace period")
		}
		subscriptions.StopWorkers()
		// 監視を外し、新しい取り込みを始めないようにしてから、走査を止める。
		autoImport.Stop()
		stopBackground()
		workersDone.Wait()
		// 走査は取り消しを見て止まり、終わりの記録と、消した動画の知らせを出す。
		// バスを閉じる前に待たないと、その知らせが捨てられて生成物が残り続ける。
		// 読み取りが戻らないときは待ち切らずに進む。走査の記録は running のまま
		// 残り、次の起動の RecoverInterrupted が閉じる。
		if !waitAtMost(scans.Wait, scanStopGrace) {
			logger.Warn("the scan did not stop within the grace period; artifacts of videos it removed may remain",
				slog.String("grace", scanStopGrace.String()))
		}
		// 積んである変化（生成物の削除）を渡し終え、背後で動いている生成物の削除を、
		// データベースを閉じる前に終える。途中で閉じると、消すはずの生成物が残り続ける。
		bus.Close()
		ingest.Wait()
		logger.Info("stopped ingest and jobs")
	}()
	// 前回の停止で中断した走査を、ワーカーを動かしてから始め直す
	// （specs/037-windows-app/research.md R-9）。始め直せなくても起動は止めず、
	// 利用者が取り込みを始められる。
	if _, err := scans.ResumeInterrupted(backgroundCtx); err != nil {
		logger.Warn("could not resume the interrupted scan", slog.Any("error", err))
	}
	// 自動の取り込みを始める。監視は背後で張り、走査は始めない
	// （specs/042-folder-watch-import/research.md R-7）。始められなくても起動は止めない。
	if err := autoImport.Start(backgroundCtx); err != nil {
		logger.Warn("could not start the auto-import", slog.Any("error", err))
	}

	// request単位のtranscode processはHTTP requestより長生きさせない。Shutdownは
	// 実行中requestのcontextを取り消さないため、server寿命を別に持って先にcancelする。
	requestMediaCtx, stopRequestMedia := context.WithCancel(context.Background())
	defer stopRequestMedia()

	// 既定アプリを起動できる環境かどうかは、ここで1度だけ決める。要求のたびには
	// コマンドを探さない。
	fileOpener := opener.New()
	logger.Info("open-file feature status", slog.Bool("available", fileOpener.Available()),
		slog.String("command", fileOpener.Command()))

	// 動画の応答に要る判断（消えたプレビューの作り直し、シーク用プレビューの
	// 状態）と関連動画の組み立ては、アプリケーション層が行う。
	catalog := app.NewCatalog(app.CatalogOptions{
		Index: libraryStore, Ingest: ingestStore, Files: artifactStore, Logger: logger,
	})
	// メディアフォルダとその内側のファイルへのアクセスの規則は mediafs が1か所で
	// 持ち、配信・既定アプリで開く機能・フォルダの登録・ディレクトリ選択が共有する。
	mediaFiles := mediafs.New()
	// 設定画面のメディアフォルダは、パスをファイルシステムで確かめてから保存する。
	mediaFolders := app.NewMediaFolders(app.MediaFoldersOptions{Store: settingsStore, Checker: mediaFiles, Changes: autoImport})

	// ライブ変換の映像エンコード方式。起動時の確認は背後で走り、HTTP の待ち受けを
	// 待たせない。停止の指示で確認を止める。
	transcodeSettings, err = startTranscodeSettings(checksCtx, settingsStore, media.NewEncoderCheck(), runtime.GOOS, logger)
	if err != nil {
		return err
	}

	httpAuthenticator, apiTokens := newHTTPAuth(authStore)
	handler := httpapi.NewRouter(httpapi.Options{
		Build:        build,
		Pinger:       db,
		Videos:       libraryStore,
		Playback:     playbackStore,
		Scans:        scans,
		MediaFolders: mediaFolders,
		Tags:         db.Tags(),
		Visibility:   db.Visibility(),
		Favorites:    db.Favorites(),
		WatchHistory: playbackStore,
		Overrides:    db.Overrides(),
		Versions:     db.Versions(),
		// 代表サムネイルの位置は、取り込みの job と同じ生成の錠の中で作り直して記録する。
		ThumbnailPicker: ingest,
		Folders:         libraryStore,
		FolderGroups:    db.FolderGroups(),
		Library:         libraryStore,
		// 外部連携 API の動画の一覧と引き当ても、画面の一覧と同じ LibraryStore が読む。
		ExternalVideos: libraryStore,
		Transcoder:     media.NewLiveTranscoder(requestMediaCtx.Done()),
		// 要求ごとに今の方式を読むので、方式の変更は再起動なしに次の要求から効く。
		TranscodeSettings: transcodeSettings,
		NetworkSettings:   networkSettings,
		AutoImport:        autoImport,
		// ライブ変換がその場で解析した結果は、取り込みの結果と同じ IngestStore が保存する。
		TranscodeProbes: ingestStore,
		Artifacts:       artifactStore,
		Catalog:         catalog,
		Opener:          fileOpener,
		Files:           mediaFiles,
		// 字幕は要求 1 回で読んで変換して返すので、配信と同じく app を通さない。
		Subtitles: media.NewSubtitleConverter(),
		Events:    events,
		Assets:    web.Dist(),
		Logger:    logger,
		Auth:      httpAuthenticator,
		APITokens: apiTokens,
		// 信頼するプロキシからの要求でだけ転送ヘッダーを読む。
		TrustedProxies: cfg.TrustedProxies,
	})

	// 変化の知らせの接続は終わりが無いので、停止の猶予待ちより先に閉じる。
	// 閉じた接続へ書かないよう、先に画面への知らせの購読をやめる。
	beforeShutdown := func() {
		stopChecks()
		stopRequestMedia()
		subscriptions.StopScreen()
		events.Close()
	}
	// 戻ったあと、走査とワーカーは上の defer が止める。
	return serveUntil(opts.NotifyStop, opts.Listener, cfg, handler, logger, opts.OnListening, beforeShutdown)
}

// startupStage は起動のどの段階で失敗したかである。デスクトップ版は、これで
// 示すダイアログを選ぶ（specs/037-windows-app/research.md R-10）。
type startupStage int

const (
	// stageOther は下の段階に当たらない失敗。
	stageOther startupStage = iota
	// stageDatabase はデータベースを開くか移行するところでの失敗。
	stageDatabase
	// stageListen は待ち受けを開くところでの失敗。
	stageListen
)

// startupError は起動の失敗に段階を添える。文は元の誤りのままにする。
type startupError struct {
	stage startupStage
	err   error
}

func (e *startupError) Error() string { return e.err.Error() }

func (e *startupError) Unwrap() error { return e.err }

// startupStageOf は run が返した誤りの段階を返す。
func startupStageOf(err error) startupStage {
	var startup *startupError
	if errors.As(err, &startup) {
		return startup.stage
	}
	return stageOther
}

// waitAtMost は wait の終わりを limit まで待ち、終わったかを返す。終わらなければ
// wait は背後に残る。
func waitAtMost(wait func(), limit time.Duration) bool {
	done := make(chan struct{})
	go func() {
		wait()
		close(done)
	}()
	timer := time.NewTimer(limit)
	defer timer.Stop()
	select {
	case <-done:
		return true
	case <-timer.C:
		return false
	}
}

// serveUntil は listener で HTTP サーバーを起動し、停止の指示を待つ。
//
// 停止の指示（タグなしの main では SIGINT / SIGTERM）を受けたら新規の接続受付を
// 止め、処理中の要求を猶予時間まで待ってから終了する。正常終了の終了コードは 0 である。
// 待ち受けを失ったとき（開き直しで元のアドレスも開けなかったときなど）も同じ手順で
// 止め、待ち受けを失った理由を返す。
func serveUntil(
	notifyStop stopNotifier,
	listener *reopenableListener,
	cfg Config,
	handler http.Handler,
	logger *slog.Logger,
	onListening func(),
	beforeShutdown func(),
) error {
	stopRequested, release := notifyStop()
	// 停止の指示では先に戻し、戻るときにもう一度呼ぶ。受け取りをやめる関数が
	// 2 度の呼び出しに耐えるとは限らないので、1 度だけ呼ぶ。
	var releaseOnce sync.Once
	restoreSignals := func() {
		releaseOnce.Do(func() {
			if release != nil {
				release()
			}
		})
	}
	defer restoreSignals()

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
	}

	if err := listener.Listen(cfg.Addr); err != nil {
		return &startupError{stage: stageListen, err: err}
	}
	defer func() { _ = listener.Close() }()

	logger.Info("listening", slog.String("addr", cfg.Addr))
	if onListening != nil {
		onListening()
	}
	listener.Serve(srv)

	// 停止の指示でも待ち受けを失ったときでも、処理中の要求を待ってから戻る。
	// 戻ったあとに呼び出し元はデータベースを閉じるので、要求を残して戻らない。
	shutdown := func() error {
		if beforeShutdown != nil {
			beforeShutdown()
		}

		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
		defer cancel()

		// 停止の途中に開き直されないよう、待ち受けを先に閉じる。Serve がまだ
		// 閉じた待ち受けの登録を外していなければ、Shutdown がもう一度閉じて
		// net.ErrClosed を返す。それは停止の失敗ではない。
		_ = listener.Close()
		if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, net.ErrClosed) {
			return fmt.Errorf("could not stop within %s: %w", shutdownGrace, err)
		}
		logger.Info("stopped")
		return nil
	}

	select {
	case err := <-listener.Failed():
		if err == nil || errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		// 開き直しで元のアドレスも開けなかったときなど。確立済みの接続は
		// まだ応答しているので、停止の指示と同じ手順で止める。
		listenErr := fmt.Errorf("cannot listen on %s: %w", listener.Addr(), err)
		logger.Error("lost the listener; waiting for in-flight requests",
			slog.String("error", err.Error()), slog.String("grace", shutdownGrace.String()))
		if stopErr := shutdown(); stopErr != nil {
			return errors.Join(listenErr, stopErr)
		}
		return listenErr

	case <-stopRequested:
		// 2 度目の指示で即座に終われるよう、通知の受け取りは戻しておく。
		restoreSignals()
		logger.Info("received a stop signal; waiting for in-flight requests",
			slog.String("grace", shutdownGrace.String()))
		return shutdown()
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
		return joinProblems("startup check failed", problems)
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
