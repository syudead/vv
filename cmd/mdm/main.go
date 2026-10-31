// Command mdm serves a JSON API and embedded SPA from one binary.
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
	"strings"
	"sync"
	"syscall"
	"time"
	// Embed zone data so client-supplied IANA zones work on Windows and minimal containers.
	_ "time/tzdata"

	"github.com/syudead/vv/internal/app"
	"github.com/syudead/vv/internal/artifacts"
	"github.com/syudead/vv/internal/comfyui"
	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/eventbus"
	"github.com/syudead/vv/internal/httpapi"
	"github.com/syudead/vv/internal/jobs"
	"github.com/syudead/vv/internal/media"
	"github.com/syudead/vv/internal/mediafs"
	"github.com/syudead/vv/internal/ollama"
	"github.com/syudead/vv/internal/opener"
	"github.com/syudead/vv/internal/scanner"
	"github.com/syudead/vv/internal/store"
	"github.com/syudead/vv/web"
)

// Override version at build time with -ldflags "-X main.version=...".
var version = domain.DefaultVersion

var _ app.ImageEditor = (*comfyui.Client)(nil)

// Fallback metadata for builds without .git: -ldflags "-X main.commit=... -X main.builtAt=...";
// builtAt is the commit time in RFC 3339, like vcs.time.
var (
	commit  string
	builtAt string
)

const shutdownGrace = 10 * time.Second

// Bound scan shutdown because cancellation cannot interrupt reads from an unresponsive NAS.
const scanStopGrace = 10 * time.Second

// Read-header deadline.
const readHeaderTimeout = 10 * time.Second

// runOptions lets server and desktop entry points share startup and shutdown with different
// configuration and signal sources.
type runOptions struct {
	Config    Config
	LogOutput io.Writer
	Listener  *reopenableListener
	// Optional callback immediately after binding.
	OnListening func()
	// Start stop notifications just before serving HTTP.
	NotifyStop stopNotifier
	// Desktop selects the host from saved LAN access and enables /api/settings/network; server mode
	// uses Config.Addr and leaves that route unavailable.
	Desktop bool
	// Optional busy-state callback installed after scan setup and before listening, for desktop close
	// confirmation.
	OnBusyProbe func(busy func(context.Context) (bool, error))
}

// stopNotifier returns a stop channel and optional cleanup function.
type stopNotifier func() (stopRequested <-chan struct{}, release func())

func notifySignals() (<-chan struct{}, func()) {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	return ctx.Done(), stop
}

