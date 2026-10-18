package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/syudead/vv/internal/domain"
)

type ScanStore interface {
	// StartScan returns the existing running scan with started=false instead of opening another.
	StartScan(ctx context.Context) (scan domain.Scan, started bool, err error)
	// ResumeScan carries interrupted scan membership and stage failures/substitutions into the new
	// scan.
	ResumeScan(ctx context.Context, from int64) (scan domain.Scan, started bool, err error)
	// Watch scans retain prior issues outside their scope.
	StartWatchScan(ctx context.Context, dirs []domain.DirtyDirectory) (scan domain.Scan, started bool, err error)
	CurrentScan(ctx context.Context) (domain.Scan, error)
	UpdateScanProgress(ctx context.Context, id int64, progress domain.ScanProgress) error
	// FinishScan persists ScanFailure code/location and the initiating decider for failed scans.
	FinishScan(ctx context.Context, id int64, state domain.ScanState, cause error, decider domain.ActivityDecider) error
	FailInterruptedScans(ctx context.Context) (int64, error)
	RecordScanIssue(ctx context.Context, issue domain.ScanFileIssue) error
	// Exclude issues whose locations belong to no registered folder.
	ScanIssues(ctx context.Context, scanID int64) ([]domain.ScanIssue, error)
}

type JobRecoveryStore interface {
	RequeueRunningJobs(ctx context.Context) (int64, error)
}

type UnfinishedJobStore interface {
	HasUnfinishedJobs(ctx context.Context) (bool, error)
}

// Storage marks the folder index stale before returning rebuild failures.
type FolderIndexStore interface {
	RebuildFolderIndex(ctx context.Context) error
}

type Scanner interface {
	Scan(ctx context.Context, decider domain.ActivityDecider) (domain.ScanResult, error)
	// Superseded scoped scans delete nothing and return ErrScanSuperseded.
	ScanScoped(ctx context.Context, dirs []domain.DirtyDirectory) (domain.ScanResult, error)
}

type ScanReporter interface {
	ReportScanProgress(ctx context.Context, result domain.ScanResult) error
	ReportScanIssue(ctx context.Context, issue domain.ScanFileIssue) error
	ReportScanFile(path string, videoID int64)
}

type ActivityFolderStore interface {
	ListMediaFolders(ctx context.Context) ([]domain.MediaFolder, error)
}

// Publishers do not know subscribers; cmd/mdm wires delivery.
type Publisher interface {
	Publish(events ...domain.Event)
}

type ScansOptions struct {
	Store ScanStore
	Jobs  JobRecoveryStore
	// Nil makes Busy consider only the active scan.
	UnfinishedJobs UnfinishedJobStore
	// Optional rebuilds before scan completion and after interrupted-scan recovery.
	FolderIndex FolderIndexStore
	// Inject a factory to resolve the scanner/reporter construction dependency.
	NewScanner func(ScanReporter) Scanner
	// Long-lived scan context; nil uses context.Background and shutdown cancellation closes scans as
	// failed.
	Context context.Context
	// Nil omits the activity folder.
	Folders ActivityFolderStore
	// Nil disables events.
	Publisher Publisher
	// Nil uses slog.Default.
	Logger *slog.Logger
}

type Scans struct {
	store   ScanStore
	jobs    JobRecoveryStore
	pending UnfinishedJobStore
	folders FolderIndexStore
	roots   ActivityFolderStore
	scanner Scanner
	// Inspect lifetime directly too because stopRuns propagates cancellation asynchronously.
	lifetime  context.Context
	publisher Publisher
	logger    *slog.Logger
	activity  *activities

	// The database's partial unique index owns running-scan exclusivity; mu only prevents duplicate
	// goroutines.
	mu          sync.Mutex
	running     bool
	cancelRun   context.CancelCauseFunc
	runOrigin   domain.ScanOrigin
	runFinished chan struct{}
	// Optional callback invoked under the scan lock after closure.
	onFinish func(origin domain.ScanOrigin, state domain.ScanState)
	// Optional callback invoked under the scan lock at startup.
	onStart func(origin domain.ScanOrigin)
	// Later starts are canceled immediately after lifecycle shutdown.
	stopped bool
	done    sync.WaitGroup
}

func NewScans(opts ScansOptions) *Scans {
	s := &Scans{
		store:     opts.Store,
		jobs:      opts.Jobs,
		pending:   opts.UnfinishedJobs,
		folders:   opts.FolderIndex,
		roots:     opts.Folders,
		lifetime:  opts.Context,
		publisher: opts.Publisher,
		logger:    opts.Logger,
	}
	if s.lifetime == nil {
		s.lifetime = context.Background()
	}
	context.AfterFunc(s.lifetime, s.stopRuns)
	if s.logger == nil {
		s.logger = slog.Default()
	}
	s.activity = newActivities(s.activityChanged)
	if opts.NewScanner != nil {
		s.scanner = opts.NewScanner(s)
	}
	return s
}

