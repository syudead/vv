package jobs

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/syudead/vv/internal/domain"
)

type jobFailure struct {
	job   domain.Job
	cause error
}

// The queue hands out scripted jobs per kind, without reproducing the store's claim conditions or retry policy.
type fakeQueue struct {
	mu        sync.Mutex
	queued    map[domain.JobKind][]domain.Job
	claimErr  error
	claims    []domain.JobKind
	completed []domain.Job
	failures  []jobFailure
	observe   func(string)
	// runAfter holds a kind's jobs back until that time, as a retry delay does.
	runAfter time.Time
}

func newFakeQueue(jobs ...domain.Job) *fakeQueue {
	q := &fakeQueue{queued: map[domain.JobKind][]domain.Job{}}
	q.add(jobs...)
	return q
}

func (q *fakeQueue) add(jobs ...domain.Job) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, job := range jobs {
		q.queued[job.Kind] = append(q.queued[job.Kind], job)
	}
}

func (q *fakeQueue) ClaimJob(_ context.Context, kind domain.JobKind) (domain.Job, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.claims = append(q.claims, kind)
	if err := q.claimErr; err != nil {
		q.claimErr = nil
		return domain.Job{}, err
	}
	queued := q.queued[kind]
	if len(queued) == 0 || time.Now().Before(q.runAfter) {
		return domain.Job{}, domain.ErrNoJob
	}
	q.queued[kind] = queued[1:]
	return queued[0], nil
}

func (q *fakeQueue) claimed(kind domain.JobKind) int {
	q.mu.Lock()
	defer q.mu.Unlock()
	n := 0
	for _, got := range q.claims {
		if got == kind {
			n++
		}
	}
	return n
}

func (q *fakeQueue) claimCount() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.claims)
}

func (q *fakeQueue) CompleteClaimedJob(_ context.Context, job domain.Job) error {
	q.mu.Lock()
	q.completed = append(q.completed, job)
	q.mu.Unlock()
	if q.observe != nil {
		q.observe("complete")
	}
	return nil
}

func (q *fakeQueue) FailClaimedJob(_ context.Context, job domain.Job, cause error) error {
	q.mu.Lock()
	q.failures = append(q.failures, jobFailure{job, cause})
	q.mu.Unlock()
	if q.observe != nil {
		q.observe("fail")
	}
	return nil
}

func (q *fakeQueue) NextJobRunAfter(context.Context) (time.Time, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if !time.Now().Before(q.runAfter) {
		return time.Time{}, nil
	}
	return q.runAfter, nil
}

func (q *fakeQueue) completedIDs() []int64 {
	q.mu.Lock()
	defer q.mu.Unlock()
	ids := make([]int64, 0, len(q.completed))
	for _, job := range q.completed {
		ids = append(ids, job.ID)
	}
	slices.Sort(ids)
	return ids
}

// blockingJobs holds each job in its handler until the test releases it, so a test sees which grants are held at
// once.
type blockingJobs struct {
	mu      sync.Mutex
	running map[int64]chan struct{}
}

func newBlockingJobs() *blockingJobs {
	return &blockingJobs{running: map[int64]chan struct{}{}}
}

func (b *blockingJobs) handle(ctx context.Context, job domain.Job) error {
	release := make(chan struct{})
	b.mu.Lock()
	b.running[job.ID] = release
	b.mu.Unlock()
	select {
	case <-release:
	case <-ctx.Done():
	}
	b.mu.Lock()
	delete(b.running, job.ID)
	b.mu.Unlock()
	return nil
}