func run(opts runOptions) error {
	cfg := opts.Config
	logger := slog.New(slog.NewJSONHandler(opts.LogOutput, &slog.HandlerOptions{Level: cfg.Level()}))
	slog.SetDefault(logger)

	build := buildInfo()

	// Record effective configuration and build identity together for diagnostics.
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

	if err := refreshLibraryInformation(context.Background(), db, logger); err != nil {
		return err
	}

	authStore := db.Auth()
	prepareAuth(context.Background(), authStore, time.Now(), logger)

	// Load desktop LAN access before jobs and listeners start; nil leaves the settings route
	// unavailable.
	var networkSettings httpapi.NetworkSettings
	if opts.Desktop {
		addr, settings, err := startNetworkSettings(context.Background(), db.Settings(), opts.Listener, cfg.Addr, logger)
		if err != nil {
			return &startupError{stage: stageDatabase, err: err}
		}
		cfg.Addr = addr
		networkSettings = settings
	}

	// Scans and jobs outlive individual HTTP requests; shutdown cancellation leaves work recoverable
	// on restart.
	backgroundCtx, stopBackground := context.WithCancel(context.Background())
	defer stopBackground()

	// Publishers know only the bus; subscribeEvents wires consumers.
	bus := eventbus.New(logger)
	db.PublishTo(bus)
	events := httpapi.NewEvents()

	ingestStore := db.Ingest()
	scanStore := db.Scans()
	scanIndexStore := db.ScanIndex()
	settingsStore := db.Settings()
	playbackStore := db.Playback()

	// Scan content-key reads take read slots from the scheduler; see docs/design-docs/job-scheduling.md.
	var scheduler *jobs.Scheduler
	scans := app.NewScans(app.ScansOptions{
		Store:          scanStore,
		Jobs:           ingestStore,
		UnfinishedJobs: ingestStore,
		FolderIndex:    scanIndexStore,
		NewScanner: func(reporter app.ScanReporter) app.Scanner {
			return scanner.New(scanner.Options{
				Index: scanIndexStore, Queue: ingestStore, Reporter: reporter,
				ReadSlots: scanReadSlots{scheduler: &scheduler}, Logger: logger,
			})
		},
		Folders:   scanIndexStore,
		Context:   backgroundCtx,
		Publisher: bus,
		Logger:    logger,
	})

	artifactStore := artifacts.New(cfg.ThumbnailsDir())
	// Clean temporary artifacts before workers start so active output cannot be removed.
	if err := artifactStore.RemoveTemporary(); err != nil {
		logger.Warn("could not delete unfinished artifacts", slog.Any("error", err))
	}
	ingest := app.NewIngest(app.IngestOptions{
		Store: ingestStore, Generator: media.NewAssets(), Artifacts: artifactStore, Publisher: bus, Logger: logger,
	})
	// Provider adapters own connection settings and expose app.Classifier; see
	// docs/design-docs/ai-layer.md.
	classifier := ollama.New(settingsStore)
	// Auto-tagging runs as auto_tag jobs on the scheduler, paused while the feature is off; see
	// docs/design-docs/auto-tagging.md. Startup recovery has already requeued interrupted jobs.
	autoTagger := app.NewAutoTagger(app.AutoTaggerOptions{
		Queue:      db.AutoTags(),
		Settings:   settingsStore,
		Undo:       db.Activity(),
		Classifier: classifier,
		Images:     videoImageSource{artifacts: artifactStore},
		Wake:       func() { scheduler.Wake() },
		Logger:     logger,
	})
	// Face analysis, face-image embedding and the cast rule run as jobs on the face lane, paused while the feature is
	// off or its runtime is not ready; see docs/design-docs/face-recognition.md.
	faces := newFaceRecognition(backgroundCtx, cfg.DataDir, db, func() { scheduler.Wake() }, logger)
	// Duplicate judging runs as version_judge jobs on the scheduler, paused while judging is off; it compares
	// fingerprints before asking the classifier about mismatched intervals. See docs/design-docs/duplicate-judging.md.
	versionJudge := app.NewVersionJudge(app.VersionJudgeOptions{
		Store:      db.Versions(),
		Settings:   settingsStore,
		Classifier: classifier,
		Frames:     artifactStore,
		Wake:       func() { scheduler.Wake() },
		Logger:     logger,
	})
	// The scheduler wakes after committed queue changes instead of polling; see docs/design-docs/job-scheduling.md.
	handlers := make(map[domain.JobKind]jobs.Handler, len(domain.ScheduledJobKinds))
	for _, kind := range domain.JobKinds {
		handlers[kind] = ingest.Handler(kind)
	}
	handlers[domain.JobAutoTag] = autoTagger.HandleJob
	for _, kind := range []domain.JobKind{
		domain.JobFace, domain.JobFaceImage, domain.JobFaceCast, domain.JobQueueFaceImages,
	} {
		handlers[kind] = faces.analysis.HandleJob
	}
	handlers[domain.JobVersionJudge] = versionJudge.HandleJob
	addBackfillHandlers(handlers, ingestStore, ingest)
	scheduler = jobs.NewScheduler(jobs.Options{
		Queue:    ingestStore,
		Folders:  scanIndexStore,
		Disks:    mediafs.New().ResolveDisk,
		Handlers: handlers,
		CPUSlots: domain.JobCPUSlots(runtime.NumCPU()),
		Paused:   app.PausedJobKinds(autoTagger, faces.analysis, versionJudge),
		Started:  scans.JobStarted,
		Finished: func(job domain.Job) {
			scans.JobFinished(job)
			ingest.JobFinished(job)
		},
		Logger: logger,
	})
	// Start filesystem watching only after interrupted scans have been recovered and restarted.
	autoImport := newAutoImport(settingsStore, scanIndexStore, scans, scheduler.RecheckRoots, logger)

	if err := scans.RecoverInterrupted(backgroundCtx); err != nil {
		return err
	}
	if opts.OnBusyProbe != nil {
		opts.OnBusyProbe(scans.Busy)
	}

	wakers := make(map[domain.JobKind]waker, len(domain.ScheduledJobKinds))
	for _, kind := range domain.ScheduledJobKinds {
		wakers[kind] = scheduler
	}
	ollamaConnection := app.NewOllamaConnection(settingsStore, classifier)
	comfyUIClient := comfyui.NewWithOptions(settingsStore, comfyui.Options{DiffusionModel: cfg.ComfyUIDiffusionModel})
	comfyUIConnection := app.NewComfyUIConnection(settingsStore, comfyUIClient)
	// Thumbnail edits share ingestion's generation lock; see docs/design-docs/thumbnail-editing.md.
	thumbnailEdits := app.NewThumbnailEdits(app.ThumbnailEditsOptions{
		Settings: settingsStore, Editor: comfyUIClient, Frames: media.NewAssets(), Foreground: scheduler,
		Images: db.Overrides(), Applier: ingest, Logger: logger,
	})
	// Recover activity before workers start; recovery needs only storage, not retry/cancel
	// dispatchers.
	recoverActivity(backgroundCtx, app.NewActivity(app.ActivityOptions{Store: db.Activity()}), logger)
	if err := versionJudge.Recover(backgroundCtx); err != nil {
		logger.Warn("could not requeue interrupted duplicate judgements", slog.Any("error", err))
	}
	ollamaConnection.OnSaved(versionJudge.ConnectionSaved)
	// The assistant runs requests as tool loops over the generation model, judges review sets with the classifier
	// and executes approved proposals through the screen API; its runner is registered once the HTTP server exists.
	assistant := app.NewAssistant(app.AssistantOptions{
		Store: db.Assistant(), Turns: db.Assistant(), Chat: ollama.NewChat(settingsStore), Vocabulary: db.Assistant(),
		Videos: libraryStore, SmartFolders: db.SmartFolders(), Groups: libraryStore,
		Reviews: db.Assistant(), Classifier: classifier, Clef: classifier, Model: ollamaConnection, Logger: logger,
	})
	if err := assistant.Recover(backgroundCtx); err != nil {
		logger.Warn("could not recover the assistant proposals and turns", slog.Any("error", err))
	}
	versionJudgementActivity := app.NewVersionJudgementActivity(db.Versions(), db.Activity(), versionJudge)
	activity := app.NewActivity(app.ActivityOptions{
		Store: db.Activity(),
		Retriers: map[domain.ActivityKind]app.ActivityRetrier{
			domain.ActivityAutoTag:          autoTagger,
			domain.ActivityImport:           app.NewImportRetrier(ingestStore),
			domain.ActivityScan:             scans,
			domain.ActivityVersionJudgement: versionJudgementActivity,
			domain.ActivityFaceAnalysis:     faces.analysis,
		},
		Undoers: map[domain.ActivityKind]app.ActivityUndoer{
			domain.ActivityAutoTag:          autoTagger,
			domain.ActivityVersionJudgement: versionJudgementActivity,
			domain.ActivityFaceAnalysis:     app.NewFaceLinkUndoer(db.Faces()),
			domain.ActivityFaceMerge:        app.NewFaceMergeActivity(db.Activity()),
		},
	})
	subscriptions := subscribeEvents(bus, eventSubscribers{
		Screen:           events.Handle,
		Workers:          wakers,
		ReleaseArtifacts: ingest.ReleaseArtifacts,
		ImagesFinished: func(videoID int64) {
			autoTagger.VideoImagesFinished(backgroundCtx, videoID)
			faces.analysis.VideoImagesFinished(backgroundCtx, videoID)
		},
		VersionJudge: versionJudge,
	})
	checksCtx, stopChecks := context.WithCancel(backgroundCtx)
	defer stopChecks()
	var transcodeSettings *app.TranscodeSettings

	var workersDone sync.WaitGroup
	workersDone.Go(func() {
		scheduler.Run(backgroundCtx)
		// No face job runs any more, so the helper can stop.
		faces.analysis.Close()
	})

	stopFaceRuntime := faces.start(backgroundCtx, logger)
	defer stopFaceRuntime()
	workersDone.Go(func() { thumbnailEdits.Run(backgroundCtx) })
	workersDone.Go(func() { assistant.Run(backgroundCtx) })
	workersDone.Go(func() { assistant.RunReviews(backgroundCtx) })
	workersDone.Go(func() { assistant.RunTurns(backgroundCtx) })
	// Always stop scans and workers before closing the database, including listener failures and
	// grace-period expiry. Register this defer after database cleanup so it runs first.
	defer func() {
		// Unsubscribe worker wakeups before stopping them; unfinished running jobs are requeued on
		// restart. Wait for the canceled encoder probe to exit.
		stopChecks()
		if transcodeSettings != nil && !waitAtMost(func() { <-transcodeSettings.Done() }, scanStopGrace) {
			logger.Warn("the hardware encoder checks did not stop within the grace period")
		}
		subscriptions.StopWorkers()
		// Stop watching before canceling scans to prevent new ingestion.
		autoImport.Stop()
		stopBackground()
		workersDone.Wait()
		// Wait for scans before closing the bus so deletion events can release artifacts. Unresponsive
		// reads may outlast the deadline; startup recovery closes their running records.
		if !waitAtMost(scans.Wait, scanStopGrace) {
			logger.Warn("the scan did not stop within the grace period; artifacts of videos it removed may remain",
				slog.String("grace", scanStopGrace.String()))
		}
		// Drain deletion events and finish artifact cleanup before closing the database to prevent leaks.
		bus.Close()
		ingest.Wait()
		logger.Info("stopped ingest and jobs")
	}()
	// Restart interrupted scans after workers start; failure must not block manual ingestion or
	// startup.
	if _, err := scans.ResumeInterrupted(backgroundCtx); err != nil {
		logger.Warn("could not resume the interrupted scan", slog.Any("error", err))
	}
	// Watcher setup runs asynchronously without starting a scan; failure does not block startup.
	if err := autoImport.Start(backgroundCtx); err != nil {
		logger.Warn("could not start the auto-import", slog.Any("error", err))
	}

	// Shutdown does not cancel active HTTP contexts, so cancel a separate server context first to stop
	// request-owned transcoders.
	requestMediaCtx, stopRequestMedia := context.WithCancel(context.Background())
	defer stopRequestMedia()

	// Detect opener support once instead of searching for commands per request.
	fileOpener := opener.New()
	logger.Info("open-file feature status", slog.Bool("available", fileOpener.Available()),
		slog.String("command", fileOpener.Command()))

	catalog := app.NewCatalog(app.CatalogOptions{
		Index: libraryStore, Ingest: ingestStore, Files: artifactStore, Logger: logger,
	})
	// Share mediafs access rules across streaming, opening, registration, and directory selection.
	mediaFiles := mediafs.New()
	mediaFolders := app.NewMediaFolders(app.MediaFoldersOptions{
		Store: settingsStore, Checker: mediaFiles, Changes: autoImport, VideoBytes: settingsStore, Disk: mediaFiles,
	})

	// Probe encoders asynchronously so HTTP startup does not wait; cancel probing on shutdown.
	transcodeSettings, err = startTranscodeSettings(checksCtx, settingsStore, media.NewEncoderCheck(), runtime.GOOS, logger)
	if err != nil {
		return err
	}

	httpAuthenticator, apiTokens := newHTTPAuth(authStore)
	pairing, peerLink := newPairing(db.Peers(), build, bus, logger)
	stopPairing := startPairingConnection(pairing)
	defer stopPairing()
	// Close the plugin runtime after HTTP stops and before the database closes; read proxy settings
	// per execution so changes apply immediately.
	pluginProxy := app.NewWorkInfoPluginProxy(settingsStore)
	personImages := app.NewPersonImages(db.Credits(), ingest)
	workInfoPlugins, closeWorkInfoPlugins := newWorkInfoPlugins(context.Background(), db, pluginProxy, ingest, personImages, logger)
	defer closeWorkInfoPlugins()
	handler, assistantRunner := httpapi.NewServer(httpapi.Options{
		Build:            build,
		Pinger:           db,
		Videos:           libraryStore,
		Playback:         playbackStore,
		Scans:            scans,
		MediaFolders:     mediaFolders,
		FolderUsage:      mediaFolders,
		Tags:             db.Tags(),
		Credits:          db.Credits(),
		WorkInfo:         db.WorkInfo(),
		MediaInfo:        libraryStore,
		Visibility:       db.Visibility(),
		VideoEdits:       db.VideoEdits(),
		VideoCodeEdits:   db.VideoEdits(),
		Favorites:        db.Favorites(),
		Playlists:        db.Playlists(),
		PlaylistPlayback: db.Playlists(),
		WatchHistory:     playbackStore,
		Activity:         activity,
		Overrides:        db.Overrides(),
		VirtualVideos:    db.VirtualVideos(),
		Versions:         db.Versions(),
		// Thumbnail position changes share ingestion's generation lock.
		ThumbnailPicker: ingest,
		Folders:         libraryStore,
		FolderGroups:    db.FolderGroups(),
		Groups:          db.Groups(),
		SmartFolders:    db.SmartFolders(),
		Collections:     db.Collections(),
		// Keep drag undo records in memory for ten minutes.
		Organizer:      app.NewOrganizer(app.OrganizerOptions{Store: db.Organize()}),
		Library:        libraryStore,
		ExternalVideos: libraryStore,
		Transcoder:     media.NewLiveTranscoder(requestMediaCtx.Done()),
		// Read encoder settings per request so changes need no restart.
		TranscodeSettings: transcodeSettings,
		NetworkSettings:   networkSettings,
		PluginProxy:       pluginProxy,
		AutoImport:        autoImport,
		AutoTagging:       autoTagger,
		Onboarding:        app.NewOnboarding(db.Onboarding(), autoTagger),
		VersionJudging:    versionJudge,
		VersionReview:     app.NewVersionJudgeReview(db.Versions(), artifactStore, versionJudge),
		FaceRecognition:   faces.settings,
		FaceAnalysis:      faces.analysis,
		Ollama:            ollamaConnection,
		Assistant:         assistant,
		ComfyUI:           comfyUIConnection,
		ThumbnailEdits:    thumbnailEdits,
		ThumbnailUploads:  ingest,
		PersonImages:      personImages,
		WorkInfoPlugins:   workInfoPlugins,
		TranscodeProbes:   ingestStore,
		Artifacts:         artifactStore,
		Catalog:           catalog,
		Opener:            fileOpener,
		Files:             mediaFiles,
		Foreground:        scheduler,
		Subtitles:         media.NewSubtitleConverter(),
		Events:            events,
		Assets:            web.Dist(),
		Logger:            logger,
		Auth:              httpAuthenticator,
		APITokens:         apiTokens,
		Pairing:           pairing,
		PeerLink:          peerLink,
		TrustedProxies:    cfg.TrustedProxies,
	})
	assistant.UseRunner(assistantRunner)

	// SSE has no natural end; unsubscribe screen events and close connections before waiting for HTTP
	// shutdown. The stream to the peer closes with them.
	beforeShutdown := func() {
		stopChecks()
		stopRequestMedia()
		subscriptions.StopScreen()
		events.Close()
		stopPairing()
	}
	// Backfills are queued once the listener is open, so startup never waits for them; see
	// docs/design-docs/process-lifecycle.md.
	onListening := func() {
		if opts.OnListening != nil {
			opts.OnListening()
		}
		if err := ingestStore.EnqueueStartupJobs(backgroundCtx); err != nil {
			logger.Warn("could not queue the startup jobs", slog.Any("error", err))
		}
	}
	return serveUntil(opts.NotifyStop, opts.Listener, cfg, handler, logger, onListening, beforeShutdown)
}