// StartScan responds immediately and uses the long-lived lifecycle context rather than request
// cancellation; storage atomically decides started versus existing. Supersede watcher scans before
// manual starts, retrying if another watcher wins the race.
func (s *Scans) StartScan(ctx context.Context) (domain.Scan, bool, error) {
	const attempts = 3
	for attempt := 1; ; attempt++ {
		s.SupersedeWatchScan(ctx)
		scan, started, err := s.start(ctx, s.store.StartScan, nil, domain.ActivityByOwner)
		if err != nil || started || scan.Origin != domain.ScanOriginWatch || attempt >= attempts {
			return scan, started, err
		}
	}
}

// StartWatchScan returns any existing scan without starting another and limits new work to dirs.
func (s *Scans) StartWatchScan(ctx context.Context, dirs []domain.DirtyDirectory) (domain.Scan, bool, error) {
	dirs = domain.NormalizeDirtyDirectories(dirs)
	return s.start(ctx, func(ctx context.Context) (domain.Scan, bool, error) {
		return s.store.StartWatchScan(ctx, dirs)
	}, &dirs, domain.ActivityByRule)
}

// SupersedeWatchScan waits for watcher-only cancellation between files, closing done without
// deleting locations; manual scans remain untouched.
func (s *Scans) SupersedeWatchScan(ctx context.Context) bool {
	s.mu.Lock()
	if !s.running || s.runOrigin != domain.ScanOriginWatch || s.cancelRun == nil {
		s.mu.Unlock()
		return false
	}
	finished := s.runFinished
	s.cancelRun(domain.ErrScanSuperseded)
	s.mu.Unlock()
	select {
	case <-finished:
	case <-ctx.Done():
	}
	return true
}

// The initiator propagates to jobs and failed activity records: owner for manual starts, rule for
// watcher/recovery starts.
func (s *Scans) start(
	ctx context.Context, open func(context.Context) (domain.Scan, bool, error), dirs *[]domain.DirtyDirectory,
	decider domain.ActivityDecider,
) (domain.Scan, bool, error) {
	scan, started, err := open(ctx)
	if err != nil {
		return domain.Scan{}, false, err
	}
	if !started {
		scan, err = s.withImport(ctx, scan)
		return scan, false, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		scan, err = s.withImport(ctx, scan)
		return scan, true, err
	}
	s.running = true
	s.done.Add(1)

	// Detach request cancellation so returning the HTTP response cannot stop ingestion; stopRuns
	// propagates lifecycle cancellation instead.
	runCtx, cancel := context.WithCancelCause(context.WithoutCancel(ctx))
	s.cancelRun = cancel
	s.runOrigin = domain.ScanOriginManual
	s.runFinished = make(chan struct{})
	if dirs != nil {
		s.runOrigin = domain.ScanOriginWatch
	}
	if s.stopped {
		cancel(nil)
	}
	if s.onStart != nil {
		s.onStart(s.runOrigin)
	}
	go s.run(runCtx, scan.ID, s.runFinished, dirs, decider)

	s.scanChanged()
	scan, err = s.withImport(ctx, scan)
	return scan, true, err
}

func (s *Scans) CurrentScan(ctx context.Context) (domain.Scan, error) {
	scan, err := s.store.CurrentScan(ctx)
	if err != nil {
		return domain.Scan{}, err
	}
	return s.withImport(ctx, scan)
}

// No previous scan returns ErrNotFound; malformed cursors return ErrInvalidCursor.
func (s *Scans) ListScanIssues(ctx context.Context, cursor string, limit int) (domain.ScanIssuePage, error) {
	scan, err := s.store.CurrentScan(ctx)
	if err != nil {
		return domain.ScanIssuePage{}, err
	}
	issues, err := s.store.ScanIssues(ctx, scan.ID)
	if err != nil {
		return domain.ScanIssuePage{}, err
	}
	items, next, err := domain.PageScanIssues(issues, cursor, limit)
	if err != nil {
		return domain.ScanIssuePage{}, err
	}
	return domain.ScanIssuePage{ScanID: scan.ID, Items: items, NextCursor: next}, nil
}

// Count issues per video, or per file before registration; domain determines import status.
func (s *Scans) withImport(ctx context.Context, scan domain.Scan) (domain.Scan, error) {
	issues, err := s.store.ScanIssues(ctx, scan.ID)
	if err != nil {
		return domain.Scan{}, err
	}
	scan.Issues = domain.CountScanIssues(issues)
	scan.Import = scan.Tally(scan.Issues.Unregistered, scan.Issues.Failed).Progress(scan.SettledAt)
	activity, err := s.currentActivity(ctx)
	if err != nil {
		return domain.Scan{}, err
	}
	scan.Activity = activity
	return scan, nil
}

