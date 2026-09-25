package app

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// fakeScanStore は scans 表の偽物である。実行中は1件だけという制約を持つ。
type fakeScanStore struct {
	mu       sync.Mutex
	nextID   int64
	current  domain.Scan
	hasScan  bool
	progress map[int64][]domain.ScanProgress
	finished chan domain.Scan

	interrupted int64
	requeued    int64
	recovered   []string

	// rebuilds はフォルダの索引を作り直した回数、rebuildErr はその失敗である。
	rebuilds   int
	rebuildErr error
	// rebuiltBeforeFinish は閉じる時点までに作り直した回数である。
	rebuiltBeforeFinish []int
}

func newFakeScanStore() *fakeScanStore {
	return &fakeScanStore{progress: map[int64][]domain.ScanProgress{}, finished: make(chan domain.Scan, 4)}
}

func (f *fakeScanStore) StartScan(context.Context) (domain.Scan, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.hasScan && f.current.State == domain.ScanRunning {
		return f.current, false, nil
	}
	f.nextID++
	f.current = domain.Scan{ID: f.nextID, State: domain.ScanRunning, StartedAt: time.Now()}
	f.hasScan = true
	return f.current, true, nil
}

func (f *fakeScanStore) CurrentScan(context.Context) (domain.Scan, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.hasScan {
		return domain.Scan{}, domain.ErrNotFound
	}
	return f.current, nil
}

func (f *fakeScanStore) UpdateScanProgress(_ context.Context, id int64, progress domain.ScanProgress) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.progress[id] = append(f.progress[id], progress)
	if f.current.ID == id {
		f.current.Total, f.current.Completed, f.current.Failed = progress.Total, progress.Completed, progress.Failed
	}
	return nil
}

func (f *fakeScanStore) FinishScan(ctx context.Context, id int64, state domain.ScanState, reason string) error {
	// 閉じる書き込みは、停止で取り消された context でも届かなければならない。
	if ctx.Err() != nil {
		return ctx.Err()
	}
	f.mu.Lock()
	f.current.State, f.current.Error = state, reason
	f.rebuiltBeforeFinish = append(f.rebuiltBeforeFinish, f.rebuilds)
	scan := f.current
	f.mu.Unlock()
	f.finished <- scan
	return nil
}

func (f *fakeScanStore) FailInterruptedScans(context.Context) (int64, error) {
	f.recovered = append(f.recovered, "scans")
	return f.interrupted, nil
}

func (f *fakeScanStore) RebuildFolderIndex(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rebuilds++
	f.recovered = append(f.recovered, "folders")
	return f.rebuildErr
}

func (f *fakeScanStore) RequeueRunningJobs(context.Context) (int64, error) {
	f.recovered = append(f.recovered, "jobs")
	return f.requeued, nil
}

func (f *fakeScanStore) waitFinished(t *testing.T) domain.Scan {
	t.Helper()
	select {
	case scan := <-f.finished:
		return scan
	case <-time.After(5 * time.Second):
		t.Fatal("走査が閉じられない")
		return domain.Scan{}
	}
}

// fakeScanner は決め打ちの結果を返す。進捗を1回報告してから、release が
// 閉じられるまで（または ctx が取り消されるまで）待つ。
type fakeScanner struct {
	reporter ScanReporter
	result   domain.ScanResult
	err      error
	panics   bool
	release  chan struct{}
	started  chan struct{}
	runs     int
	mu       sync.Mutex
}

func (f *fakeScanner) Scan(ctx context.Context) (domain.ScanResult, error) {
	f.mu.Lock()
	f.runs++
	f.mu.Unlock()
	if f.panics {
		panic("壊れた走査")
	}
	partial := domain.ScanResult{Total: f.result.Total, Processed: 1}
	if err := f.reporter.ReportScanProgress(ctx, partial); err != nil {
		return domain.ScanResult{}, err
	}
	if f.started != nil {
		close(f.started)
	}
	if f.release != nil {
		select {
		case <-f.release:
		case <-ctx.Done():
			return partial, ctx.Err()
		}
	}
	return f.result, f.err
}

func newTestScans(t *testing.T, ctx context.Context, scanner *fakeScanner) (*Scans, *fakeScanStore, *fakePublisher) {
	t.Helper()
	store := newFakeScanStore()
	publisher := &fakePublisher{}
	scans := NewScans(ScansOptions{
		Store:       store,
		Jobs:        store,
		FolderIndex: store,
		NewScanner: func(reporter ScanReporter) Scanner {
			scanner.reporter = reporter
			return scanner
		},
		Context:   ctx,
		Publisher: publisher,
		Logger:    discardLogger(),
	})
	return scans, store, publisher
}

// 走査が最後まで走れば、最終の進捗を記録して done で閉じ、変化を発行する。
// 要求の context が応答とともに終わっても、走査は打ち切られない。
func TestScanCompletes(t *testing.T) {
	scanner := &fakeScanner{result: domain.ScanResult{Total: 3, Processed: 2, Added: 2, Failed: 1}}
	scans, store, publisher := newTestScans(t, context.Background(), scanner)

	requestCtx, cancelRequest := context.WithCancel(context.Background())
	scan, err := scans.StartScan(requestCtx)
	cancelRequest()
	if err != nil {
		t.Fatal(err)
	}
	if scan.State != domain.ScanRunning {
		t.Fatalf("開始直後の状態 = %q, want running", scan.State)
	}

	closed := store.waitFinished(t)
	scans.Wait()
	if closed.State != domain.ScanDone || closed.Error != "" {
		t.Fatalf("閉じた状態 = %q (%q), want done", closed.State, closed.Error)
	}
	want := domain.ScanProgress{Total: 3, Completed: 2, Failed: 1}
	progress := store.progress[scan.ID]
	if len(progress) != 2 || progress[len(progress)-1] != want {
		t.Fatalf("進捗 = %+v, want 途中1回と最終 %+v", progress, want)
	}
	// 開始・途中の進捗・終了の3回。
	want3 := []domain.Event{domain.ScanChanged{}, domain.ScanChanged{}, domain.ScanChanged{}}
	if got := publisher.published(); !slices.Equal(got, want3) {
		t.Fatalf("走査の発行 = %v, want %v", got, want3)
	}
}

