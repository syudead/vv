package app

import (
	"context"
	"log/slog"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/syudead/vv/internal/domain"
)

const (
	// Bound watcher teardown so an unresponsive mount cannot block process shutdown.
	autoImportStopWait = 5 * time.Second
	autoImportQuiet    = 2 * time.Second
	autoImportSettle   = 10 * time.Second
)

// cmd/mdm connects watcher events to Changed and WatchProblem.
type FolderWatcher interface {
	// Arm enumerates directories only, replacing all existing watches.
	Arm(roots []string) error
	Disarm()
}

type WatchScans interface {
	StartWatchScan(ctx context.Context, dirs []domain.DirtyDirectory) (domain.Scan, bool, error)
	SupersedeWatchScan(ctx context.Context) bool
}

type AutoImportStore interface {
	// Unset defaults to enabled.
	AutoImport(ctx context.Context) (bool, error)
	SaveAutoImport(ctx context.Context, enabled bool) error
}

// Return the newest mtime in scope, or zero for absent/empty directories.
type ChangeProbe func(dir domain.DirtyDirectory) (time.Time, error)

// Replaceable clock for deterministic tests.
type Clock interface {
	Now() time.Time
	// AfterFunc runs f asynchronously after d and returns a cancelable timer.
	AfterFunc(d time.Duration, f func()) ClockTimer
}

type ClockTimer interface {
	Stop() bool
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

func (systemClock) AfterFunc(d time.Duration, f func()) ClockTimer { return time.AfterFunc(d, f) }

type AutoImportOptions struct {
	Store   AutoImportStore
	Folders ActivityFolderStore
	Watcher FolderWatcher
	Scans   WatchScans
	// Nil skips settling probes and imports after the quiet period.
	Probe ChangeProbe
	// Nil uses real time.
	Clock Clock
	// Zero uses two-second Quiet and ten-second Settle defaults.
	Quiet  time.Duration
	Settle time.Duration
	// Nil uses slog.Default.
	Logger *slog.Logger
	// Zero uses a five-second shutdown wait.
	StopWait time.Duration
}

// AutoImport batches changed directories into one watcher scan after settling; startup and enabling
// never initiate a scan.
type AutoImport struct {
	store    AutoImportStore
	folders  ActivityFolderStore
	watcher  FolderWatcher
	scans    WatchScans
	probe    ChangeProbe
	clock    Clock
	quiet    time.Duration
	settle   time.Duration
	logger   *slog.Logger
	stopWait time.Duration

	// Serialize enable changes and folder-triggered rearming.
	setMu sync.Mutex
	// Change enabled/stopped/holding before crossing startMu so earlier starts are superseded and
	// later starts abort; acquire startMu before mu.
	startMu sync.Mutex
	// Serialize generation checks with Arm/Disarm so an old generation cannot replace newer watches.
	armMu sync.Mutex

	mu      sync.Mutex
	ctx     context.Context
	started bool
	stopped bool
	enabled bool
	state   domain.FolderWatchState
	problem *domain.FolderWatchProblem
	// Increment on rearming to discard stale results.
	armGen int
	arms   sync.WaitGroup

	// Deduplicate dirty directories so repeated events grow memory only with distinct directories.
	dirty     []domain.DirtyDirectory
	dirtyKeys map[domain.DirtyDirectory]struct{}
	inflight  []domain.DirtyDirectory
	timer     ClockTimer
	timerSeq  int
	// rerun records scheduling requests received while batching.
	batching bool
	rerun    bool
	// A nonzero count blocks scan starts during folder changes.
	holding int
	// Detect scans finishing during a start decision.
	finishes int
	// A manual scan clears only losses reported before it started; newer losses may concern
	// directories already scanned.
	lostGen     int
	scanLostGen int
}

// NewAutoImport watches nothing until Start.
func NewAutoImport(opts AutoImportOptions) *AutoImport {
	a := &AutoImport{
		store:   opts.Store,
		folders: opts.Folders,
		watcher: opts.Watcher,
		scans:   opts.Scans,
		probe:   opts.Probe,
		clock:   opts.Clock,
		quiet:   opts.Quiet,
		settle:  opts.Settle,
		logger:  opts.Logger,
		enabled: true,
		state:   domain.FolderWatchOff,
	}
	if a.clock == nil {
		a.clock = systemClock{}
	}
	if a.quiet <= 0 {
		a.quiet = autoImportQuiet
	}
	if a.settle <= 0 {
		a.settle = autoImportSettle
	}
	a.stopWait = opts.StopWait
	if a.stopWait <= 0 {
		a.stopWait = autoImportStopWait
	}
	if a.logger == nil {
		a.logger = slog.Default()
	}
	return a
}

// Start arms asynchronously from saved settings without scanning; ctx must outlive later watcher
// scans.
func (a *AutoImport) Start(ctx context.Context) error {
	enabled, err := a.store.AutoImport(ctx)
	if err != nil {
		return err
	}
	a.setMu.Lock()
	defer a.setMu.Unlock()
	a.mu.Lock()
	a.ctx = ctx
	a.started = true
	a.enabled = enabled
	a.mu.Unlock()
	if enabled {
		a.arm(ctx)
	}
	return nil
}

// Stop prevents later starts and bounds watch teardown; active scans stop through their own
// lifecycle context, and stale arm generations clean themselves up afterward.
func (a *AutoImport) Stop() {
	a.mu.Lock()
	a.stopped = true
	a.cancelTimerLocked()
	a.armGen++
	a.mu.Unlock()
	// Wait for starts already deciding; later starts observe stopped.
	a.quiesceStarts()
	// Disarm may wait on an unresponsive mount, so include it within the shutdown deadline.
	done := make(chan struct{})
	go func() {
		a.watcher.Disarm()
		a.arms.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(a.stopWait):
		a.logger.Warn("stopping while the media folders are still being armed")
	}
}

func (a *AutoImport) quiesceStarts() {
	a.startMu.Lock()
	defer a.startMu.Unlock()
}

// holdStarts blocks new starts and supersedes active watcher scans; callers must releaseStarts and
// may requeue the returned directories.
func (a *AutoImport) holdStarts(ctx context.Context) []domain.DirtyDirectory {
	a.mu.Lock()
	a.holding++
	a.mu.Unlock()
	a.quiesceStarts()
	a.mu.Lock()
	batch := slices.Clone(a.inflight)
	a.mu.Unlock()
	if !a.scans.SupersedeWatchScan(ctx) {
		return nil
	}
	return batch
}

func (a *AutoImport) requeueLocked(batch []domain.DirtyDirectory) {
	if len(batch) == 0 {
		return
	}
	a.addDirtyLocked(batch...)
	a.inflight = nil
}

func (a *AutoImport) addDirtyLocked(dirs ...domain.DirtyDirectory) {
	for _, dir := range dirs {
		if dir.Path == "" {
			continue
		}
		dir.Path = filepath.Clean(dir.Path)
		if _, ok := a.dirtyKeys[dir]; ok {
			continue
		}
		if a.dirtyKeys == nil {
			a.dirtyKeys = map[domain.DirtyDirectory]struct{}{}
		}
		a.dirtyKeys[dir] = struct{}{}
		a.dirty = append(a.dirty, dir)
	}
}

func (a *AutoImport) takeDirtyLocked() []domain.DirtyDirectory {
	taken := a.dirty
	a.dirty, a.dirtyKeys = nil, nil
	return taken
}

func (a *AutoImport) Status() domain.AutoImportStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.statusLocked()
}

