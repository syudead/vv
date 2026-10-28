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

// The test folders: rootA on disk "a" and rootB on disk "b", unless a test resolves them otherwise.
const (
	rootA int64 = 1
	rootB int64 = 2
)

// The queue hands out scripted jobs per kind in order, honouring the claim's class and folders, without reproducing
// the store's claim conditions or retry policy. A snoozed job goes back to the queue.
type fakeQueue struct {
	mu       sync.Mutex
	queued   map[domain.JobKind][]domain.Job
	claimErr error
	claims   []domain.JobKind
	// claimRecords holds every claim the scheduler made, in order.
	claimRecords []domain.JobClaim
	completed    []domain.Job
	failures     []jobFailure
	observe      func(string)
	// runAfter holds a kind's jobs back until that time, as a retry delay does.
	runAfter time.Time
	// claimDelay passes after each claim answers, so time moves on between a turn's claims and its lookup.
	claimDelay time.Duration
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
		if job.RootID == 0 {
			job.RootID = rootA
		}
		q.queued[job.Kind] = append(q.queued[job.Kind], job)
	}
}

func (q *fakeQueue) ClaimJobOn(_ context.Context, claim domain.JobClaim) (domain.Job, error) {
	q.mu.Lock()
	// Deferred first, so the delay runs after the unlock.
	defer time.Sleep(q.claimDelay)
	defer q.mu.Unlock()
	q.claims = append(q.claims, claim.Kind)
	q.claimRecords = append(q.claimRecords, claim)
	if err := q.claimErr; err != nil {
		q.claimErr = nil
		return domain.Job{}, err
	}
	if time.Now().Before(q.runAfter) {
		return domain.Job{}, domain.ErrNoJob
	}
	queued := q.queued[claim.Kind]
	for i, job := range queued {
		if job.Priority > claim.MaxPriority || slices.Contains(claim.Unavailable, job.RootID) ||
			(claim.Roots != nil && !slices.Contains(claim.Roots, job.RootID)) {
			continue
		}
		q.queued[claim.Kind] = slices.Delete(slices.Clone(queued), i, i+1)
		return job, nil
	}
	return domain.Job{}, domain.ErrNoJob
}

func (q *fakeQueue) DueJobKinds(context.Context) (map[domain.JobKind]domain.JobPriority, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	due := map[domain.JobKind]domain.JobPriority{}
	if time.Now().Before(q.runAfter) {
		return due, nil
	}
	for kind, queued := range q.queued {
		for _, job := range queued {
			if class, ok := due[kind]; !ok || job.Priority < class {
				due[kind] = job.Priority
			}
		}
	}
	return due, nil
}

// lastUnavailable reports the unavailable folders the last claim passed over.
func (q *fakeQueue) lastUnavailable() []int64 {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.claimRecords) == 0 {
		return nil
	}
	return q.claimRecords[len(q.claimRecords)-1].Unavailable
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
	if errors.Is(cause, domain.ErrJobSnoozed) {
		q.add(job)
	}
	if q.observe != nil {
		q.observe("fail")
	}
	return nil
}

