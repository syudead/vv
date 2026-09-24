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
// ワーカーの振る舞い（直列・再試行・停止・通知での起床）を SQLite 抜きで検証する。
type fakeQueue struct {
	mu sync.Mutex

	all     []*fakeJob
	queued  []*fakeJob
	running int
	done    []int64
	failed  []int64
	claims  int
}

type fakeJob struct {
	id       int64
	kind     domain.JobKind
	attempts int
}

func (q *fakeQueue) ClaimJob(_ context.Context, kind domain.JobKind) (domain.Job, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.claims++
	for i, job := range q.queued {
		if job.kind != kind {
			continue
		}
		q.queued = append(q.queued[:i:i], q.queued[i+1:]...)
		job.attempts++
		q.running++
		return domain.Job{ID: job.id, Kind: job.kind, VideoID: job.id, Attempts: job.attempts}, nil
	}
	return domain.Job{}, domain.ErrNoJob
}

func (q *fakeQueue) CompleteClaimedJob(_ context.Context, job domain.Job) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.running--
	q.done = append(q.done, job.ID)
	return nil
}

func (q *fakeQueue) FailClaimedJob(_ context.Context, failed domain.Job, _ string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.running--
	// 上限に達していなければ待ち行列へ戻す（store と同じ規則）。
	for _, job := range q.all {
		if job.id == failed.ID && job.attempts < domain.MaxJobAttempts {
			q.queued = append(q.queued, job)
			return nil
		}
	}
	q.failed = append(q.failed, failed.ID)
	return nil
}

// add はジョブを投入する。
func (q *fakeQueue) add(job *fakeJob) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.all = append(q.all, job)
	q.queued = append(q.queued, job)
}

// counts は結果を読み出す。
func (q *fakeQueue) counts() (done, failed int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.done), len(q.failed)
}

func (q *fakeQueue) claimCount() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.claims
}

// idle は kind の仕事が待ち行列にも処理中にも無いかを返す。
func (q *fakeQueue) idle(kind domain.JobKind) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.running > 0 {
		return false
	}
	for _, job := range q.queued {
		if job.kind == kind {
			return false
		}
	}
	return true
}

// startWorker はワーカーを起動し、止める関数を返す。
func startWorker(t *testing.T, opts Options) (*Worker, func()) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	worker := New(opts)
	finished := make(chan struct{})
	go func() {
		worker.Run(ctx)
		close(finished)
	}()
	return worker, func() {
		cancel()
		select {
		case <-finished:
		case <-time.After(2 * time.Second):
			t.Fatal("context の取り消しでワーカーが止まらない")
		}
	}
}

// waitUntil は条件が成り立つまで待つ。
func waitUntil(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("条件が成り立たないまま時間切れになった")
		}
		time.Sleep(time.Millisecond)
	}
}

// runUntilIdle はワーカーを起動し、その段階の仕事が無くなるまで待ってから止める。
func runUntilIdle(t *testing.T, queue *fakeQueue, kind domain.JobKind, handler Handler) {
	t.Helper()
	_, stop := startWorker(t, Options{Kind: kind, Queue: queue, Handler: handler})
	waitUntil(t, func() bool { return queue.idle(kind) })
	stop()
}

// ワーカーは直列（並列度1）で処理する。初回スキャンで HDD の I/O を飽和
// させないための決定である。
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

	runUntilIdle(t, queue, domain.JobProbe, handler)

	if maxSeen != 1 {
		t.Errorf("同時に処理していた数 = %d, want 1（並列度1）", maxSeen)
	}
	if done, _ := queue.counts(); done != 10 {
		t.Errorf("完了 = %d 件, want 10", done)
	}
}

// ワーカーは自分の段階の仕事だけを取り出す。別の段階の仕事は、その段階の
// ワーカーが受け持つ。
func TestWorkerClaimsOnlyItsOwnKind(t *testing.T) {
	queue := &fakeQueue{}
	queue.add(&fakeJob{id: 1, kind: domain.JobThumbnail})
	queue.add(&fakeJob{id: 2, kind: domain.JobProbe})

	var handled []int64
	runUntilIdle(t, queue, domain.JobProbe, func(_ context.Context, job domain.Job) error {
		if job.Kind != domain.JobProbe {
			t.Errorf("別の段階の仕事を受け取った: %s", job.Kind)
		}
		handled = append(handled, job.ID)
		return nil
	})

	if len(handled) != 1 || handled[0] != 2 {
		t.Errorf("処理した仕事 = %v, want [2]", handled)
	}
	if queue.idle(domain.JobThumbnail) {
		t.Error("サムネイルの仕事が取り出された")
	}
}