func (a *AutoImport) statusLocked() domain.AutoImportStatus {
	status := domain.AutoImportStatus{Enabled: a.enabled, State: a.state}
	if !a.enabled {
		status.State = domain.FolderWatchOff
	}
	if status.State == domain.FolderWatchLimited && a.problem != nil {
		problem := *a.problem
		status.Problem = &problem
	}
	return status
}

// SetEnabled persists then arms/disarms without scanning; disabling discards pending directories,
// and unchanged settings are a no-op.
func (a *AutoImport) SetEnabled(ctx context.Context, enabled bool) (domain.AutoImportStatus, error) {
	a.setMu.Lock()
	defer a.setMu.Unlock()
	a.mu.Lock()
	same := a.enabled == enabled
	a.mu.Unlock()
	if same {
		return a.Status(), nil
	}
	var batch []domain.DirtyDirectory
	if !enabled {
		// Stop new and active watcher scans before disabling.
		batch = a.holdStarts(ctx)
	}
	if err := a.store.SaveAutoImport(ctx, enabled); err != nil {
		if !enabled {
			// Restore superseded work if disabling fails.
			a.mu.Lock()
			a.holding--
			a.requeueLocked(batch)
			if a.enabled && !a.stopped && len(a.dirty) > 0 {
				a.scheduleLocked(a.quiet)
			}
			a.mu.Unlock()
		}
		return domain.AutoImportStatus{}, err
	}
	a.mu.Lock()
	a.enabled = enabled
	a.problem = nil
	if !enabled {
		a.holding--
		a.state = domain.FolderWatchOff
		a.takeDirtyLocked()
		a.inflight = nil
		a.cancelTimerLocked()
		a.armGen++
	}
	a.mu.Unlock()
	if enabled {
		a.arm(context.WithoutCancel(ctx))
	} else {
		a.watcher.Disarm()
	}
	return a.Status(), nil
}