func (q *fakeQueue) NextJobRunAfter(_ context.Context, since time.Time) (time.Time, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.runAfter.After(since) {
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
	for _, kind := range domain.ScheduledJobKinds {
		handlers[kind] = b.handle
	}
	return handlers
}

// fakeDisks resolves each test folder path to its disk, or fails for a folder the test made unreadable.
type fakeDisks struct {
	mu         sync.Mutex
	disks      map[string]domain.Disk
	unreadable map[string]bool
}

func newFakeDisks(disks map[string]domain.Disk) *fakeDisks {
	return &fakeDisks{disks: disks, unreadable: map[string]bool{}}
}

func (d *fakeDisks) resolve(path string) (domain.Disk, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.unreadable[path] {
		return domain.Disk{}, errors.New("the folder cannot be read")
	}
	return d.disks[path], nil
}

func (d *fakeDisks) setReadable(path string, readable bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.unreadable[path] = !readable
}

type fakeFolders []domain.MediaFolder

func (f fakeFolders) ListMediaFolders(context.Context) ([]domain.MediaFolder, error) {
	return f, nil
}

// twoFolders places rootA at /a and rootB at /b.
var twoFolders = fakeFolders{{ID: rootA, Path: "/a"}, {ID: rootB, Path: "/b"}}

// oneDisk puts every test folder on one disk of the given type.
func oneDisk(diskType domain.DiskType) *fakeDisks {
	disk := domain.Disk{Key: "a", Type: diskType}
	return newFakeDisks(map[string]domain.Disk{"/a": disk, "/b": disk})
}

// twoDisks puts rootA and rootB on two rotational disks.
func twoDisks() *fakeDisks {
	return newFakeDisks(map[string]domain.Disk{
		"/a": {Key: "a", Type: domain.DiskRotational}, "/b": {Key: "b", Type: domain.DiskRotational},
	})
}

// Call only inside a synctest bubble. A scheduler that cannot stop makes the bubble fail instead of consuming a
// wall-clock timeout. Without folders, the test folders sit on one rotational disk.
func startScheduler(opts Options) (*Scheduler, func()) {
	if opts.Folders == nil {
		opts.Folders = twoFolders
	}
	if opts.Disks == nil {
		opts.Disks = oneDisk(domain.DiskRotational).resolve
	}
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
			Queue: queue, Handlers: jobs.handlers(), CPUSlots: 1,
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
			Queue: queue, Handlers: jobs.handlers(), CPUSlots: 2,
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
			Queue: queue, Handlers: jobs.handlers(), CPUSlots: 1,
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
		// A solid-state disk has 4 read slots.
		queue := newFakeQueue(
			domain.Job{ID: 1, Kind: domain.JobProbe},
			domain.Job{ID: 2, Kind: domain.JobProbe},
			domain.Job{ID: 3, Kind: domain.JobProbe},
			domain.Job{ID: 4, Kind: domain.JobProbe},
			domain.Job{ID: 5, Kind: domain.JobProbe},
		)
		jobs := newBlockingJobs()
		_, stop := startScheduler(Options{
			Queue: queue, Handlers: jobs.handlers(), CPUSlots: 2, Disks: oneDisk(domain.DiskSolidState).resolve,
		})
		defer stop()
		synctest.Wait()
		if got, want := jobs.ids(), []int64{1, 2, 3, 4}; !slices.Equal(got, want) {
			t.Fatalf("running = %v, want %v", got, want)
		}
		if n := queue.claimed(domain.JobProbe); n != 4 {
			t.Fatalf("probe claims at capacity = %d, want 4", n)
		}
		jobs.release(1)
		synctest.Wait()
		if got, want := jobs.ids(), []int64{2, 3, 4, 5}; !slices.Equal(got, want) {
			t.Fatalf("running after a release = %v, want %v", got, want)
		}
		if got, want := queue.completedIDs(), []int64{1}; !slices.Equal(got, want) {
			t.Fatalf("completed = %v, want %v", got, want)
		}
	})
}

// Auto-tagging reads no source file, so it runs while every read slot is held, but the classifier takes one job at
// a time.
func TestSchedulerGrantsAutoTagOnTheClassifierSlotOnly(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		queue := newFakeQueue(
			domain.Job{ID: 1, Kind: domain.JobProbe},
			domain.Job{ID: 2, Kind: domain.JobAutoTag},
			domain.Job{ID: 3, Kind: domain.JobAutoTag},
		)
		jobs := newBlockingJobs()
		_, stop := startScheduler(Options{
			Queue: queue, Handlers: jobs.handlers(), CPUSlots: 1,
		})
		defer stop()
		synctest.Wait()
		if got, want := jobs.ids(), []int64{1, 2}; !slices.Equal(got, want) {
			t.Fatalf("running = %v, want %v", got, want)
		}
		jobs.release(2)
		synctest.Wait()
		if got, want := jobs.ids(), []int64{1, 3}; !slices.Equal(got, want) {
			t.Fatalf("running after the first classification = %v, want %v", got, want)
		}
	})
}