func (s *Scans) currentActivity(ctx context.Context) (domain.ScanActivity, error) {
	activity := s.activity.current()
	if !activity.Active() || s.roots == nil {
		return activity, nil
	}
	roots, err := s.roots.ListMediaFolders(ctx)
	if err != nil {
		return domain.ScanActivity{}, err
	}
	return activity.Locate(roots), nil
}

// Unknown videoID is zero; empty path marks registration completion.
func (s *Scans) ReportScanFile(path string, videoID int64) {
	if path == "" {
		s.activity.end(scanActivityKey)
		return
	}
	s.activity.begin(scanActivityKey, domain.ScanActivity{
		Kind: domain.ActivityRegistering, VideoID: videoID, Path: path,
	})
}

func (s *Scans) JobStarted(job domain.Job) {
	kind, ok := domain.ActivityKindOf(job.Kind)
	if !ok || job.LocationPath == "" {
		return
	}
	s.activity.begin(jobActivityKey(job), domain.ScanActivity{
		Kind: kind, VideoID: job.VideoID, Path: job.LocationPath,
	})
}

func (s *Scans) JobFinished(job domain.Job) {
	s.activity.end(jobActivityKey(job))
}

// Issue counts affect scan state, so publish a scan change.
func (s *Scans) ReportScanIssue(ctx context.Context, issue domain.ScanFileIssue) error {
	if err := s.store.RecordScanIssue(ctx, issue); err != nil {
		return err
	}
	s.scanChanged()
	return nil
}

// Keep progress updates to one row so listing and playback remain responsive.
func (s *Scans) ReportScanProgress(ctx context.Context, result domain.ScanResult) error {
	if err := s.store.UpdateScanProgress(ctx, s.currentScanID(ctx), progressOf(result)); err != nil {
		return err
	}
	s.scanChanged()
	return nil
}

// Register once before scans start; callbacks run under the scan lock and must return quickly
// without reacquiring it.
func (s *Scans) OnFinish(f func(origin domain.ScanOrigin, state domain.ScanState)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onFinish = f
}

// Register once before scans start; callbacks run under the scan lock and must not access scans.
func (s *Scans) OnStart(f func(origin domain.ScanOrigin)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onStart = f
}

// Wait after canceling the lifecycle context and before closing the event bus; otherwise scan
// deletion events can be lost and artifacts leaked.
func (s *Scans) Wait() {
	s.done.Wait()
}

// Busy includes resumable ingestion scans/jobs, excluding live transcoding.
func (s *Scans) Busy(ctx context.Context) (bool, error) {
	// Watcher scans are background ingestion and do not count as user-visible busy work.
	s.mu.Lock()
	running := s.running && s.runOrigin != domain.ScanOriginWatch
	s.mu.Unlock()
	if running {
		return true, nil
	}
	if s.pending == nil {
		return false, nil
	}
	pending, err := s.pending.HasUnfinishedJobs(ctx)
	if err != nil {
		return false, fmt.Errorf("cannot tell whether jobs are unfinished: %w", err)
	}
	return pending, nil
}

// RecoverInterrupted closes stale running scans to release the uniqueness constraint, rebuilds
// interrupted folder indexes, and requeues running jobs.
func (s *Scans) RecoverInterrupted(ctx context.Context) error {
	closed, err := s.store.FailInterruptedScans(ctx)
	if err != nil {
		return err
	}
	if closed > 0 {
		s.logger.Info("closed interrupted scans", slog.Int64("count", closed))
		s.rebuildFolderIndex(ctx)
	}

	restored, err := s.jobs.RequeueRunningJobs(ctx)
	if err != nil {
		return err
	}
	if restored > 0 {
		s.logger.Info("requeued interrupted jobs", slog.Int64("count", restored))
	}
	return nil
}

// ResumeInterrupted runs once after recovery and worker startup, resuming only the latest
// interrupted manual scan. Carry prior target membership and stage issues because unchanged files
// do not requeue exhausted work.
func (s *Scans) ResumeInterrupted(ctx context.Context) (bool, error) {
	latest, err := s.store.CurrentScan(ctx)
	if errors.Is(err, domain.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	// Do not resume watcher scans at startup; manual scans handle changes made while offline.
	if latest.State != domain.ScanFailed || latest.ErrorCode != domain.ScanErrorInterrupted ||
		latest.Origin == domain.ScanOriginWatch {
		return false, nil
	}
	scan, started, err := s.start(ctx, func(ctx context.Context) (domain.Scan, bool, error) {
		return s.store.ResumeScan(ctx, latest.ID)
	}, nil, domain.ActivityByRule)
	if err != nil {
		return false, err
	}
	if started {
		s.logger.Info("resumed the interrupted scan",
			slog.Int64("interrupted_scan", latest.ID), slog.Int64("scan", scan.ID))
	}
	return started, nil
}

// Failure to find the current ID returns zero so progress writes match nothing while scanning
// continues.
func (s *Scans) currentScanID(ctx context.Context) int64 {
	scan, err := s.store.CurrentScan(ctx)
	if err != nil {
		return 0
	}
	return scan.ID
}

// Lifecycle shutdown also cancels subsequently started scans immediately.
func (s *Scans) stopRuns() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopped = true
	if s.cancelRun != nil {
		s.cancelRun(nil)
	}
}