// Call FoldersChanging before persistence, which rejects changes during scans; stop/requeue watcher
// scans and block starts until FoldersChanged.
func (a *AutoImport) FoldersChanging(ctx context.Context) {
	batch := a.holdStarts(ctx)
	a.mu.Lock()
	a.requeueLocked(batch)
	a.mu.Unlock()
}

// Call FoldersChanged after every attempted change, including failures, to rearm and reschedule
// retained work.
func (a *AutoImport) FoldersChanged(ctx context.Context) {
	a.setMu.Lock()
	defer a.setMu.Unlock()
	a.mu.Lock()
	a.holding--
	enabled := a.enabled && a.started && !a.stopped
	if enabled {
		a.problem = nil
		if len(a.dirty) > 0 {
			a.scheduleLocked(a.quiet)
		}
	}
	a.mu.Unlock()
	if enabled {
		a.arm(context.WithoutCancel(ctx))
	}
}

func (a *AutoImport) Changed(dir domain.DirtyDirectory) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.enabled || !a.started || a.stopped {
		return
	}
	a.addDirtyLocked(dir)
	a.scheduleLocked(a.quiet)
}

func (a *AutoImport) WatchProblem(problem domain.FolderWatchProblem) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.enabled || a.stopped {
		return
	}
	if problem.Kind == domain.FolderWatchProblemEventsLost {
		a.lostGen++
	}
	a.problem = &problem
	a.state = domain.FolderWatchLimited
}

// Caller must hold setMu; no folders means off.
func (a *AutoImport) arm(ctx context.Context) {
	a.mu.Lock()
	if a.stopped {
		a.mu.Unlock()
		return
	}
	a.armGen++
	gen := a.armGen
	a.state = domain.FolderWatchStarting
	a.problem = nil
	a.arms.Add(1)
	a.mu.Unlock()
	go func() {
		defer a.arms.Done()
		a.armTree(ctx, gen)
	}()
}

func (a *AutoImport) armTree(ctx context.Context, gen int) {
	folders, err := a.folders.ListMediaFolders(ctx)
	if err != nil {
		a.logger.Warn("could not list the media folders to watch", slog.Any("error", err))
		a.mu.Lock()
		if gen == a.armGen {
			a.state = domain.FolderWatchLimited
			a.problem = &domain.FolderWatchProblem{Kind: domain.FolderWatchProblemFolderUnreachable}
		}
		a.mu.Unlock()
		return
	}
	roots := make([]string, 0, len(folders))
	for _, folder := range folders {
		roots = append(roots, folder.Path)
	}
	// Check generation while serializing Arm/Disarm; stale generations must not alter newer watches.
	a.armMu.Lock()
	defer a.armMu.Unlock()
	a.mu.Lock()
	current := gen == a.armGen
	a.mu.Unlock()
	if !current {
		a.disarmIfOff()
		return
	}
	if len(roots) == 0 {
		a.watcher.Disarm()
		a.mu.Lock()
		if gen == a.armGen {
			a.state = domain.FolderWatchOff
		}
		a.mu.Unlock()
		return
	}
	armErr := a.watcher.Arm(roots)
	if armErr != nil {
		a.logger.Warn("could not watch the media folders", slog.Any("error", armErr))
	}
	a.mu.Lock()
	if gen != a.armGen {
		a.mu.Unlock()
		a.disarmIfOff()
		return
	}
	defer a.mu.Unlock()
	if armErr != nil && a.problem == nil {
		// No installed watch means inactive even before a problem notification arrives.
		a.problem = &domain.FolderWatchProblem{Kind: domain.FolderWatchProblemFolderUnreachable}
	}
	// Do not overwrite a problem received during arming with active status.
	if a.problem != nil {
		a.state = domain.FolderWatchLimited
	} else {
		a.state = domain.FolderWatchActive
	}
}

// Caller holds armMu; disabled/stopped stale generations disarm, while replacement generations
// install their own watches.
func (a *AutoImport) disarmIfOff() {
	a.mu.Lock()
	off := !a.enabled || a.stopped
	a.mu.Unlock()
	if off {
		a.watcher.Disarm()
	}
}

// Reschedule pending directories when scans finish; successful manual scans clear only event losses
// recorded before their start, preserving later losses for the next manual scan.
func (a *AutoImport) ScanFinished(origin domain.ScanOrigin, state domain.ScanState) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.finishes++
	if origin == domain.ScanOriginWatch {
		a.inflight = nil
	}
	if origin == domain.ScanOriginManual && state == domain.ScanDone &&
		a.problem != nil && a.problem.Kind == domain.FolderWatchProblemEventsLost &&
		a.lostGen == a.scanLostGen {
		a.problem = nil
		if a.state == domain.FolderWatchLimited {
			a.state = domain.FolderWatchActive
		}
	}
	if a.enabled && !a.stopped && len(a.dirty) > 0 {
		a.scheduleLocked(a.quiet)
	}
}