// The face kinds share one lane, so an analysis, an embedding and the cast rule run one at a time while other slots
// are free.
func TestSchedulerGrantsTheFaceKindsOneAtATime(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		queue := newFakeQueue(
			domain.Job{ID: 1, Kind: domain.JobFace},
			domain.Job{ID: 2, Kind: domain.JobFaceImage},
			domain.Job{ID: 3, Kind: domain.JobFaceCast},
		)
		jobs := newBlockingJobs()
		_, stop := startScheduler(Options{
			Queue: queue, Handlers: jobs.handlers(), CPUSlots: 4, Disks: oneDisk(domain.DiskSolidState).resolve,
		})
		defer stop()
		for range 3 {
			synctest.Wait()
			running := jobs.ids()
			if len(running) != 1 {
				t.Fatalf("running = %v, want one face-lane job", running)
			}
			jobs.release(running[0])
		}
		synctest.Wait()
		if got := queue.completedIDs(); len(got) != 3 {
			t.Fatalf("completed = %v, want every face-lane job in turn", got)
		}
	})
}

// A paused kind is not claimed, so its job keeps its attempts; resuming it and waking the scheduler claims the job.
func TestSchedulerLeavesPausedKindsQueuedUntilWoken(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		queue := newFakeQueue(domain.Job{ID: 1, Kind: domain.JobAutoTag})
		var mu sync.Mutex
		paused := true
		jobs := newBlockingJobs()
		scheduler, stop := startScheduler(Options{
			Queue: queue, Handlers: jobs.handlers(), CPUSlots: 1,
			Paused: func(context.Context) (map[domain.JobKind]bool, error) {
				mu.Lock()
				defer mu.Unlock()
				return map[domain.JobKind]bool{domain.JobAutoTag: paused}, nil
			},
		})
		defer stop()
		synctest.Wait()
		if n := queue.claimed(domain.JobAutoTag); n != 0 {
			t.Fatalf("claims of a paused kind = %d, want 0", n)
		}
		mu.Lock()
		paused = false
		mu.Unlock()
		scheduler.Wake()
		synctest.Wait()
		if got, want := jobs.ids(), []int64{1}; !slices.Equal(got, want) {
			t.Fatalf("running after resuming = %v, want %v", got, want)
		}
	})
}

