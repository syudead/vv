package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// Retry only failed claims; empty queues sleep until Wake instead of polling.
const defaultRetryDelay = 5 * time.Second

// Queue hides persistence from workers.
type Queue interface {
	// ClaimJob returns domain.ErrNoJob for an empty queue.
	ClaimJob(ctx context.Context, kind domain.JobKind) (domain.Job, error)
	// Complete only if the claimed location is still current.
	CompleteClaimedJob(ctx context.Context, job domain.Job) error
	// Failures below the attempt limit requeue; storage extracts the message and ProbeFailure code,
	// defaulting to internal.
	FailClaimedJob(ctx context.Context, job domain.Job, cause error) error
}

// Handler implementations are injected by cmd/mdm; jobs owns claiming and completion, without
// importing app, media, or store. See ARCHITECTURE.md.
type Handler func(ctx context.Context, job domain.Job) error

type Options struct {
	Kind    domain.JobKind
	Queue   Queue
	Handler Handler
	// Optional callback after claiming and immediately before handling.
	Started func(job domain.Job)
	// Optional callback after recording the outcome.
	Finished func(job domain.Job)
	// Zero uses the default claim retry delay.
	RetryDelay time.Duration
	// Nil uses slog.Default.
	Logger *slog.Logger
}

// Worker processes one stage serially to avoid saturating HDD I/O during initial scans; empty
// queues sleep until commit notifications call Wake.
type Worker struct {
	kind     domain.JobKind
	queue    Queue
	handler  Handler
	started  func(domain.Job)
	finished func(domain.Job)
	retry    time.Duration
	logger   *slog.Logger
	wake     chan struct{}
}

func New(opts Options) *Worker {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	retry := opts.RetryDelay
	if retry <= 0 {
		retry = defaultRetryDelay
	}
	return &Worker{
		kind:     opts.Kind,
		queue:    opts.Queue,
		handler:  opts.Handler,
		started:  opts.Started,
		finished: opts.Finished,
		retry:    retry,
		logger:   logger.With(slog.String("stage", string(opts.Kind))),
		// Buffer one notification to avoid losing a wakeup just before sleep.
		wake: make(chan struct{}, 1),
	}
}

func (w *Worker) Kind() domain.JobKind {
	return w.kind
}

// Wake is nonblocking.
func (w *Worker) Wake() {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

// Run checks the queue immediately on startup to recover leftover work before waiting for
// notifications.
func (w *Worker) Run(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}

		job, err := w.queue.ClaimJob(ctx, w.kind)
		switch {
		case errors.Is(err, domain.ErrNoJob):
			if !w.sleep(ctx, nil) {
				return
			}
			continue
		case err != nil:
			if ctx.Err() != nil {
				return
			}
			w.logger.Warn("could not claim a job", slog.Any("error", err))
			timer := time.NewTimer(w.retry)
			ok := w.sleep(ctx, timer.C)
			timer.Stop()
			if !ok {
				return
			}
			continue
		}

		w.process(ctx, job)
	}
}

func (w *Worker) sleep(ctx context.Context, retry <-chan time.Time) bool {
	select {
	case <-ctx.Done():
		return false
	case <-w.wake:
		return true
	case <-retry:
		return true
	}
}

// Cancellation leaves running jobs for startup recovery instead of consuming retry attempts as
// failures.
func (w *Worker) process(ctx context.Context, job domain.Job) {
	if w.started != nil {
		w.started(job)
	}
	var err error
	if w.handler == nil {
		// Retry cannot change this result and would only block the queue.
		err = fmt.Errorf("unknown job kind: %s", job.Kind)
	} else {
		err = w.handler(ctx, job)
	}
	if ctx.Err() != nil {
		return
	}

	if err != nil {
		w.fail(ctx, job, err)
	} else if completeErr := w.queue.CompleteClaimedJob(ctx, job); completeErr != nil {
		w.logger.Warn("could not record job completion",
			slog.Int64("job", job.ID), slog.Any("error", completeErr))
	}
	if w.finished != nil {
		w.finished(job)
	}
}

// The queue owns the retry-limit decision.
func (w *Worker) fail(ctx context.Context, job domain.Job, cause error) {
	w.logger.Warn("job failed",
		slog.Int64("job", job.ID),
		slog.String("kind", string(job.Kind)),
		slog.Int64("video", job.VideoID),
		slog.Int("attempts", job.Attempts),
		slog.Any("error", cause),
	)

	if err := w.queue.FailClaimedJob(ctx, job, cause); err != nil {
		w.logger.Warn("could not record job failure",
			slog.Int64("job", job.ID), slog.Any("error", err))
	}
}
