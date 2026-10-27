package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// Queue hides persistence from the scheduler.
type Queue interface {
	// ClaimJobOn returns domain.ErrNoJob when no job matches the claim.
	ClaimJobOn(ctx context.Context, claim domain.JobClaim) (domain.Job, error)
	// DueJobKinds returns the most urgent class among each kind's queued jobs that are due now.
	DueJobKinds(ctx context.Context) (map[domain.JobKind]domain.JobPriority, error)
	// Complete only if the claimed location is still current.
	CompleteClaimedJob(ctx context.Context, job domain.Job) error
	// Failures below the attempt limit requeue; storage extracts the message and ProbeFailure code,
	// defaulting to internal.
	FailClaimedJob(ctx context.Context, job domain.Job, cause error) error
	// NextJobRunAfter returns the earliest run-after time of a queued job after since, or the zero time when none
	// is; a time already past means the job came due after claims that began at since.
	NextJobRunAfter(ctx context.Context, since time.Time) (time.Time, error)
}

// Folders lists the registered media folders, whose disks the scheduler serves in turn.
type Folders interface {
	ListMediaFolders(ctx context.Context) ([]domain.MediaFolder, error)
}

// Handler implementations are injected by cmd/mdm; jobs owns claiming and completion, without
// importing app, media, or store. See ARCHITECTURE.md.
type Handler func(ctx context.Context, job domain.Job) error

type Options struct {
	Queue   Queue
	Folders Folders
	// Disks resolves a media folder's disk; an error marks the folder unavailable.
	Disks DiskResolver
	// A kind of domain.ScheduledJobKinds without a handler fails its jobs.
	Handlers map[domain.JobKind]Handler
	CPUSlots int
	// Paused reports the kinds whose feature is off; their jobs stay queued and are not claimed. Whoever turns a
	// feature back on must Wake the scheduler. Nil pauses nothing.
	Paused func(ctx context.Context) (map[domain.JobKind]bool, error)
	// Optional callback after claiming and immediately before handling.
	Started func(job domain.Job)
	// Optional callback after recording the outcome.
	Finished func(job domain.Job)
	// Nil uses slog.Default.
	Logger *slog.Logger
}

// Scheduler grants each claimed job its kind's demand from free capacity and runs it, so every ingest stage shares
// one CPU limit and each disk's read limit. It sleeps on a Loop until a commit notification or a release calls Wake.
// See docs/design-docs/job-scheduling.md.
type Scheduler struct {
	loop     *Loop
	queue    Queue
	folders  Folders
	disks    DiskResolver
	handlers map[domain.JobKind]Handler
	started  func(domain.Job)
	finished func(domain.Job)
	paused   func(context.Context) (map[domain.JobKind]bool, error)
	logger   *slog.Logger
	capacity *capacity
	roots    *roots
	// The rotation positions are used only by step, on the loop goroutine: nextGroup over the disks, and nextKind
	// over the kinds within each disk.
	nextGroup int
	nextKind  map[string]int
	running   sync.WaitGroup
}

func NewScheduler(opts Options) *Scheduler {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	s := &Scheduler{
		loop:     NewLoop(),
		queue:    opts.Queue,
		folders:  opts.Folders,
		disks:    opts.Disks,
		handlers: opts.Handlers,
		started:  opts.Started,
		finished: opts.Finished,
		paused:   opts.Paused,
		logger:   logger,
		capacity: newCapacity(opts.CPUSlots),
		nextKind: map[string]int{},
	}
	s.roots = newRoots(opts.Disks, logger, s.Wake)
	return s
}

func (s *Scheduler) Wake() {
	s.loop.Wake()
}

// RecheckRoots checks the unavailable media folders again, as after a scan starts or the folder watcher reports a
// change, so a folder that is readable again has its jobs claimed.
func (s *Scheduler) RecheckRoots() {
	s.roots.requestRecheck()
	s.Wake()
}

