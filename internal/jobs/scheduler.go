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
	// ClaimJob returns domain.ErrNoJob for an empty queue.
	ClaimJob(ctx context.Context, kind domain.JobKind) (domain.Job, error)
	// Complete only if the claimed location is still current.
	CompleteClaimedJob(ctx context.Context, job domain.Job) error
	// Failures below the attempt limit requeue; storage extracts the message and ProbeFailure code,
	// defaulting to internal.
	FailClaimedJob(ctx context.Context, job domain.Job, cause error) error
	// NextJobRunAfter returns the earliest future run-after time of a queued job, or the zero time when none waits.
	NextJobRunAfter(ctx context.Context) (time.Time, error)
}

// Handler implementations are injected by cmd/mdm; jobs owns claiming and completion, without
// importing app, media, or store. See ARCHITECTURE.md.
type Handler func(ctx context.Context, job domain.Job) error

type Options struct {
	Queue Queue
	// A kind of domain.JobKinds without a handler fails its jobs.
	Handlers map[domain.JobKind]Handler
	Capacity domain.JobResources
	// Optional callback after claiming and immediately before handling.
	Started func(job domain.Job)
	// Optional callback after recording the outcome.
	Finished func(job domain.Job)
	// Nil uses slog.Default.
	Logger *slog.Logger
}

// Scheduler grants each claimed job its kind's demand from free capacity and runs it, so every ingest stage shares
// one set of read and CPU limits. It sleeps on a Loop until a commit notification or a release calls Wake. See
// docs/design-docs/job-scheduling.md.
type Scheduler struct {
	loop     *Loop
	queue    Queue
	handlers map[domain.JobKind]Handler
	capacity domain.JobResources
	started  func(domain.Job)
	finished func(domain.Job)
	logger   *slog.Logger
	// next indexes domain.JobKinds; only step, on the loop goroutine, uses it.
	next int

	mu      sync.Mutex
	used    domain.JobResources
	running sync.WaitGroup
}

func NewScheduler(opts Options) *Scheduler {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Scheduler{
		loop:     NewLoop(),
		queue:    opts.Queue,
		handlers: opts.Handlers,
		capacity: opts.Capacity,
		started:  opts.Started,
		finished: opts.Finished,
		logger:   logger,
	}
}

func (s *Scheduler) Wake() {
	s.loop.Wake()
}

// Run grants at once on startup to recover leftover work, and returns when ctx ends and every running job has
// returned.
func (s *Scheduler) Run(ctx context.Context) {
	s.loop.Run(ctx, s.step)
	s.running.Wait()
}

// step visits the kinds in turn, claiming one job per kind, until a full turn grants nothing. A kind whose demand
// does not fit is skipped, not waited for, so it never holds back a kind that fits. The turn resumes after the last
// granted kind, so a freed slot goes to the next kind instead of always to the first stage. With nothing granted, it
// sleeps until the next run-after time.
func (s *Scheduler) step(ctx context.Context) domain.WorkerWait {
	kinds := domain.JobKinds
	empty := make(map[domain.JobKind]bool, len(kinds))
	for idle := 0; idle < len(kinds); s.next = (s.next + 1) % len(kinds) {
		idle++
		kind := kinds[s.next]
		if empty[kind] {
			continue
		}
		demand := domain.JobDemand(kind)
		if !s.reserve(demand) {
			continue
		}
		job, err := s.queue.ClaimJob(ctx, kind)
		if err != nil {
			s.unreserve(demand)
			if errors.Is(err, domain.ErrNoJob) {
				empty[kind] = true
				continue
			}
			if ctx.Err() == nil {
				s.logger.Warn("could not claim a job", slog.String("stage", string(kind)), slog.Any("error", err))
			}
			return domain.WorkerRetry
		}
		s.grant(ctx, job, demand)
		idle = 0
	}
	return s.untilNextRunAfter(ctx)
}

// untilNextRunAfter lets a job waiting out a retry delay run when the delay ends, without a wakeup.
func (s *Scheduler) untilNextRunAfter(ctx context.Context) domain.WorkerWait {
	next, err := s.queue.NextJobRunAfter(ctx)
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

func (s *Scheduler) reserve(demand domain.JobResources) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.used.Plus(demand)
	if !next.Within(s.capacity) {
		return false
	}
	s.used = next
	return true
}

func (s *Scheduler) unreserve(demand domain.JobResources) domain.JobResources {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.used = s.used.Minus(demand)
	return s.used
}

func (s *Scheduler) grant(ctx context.Context, job domain.Job, demand domain.JobResources) {
	s.mu.Lock()
	used := s.used
	s.mu.Unlock()
	s.logger.Debug("granted a job", jobAttrs(job, demand, used)...)
	s.running.Go(func() {
		s.process(ctx, job)
		used := s.unreserve(demand)
		s.logger.Debug("released a job", jobAttrs(job, demand, used)...)
		s.Wake()
	})
}

func jobAttrs(job domain.Job, demand, used domain.JobResources) []any {
	return []any{
		slog.Int64("job", job.ID), slog.String("stage", string(job.Kind)),
		slog.Int("read", demand.Read), slog.Int("cpu", demand.CPU),
		slog.Int("readInUse", used.Read), slog.Int("cpuInUse", used.CPU),
	}
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
		s.fail(ctx, job, err)
	} else if completeErr := s.queue.CompleteClaimedJob(ctx, job); completeErr != nil {
		s.logger.Warn("could not record job completion",
			slog.Int64("job", job.ID), slog.Any("error", completeErr))
	}
	if s.finished != nil {
		s.finished(job)
	}
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