// addBackfillHandlers handles the jobs every start queues and the per-video jobs they queue.
func addBackfillHandlers(handlers map[domain.JobKind]jobs.Handler, ingestStore *store.IngestStore, ingest *app.Ingest) {
	keyframes := app.KeyframeBackfill(ingestStore, media.ProbeKeyframes)
	mediaInfo := app.MediaInfoBackfill(ingestStore, media.ProbeMediaInfo)
	handlers[domain.JobQueueKeyframes] = keyframes.QueueAll
	handlers[domain.JobKeyframes] = keyframes.Read
	handlers[domain.JobQueueMediaInfo] = mediaInfo.QueueAll
	handlers[domain.JobMediaInfo] = mediaInfo.Read
	handlers[domain.JobVirtualThumbnails] = func(ctx context.Context, _ domain.Job) error {
		return ingest.RepublishVirtualThumbnails(ctx)
	}
}

// recoverActivity marks interrupted processing as attention/interrupted, leaving waiting records
// intact; see docs/design-docs/process-lifecycle.md. Failures do not block startup and retry next
// time.
func recoverActivity(ctx context.Context, activity *app.Activity, logger *slog.Logger) {
	recovered, err := activity.RecoverInterrupted(ctx)
	if err != nil {
		logger.Warn("could not mark the interrupted activity entries", slog.Any("error", err))
		return
	}
	if recovered > 0 {
		logger.Info("marked the interrupted activity entries", slog.Int("entries", recovered))
	}
}