// Run grants at once on startup to recover leftover work, and returns when ctx ends and every running job has
// returned.
func (s *Scheduler) Run(ctx context.Context) {
	s.loop.Run(ctx, s.step)
	s.running.Wait()
}

// turn is what one step knows while it grants: the disks in rotation, the most urgent due class of each kind, the
// paused kinds, the class being served and the unavailable folders.
type turn struct {
	groups      []diskGroup
	due         map[domain.JobKind]domain.JobPriority
	paused      map[domain.JobKind]bool
	class       domain.JobPriority
	unavailable []int64
}

// step serves the classes strictly in order. Within a class it visits the disks in turn, claiming one job per visit,
// until a full turn grants nothing. With nothing left to grant, it sleeps until the next run-after time or the next
// recheck of an unavailable folder.
func (s *Scheduler) step(ctx context.Context) domain.WorkerWait {
	// The turn's claims see every job due by now, so the lookup from now also finds one that came due mid-turn.
	since := time.Now()
	t, err := s.prepare(ctx, since)
	if err != nil {
		if ctx.Err() == nil {
			s.logger.Warn("could not prepare the job grants", slog.Any("error", err))
		}
		return domain.WorkerRetry
	}
	for t.class = domain.JobPriorityOwner; t.class <= domain.JobPriorityBackfill; t.class++ {
		if err := s.grantClass(ctx, t); err != nil {
			return domain.WorkerRetry
		}
	}
	return s.untilNextCheck(ctx, since)
}

func (s *Scheduler) prepare(ctx context.Context, now time.Time) (turn, error) {
	folders, err := s.folders.ListMediaFolders(ctx)
	if err != nil {
		return turn{}, fmt.Errorf("cannot list the media folders: %w", err)
	}
	s.roots.refresh(folders, now)
	var t turn
	var readSlots map[string]int
	t.groups, readSlots, t.unavailable = s.roots.snapshot()
	s.capacity.setDisks(readSlots)
	if t.due, err = s.queue.DueJobKinds(ctx); err != nil {
		return turn{}, err
	}
	if s.paused != nil {
		if t.paused, err = s.paused(ctx); err != nil {
			return turn{}, fmt.Errorf("cannot read the paused job kinds: %w", err)
		}
	}
	return t, nil
}

// grantClass visits the disks round robin from where the last turn stopped, so a disk with a long queue takes turns
// with the others instead of going first.
func (s *Scheduler) grantClass(ctx context.Context, t turn) error {
	empty := map[string]bool{}
	for idle := 0; idle < len(t.groups); {
		at := s.nextGroup % len(t.groups)
		s.nextGroup = (at + 1) % len(t.groups)
		granted, err := s.grantIn(ctx, t.groups[at], t, empty)
		if err != nil {
			return err
		}
		if granted {
			idle = 0
		} else {
			idle++
		}
	}
	return nil
}

// grantIn claims one job of the disk's kinds in turn, resuming after the last granted kind so a freed slot goes to the
// next stage instead of always to the first. A kind whose demand does not fit is skipped, not waited for, and a
// paused kind is skipped like an empty one.
func (s *Scheduler) grantIn(ctx context.Context, g diskGroup, t turn, empty map[string]bool) (bool, error) {
	start := s.nextKind[g.disk]
	for i := range g.kinds {
		at := (start + i) % len(g.kinds)
		kind := g.kinds[at]
		key := g.disk + "\x00" + string(kind)
		if class, ok := t.due[kind]; !ok || class > t.class || empty[key] || t.paused[kind] {
			continue
		}
		demand := domain.JobDemand(kind)
		if !s.capacity.reserve(g.disk, demand) {
			continue
		}
		job, err := s.queue.ClaimJobOn(ctx, domain.JobClaim{
			Kind: kind, MaxPriority: t.class, Roots: g.roots, Unavailable: t.unavailable,
		})
		if err != nil {
			s.capacity.release(g.disk, demand)
			if errors.Is(err, domain.ErrNoJob) {
				empty[key] = true
				continue
			}
			if ctx.Err() == nil {
				s.logger.Warn("could not claim a job", slog.String("stage", string(kind)), slog.Any("error", err))
			}
			return false, err
		}
		s.nextKind[g.disk] = at + 1
		s.grant(ctx, job, g.disk, demand)
		return true, nil
	}
	return false, nil
}

