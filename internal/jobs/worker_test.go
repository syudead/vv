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

type claimResult struct {
	job domain.Job
	err error
}

type jobFailure struct {
	job   domain.Job
	cause error
}

// The queue supplies scripted results, without reproducing the store's retry
// policy. Concurrent tests access its records only after synctest.Wait.
type fakeQueue struct {
	claimsMu  sync.Mutex
	next      []claimResult
	claims    []domain.JobKind
	completed []domain.Job
	failures  []jobFailure
	observe   func(string)
}

func (q *fakeQueue) ClaimJob(_ context.Context, kind domain.JobKind) (domain.Job, error) {
	q.claimsMu.Lock()
	q.claims = append(q.claims, kind)
	q.claimsMu.Unlock()
	if len(q.next) == 0 {
		return domain.Job{}, domain.ErrNoJob
	}
	result := q.next[0]
	q.next = q.next[1:]
	return result.job, result.err
}

// Timers can resume claims after a virtual-time assertion, so synchronize
// reads made before advancing time as well as waiting for earlier writes.
func (q *fakeQueue) claimCount() int {
	q.claimsMu.Lock()
	defer q.claimsMu.Unlock()
	return len(q.claims)
}

func (q *fakeQueue) CompleteClaimedJob(_ context.Context, job domain.Job) error {
	q.completed = append(q.completed, job)
	if q.observe != nil {
		q.observe("complete")
	}
	return nil
}

func (q *fakeQueue) FailClaimedJob(_ context.Context, job domain.Job, cause error) error {
	q.failures = append(q.failures, jobFailure{job, cause})
	if q.observe != nil {
		q.observe("fail")
	}
	return nil
}

// Call only inside a synctest bubble. A worker that cannot stop makes the
// bubble fail instead of consuming a wall-clock timeout.
func startWorker(opts Options) (*Worker, func()) {
	ctx, cancel := context.WithCancel(context.Background())
	worker := New(opts)
	finished := make(chan struct{})
	go func() {
		worker.Run(ctx)
		close(finished)
	}()
	return worker, func() { cancel(); <-finished }
}

func TestWorkerProcessesJobsSerially(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		jobs := []domain.Job{
			{ID: 1, Kind: domain.JobThumbnail, VideoID: 7, Attempts: 1},
			{ID: 2, Kind: domain.JobThumbnail, VideoID: 8, Attempts: 2},
		}
		queue := &fakeQueue{next: []claimResult{{job: jobs[0]}, {job: jobs[1]}}}
		started := make(chan domain.Job, 2)
		release := make(chan struct{})
		_, stop := startWorker(Options{
			Kind: domain.JobThumbnail, Queue: queue,
			Handler: func(ctx context.Context, job domain.Job) error {
				started <- job
				select {
				case <-release:
				case <-ctx.Done():
				}
				return nil
			},
		})
		defer stop()
		for i, want := range jobs {
			if got := <-started; !reflect.DeepEqual(got, want) {
				t.Fatalf("handled job = %+v, want %+v", got, want)
			}
			synctest.Wait()
			if len(queue.claims) != i+1 || len(queue.completed) != i {
				t.Fatalf("while job %d is blocked: claims=%v completed=%v", want.ID, queue.claims, queue.completed)
			}
			release <- struct{}{}
		}
		synctest.Wait()
		if !reflect.DeepEqual(queue.completed, jobs) {
			t.Fatalf("completed = %v, want %v", queue.completed, jobs)
		}
		if want := []domain.JobKind{domain.JobThumbnail, domain.JobThumbnail, domain.JobThumbnail}; !slices.Equal(queue.claims, want) {
			t.Fatalf("claim kinds = %v, want %v", queue.claims, want)
		}
	})
}

func TestWorkerSleepsUntilWoken(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		queue := &fakeQueue{}
		worker, stop := startWorker(Options{Kind: domain.JobProbe, Queue: queue, Handler: func(context.Context, domain.Job) error { return nil }})
		defer stop()
		synctest.Wait()
		job := domain.Job{ID: 7, Kind: domain.JobProbe}
		queue.next = []claimResult{{job: job}}
		// Even the retry interval must not poll an empty queue. Time is virtual.
		time.Sleep(time.Duration(domain.WorkerRetry))
		synctest.Wait()
		if len(queue.claims) != 1 || len(queue.completed) != 0 {
			t.Fatalf("before Wake: claims=%v completed=%v", queue.claims, queue.completed)
		}
		worker.Wake()
		synctest.Wait()
		if !reflect.DeepEqual(queue.completed, []domain.Job{job}) || len(queue.claims) != 3 {
			t.Fatalf("after Wake: claims=%v completed=%v", queue.claims, queue.completed)
		}
	})
}