// startupStage selects the desktop failure dialog.
type startupStage int

const (
	stageOther startupStage = iota
	stageDatabase
	stageListen
)

// Preserve the original error text while attaching its startup stage.
type startupError struct {
	stage startupStage
	err   error
}

func (e *startupError) Error() string { return e.err.Error() }

func (e *startupError) Unwrap() error { return e.err }

func startupStageOf(err error) startupStage {
	var startup *startupError
	if errors.As(err, &startup) {
		return startup.stage
	}
	return stageOther
}

// waitAtMost leaves wait running in the background if the deadline expires.
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

// serveUntil drains requests up to the shutdown deadline on a stop signal or listener failure;
// listener failures are returned after cleanup.
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
	// Cleanup may not be idempotent; invoke it only once across early shutdown and deferred cleanup.
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

	// Drain active requests before returning because the caller closes the database next.
	shutdown := func() error {
		if beforeShutdown != nil {
			beforeShutdown()
		}

		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
		defer cancel()

		// Close the listener first to prevent reopening during shutdown; a duplicate close from Shutdown
		// is harmless.
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
		// Existing connections remain active after listener failure and require the same drain procedure.
		listenErr := fmt.Errorf("cannot listen on %s: %w", listener.Addr(), err)
		logger.Error("lost the listener; waiting for in-flight requests",
			slog.String("error", err.Error()), slog.String("grace", shutdownGrace.String()))
		if stopErr := shutdown(); stopErr != nil {
			return errors.Join(listenErr, stopErr)
		}
		return listenErr

	case <-stopRequested:
		// Restore default signal behavior so a second signal terminates immediately.
		restoreSignals()
		logger.Info("received a stop signal; waiting for in-flight requests",
			slog.String("grace", shutdownGrace.String()))
		return shutdown()
	}
}