// untilNextCheck lets a job waiting out a retry delay run when the delay ends, and an unavailable folder be checked
// again, without a wakeup.
func (s *Scheduler) untilNextCheck(ctx context.Context, since time.Time) domain.WorkerWait {
	wait := s.untilNextRunAfter(ctx, since)
	recheck, pending := s.roots.untilRecheck(time.Now())
	if pending && (wait == domain.WorkerIdle || domain.WorkerWait(recheck) < wait) {
		return domain.WorkerWait(recheck)
	}
	return wait
}

func (s *Scheduler) untilNextRunAfter(ctx context.Context, since time.Time) domain.WorkerWait {
	next, err := s.queue.NextJobRunAfter(ctx, since)
	if err != nil {
		if ctx.Err() == nil {
			s.logger.Warn("could not read the next job retry time", slog.Any("error", err))
		}
		return domain.WorkerRetry
	}
	if next.IsZero() {
		return domain.WorkerIdle
	}
	return domain.WorkerWait(max(time.Until(next), 0))
}

func (s *Scheduler) grant(ctx context.Context, job domain.Job, disk string, demand domain.JobResources) {
	s.logger.Debug("granted a job", s.slotAttrs(job, disk, demand)...)
	jobCtx := domain.WithJobSlots(ctx, lentSlots{scheduler: s, job: job, disk: disk})
	s.running.Go(func() {
		s.process(jobCtx, job)
		s.capacity.release(disk, demand)
		s.logger.Debug("released a job", s.slotAttrs(job, disk, demand)...)
		s.Wake()
	})
}

// AcquireRead waits for a read slot on the disk of the media folder at root, for one content-key read of a scan. A
// waiting scan read takes the next free slot of its disk before any job, but never stops a running job. A folder whose
// disk cannot be resolved returns the error, so the read never bypasses the disk's limit. Release returns the slot.
func (s *Scheduler) AcquireRead(ctx context.Context, root string) (release func(), err error) {
	disk, err := s.scanDisk(root)
	if err != nil {
		return nil, err
	}
	if err := s.capacity.acquireScanRead(ctx, disk); err != nil {
		// A job refused while this read waited needs another turn.
		s.Wake()
		return nil, err
	}
	s.logger.Debug("granted a scan read", s.scanAttrs(disk.Key)...)
	return sync.OnceFunc(func() {
		s.capacity.releaseScanRead(disk.Key)
		s.logger.Debug("released a scan read", s.scanAttrs(disk.Key)...)
		s.Wake()
	}), nil
}

// KeepAhead keeps one read slot of the disk of the media folder at root free from jobs between the scan's content-key
// reads there, so a job does not take the disk each time a read returns its slot. Done ends it; a folder whose disk
// cannot be resolved keeps nothing, and its reads fail in AcquireRead.
func (s *Scheduler) KeepAhead(root string) (done func()) {
	disk, err := s.scanDisk(root)
	if err != nil {
		return func() {}
	}
	end := s.capacity.keepScanAhead(disk.Key)
	return sync.OnceFunc(func() {
		end()
		s.Wake()
	})
}

// scanDisk returns the disk of the media folder at root, resolving it when the scheduler has not.
func (s *Scheduler) scanDisk(root string) (domain.Disk, error) {
	if disk, ok := s.roots.disk(root); ok {
		return disk, nil
	}
	disk, err := s.disks(root)
	if err != nil {
		return domain.Disk{}, fmt.Errorf("cannot resolve the disk of the media folder (%s): %w", root, err)
	}
	return disk, nil
}