func TestSchedulerSleepsUntilWoken(t *testing.T) {

	synctest.Test(t, func(t *testing.T) {
		queue := newFakeQueue()
		scheduler, stop := startScheduler(Options{
			Queue: queue, CPUSlots: 1,
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
			Queue: queue, CPUSlots: 1,
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

func TestSchedulerRunsAJobThatCameDueDuringItsClaims(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		queue := newFakeQueue(domain.Job{ID: 7, Kind: domain.JobProbe})
		// The probe claim misses the job by a second, and it is due before the turn's lookup.
		queue.runAfter = time.Now().Add(time.Second)
		queue.claimDelay = 2 * time.Second
		_, stop := startScheduler(Options{
			Queue: queue, CPUSlots: 1,
			Handlers: map[domain.JobKind]Handler{domain.JobProbe: func(context.Context, domain.Job) error { return nil }},
		})
		defer stop()
		// No wakeup arrives; time is virtual.
		time.Sleep(time.Minute)
		synctest.Wait()
		if got, want := queue.completedIDs(), []int64{7}; !slices.Equal(got, want) {
			t.Fatalf("completed = %v, want %v", got, want)
		}
	})
}

func TestSchedulerRetriesClaimFailure(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		job := domain.Job{ID: 7, Kind: domain.JobProbe}
		queue := newFakeQueue(job)
		queue.claimErr = errors.New("queue unavailable")
		_, stop := startScheduler(Options{
			Queue: queue, CPUSlots: 1,
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
		_, stop := startScheduler(Options{Queue: queue, CPUSlots: 1})
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
			Queue: queue, Handlers: jobs.handlers(), CPUSlots: 1,
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

func (q *fakeQueue) failureCauses() []error {
	q.mu.Lock()
	defer q.mu.Unlock()
	causes := make([]error, 0, len(q.failures))
	for _, failure := range q.failures {
		causes = append(causes, failure.cause)
	}
	return causes
}

// A scan read waits for a running job instead of stopping it, then takes the freed slot ahead of a queued owner job,
// while jobs keep the disk's other slots.
func TestSchedulerGrantsAWaitingScanReadBeforeQueuedJobs(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		queue := newFakeQueue()
		for id := int64(1); id <= 5; id++ {
			queue.add(domain.Job{ID: id, Kind: domain.JobProbe, Priority: domain.JobPriorityOwner})
		}
		jobs := newBlockingJobs()
		scheduler, stop := startScheduler(Options{
			Queue: queue, Handlers: jobs.handlers(), CPUSlots: 1, Disks: oneDisk(domain.DiskSolidState).resolve,
		})
		defer stop()
		synctest.Wait()
		if got, want := jobs.ids(), []int64{1, 2, 3, 4}; !slices.Equal(got, want) {
			t.Fatalf("running = %v, want %v", got, want)
		}

		var release func()
		go func() {
			var err error
			if release, err = scheduler.AcquireRead(context.Background(), "/a"); err != nil {
				t.Errorf("AcquireRead: %v", err)
			}
		}()
		synctest.Wait()
		if release != nil {
			t.Fatal("the scan read was granted while every slot was held")
		}
		jobs.release(1)
		synctest.Wait()
		if release == nil {
			t.Fatal("the scan read was not granted the freed slot")
		}
		if got, want := jobs.ids(), []int64{2, 3, 4}; !slices.Equal(got, want) {
			t.Fatalf("running during the scan read = %v, want %v", got, want)
		}
		release()
		synctest.Wait()
		if got, want := jobs.ids(), []int64{2, 3, 4, 5}; !slices.Equal(got, want) {
			t.Fatalf("running after the scan read = %v, want %v", got, want)
		}
	})
}

// Between two reads of one scan, a slot the scan returned stays free from jobs until the scan is done with the disk,
// while jobs keep the disk's other slots.
func TestSchedulerKeepsASlotForAScanBetweenItsReads(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		queue := newFakeQueue()
		for id := int64(1); id <= 5; id++ {
			queue.add(domain.Job{ID: id, Kind: domain.JobProbe, Priority: domain.JobPriorityOwner})
		}
		jobs := newBlockingJobs()
		scheduler, stop := startScheduler(Options{
			Queue: queue, Handlers: jobs.handlers(), CPUSlots: 1, Disks: oneDisk(domain.DiskSolidState).resolve,
		})
		defer stop()
		synctest.Wait()

		done := scheduler.KeepAhead("/a")
		jobs.release(1)
		synctest.Wait()
		if got, want := jobs.ids(), []int64{2, 3, 4}; !slices.Equal(got, want) {
			t.Fatalf("running while the scan is ahead = %v, want %v", got, want)
		}
		release, err := scheduler.AcquireRead(context.Background(), "/a")
		if err != nil {
			t.Fatalf("AcquireRead: %v", err)
		}
		release()
		synctest.Wait()
		if got, want := jobs.ids(), []int64{2, 3, 4}; !slices.Equal(got, want) {
			t.Fatalf("running between the scan's reads = %v, want %v", got, want)
		}
		done()
		synctest.Wait()
		if got, want := jobs.ids(), []int64{2, 3, 4, 5}; !slices.Equal(got, want) {
			t.Fatalf("running after the scan = %v, want %v", got, want)
		}
	})
}

// A folder whose disk cannot be resolved refuses the scan read instead of letting it read outside every limit.
func TestSchedulerRefusesAScanReadOnAnUnreadableFolder(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		disks := oneDisk(domain.DiskSolidState)
		disks.setReadable("/b", false)
		scheduler, stop := startScheduler(Options{Queue: newFakeQueue(), Disks: disks.resolve})
		defer stop()
		synctest.Wait()

		release, err := scheduler.AcquireRead(context.Background(), "/b")
		if err == nil || release != nil {
			t.Fatalf("AcquireRead = (release %v, %v), want an error and no release", release != nil, err)
		}
	})
}

func TestSchedulerGrantsTwoDisksAtOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		queue := newFakeQueue(
			domain.Job{ID: 1, Kind: domain.JobProbe, RootID: rootA},
			domain.Job{ID: 2, Kind: domain.JobProbe, RootID: rootA},
			domain.Job{ID: 3, Kind: domain.JobProbe, RootID: rootB},
		)
		jobs := newBlockingJobs()
		_, stop := startScheduler(Options{Queue: queue, Handlers: jobs.handlers(), CPUSlots: 1, Disks: twoDisks().resolve})
		defer stop()
		synctest.Wait()
		// Disk a has one slot, so its second probe waits while disk b reads.
		if got, want := jobs.ids(), []int64{1, 3}; !slices.Equal(got, want) {
			t.Fatalf("running = %v, want %v", got, want)
		}
		jobs.release(1)
		synctest.Wait()
		if got, want := jobs.ids(), []int64{2, 3}; !slices.Equal(got, want) {
			t.Fatalf("running after disk a's first probe = %v, want %v", got, want)
		}
	})
}