// Report configuration and missing-command errors together to avoid repeated fix/restart cycles.
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

// Prefer Go's embedded VCS metadata, falling back to ldflags fields only when missing.
func buildInfo() domain.BuildInfo {
	var settings []debug.BuildSetting
	if read, ok := debug.ReadBuildInfo(); ok {
		settings = read.Settings
	}
	return resolveBuildInfo(version, commit, builtAt, settings)
}

// Omit empty values and malformed timestamps.
func resolveBuildInfo(release, fallbackCommit, fallbackTime string, settings []debug.BuildSetting) domain.BuildInfo {
	info := domain.BuildInfo{Version: release}
	if info.Version == "" {
		info.Version = domain.DefaultVersion
	}

	vcsCommit, vcsTime := "", ""
	for _, setting := range settings {
		switch setting.Key {
		case "vcs.revision":
			vcsCommit = setting.Value
		case "vcs.time":
			vcsTime = setting.Value
		}
	}
	if vcsCommit == "" {
		vcsCommit, vcsTime = fallbackCommit, fallbackTime
	}

	info.Commit = strings.TrimSpace(vcsCommit)
	if at, err := time.Parse(time.RFC3339, strings.TrimSpace(vcsTime)); err == nil {
		info.BuiltAt = at
	}
	return info
}

