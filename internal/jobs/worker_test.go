package jobs

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// fakeQueue は待ち行列の代わりに、状態遷移だけを再現する。
// ワーカーの振る舞い（直列・再試行・停止）を SQLite 抜きで検証する。
type fakeQueue struct {
	mu sync.Mutex

	all      []*fakeJob
	queued   []*fakeJob
	running  int
	requeued int
	done     []int64
	failed   []int64
}

type fakeJob struct {
	id       int64
	kind     domain.JobKind
	attempts int
}

func (q *fakeQueue) ClaimJob(context.Context) (domain.Job, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.queued) == 0 {
		return domain.Job{}, domain.ErrNoJob
	}
	job := q.queued[0]
	q.queued = q.queued[1:]
	job.attempts++
	q.running++

	return domain.Job{ID: job.id, Kind: job.kind, VideoID: job.id, Attempts: job.attempts}, nil
}

func (q *fakeQueue) CompleteJob(_ context.Context, id int64) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.running--
	q.done = append(q.done, id)
	return nil
}

func (q *fakeQueue) FailJob(_ context.Context, id int64, _ string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.running--
	// 上限に達していなければ待ち行列へ戻す（store と同じ規則）。
	for _, job := range append([]*fakeJob{}, q.allJobs()...) {
		if job.id == id && job.attempts < domain.MaxJobAttempts {
			q.queued = append(q.queued, job)
			return nil
		}
	}
	q.failed = append(q.failed, id)
	return nil
}

func (q *fakeQueue) RequeueRunningJobs(context.Context) (int64, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.requeued++
	return 0, nil
}

// add はジョブを投入する。
func (q *fakeQueue) add(job *fakeJob) {
	q.all = append(q.all, job)
	q.queued = append(q.queued, job)
}

// allJobs は投入されたすべてのジョブを返す（待ち行列に無いものを含む）。
func (q *fakeQueue) allJobs() []*fakeJob { return q.all }

// counts は結果を読み出す。
func (q *fakeQueue) counts() (done, failed int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.done), len(q.failed)
}

// runWorker はワーカーを起動し、待ち行列が空になるまで待ってから止める。
func runWorker(t *testing.T, queue *fakeQueue, handlers map[domain.JobKind]Handler) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	worker := New(Options{Queue: queue, Handlers: handlers, IdleInterval: time.Millisecond})

	finished := make(chan struct{})
	go func() {
		worker.Run(ctx)
		close(finished)
	}()

	// 待ち行列が空になるまで待つ。
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		queue.mu.Lock()
		empty := len(queue.queued) == 0 && queue.running == 0
		queue.mu.Unlock()
		if empty {
			break
		}
		time.Sleep(time.Millisecond)
	}

	cancel()
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("context の取り消しでワーカーが止まらない")
	}
}

// ワーカーは直列（並列度1）で処理する。初回スキャンで HDD の I/O を飽和
// させないための決定である（R-106）。
func TestWorkerProcessesJobsSerially(t *testing.T) {
	queue := &fakeQueue{}
	for i := int64(1); i <= 10; i++ {
		queue.add(&fakeJob{id: i, kind: domain.JobProbe})
	}

	var (
		mu       sync.Mutex
		inFlight int
		maxSeen  int
	)
	handler := func(context.Context, domain.Job) error {
		mu.Lock()
		inFlight++
		if inFlight > maxSeen {
			maxSeen = inFlight
		}
		mu.Unlock()

		time.Sleep(time.Millisecond)

		mu.Lock()
		inFlight--
		mu.Unlock()
		return nil
	}

	runWorker(t, queue, map[domain.JobKind]Handler{domain.JobProbe: handler})

	if maxSeen != 1 {
		t.Errorf("同時に処理していた数 = %d, want 1（並列度1）", maxSeen)
	}
	if done, _ := queue.counts(); done != 10 {
		t.Errorf("完了 = %d 件, want 10", done)
	}
}

// 失敗したジョブは再試行し、3 回で止める。壊れたファイル1つがワーカーを
// 永久に占有してはならない。
func TestWorkerRetriesThenGivesUp(t *testing.T) {
	queue := &fakeQueue{}
	queue.add(&fakeJob{id: 1, kind: domain.JobProbe})

	attempts := 0
	handler := func(context.Context, domain.Job) error {
		attempts++
		return errors.New("いつも失敗する")
	}

	runWorker(t, queue, map[domain.JobKind]Handler{domain.JobProbe: handler})

	if attempts != domain.MaxJobAttempts {
		t.Errorf("試行回数 = %d, want %d", attempts, domain.MaxJobAttempts)
	}
	if _, failed := queue.counts(); failed != 1 {
		t.Errorf("諦めた数 = %d, want 1", failed)
	}
}

// 一度失敗しても、次の試行で成功すれば完了になる。
func TestWorkerRecoversAfterTransientFailure(t *testing.T) {
	queue := &fakeQueue{}
	queue.add(&fakeJob{id: 1, kind: domain.JobProbe})

	attempts := 0
	handler := func(context.Context, domain.Job) error {
		attempts++
		if attempts == 1 {
			return errors.New("一時的な失敗")
		}
		return nil
	}

	runWorker(t, queue, map[domain.JobKind]Handler{domain.JobProbe: handler})

	done, failed := queue.counts()
	if done != 1 || failed != 0 {
		t.Errorf("完了 = %d, 諦め = %d, want 1 と 0", done, failed)
	}
}

// 起動時に running を巻き戻してから処理を始める。取り込み中に止めても
// 次の起動で再開できる（R-106）。
func TestWorkerRequeuesRunningJobsBeforeStarting(t *testing.T) {
	queue := &fakeQueue{}
	queue.add(&fakeJob{id: 1, kind: domain.JobProbe})

	claimedBeforeRequeue := false
	handler := func(context.Context, domain.Job) error {
		queue.mu.Lock()
		defer queue.mu.Unlock()
		if queue.requeued == 0 {
			claimedBeforeRequeue = true
		}
		return nil
	}

	runWorker(t, queue, map[domain.JobKind]Handler{domain.JobProbe: handler})

	if queue.requeued == 0 {
		t.Error("起動時の巻き戻しが行われていない")
	}
	if claimedBeforeRequeue {
		t.Error("巻き戻しの前にジョブを処理し始めた")
	}
}

// context の取り消しで安全に止まる。処理中のジョブは queued に残り、
// 次の起動で再開できる状態で終える。
func TestWorkerStopsOnCancel(t *testing.T) {
	queue := &fakeQueue{}

	worker := New(Options{Queue: queue, IdleInterval: time.Millisecond})

	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan struct{})
	go func() {
		worker.Run(ctx)
		close(finished)
	}()

	time.Sleep(5 * time.Millisecond)
	cancel()

	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("取り消しで止まらない")
	}
}

// 扱いを知らない種類のジョブは、再試行せず諦める。再試行しても結果は
// 変わらないので、待ち行列を塞ぐだけになる。
func TestWorkerGivesUpOnUnknownKind(t *testing.T) {
	queue := &fakeQueue{}
	queue.add(&fakeJob{id: 1, kind: domain.JobKind("未知")})

	runWorker(t, queue, map[domain.JobKind]Handler{domain.JobProbe: func(context.Context, domain.Job) error {
		t.Error("別の種類のハンドラが呼ばれた")
		return nil
	}})

	if _, failed := queue.counts(); failed != 1 {
		t.Errorf("諦めた数 = %d, want 1", failed)
	}
}