func (b *blockingJobs) ids() []int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	ids := make([]int64, 0, len(b.running))
	for id := range b.running {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

func (b *blockingJobs) release(id int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	close(b.running[id])
}

func (b *blockingJobs) handlers() map[domain.JobKind]Handler {
	handlers := map[domain.JobKind]Handler{}
	for _, kind := range domain.JobKinds {
		handlers[kind] = b.handle
	}
	return handlers
}

// Call only inside a synctest bubble. A scheduler that cannot stop makes the bubble fail instead of consuming a
// wall-clock timeout.
func startScheduler(opts Options) (*Scheduler, func()) {
	ctx, cancel := context.WithCancel(context.Background())
	scheduler := NewScheduler(opts)
	finished := make(chan struct{})
	go func() {
		scheduler.Run(ctx)
		close(finished)
	}()
	return scheduler, func() { cancel(); <-finished }
}

func TestSchedulerGrantsCPUOnlyJobWhileReadSlotsAreFull(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		queue := newFakeQueue(
			domain.Job{ID: 1, Kind: domain.JobProbe},
			domain.Job{ID: 2, Kind: domain.JobProbe},
			domain.Job{ID: 3, Kind: domain.JobFingerprint},
		)
		jobs := newBlockingJobs()
		_, stop := startScheduler(Options{
			Queue: queue, Handlers: jobs.handlers(), Capacity: domain.JobResources{Read: 1, CPU: 1},
		})
		defer stop()
		synctest.Wait()
		if got, want := jobs.ids(), []int64{1, 3}; !slices.Equal(got, want) {
			t.Fatalf("running = %v, want %v", got, want)
		}
	})
}

func TestSchedulerSkipsJobThatDoesNotFit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// The thumbnail ranks before the fingerprint but needs the read slot the probe holds.
		queue := newFakeQueue(
			domain.Job{ID: 1, Kind: domain.JobProbe},
			domain.Job{ID: 2, Kind: domain.JobThumbnail},
			domain.Job{ID: 3, Kind: domain.JobFingerprint},
		)
		jobs := newBlockingJobs()
		_, stop := startScheduler(Options{
			Queue: queue, Handlers: jobs.handlers(), Capacity: domain.JobResources{Read: 1, CPU: 2},
		})
		defer stop()
		synctest.Wait()
		if got, want := jobs.ids(), []int64{1, 3}; !slices.Equal(got, want) {
			t.Fatalf("running = %v, want %v", got, want)
		}
		if n := queue.claimed(domain.JobThumbnail); n != 0 {
			t.Fatalf("thumbnail claims while its read slot is held = %d, want 0", n)
		}
		// The release alone wakes the scheduler for the skipped job.
		jobs.release(1)
		synctest.Wait()
		if got, want := jobs.ids(), []int64{2, 3}; !slices.Equal(got, want) {
			t.Fatalf("running after the probe = %v, want %v", got, want)
		}
	})
}

func TestSchedulerGivesFreedSlotToNextKind(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// A probe backlog must not keep the shared read slot from the queued thumbnail.
		queue := newFakeQueue(
			domain.Job{ID: 1, Kind: domain.JobProbe},
			domain.Job{ID: 2, Kind: domain.JobProbe},
			domain.Job{ID: 3, Kind: domain.JobThumbnail},
		)
		jobs := newBlockingJobs()
		_, stop := startScheduler(Options{
			Queue: queue, Handlers: jobs.handlers(), Capacity: domain.JobResources{Read: 1, CPU: 1},
		})
		defer stop()
		synctest.Wait()
		if got, want := jobs.ids(), []int64{1}; !slices.Equal(got, want) {
			t.Fatalf("running = %v, want %v", got, want)
		}
		jobs.release(1)
		synctest.Wait()
		if got, want := jobs.ids(), []int64{3}; !slices.Equal(got, want) {
			t.Fatalf("running after the probe = %v, want %v", got, want)
		}
	})
}