func TestWorkerKeepsWakeReceivedWhileBusy(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		first := domain.Job{ID: 1, Kind: domain.JobProbe}
		second := domain.Job{ID: 2, Kind: domain.JobProbe}
		// The empty claim makes progress depend on the retained notification.
		queue := &fakeQueue{next: []claimResult{{job: first}, {err: domain.ErrNoJob}, {job: second}}}
		release := make(chan struct{})
		worker, stop := startWorker(Options{
			Kind: domain.JobProbe, Queue: queue,
			Handler: func(ctx context.Context, job domain.Job) error {
				if job.ID == first.ID {
					select {
					case <-release:
					case <-ctx.Done():
					}
				}
				return nil
			},
		})
		defer stop()
		synctest.Wait()
		if len(queue.claims) != 1 {
			t.Fatalf("claims before Wake = %v", queue.claims)
		}
		worker.Wake()
		worker.Wake() // Multiple notifications coalesce without blocking the caller.
		close(release)
		synctest.Wait()
		if !reflect.DeepEqual(queue.completed, []domain.Job{first, second}) || len(queue.claims) != 4 {
			t.Fatalf("claims=%v completed=%v", queue.claims, queue.completed)
		}
	})
}

func TestWorkerRetriesClaimFailure(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		job := domain.Job{ID: 7, Kind: domain.JobProbe}
		queue := &fakeQueue{next: []claimResult{{err: errors.New("queue unavailable")}, {job: job}}}
		_, stop := startWorker(Options{Kind: domain.JobProbe, Queue: queue, Handler: func(context.Context, domain.Job) error { return nil }})
		defer stop()
		synctest.Wait()
		time.Sleep(time.Duration(domain.WorkerRetry) - time.Nanosecond)
		synctest.Wait()
		if claims := queue.claimCount(); claims != 1 {
			t.Fatalf("claims before retry = %d", claims)
		}
		time.Sleep(time.Nanosecond)
		synctest.Wait()
		if !reflect.DeepEqual(queue.completed, []domain.Job{job}) || len(queue.claims) != 3 {
			t.Fatalf("after retry: claims=%v completed=%v", queue.claims, queue.completed)
		}
	})
}

func TestWorkerContinuesAfterHandlerFailure(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		first := domain.Job{ID: 1, Kind: domain.JobProbe}
		second := domain.Job{ID: 2, Kind: domain.JobProbe}
		cause := errors.New("handler failed")
		queue := &fakeQueue{next: []claimResult{{job: first}, {job: second}}}
		_, stop := startWorker(Options{Kind: domain.JobProbe, Queue: queue, Handler: func(_ context.Context, job domain.Job) error {
			if job.ID == first.ID {
				return cause
			}
			return nil
		}})
		defer stop()
		synctest.Wait()
		if len(queue.failures) != 1 || !reflect.DeepEqual(queue.failures[0].job, first) || !errors.Is(queue.failures[0].cause, cause) {
			t.Fatalf("failures = %+v", queue.failures)
		}
		if !reflect.DeepEqual(queue.completed, []domain.Job{second}) {
			t.Fatalf("completed = %+v", queue.completed)
		}
	})
}

func TestWorkerStopsOnCancel(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"idle", domain.ErrNoJob}, {"retry", errors.New("queue unavailable")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				queue := &fakeQueue{next: []claimResult{{err: tc.err}}}
				_, stop := startWorker(Options{Kind: domain.JobProbe, Queue: queue})
				defer stop()
				synctest.Wait()
				if len(queue.claims) != 1 {
					t.Fatalf("claims = %v", queue.claims)
				}
				before := time.Now()
				stop()
				if !time.Now().Equal(before) {
					t.Fatal("cancellation waited for the retry timer")
				}
			})
		})
	}
}

func TestWorkerProcessResult(t *testing.T) {
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
			queue := &fakeQueue{observe: record}
			opts := Options{Kind: job.Kind, Queue: queue,
				Started:  func(got domain.Job) { recordJob("started", got) },
				Finished: func(got domain.Job) { recordJob("finished", got) },
			}
			if !tc.noHandler {
				opts.Handler = func(gotCtx context.Context, got domain.Job) error {
					if gotCtx != ctx {
						t.Fatal("handler context was replaced")
					}
					recordJob("handle", got)
					if tc.cancel {
						cancel()
					}
					return tc.handlerErr
				}
			}
			New(opts).process(ctx, job)
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