func (s *Scheduler) scanAttrs(disk string) []any {
	return []any{slog.String("disk", disk), slog.Int("readInUse", s.capacity.inUse(disk).Read)}
}

func (s *Scheduler) slotAttrs(job domain.Job, disk string, demand domain.JobResources) []any {
	used := s.capacity.inUse(disk)
	return []any{
		slog.Int64("job", job.ID), slog.String("stage", string(job.Kind)), slog.String("disk", disk),
		slog.Int("read", demand.Read), slog.Int("cpu", demand.CPU),
		slog.Int("classifier", demand.Classifier),
		slog.Int("readInUse", used.Read), slog.Int("cpuInUse", used.CPU), slog.Int("classifierInUse", used.Classifier),
	}
}

// lentSlots lends a running job further slots on its disk, so a job that splits its work uses idle slots and returns
// each as its part ends.
type lentSlots struct {
	scheduler *Scheduler
	job       domain.Job
	disk      string
}

func (l lentSlots) TryAcquire(demand domain.JobResources) bool {
	if !l.scheduler.capacity.reserve(l.disk, demand) {
		return false
	}
	l.scheduler.logger.Debug("lent slots to a job", l.scheduler.slotAttrs(l.job, l.disk, demand)...)
	return true
}

func (l lentSlots) Release(demand domain.JobResources) {
	l.scheduler.capacity.release(l.disk, demand)
	l.scheduler.logger.Debug("returned lent slots", l.scheduler.slotAttrs(l.job, l.disk, demand)...)
	l.scheduler.Wake()
}

// Cancellation leaves running jobs for startup recovery instead of consuming retry attempts as
// failures.
func (s *Scheduler) process(ctx context.Context, job domain.Job) {
	if s.started != nil {
		s.started(job)
	}
	var err error
	if handler := s.handlers[job.Kind]; handler == nil {
		// Retry cannot change this result and would only block the queue.
		err = fmt.Errorf("unknown job kind: %s", job.Kind)
	} else {
		err = handler(ctx, job)
	}
	if ctx.Err() != nil {
		return
	}

	if err != nil {
		s.fail(ctx, job, s.snoozeIfRootUnreadable(job, err))
	} else if completeErr := s.queue.CompleteClaimedJob(ctx, job); completeErr != nil {
		s.logger.Warn("could not record job completion",
			slog.Int64("job", job.ID), slog.Any("error", completeErr))
	}
	if s.finished != nil {
		s.finished(job)
	}
}

// snoozeIfRootUnreadable turns a failure into a snooze when the job's media folder cannot be read, and parks the
// folder's jobs; a file missing under a readable folder was deleted, so it stays a failure.
func (s *Scheduler) snoozeIfRootUnreadable(job domain.Job, cause error) error {
	if job.RootID == 0 {
		return cause
	}
	path, ok := s.roots.path(job.RootID)
	if !ok {
		return cause
	}
	if !errors.Is(cause, domain.ErrJobSnoozed) {
		if _, err := s.disks(path); err == nil {
			return cause
		}
		cause = fmt.Errorf("%w: the media folder cannot be read: %w", domain.ErrJobSnoozed, cause)
	}
	s.roots.markUnavailable(job.RootID, time.Now())
	s.logger.Warn("parked the jobs of a media folder that cannot be read", slog.String("path", path))
	return cause
}

// The queue owns the retry-limit decision.
func (s *Scheduler) fail(ctx context.Context, job domain.Job, cause error) {
	s.logger.Warn("job failed",
		slog.Int64("job", job.ID),
		slog.String("kind", string(job.Kind)),
		slog.Int64("video", job.VideoID),
		slog.Int("attempts", job.Attempts),
		slog.Any("error", cause),
	)

	if err := s.queue.FailClaimedJob(ctx, job, cause); err != nil {
		s.logger.Warn("could not record job failure",
			slog.Int64("job", job.ID), slog.Any("error", err))
	}
}