func TestSchedulerGrantsNoMoreThanCapacity(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		queue := newFakeQueue(
			domain.Job{ID: 1, Kind: domain.JobProbe},
			domain.Job{ID: 2, Kind: domain.JobProbe},
			domain.Job{ID: 3, Kind: domain.JobProbe},
		)
		jobs := newBlockingJobs()
		_, stop := startScheduler(Options{
			Queue: queue, Handlers: jobs.handlers(), Capacity: domain.JobResources{Read: 2, CPU: 2},
		})
		defer stop()
		synctest.Wait()
		if got, want := jobs.ids(), []int64{1, 2}; !slices.Equal(got, want) {
			t.Fatalf("running = %v, want %v", got, want)
		}
		if n := queue.claimed(domain.JobProbe); n != 2 {
			t.Fatalf("probe claims at capacity = %d, want 2", n)
		}
		jobs.release(1)
		synctest.Wait()
		if got, want := jobs.ids(), []int64{2, 3}; !slices.Equal(got, want) {
			t.Fatalf("running after a release = %v, want %v", got, want)
		}
		if got, want := queue.completedIDs(), []int64{1}; !slices.Equal(got, want) {
			t.Fatalf("completed = %v, want %v", got, want)
		}
	})
}

func TestSchedulerSleepsUntilWoken(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		queue := newFakeQueue()
		scheduler, stop := startScheduler(Options{
			Queue: queue, Capacity: domain.JobResources{Read: 1, CPU: 1},
			Handlers: map[domain.JobKind]Handler{domain.JobProbe: func(context.Context, domain.Job) error { return nil }},
		})
		defer stop()
		synctest.Wait()
		claims := queue.claimCount()
		job := domain.Job{ID: 7, Kind: domain.JobProbe}
		queue.add(job)
		// Even the retry interval must not poll an empty queue. Time is virtual.
		time.Sleep(time.Duration(domain.WorkerRetry))
		synctest.Wait()
		if got := queue.claimCount(); got != claims || len(queue.completedIDs()) != 0 {
			t.Fatalf("before Wake: claims=%d, want %d; completed=%v", got, claims, queue.completedIDs())
		}
		scheduler.Wake()
		synctest.Wait()
		if got, want := queue.completedIDs(), []int64{job.ID}; !slices.Equal(got, want) {
			t.Fatalf("after Wake: completed=%v, want %v", got, want)
		}
	})
}

func TestSchedulerWaitsUntilTheNextRunAfterTime(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		queue := newFakeQueue(domain.Job{ID: 7, Kind: domain.JobProbe})
		queue.runAfter = start.Add(30 * time.Second)
		var startedAt time.Time
		_, stop := startScheduler(Options{
			Queue: queue, Capacity: domain.JobResources{Read: 1, CPU: 1},
			Handlers: map[domain.JobKind]Handler{domain.JobProbe: func(context.Context, domain.Job) error { return nil }},
			Started:  func(domain.Job) { startedAt = time.Now() },
		})
		defer stop()
		synctest.Wait()
		if len(queue.completedIDs()) != 0 {
			t.Fatalf("completed before the run-after time = %v", queue.completedIDs())
		}
		// No wakeup arrives; time is virtual.
		time.Sleep(time.Minute)
		synctest.Wait()
		if got, want := startedAt.Sub(start), 30*time.Second; got != want || len(queue.completedIDs()) != 1 {
			t.Fatalf("started after %v with completed %v, want after %v", got, queue.completedIDs(), want)
		}
	})
}

func TestSchedulerRetriesClaimFailure(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		job := domain.Job{ID: 7, Kind: domain.JobProbe}
		queue := newFakeQueue(job)
		queue.claimErr = errors.New("queue unavailable")
		_, stop := startScheduler(Options{
			Queue: queue, Capacity: domain.JobResources{Read: 1, CPU: 1},
			Handlers: map[domain.JobKind]Handler{domain.JobProbe: func(context.Context, domain.Job) error { return nil }},
		})
		defer stop()
		synctest.Wait()
		time.Sleep(time.Duration(domain.WorkerRetry) - time.Nanosecond)
		synctest.Wait()
		if claims := queue.claimCount(); claims != 1 {
			t.Fatalf("claims before retry = %d, want 1", claims)
		}
		time.Sleep(time.Nanosecond)
		synctest.Wait()
		if got, want := queue.completedIDs(), []int64{job.ID}; !slices.Equal(got, want) {
			t.Fatalf("after retry: completed=%v, want %v", got, want)
		}
	})
}