// 待ち行列が空になったら、知らせが来るまで待ち行列を問い合わせない。
// 知らせを受けたら、積まれた仕事を処理する。
func TestWorkerSleepsUntilWoken(t *testing.T) {
	queue := &fakeQueue{}
	processed := make(chan int64, 1)
	worker, stop := startWorker(t, Options{
		Kind:  domain.JobProbe,
		Queue: queue,
		Handler: func(_ context.Context, job domain.Job) error {
			processed <- job.ID
			return nil
		},
	})
	defer stop()

	// 起動直後の1回だけ見に行き、空なので眠る。
	waitUntil(t, func() bool { return queue.claimCount() == 1 })
	time.Sleep(50 * time.Millisecond)
	if claims := queue.claimCount(); claims != 1 {
		t.Fatalf("眠っている間に待ち行列を %d 回問い合わせた, want 1", claims)
	}

	queue.add(&fakeJob{id: 7, kind: domain.JobProbe})
	select {
	case id := <-processed:
		t.Fatalf("知らせる前に仕事 %d を処理した", id)
	case <-time.After(50 * time.Millisecond):
	}

	worker.Wake()
	select {
	case id := <-processed:
		if id != 7 {
			t.Errorf("処理した仕事 = %d, want 7", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("知らせても仕事を処理しない")
	}
}

// 処理中に届いた知らせは失われない。処理を終えたあと、もう一度待ち行列を見る。
func TestWorkerKeepsWakeReceivedWhileBusy(t *testing.T) {
	queue := &fakeQueue{}
	queue.add(&fakeJob{id: 1, kind: domain.JobProbe})
	release := make(chan struct{})
	started := make(chan struct{}, 2)
	worker, stop := startWorker(t, Options{
		Kind:  domain.JobProbe,
		Queue: queue,
		Handler: func(_ context.Context, job domain.Job) error {
			started <- struct{}{}
			if job.ID == 1 {
				<-release
			}
			return nil
		},
	})
	defer stop()

	<-started
	queue.add(&fakeJob{id: 2, kind: domain.JobProbe})
	worker.Wake()
	close(release)

	waitUntil(t, func() bool {
		done, _ := queue.counts()
		return done == 2
	})
}

// 失敗したジョブは再試行し、3 回で止める。壊れたファイル1つがワーカーを
// 永久に占有してはならない。
func TestWorkerRetriesThenGivesUp(t *testing.T) {
	queue := &fakeQueue{}
	queue.add(&fakeJob{id: 1, kind: domain.JobProbe})

	attempts := 0
	runUntilIdle(t, queue, domain.JobProbe, func(context.Context, domain.Job) error {
		attempts++
		return errors.New("いつも失敗する")
	})

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
	runUntilIdle(t, queue, domain.JobProbe, func(context.Context, domain.Job) error {
		attempts++
		if attempts == 1 {
			return errors.New("一時的な失敗")
		}
		return nil
	})

	done, failed := queue.counts()
	if done != 1 || failed != 0 {
		t.Errorf("完了 = %d, 諦め = %d, want 1 と 0", done, failed)
	}
}

// 成否を記録するたびに Finished が呼ばれる。画面へ動画の変化を知らせるのに使う。
func TestWorkerReportsFinishedJobs(t *testing.T) {
	queue := &fakeQueue{}
	queue.add(&fakeJob{id: 1, kind: domain.JobProbe})
	queue.add(&fakeJob{id: 2, kind: domain.JobProbe})

	var mu sync.Mutex
	var finished []int64
	_, stop := startWorker(t, Options{
		Kind:  domain.JobProbe,
		Queue: queue,
		Handler: func(_ context.Context, job domain.Job) error {
			if job.ID == 2 {
				return errors.New("失敗")
			}
			return nil
		},
		Finished: func(job domain.Job) {
			mu.Lock()
			defer mu.Unlock()
			finished = append(finished, job.ID)
		},
	})
	waitUntil(t, func() bool { return queue.idle(domain.JobProbe) })
	stop()

	mu.Lock()
	defer mu.Unlock()
	// 2 は上限まで失敗するので、試行のたびに知らせる。
	if len(finished) != 1+domain.MaxJobAttempts {
		t.Errorf("知らせた回数 = %d (%v), want %d", len(finished), finished, 1+domain.MaxJobAttempts)
	}
}

// context の取り消しで、眠っている間でも安全に止まる。
func TestWorkerStopsOnCancel(t *testing.T) {
	queue := &fakeQueue{}
	_, stop := startWorker(t, Options{Kind: domain.JobProbe, Queue: queue})
	waitUntil(t, func() bool { return queue.claimCount() == 1 })
	stop()
}

// 停止指示で打ち切った仕事は、完了も失敗も記録しない。running のまま残り、
// 次の起動で queued へ戻る。
func TestWorkerDoesNotCompleteJobAfterHandlerCancels(t *testing.T) {
	queue := &fakeQueue{}
	queue.add(&fakeJob{id: 1, kind: domain.JobProbe})
	ctx, cancel := context.WithCancel(context.Background())
	job, err := queue.ClaimJob(ctx, domain.JobProbe)
	if err != nil {
		t.Fatal(err)
	}
	worker := New(Options{
		Kind:  domain.JobProbe,
		Queue: queue,
		Handler: func(context.Context, domain.Job) error {
			cancel()
			return nil
		},
	})
	worker.process(ctx, job)
	done, failed := queue.counts()
	if done != 0 || failed != 0 {
		t.Fatalf("cancelled result was recorded: done=%d failed=%d", done, failed)
	}
	queue.mu.Lock()
	running := queue.running
	queue.mu.Unlock()
	if running != 1 {
		t.Fatalf("running jobs = %d, want 1 for startup recovery", running)
	}
}

// ハンドラの無い段階の仕事は、再試行せず諦める。再試行しても結果は変わらない
// ので、待ち行列を塞ぐだけになる。
func TestWorkerGivesUpWithoutHandler(t *testing.T) {
	queue := &fakeQueue{}
	queue.add(&fakeJob{id: 1, kind: domain.JobProbe})

	runUntilIdle(t, queue, domain.JobProbe, nil)

	if _, failed := queue.counts(); failed != 1 {
		t.Errorf("諦めた数 = %d, want 1", failed)
	}
}