func TestSchedulerAlternatesDisksWhileBothHaveWork(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// One CPU slot runs one thumbnail at a time, so the start order shows the turns.
		var queued []domain.Job
		for id := int64(1); id <= 4; id++ {
			queued = append(queued, domain.Job{ID: id, Kind: domain.JobThumbnail, RootID: rootA})
		}
		for id := int64(5); id <= 6; id++ {
			queued = append(queued, domain.Job{ID: id, Kind: domain.JobThumbnail, RootID: rootB})
		}
		queue := newFakeQueue(queued...)
		jobs := newBlockingJobs()
		_, stop := startScheduler(Options{Queue: queue, Handlers: jobs.handlers(), CPUSlots: 1, Disks: twoDisks().resolve})
		defer stop()
		var order []int64
		for range queued {
			synctest.Wait()
			running := jobs.ids()
			if len(running) != 1 {
				t.Fatalf("running = %v, want one thumbnail", running)
			}
			order = append(order, running[0])
			jobs.release(running[0])
		}
		if want := []int64{1, 5, 2, 6, 3, 4}; !slices.Equal(order, want) {
			t.Fatalf("start order = %v, want %v", order, want)
		}
	})
}

func TestSchedulerLendsExtractionSlotsUpToTheDiskSlots(t *testing.T) {
	for _, tc := range []struct {
		disk domain.DiskType
		lent int
	}{{domain.DiskRotational, 0}, {domain.DiskSolidState, 3}} {
		t.Run(tc.disk.String(), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				queue := newFakeQueue(domain.Job{ID: 1, Kind: domain.JobSeekThumbnail})
				var lent []int
				// The job waits until the scheduler is idle, because a turn's empty claim holds a pair for a moment,
				// then asks for one more read and CPU pair per extraction until the scheduler refuses.
				proceed := make(chan struct{})
				extract := func(ctx context.Context, _ domain.Job) error {
					<-proceed
					slots := domain.JobSlotsFrom(ctx)
					pair := domain.JobResources{Read: 1, CPU: 1}
					n := 0
					for n < domain.MaxDiskReadSlots && slots.TryAcquire(pair) {
						n++
					}
					for range n {
						slots.Release(pair)
					}
					lent = append(lent, n)
					return nil
				}
				scheduler, stop := startScheduler(Options{
					Queue: queue, CPUSlots: 8, Disks: oneDisk(tc.disk).resolve,
					Handlers: map[domain.JobKind]Handler{domain.JobSeekThumbnail: extract},
				})
				defer stop()
				synctest.Wait()
				proceed <- struct{}{}
				synctest.Wait()
				// A second job gets the same pairs, so the first returned them.
				queue.add(domain.Job{ID: 2, Kind: domain.JobSeekThumbnail})
				scheduler.Wake()
				synctest.Wait()
				proceed <- struct{}{}
				synctest.Wait()
				if want := []int{tc.lent, tc.lent}; !slices.Equal(lent, want) {
					t.Fatalf("lent pairs = %v, want %v", lent, want)
				}
			})
		})
	}
}

// extractionRounds runs a seek-sprite job that, on each round the test sends, returns its lent pairs and borrows as
// many as the scheduler lends, as one extraction batch does; closing rounds ends the job.
type extractionRounds struct {
	rounds chan struct{}
	mu     sync.Mutex
	lent   []int
}

func (e *extractionRounds) handle(ctx context.Context, _ domain.Job) error {
	slots := domain.JobSlotsFrom(ctx)
	pair := domain.JobResources{Read: 1, CPU: 1}
	held := 0
	for range e.rounds {
		for range held {
			slots.Release(pair)
		}
		held = 0
		for held < domain.MaxDiskReadSlots-1 && slots.TryAcquire(pair) {
			held++
		}
		e.mu.Lock()
		e.lent = append(e.lent, held)
		e.mu.Unlock()
	}
	for range held {
		slots.Release(pair)
	}
	return nil
}

func (e *extractionRounds) lastLent() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.lent[len(e.lent)-1]
}