func (s *Scans) run(
	ctx context.Context, scanID int64, finished chan struct{}, dirs *[]domain.DirtyDirectory, decider domain.ActivityDecider,
) {
	defer s.done.Done()
	origin := domain.ScanOriginManual
	if dirs != nil {
		origin = domain.ScanOriginWatch
	}
	endState := domain.ScanFailed
	defer func() {
		s.mu.Lock()
		s.running = false
		s.cancelRun(nil)
		s.cancelRun = nil
		// Invoke completion under the scan lock so the next scan's start cannot overtake it; callbacks
		// must not reacquire that lock.
		if s.onFinish != nil {
			s.onFinish(origin, endState)
		}
		s.mu.Unlock()
		close(finished)
	}()

	result, scanErr := func() (result domain.ScanResult, err error) {
		defer func() {
			if recovered := recover(); recovered != nil {
				err = fmt.Errorf("scan panicked: %v", recovered)
			}
		}()
		if s.scanner == nil {
			return domain.ScanResult{}, errors.New("scanner is not configured")
		}
		if dirs != nil {
			return s.scanner.ScanScoped(ctx, *dirs)
		}
		return s.scanner.Scan(ctx, decider)
	}()
	// Always clear current registration activity, including interrupted scans.
	s.activity.end(scanActivityKey)

	// Shutdown closes failed/interrupted for restart recovery, taking precedence over scanner-specific
	// errors.
	state := domain.ScanDone
	if errors.Is(scanErr, domain.ErrScanSuperseded) && s.lifetime.Err() == nil {
		// Superseded scans delete nothing and close done rather than failed.
		s.logger.Info("the watch scan was superseded", slog.Int("added", result.Added))
		scanErr = nil
	}
	if scanErr != nil {
		state = domain.ScanFailed
		if ctx.Err() != nil || s.lifetime.Err() != nil {
			scanErr = domain.NewScanFailure(domain.ScanErrorInterrupted, "",
				fmt.Errorf("the scan was stopped before it finished: %w", scanErr))
		}
		s.logger.Warn("scan did not finish", slog.Any("error", scanErr))
	} else {
		s.logger.Info("scan finished",
			slog.Int("total", result.Total),
			slog.Int("added", result.Added),
			slog.Int("updated", result.Updated),
			slog.Int("moved", result.Moved),
			slog.Int("removed", result.Removed),
			slog.Int("failed", result.Failed),
		)
	}

	// Detach cancellation for final writes so shutdown cannot leave scans stuck running.
	closeCtx := context.WithoutCancel(ctx)

	if err := s.store.UpdateScanProgress(closeCtx, scanID, progressOf(result)); err != nil {
		s.logger.Warn("could not record scan progress", slog.Any("error", err))
	}

	// Rebuild from index tables before closing either successful or failed scans, without walking the
	// filesystem.
	s.rebuildFolderIndex(closeCtx)

	if err := s.store.FinishScan(closeCtx, scanID, state, scanErr, decider); err != nil {
		s.logger.Warn("could not record the end of the scan", slog.Any("error", err))
	} else {
		endState = state
	}
	s.scanChanged()
}

// Rebuild failures are logged without failing scans; storage marks the index stale for later retry,
// including startup.
func (s *Scans) rebuildFolderIndex(ctx context.Context) {
	if s.folders == nil {
		return
	}
	if err := s.folders.RebuildFolderIndex(ctx); err != nil {
		s.logger.Warn("could not rebuild the folder index", slog.Any("error", err))
	}
}

func (s *Scans) activityChanged() {
	if s.publisher == nil {
		return
	}
	s.publisher.Publish(domain.ScanActivityChanged{})
}

func (s *Scans) scanChanged() {
	if s.publisher == nil {
		return
	}
	s.publisher.Publish(domain.ScanChanged{})
}

func progressOf(result domain.ScanResult) domain.ScanProgress {
	return domain.ScanProgress{
		Total:     result.Total,
		Completed: result.Completed(),
		Failed:    result.Failed,
	}
}