func TestSchedulerStopsWithoutWaitingForRetry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		queue := newFakeQueue()
		queue.claimErr = errors.New("queue unavailable")
		_, stop := startScheduler(Options{Queue: queue, Capacity: domain.JobResources{Read: 1, CPU: 1}})
		synctest.Wait()
		before := time.Now()
		stop()
		if !time.Now().Equal(before) {
			t.Fatal("cancellation waited for the retry timer")
		}
	})
}

// Shutdown closes the database after Run returns, so Run waits for running jobs; a cancelled job records no outcome
// and is left for startup recovery.
func TestSchedulerStopWaitsForRunningJobs(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		queue := newFakeQueue(domain.Job{ID: 1, Kind: domain.JobProbe})
		jobs := newBlockingJobs()
		_, stop := startScheduler(Options{
			Queue: queue, Handlers: jobs.handlers(), Capacity: domain.JobResources{Read: 1, CPU: 1},
		})
		synctest.Wait()
		if got := jobs.ids(); !slices.Equal(got, []int64{1}) {
			t.Fatalf("running = %v", got)
		}
		stop()
		if got := jobs.ids(); len(got) != 0 {
			t.Fatalf("running after stop = %v", got)
		}
		if len(queue.completedIDs()) != 0 || len(queue.failures) != 0 {
			t.Fatalf("outcomes after cancel: completed=%v failures=%v", queue.completedIDs(), queue.failures)
		}
	})
}

func TestSchedulerProcessResult(t *testing.T) {
	cause := errors.New("handler failed")
	for _, tc := range []struct {
		name       string
		handlerErr error
		cancel     bool
		noHandler  bool
		calls      []string
	}{
		{name: "success", calls: []string{"started", "handle", "complete", "finished"}},
		{name: "failure", handlerErr: cause, calls: []string{"started", "handle", "fail", "finished"}},
		{name: "missing handler", noHandler: true, calls: []string{"started", "fail", "finished"}},
		{name: "cancel success", cancel: true, calls: []string{"started", "handle"}},
		{name: "cancel failure", cancel: true, handlerErr: cause, calls: []string{"started", "handle"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			job := domain.Job{ID: 7, Kind: domain.JobProbe, VideoID: 17, Attempts: 2}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var calls []string
			record := func(call string) { calls = append(calls, call) }
			recordJob := func(call string, got domain.Job) {
				if !reflect.DeepEqual(got, job) {
					t.Fatalf("%s job = %+v, want %+v", call, got, job)
				}
				record(call)
			}
			queue := newFakeQueue()
			queue.observe = record
			opts := Options{Queue: queue,
				Started:  func(got domain.Job) { recordJob("started", got) },
				Finished: func(got domain.Job) { recordJob("finished", got) },
			}
			if !tc.noHandler {
				opts.Handlers = map[domain.JobKind]Handler{job.Kind: func(gotCtx context.Context, got domain.Job) error {
					if gotCtx != ctx {
						t.Fatal("handler context was replaced")
					}
					recordJob("handle", got)
					if tc.cancel {
						cancel()
					}
					return tc.handlerErr
				}}
			}
			NewScheduler(opts).process(ctx, job)
			if !slices.Equal(calls, tc.calls) {
				t.Fatalf("calls = %v, want %v", calls, tc.calls)
			}
			for _, got := range queue.completed {
				if !reflect.DeepEqual(got, job) {
					t.Fatalf("completed = %+v", got)
				}
			}
			for _, got := range queue.failures {
				if !reflect.DeepEqual(got.job, job) {
					t.Fatalf("failed = %+v", got.job)
				}
				if tc.noHandler {
					if got.cause.Error() != "unknown job kind: probe" {
						t.Fatalf("failure = %v", got.cause)
					}
				} else if !errors.Is(got.cause, cause) {
					t.Fatalf("failure = %v, want %v", got.cause, cause)
				}
			}
		})
	}
}