// A foreground reader lets running grants finish, refuses new grants and further extractions until one read is left,
// never takes that one, and frees the disk again when it closes.
func TestSchedulerHoldsAForegroundDiskToOneBackgroundRead(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		queue := newFakeQueue(domain.Job{ID: 1, Kind: domain.JobPreview}, domain.Job{ID: 2, Kind: domain.JobSeekThumbnail})
		jobs := newBlockingJobs()
		sprite := &extractionRounds{rounds: make(chan struct{})}
		handlers := jobs.handlers()
		handlers[domain.JobSeekThumbnail] = sprite.handle
		scheduler, stop := startScheduler(Options{
			Queue: queue, Handlers: handlers, CPUSlots: 8, Disks: oneDisk(domain.DiskSolidState).resolve,
		})
		defer stop()
		synctest.Wait()
		sprite.rounds <- struct{}{}
		synctest.Wait()
		if got := sprite.lastLent(); got != 2 {
			t.Fatalf("pairs lent beside the preview = %d, want 2", got)
		}

		end := scheduler.ReadInForeground("/a/movie.mp4")
		var queued []domain.Job
		for id := int64(3); id <= 7; id++ {
			queued = append(queued, domain.Job{ID: id, Kind: domain.JobThumbnail})
		}
		queue.add(queued...)
		scheduler.Wake()
		synctest.Wait()
		if got, want := jobs.ids(), []int64{1}; !slices.Equal(got, want) {
			t.Fatalf("running with a foreground reader = %v, want the preview only", got)
		}
		sprite.rounds <- struct{}{}
		synctest.Wait()
		if got := sprite.lastLent(); got != 0 {
			t.Fatalf("pairs lent with a foreground reader = %d, want 0", got)
		}
		close(sprite.rounds)
		synctest.Wait()
		if got, want := jobs.ids(), []int64{1}; !slices.Equal(got, want) {
			t.Fatalf("running after the seek sprite = %v, want the preview kept", got)
		}
		jobs.release(1)
		synctest.Wait()
		if got, want := jobs.ids(), []int64{3}; !slices.Equal(got, want) {
			t.Fatalf("running after the preview = %v, want one thumbnail", got)
		}

		end()
		synctest.Wait()
		if got, want := jobs.ids(), []int64{3, 4, 5, 6}; !slices.Equal(got, want) {
			t.Fatalf("running after the reader closed = %v, want %v", got, want)
		}
	})
}

// A scan read is a background read, so with a foreground reader open it waits for the one slot a job holds, and the
// reader's end wakes it.
func TestSchedulerHoldsAScanReadToTheForegroundLimit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		queue := newFakeQueue(domain.Job{ID: 1, Kind: domain.JobProbe})
		jobs := newBlockingJobs()
		scheduler, stop := startScheduler(Options{
			Queue: queue, Handlers: jobs.handlers(), CPUSlots: 8, Disks: oneDisk(domain.DiskSolidState).resolve,
		})
		defer stop()
		synctest.Wait()
		end := scheduler.ReadInForeground("/a/movie.mp4")

		var release func()
		go func() {
			var err error
			if release, err = scheduler.AcquireRead(context.Background(), "/a"); err != nil {
				t.Errorf("AcquireRead: %v", err)
			}
		}()
		synctest.Wait()
		if release != nil {
			t.Fatal("the scan read was granted a second read with a foreground reader open")
		}
		end()
		synctest.Wait()
		if release == nil {
			t.Fatal("the scan read was not granted after the foreground reader closed")
		}
		release()
	})
}

func TestSchedulerIgnoresAForegroundReadOutsideTheFolders(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		queue := newFakeQueue()
		jobs := newBlockingJobs()
		scheduler, stop := startScheduler(Options{
			Queue: queue, Handlers: jobs.handlers(), CPUSlots: 8, Disks: oneDisk(domain.DiskSolidState).resolve,
		})
		defer stop()
		synctest.Wait()
		defer scheduler.ReadInForeground("/elsewhere/movie.mp4")()
		queue.add(domain.Job{ID: 1, Kind: domain.JobProbe}, domain.Job{ID: 2, Kind: domain.JobProbe})
		scheduler.Wake()
		synctest.Wait()
		if got, want := jobs.ids(), []int64{1, 2}; !slices.Equal(got, want) {
			t.Fatalf("running = %v, want %v", got, want)
		}
	})
}