func refreshLibraryInformation(ctx context.Context, db *store.DB, logger *slog.Logger) error {
	// Rebuild stale search keys before jobs or HTTP start; fail startup rather than serve stale
	// results. Per-row versions make interrupted rebuilds resumable.
	refreshed, err := db.Library().RefreshSearchKeys(ctx)
	if err != nil {
		return fmt.Errorf("cannot rebuild the search keys: %w", err)
	}
	logger.Info("rebuilt the search keys", slog.Int("locations", refreshed))

	// Tag keys are independent of media folders, so no folder lock is needed; the count includes names
	// and rejected names.
	tagsRefreshed, err := db.Tags().RefreshSearchKeys(ctx)
	if err != nil {
		return fmt.Errorf("cannot rebuild the tag search keys: %w", err)
	}
	logger.Info("rebuilt the tag search keys", slog.Int("names", tagsRefreshed))

	creditsRefreshed, err := db.Credits().RefreshSearchKeys(ctx)
	if err != nil {
		return fmt.Errorf("cannot rebuild the credit search keys: %w", err)
	}
	logger.Info("rebuilt the credit search keys", slog.Int("rows", creditsRefreshed))

	// Backfill history title keys before serving searches for deleted videos; failure stops startup.
	titleKeysFilled, err := db.Playback().RefreshWatchHistoryTitleKeys(ctx)
	if err != nil {
		return fmt.Errorf("cannot fill the watch history title keys: %w", err)
	}
	logger.Info("filled the watch history title keys", slog.Int("entries", titleKeysFilled))

	// Rebuild stale or failed folder indexes after search keys. Failure keeps the old index available
	// and retries on the next relevant change or startup.
	if rebuilt, err := db.ScanIndex().RefreshFolderIndex(ctx); err != nil {
		logger.Warn("could not rebuild the folder index", slog.Any("error", err))
	} else if rebuilt {
		logger.Info("rebuilt the folder index")
	}
	if err := db.RefreshCollectionValues(ctx); err != nil {
		return fmt.Errorf("cannot update collection information: %w", err)
	}

	return nil
}