// Record the loss generation at manual-scan start; return quickly because the scan lock is held.
func (a *AutoImport) ScanStarted(origin domain.ScanOrigin) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if origin == domain.ScanOriginManual {
		a.scanLostGen = a.lostGen
	}
}

func (a *AutoImport) cancelTimerLocked() {
	a.timerSeq++
	if a.timer != nil {
		a.timer.Stop()
		a.timer = nil
	}
}

func (a *AutoImport) scheduleLocked(d time.Duration) {
	if a.stopped {
		return
	}
	a.cancelTimerLocked()
	seq := a.timerSeq
	a.timer = a.clock.AfterFunc(d, func() { a.fire(seq) })
}

// Only settled directories enter the next watcher scan; retain changing directories.
func (a *AutoImport) fire(seq int) {
	a.mu.Lock()
	if seq != a.timerSeq || a.stopped || !a.enabled || a.ctx == nil {
		a.mu.Unlock()
		return
	}
	a.timer = nil
	if a.batching {
		a.rerun = true
		a.mu.Unlock()
		return
	}
	if a.holding > 0 || a.inflight != nil || len(a.dirty) == 0 {
		// ScanFinished and FoldersChanged reschedule remaining work.
		a.mu.Unlock()
		return
	}
	a.batching = true
	taken := domain.NormalizeDirtyDirectories(a.takeDirtyLocked())
	ctx := a.ctx
	finishes := a.finishes
	a.mu.Unlock()

	settled, unsettled, wait := a.settledSplit(taken)

	a.mu.Lock()
	if a.enabled && !a.stopped {
		a.addDirtyLocked(unsettled...)
		if len(unsettled) > 0 {
			a.scheduleLocked(wait)
		}
	}
	a.mu.Unlock()

	if len(settled) > 0 {
		a.start(ctx, settled, finishes)
	}

	a.mu.Lock()
	a.batching = false
	if a.rerun {
		a.rerun = false
		if len(a.dirty) > 0 {
			a.scheduleLocked(a.quiet)
		}
	}
	a.mu.Unlock()
}

// Serialize status check and scan start under startMu so disabling, shutdown, or folder changes
// cannot race a later start; failed starts restore dirty directories.
func (a *AutoImport) start(ctx context.Context, settled []domain.DirtyDirectory, finishes int) {
	a.startMu.Lock()
	defer a.startMu.Unlock()
	a.mu.Lock()
	if a.stopped || !a.enabled {
		a.mu.Unlock()
		return
	}
	if a.holding > 0 {
		// FoldersChanged will reschedule after the active change finishes.
		a.addDirtyLocked(settled...)
		a.mu.Unlock()
		return
	}
	a.inflight = settled
	a.mu.Unlock()

	_, started, err := a.scans.StartWatchScan(ctx, settled)
	if err == nil && started {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.inflight = nil
	if a.stopped || !a.enabled {
		return
	}
	a.addDirtyLocked(settled...)
	switch {
	case err != nil:
		a.logger.Warn("could not start a watch scan", slog.Any("error", err))
		a.scheduleLocked(a.settle)
	case a.finishes != finishes:
		// The active scan finished while this start was rejected, so its completion notification missed
		// rescheduling.
		a.scheduleLocked(a.quiet)
	default:
		// ScanFinished reschedules once the other scan ends.
	}
}

// Unreadable directories count as settled so the scanner can record their actual access failure.
func (a *AutoImport) settledSplit(dirs []domain.DirtyDirectory) (settled, unsettled []domain.DirtyDirectory, wait time.Duration) {
	if a.probe == nil {
		return dirs, nil, 0
	}
	now := a.clock.Now()
	wait = a.settle
	for _, dir := range dirs {
		newest, err := a.probe(dir)
		if err != nil {
			a.logger.Warn("could not check whether a directory has settled",
				slog.String("path", dir.Path), slog.Any("error", err))
			settled = append(settled, dir)
			continue
		}
		age := now.Sub(newest)
		// Treat future mtimes as settled because the source clock may differ.
		if newest.IsZero() || age >= a.settle || age < -a.settle {
			settled = append(settled, dir)
			continue
		}
		unsettled = append(unsettled, dir)
		wait = min(wait, max(a.settle-age, time.Second))
	}
	return settled, unsettled, wait
}