// 走査が失敗すれば failed で閉じ、理由を残す。panic も失敗として閉じる。
func TestScanFailure(t *testing.T) {
	for _, tc := range []struct {
		name    string
		scanner *fakeScanner
		reason  string
	}{
		{name: "error", scanner: &fakeScanner{err: errors.New("メディアフォルダを読めません")}, reason: "メディアフォルダを読めません"},
		{name: "panic", scanner: &fakeScanner{panics: true}, reason: "取り込み処理がpanicしました: 壊れた走査"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scans, store, _ := newTestScans(t, context.Background(), tc.scanner)
			if _, err := scans.StartScan(context.Background()); err != nil {
				t.Fatal(err)
			}
			closed := store.waitFinished(t)
			if closed.State != domain.ScanFailed || closed.Error != tc.reason {
				t.Fatalf("閉じた状態 = %q (%q), want failed (%q)", closed.State, closed.Error, tc.reason)
			}
		})
	}
}

// 実行中に開始を要求しても、新しく始めずに実行中のものを返す。
func TestStartScanWhileRunningReturnsCurrent(t *testing.T) {
	scanner := &fakeScanner{release: make(chan struct{}), started: make(chan struct{})}
	scans, store, _ := newTestScans(t, context.Background(), scanner)

	first, err := scans.StartScan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	<-scanner.started
	second, err := scans.StartScan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID {
		t.Fatalf("2回目の開始 = %d, want 実行中の %d", second.ID, first.ID)
	}
	close(scanner.release)
	store.waitFinished(t)
	scans.Wait()
	if scanner.runs != 1 {
		t.Fatalf("走査の回数 = %d, want 1", scanner.runs)
	}
}

// 停止で寿命の長い context が取り消されたら、走査は打ち切られ、それでも
// failed で閉じる（閉じる書き込みは取り消しを引き継がない）。
func TestScanStoppedByShutdownIsClosedAsFailed(t *testing.T) {
	baseCtx, stop := context.WithCancel(context.Background())
	scanner := &fakeScanner{release: make(chan struct{}), started: make(chan struct{})}
	scans, store, _ := newTestScans(t, baseCtx, scanner)

	if _, err := scans.StartScan(context.Background()); err != nil {
		t.Fatal(err)
	}
	<-scanner.started
	stop()

	closed := store.waitFinished(t)
	if closed.State != domain.ScanFailed || closed.Error != context.Canceled.Error() {
		t.Fatalf("閉じた状態 = %q (%q), want failed (context canceled)", closed.State, closed.Error)
	}
}

// 起動時の回復は、残った走査を閉じ、閉じた走査があればフォルダの索引を作り直し、
// 残った仕事を戻すことを保存層へ頼む。作り直しの失敗で起動は止めない。
func TestRecoverInterrupted(t *testing.T) {
	cases := []struct {
		name        string
		interrupted int64
		rebuildErr  error
		want        []string
	}{
		{"閉じた走査があれば作り直す", 1, nil, []string{"scans", "folders", "jobs"}},
		{"作り直しに失敗しても続ける", 1, errors.New("壊れた索引"), []string{"scans", "folders", "jobs"}},
		{"閉じた走査が無ければ作り直さない", 0, nil, []string{"scans", "jobs"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			scans, store, _ := newTestScans(t, context.Background(), &fakeScanner{})
			store.interrupted, store.requeued, store.rebuildErr = tc.interrupted, 2, tc.rebuildErr
			if err := scans.RecoverInterrupted(context.Background()); err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(store.recovered, tc.want) {
				t.Fatalf("回復の順 = %v, want %v", store.recovered, tc.want)
			}
		})
	}
}

// スキャンは成功でも失敗でも、閉じる直前にフォルダの索引を作り直す。作り直しに
// 失敗してもスキャンは失敗にしない。
func TestScanRebuildsFolderIndexBeforeClosing(t *testing.T) {
	cases := []struct {
		name       string
		scanErr    error
		rebuildErr error
		want       domain.ScanState
	}{
		{"成功", nil, nil, domain.ScanDone},
		{"走査の失敗", errors.New("読めない"), nil, domain.ScanFailed},
		{"作り直しの失敗", nil, errors.New("壊れた索引"), domain.ScanDone},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			scanner := &fakeScanner{result: domain.ScanResult{Total: 1, Processed: 1}, err: tc.scanErr}
			scans, store, _ := newTestScans(t, context.Background(), scanner)
			store.rebuildErr = tc.rebuildErr
			if _, err := scans.StartScan(context.Background()); err != nil {
				t.Fatal(err)
			}
			closed := store.waitFinished(t)
			scans.Wait()
			if closed.State != tc.want {
				t.Fatalf("閉じた状態 = %q, want %q", closed.State, tc.want)
			}
			store.mu.Lock()
			defer store.mu.Unlock()
			if !slices.Equal(store.rebuiltBeforeFinish, []int{1}) {
				t.Fatalf("閉じる時点の作り直しの回数 = %v, want [1]", store.rebuiltBeforeFinish)
			}
		})
	}
}
