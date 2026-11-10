package jobs

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// cancellableJobs holds each job in its handler until its context ends, and returns the context's error as a
// handler stopped mid-run does.
type cancellableJobs struct {
	mu      sync.Mutex
	started []int64
}

func (c *cancellableJobs) handle(ctx context.Context, job domain.Job) error {
	c.mu.Lock()
	c.started = append(c.started, job.ID)
	c.mu.Unlock()
	<-ctx.Done()
	return ctx.Err()
}

func (c *cancellableJobs) startedIDs() []int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.started)
}

func pausedFailures(q *fakeQueue) []int64 {
	q.mu.Lock()
	defer q.mu.Unlock()
	var ids []int64
	for _, failure := range q.failures {
		if errors.Is(failure.cause, domain.ErrJobPaused) {
			ids = append(ids, failure.job.ID)
		}
	}
	return ids
}

// Hold cancels the running job, which the queue records as a paused snooze rather than a completion or a failure,
// and grants nothing until Release, even when woken.
func TestSchedulerHoldCancelsRunningJobsUntilRelease(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		queue := newFakeQueue(domain.Job{ID: 1, Kind: domain.JobProbe}, domain.Job{ID: 2, Kind: domain.JobProbe})
		jobs := &cancellableJobs{}
		scheduler, stop := startScheduler(Options{
			Queue: queue, CPUSlots: 1, Handlers: map[domain.JobKind]Handler{domain.JobProbe: jobs.handle},
		})
		defer stop()
		synctest.Wait()
		if got, want := jobs.startedIDs(), []int64{1}; !slices.Equal(got, want) {
			t.Fatalf("started = %v, want %v", got, want)
		}

		scheduler.Hold()
		synctest.Wait()
		if got, want := pausedFailures(queue), []int64{1}; !slices.Equal(got, want) {
			t.Fatalf("paused snoozes = %v, want %v", got, want)
		}
		if got := queue.completedIDs(); len(got) != 0 {
			t.Fatalf("completed = %v, want none", got)
		}
		claims := queue.claimCount()
		scheduler.Wake()
		time.Sleep(time.Hour)
		synctest.Wait()
		if got := queue.claimCount(); got != claims {
			t.Fatalf("claims while held = %d, want %d", got, claims)
		}

		scheduler.Release()
		synctest.Wait()
		if got, want := jobs.startedIDs(), []int64{1, 2}; !slices.Equal(got, want) {
			t.Fatalf("started after the release = %v, want %v", got, want)
		}
	})
}

// holdOnClaim holds the scheduler as its first claim answers, as a pause that lands in the middle of a round.
type holdOnClaim struct {
	*fakeQueue
	scheduler atomic.Pointer[Scheduler]
	once      sync.Once
}

func (q *holdOnClaim) ClaimJobOn(ctx context.Context, claim domain.JobClaim) (domain.Job, error) {
	job, err := q.fakeQueue.ClaimJobOn(ctx, claim)
	q.once.Do(func() { q.scheduler.Load().Hold() })
	return job, err
}

// A hold during a round grants nothing after it: the job claimed as the hold came never starts and is snoozed, and
// the job that would have shared the round is not claimed.
func TestSchedulerGrantsNothingAfterAHoldDuringARound(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		queue := &holdOnClaim{fakeQueue: newFakeQueue(
			domain.Job{ID: 1, Kind: domain.JobProbe}, domain.Job{ID: 2, Kind: domain.JobFingerprint},
		)}
		jobs := &cancellableJobs{}
		scheduler := newTestScheduler(Options{Queue: queue, CPUSlots: 2, Handlers: map[domain.JobKind]Handler{
			domain.JobProbe: jobs.handle, domain.JobFingerprint: jobs.handle,
		}})
		queue.scheduler.Store(scheduler)
		stop := runScheduler(scheduler)
		defer stop()
		synctest.Wait()
		if got := queue.claimCount(); got != 1 {
			t.Fatalf("claims = %d, want 1", got)
		}
		if got := jobs.startedIDs(); len(got) != 0 {
			t.Fatalf("started = %v, want none", got)
		}
		if got, want := pausedFailures(queue.fakeQueue), []int64{1}; !slices.Equal(got, want) {
			t.Fatalf("paused snoozes = %v, want %v", got, want)
		}
	})
}

// A scheduler created held, as at a startup with the pause saved, claims nothing; after Release it grants again,
// except a kind whose feature is off.
func TestSchedulerCreatedHeldGrantsAfterReleaseExceptFeaturePausedKinds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		queue := newFakeQueue(domain.Job{ID: 1, Kind: domain.JobAutoTag}, domain.Job{ID: 2, Kind: domain.JobProbe})
		jobs := newBlockingJobs()
		scheduler, stop := startScheduler(Options{
			Queue: queue, Handlers: jobs.handlers(), CPUSlots: 1, Held: true,
			Paused: func(context.Context) (map[domain.JobKind]bool, error) {
				return map[domain.JobKind]bool{domain.JobAutoTag: true}, nil
			},
		})
		defer stop()
		synctest.Wait()
		if got := queue.claimCount(); got != 0 {
			t.Fatalf("claims of a scheduler created held = %d, want 0", got)
		}

		scheduler.Release()
		synctest.Wait()
		if got, want := jobs.ids(), []int64{2}; !slices.Equal(got, want) {
			t.Fatalf("running after the release = %v, want %v", got, want)
		}
		if n := queue.claimed(domain.JobAutoTag); n != 0 {
			t.Fatalf("claims of a kind whose feature is off = %d, want 0", n)
		}
	})
}

// A job snoozed by a hold and claimed again before its earlier run returned stays cancellable by the next hold: the
// earlier run's untrack removes only its own registration.
func TestSchedulerEarlierRunKeepsTheNewClaimCancellable(t *testing.T) {
	scheduler := NewScheduler(Options{Logger: slog.New(slog.DiscardHandler)})
	earlier, cancelEarlier := context.WithCancelCause(context.Background())
	untrackEarlier := scheduler.track(1, cancelEarlier)
	scheduler.Hold()
	scheduler.Release()
	reclaimed, cancelReclaimed := context.WithCancelCause(context.Background())
	defer scheduler.track(1, cancelReclaimed)()

	untrackEarlier()
	scheduler.Hold()
	if !errors.Is(context.Cause(earlier), domain.ErrJobPaused) {
		t.Fatalf("earlier run's cause = %v, want the pause", context.Cause(earlier))
	}
	if got := context.Cause(reclaimed); !errors.Is(got, domain.ErrJobPaused) {
		t.Fatalf("reclaimed run's cause after the second hold = %v, want the pause", got)
	}
}