func TestSchedulerParksAnUnreadableFolderUntilARecheck(t *testing.T) {
	for _, tc := range []struct {
		name    string
		recheck func(*Scheduler)
	}{
		{name: "every minute", recheck: func(*Scheduler) { time.Sleep(time.Second) }},
		{name: "on an event", recheck: func(s *Scheduler) { s.RecheckRoots() }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				disks := twoDisks()
				queue := newFakeQueue(domain.Job{ID: 1, Kind: domain.JobProbe, RootID: rootA})
				var mu sync.Mutex
				calls := 0
				// The first run finds its folder unplugged.
				probe := func(context.Context, domain.Job) error {
					mu.Lock()
					defer mu.Unlock()
					calls++
					if calls == 1 {
						disks.setReadable("/a", false)
						return errors.New("cannot open the file")
					}
					return nil
				}
				scheduler, stop := startScheduler(Options{
					Queue: queue, CPUSlots: 1, Disks: disks.resolve,
					Handlers: map[domain.JobKind]Handler{domain.JobProbe: probe},
				})
				defer stop()
				synctest.Wait()
				causes := queue.failureCauses()
				if len(causes) != 1 || !errors.Is(causes[0], domain.ErrJobSnoozed) {
					t.Fatalf("failures = %v, want one snooze", causes)
				}
				if got, want := queue.lastUnavailable(), []int64{rootA}; !slices.Equal(got, want) {
					t.Fatalf("unavailable folders passed to the claim = %v, want %v", got, want)
				}
				disks.setReadable("/a", true)
				time.Sleep(RootRecheckInterval - time.Second)
				synctest.Wait()
				if got := queue.completedIDs(); len(got) != 0 {
					t.Fatalf("completed before the recheck = %v", got)
				}
				tc.recheck(scheduler)
				synctest.Wait()
				if got, want := queue.completedIDs(), []int64{1}; !slices.Equal(got, want) {
					t.Fatalf("completed after the recheck = %v, want %v", got, want)
				}
			})
		})
	}
}

func TestSchedulerGrantsOtherDisksWhileAFolderDoesNotAnswer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		disks := twoDisks()
		answer := make(chan struct{})
		// Disk a's folder blocks like a share that stops answering until the test lets it answer.
		resolve := func(path string) (domain.Disk, error) {
			if path == "/a" {
				<-answer
			}
			return disks.resolve(path)
		}
		queue := newFakeQueue(
			domain.Job{ID: 1, Kind: domain.JobProbe, RootID: rootA},
			domain.Job{ID: 2, Kind: domain.JobProbe, RootID: rootB},
		)
		jobs := newBlockingJobs()
		_, stop := startScheduler(Options{Queue: queue, Handlers: jobs.handlers(), CPUSlots: 1, Disks: resolve})
		defer stop()
		time.Sleep(DiskResolveTimeout)
		synctest.Wait()
		if got, want := jobs.ids(), []int64{2}; !slices.Equal(got, want) {
			t.Fatalf("running while disk a does not answer = %v, want %v", got, want)
		}
		if got, want := queue.lastUnavailable(), []int64{rootA}; !slices.Equal(got, want) {
			t.Fatalf("unavailable folders passed to the claim = %v, want %v", got, want)
		}
		close(answer)
		synctest.Wait()
		if got, want := jobs.ids(), []int64{1, 2}; !slices.Equal(got, want) {
			t.Fatalf("running after disk a answers = %v, want %v", got, want)
		}
	})
}

func TestSchedulerKeepsAFailureUnderAReadableFolder(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		queue := newFakeQueue(domain.Job{ID: 1, Kind: domain.JobProbe})
		cause := errors.New("the file was deleted")
		_, stop := startScheduler(Options{
			Queue: queue, CPUSlots: 1,
			Handlers: map[domain.JobKind]Handler{domain.JobProbe: func(context.Context, domain.Job) error { return cause }},
		})
		defer stop()
		synctest.Wait()
		causes := queue.failureCauses()
		if len(causes) != 1 || !errors.Is(causes[0], cause) || errors.Is(causes[0], domain.ErrJobSnoozed) {
			t.Fatalf("failures = %v, want %v without a snooze", causes, cause)
		}
	})
}
