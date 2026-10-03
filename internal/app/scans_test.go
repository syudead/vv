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
	// issues は記録された問題である。
	issues []domain.ScanIssue
	// resumedFrom は走査を始めるたびの持ち越し元（StartScan なら 0）である。
	resumedFrom []int64
	// unfinished は queued か running の仕事があるか、unfinishedErr はその問い合わせの失敗である。
	unfinished    bool
	unfinishedErr error
}

func newFakeScanStore() *fakeScanStore {
	return &fakeScanStore{progress: map[int64][]domain.ScanProgress{}, finished: make(chan domain.Scan, 4)}
}

func (f *fakeScanStore) StartScan(ctx context.Context) (domain.Scan, bool, error) {
	return f.ResumeScan(ctx, 0)
}

// ResumeScan は StartScan と同じで、持ち越し元を resumedFrom に残す。
func (f *fakeScanStore) ResumeScan(_ context.Context, from int64) (domain.Scan, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.hasScan && f.current.State == domain.ScanRunning {
		return f.current, false, nil
	}
	f.nextID++
	f.resumedFrom = append(f.resumedFrom, from)
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

func (f *fakeScanStore) FinishScan(ctx context.Context, id int64, state domain.ScanState, cause error) error {
	// 閉じる書き込みは、停止で取り消された context でも届かなければならない。
	if ctx.Err() != nil {
		return ctx.Err()
	}
	f.mu.Lock()
	f.current.State, f.current.Error, f.current.ErrorCode, f.current.ErrorPath = state, "", "", ""
	f.current.FinishedAt = time.Now()
	if cause != nil {
		f.current.Error = cause.Error()
		f.current.ErrorCode, f.current.ErrorPath = domain.ScanFailureOf(cause)
	}
	f.rebuiltBeforeFinish = append(f.rebuiltBeforeFinish, f.rebuilds)
	scan := f.current
	f.mu.Unlock()
	f.finished <- scan
	return nil
}

func (f *fakeScanStore) RecordScanIssue(_ context.Context, issue domain.ScanFileIssue) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.issues = append(f.issues, domain.ScanIssue{
		VideoID: issue.VideoID, Severity: issue.Kind.Severity(), Kinds: []domain.ScanIssueKind{issue.Kind},
		FileName: issue.Path, Path: issue.Path, Unregistered: issue.VideoID == 0,
	})
	return nil
}

func (f *fakeScanStore) ScanIssues(context.Context, int64) ([]domain.ScanIssue, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.issues), nil
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

func (f *fakeScanStore) HasUnfinishedJobs(context.Context) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.unfinished, f.unfinishedErr
}

func (f *fakeScanStore) setUnfinished(unfinished bool, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.unfinished, f.unfinishedErr = unfinished, err
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
	// beforeReturn は結果を返す直前に呼ぶ。nil なら呼ばない。
	beforeReturn func()
	release      chan struct{}
	started      chan struct{}
	runs         int
	mu           sync.Mutex
}

func (f *fakeScanner) Scan(ctx context.Context) (domain.ScanResult, error) {
	f.mu.Lock()
	f.runs++
	f.mu.Unlock()
	if f.panics {
		panic("broken scan")
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
	if f.beforeReturn != nil {
		f.beforeReturn()
	}
	return f.result, f.err
}

func newTestScans(t *testing.T, ctx context.Context, scanner *fakeScanner) (*Scans, *fakeScanStore, *fakePublisher) {
	t.Helper()
	store := newFakeScanStore()
	publisher := &fakePublisher{}
	scans := NewScans(ScansOptions{
		Store:          store,
		Jobs:           store,
		UnfinishedJobs: store,
		FolderIndex:    store,
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
	scan, _, err := scans.StartScan(requestCtx)
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

// 走査が失敗すれば failed で閉じ、理由とそのコードを残す。走査が包んだコードと場所は
// そのまま渡り、包まれていない失敗と panic は internal になる。
func TestScanFailure(t *testing.T) {
	folder := fixturePath("/media/unreadable")
	for _, tc := range []struct {
		name    string
		scanner *fakeScanner
		reason  string
		code    domain.ScanErrorCode
		path    string
	}{
		{
			name: "coded",
			scanner: &fakeScanner{err: domain.NewScanFailure(domain.ScanErrorMediaFolderUnreadable, folder,
				errors.New("could not read the media folder"))},
			reason: "could not read the media folder", code: domain.ScanErrorMediaFolderUnreadable, path: folder,
		},
		{
			name:    "uncoded",
			scanner: &fakeScanner{err: errors.New("database is locked")},
			reason:  "database is locked", code: domain.ScanErrorInternal,
		},
		{
			name:    "panic",
			scanner: &fakeScanner{panics: true},
			reason:  "scan panicked: broken scan", code: domain.ScanErrorInternal,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scans, store, _ := newTestScans(t, context.Background(), tc.scanner)
			if _, _, err := scans.StartScan(context.Background()); err != nil {
				t.Fatal(err)
			}
			closed := store.waitFinished(t)
			if closed.State != domain.ScanFailed || closed.Error != tc.reason {
				t.Fatalf("閉じた状態 = %q (%q), want failed (%q)", closed.State, closed.Error, tc.reason)
			}
			if closed.ErrorCode != tc.code || closed.ErrorPath != tc.path {
				t.Fatalf("理由のコード = %q (%q), want %q (%q)", closed.ErrorCode, closed.ErrorPath, tc.code, tc.path)
			}
		})
	}
}

// 実行中に開始を要求しても、新しく始めずに実行中のものを返す。
func TestStartScanWhileRunningReturnsCurrent(t *testing.T) {
	scanner := &fakeScanner{release: make(chan struct{}), started: make(chan struct{})}
	scans, store, _ := newTestScans(t, context.Background(), scanner)

	first, started, err := scans.StartScan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !started {
		t.Fatal("最初の開始の started = false, want true")
	}
	<-scanner.started
	second, started, err := scans.StartScan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID {
		t.Fatalf("2回目の開始 = %d, want 実行中の %d", second.ID, first.ID)
	}
	if started {
		t.Fatal("実行中の開始の started = true, want false")
	}
	close(scanner.release)
	store.waitFinished(t)
	scans.Wait()
	if scanner.runs != 1 {
		t.Fatalf("走査の回数 = %d, want 1", scanner.runs)
	}
}

// Busy は走っている走査があるか、queued・running の仕事があるとき真で、どちらも
// 無いとき偽（specs/037-windows-app/research.md R-7）。
func TestBusy(t *testing.T) {
	scanner := &fakeScanner{release: make(chan struct{}), started: make(chan struct{})}
	scans, store, _ := newTestScans(t, context.Background(), scanner)
	ctx := context.Background()
	assertBusy := func(want bool) {
		t.Helper()
		got, err := scans.Busy(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("Busy() = %v, want %v", got, want)
		}
	}

	assertBusy(false)

	// 走査中。
	if _, _, err := scans.StartScan(ctx); err != nil {
		t.Fatal(err)
	}
	<-scanner.started
	assertBusy(true)
	close(scanner.release)
	store.waitFinished(t)
	scans.Wait()
	assertBusy(false)

	// queued か running の仕事がある（保存先はどちらも 1 つの問い合わせで答える）。
	store.setUnfinished(true, nil)
	assertBusy(true)
	store.setUnfinished(false, nil)
	assertBusy(false)

	// 問い合わせの失敗は返す。
	store.setUnfinished(false, errors.New("disk"))
	if _, err := scans.Busy(ctx); err == nil {
		t.Error("問い合わせの失敗が返らない")
	}
}

// 停止で寿命の長い context が取り消されたら、走査は打ち切られ、それでも
// failed で閉じる（閉じる書き込みは取り消しを引き継がない）。
func TestScanStoppedByShutdownIsClosedAsFailed(t *testing.T) {
	baseCtx, stop := context.WithCancel(context.Background())
	scanner := &fakeScanner{release: make(chan struct{}), started: make(chan struct{})}
	scans, store, _ := newTestScans(t, baseCtx, scanner)

	if _, _, err := scans.StartScan(context.Background()); err != nil {
		t.Fatal(err)
	}
	<-scanner.started
	stop()

	closed := store.waitFinished(t)
	const reason = "the scan was stopped before it finished: context canceled"
	if closed.State != domain.ScanFailed || closed.Error != reason {
		t.Fatalf("閉じた状態 = %q (%q), want failed (%q)", closed.State, closed.Error, reason)
	}
	if closed.ErrorCode != domain.ScanErrorInterrupted || closed.ErrorPath != "" {
		t.Fatalf("理由のコード = %q (%q), want interrupted", closed.ErrorCode, closed.ErrorPath)
	}
}

// 停止の直後に走査が別の理由で失敗しても、中断として閉じる。停止は走査の
// context へ非同期に伝わるので、それを待たずに失敗が返る場合を確かめる。
func TestScanFailingRightAfterShutdownIsInterrupted(t *testing.T) {
	baseCtx, stop := context.WithCancel(context.Background())
	scanner := &fakeScanner{
		err:          errors.New("could not read the media folder"),
		beforeReturn: stop,
	}
	scans, store, _ := newTestScans(t, baseCtx, scanner)

	if _, _, err := scans.StartScan(context.Background()); err != nil {
		t.Fatal(err)
	}
	closed := store.waitFinished(t)
	if closed.State != domain.ScanFailed || closed.ErrorCode != domain.ScanErrorInterrupted {
		t.Fatalf("閉じた状態 = %q (%q), want failed (interrupted)", closed.State, closed.ErrorCode)
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

// 起動時に、最新の走査が中断で終わっていれば走査を 1 回始め直す。done、
// interrupted 以外の理由の failed、走査の記録が無いときは始めない
// （specs/037-windows-app/research.md R-9）。
func TestResumeInterrupted(t *testing.T) {
	cases := []struct {
		name   string
		latest *domain.Scan
		want   bool
	}{
		{"中断で終わっていれば始める", &domain.Scan{ID: 4, State: domain.ScanFailed, ErrorCode: domain.ScanErrorInterrupted}, true},
		{"done なら始めない", &domain.Scan{ID: 4, State: domain.ScanDone}, false},
		{"interrupted 以外の failed なら始めない", &domain.Scan{ID: 4, State: domain.ScanFailed, ErrorCode: domain.ScanErrorInternal}, false},
		{"理由のコードの無い failed なら始めない", &domain.Scan{ID: 4, State: domain.ScanFailed}, false},
		{"走査の記録が無ければ始めない", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			scanner := &fakeScanner{}
			scans, store, _ := newTestScans(t, context.Background(), scanner)
			if tc.latest != nil {
				store.current, store.hasScan, store.nextID = *tc.latest, true, tc.latest.ID
			}

			started, err := scans.ResumeInterrupted(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if started != tc.want {
				t.Fatalf("始めたか = %v, want %v", started, tc.want)
			}
			if tc.want {
				closed := store.waitFinished(t)
				if closed.ID != tc.latest.ID+1 || closed.State != domain.ScanDone {
					t.Fatalf("始め直した走査 = #%d %q, want #%d done", closed.ID, closed.State, tc.latest.ID+1)
				}
				store.mu.Lock()
				from := slices.Clone(store.resumedFrom)
				store.mu.Unlock()
				if !slices.Equal(from, []int64{tc.latest.ID}) {
					t.Fatalf("持ち越し元 = %v, want [%d]", from, tc.latest.ID)
				}
			}
			scans.Wait()
			scanner.mu.Lock()
			runs := scanner.runs
			scanner.mu.Unlock()
			if want := map[bool]int{true: 1, false: 0}[tc.want]; runs != want {
				t.Fatalf("走査の回数 = %d, want %d", runs, want)
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
			if _, _, err := scans.StartScan(context.Background()); err != nil {
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

// setVideos は保存側が数える対象の動画の本数と、完了の時刻を決め打ちにする。
func (f *fakeScanStore) setVideos(videos, settled int, settledAt time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.current.Videos, f.current.SettledVideos, f.current.SettledAt = videos, settled, settledAt
}

// 走査が閉じても対象に残りの仕事があるあいだは running のままで、最後の仕事の成否が
// 記録された時点で done になる。完了の時刻は走査の終了より後になる
// （specs/024-import-progress/research.md R-4）。
func TestCurrentScanStaysRunningUntilJobsSettle(t *testing.T) {
	scanner := &fakeScanner{result: domain.ScanResult{Total: 2, Processed: 2, Added: 2}}
	scans, store, _ := newTestScans(t, context.Background(), scanner)
	if _, _, err := scans.StartScan(context.Background()); err != nil {
		t.Fatal(err)
	}
	closed := store.waitFinished(t)
	scans.Wait()

	// 走査は閉じたが、2本のうち1本に仕事が残っている。
	store.setVideos(2, 1, time.Time{})
	current, err := scans.CurrentScan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if current.Import.Status != domain.ImportRunning || current.Import.Total != 2 || current.Import.Settled != 1 {
		t.Fatalf("残りがあるあいだ = %+v, want 2本のうち1本・running", current.Import)
	}
	if !current.Import.SettledAt.IsZero() {
		t.Fatalf("残りがあるのに完了の時刻を返した: %v", current.Import.SettledAt)
	}

	// 最後の仕事の成否を記録した時点で、保存側が完了の時刻を入れる。
	settledAt := closed.FinishedAt.Add(3 * time.Second)
	store.setVideos(2, 2, settledAt)
	current, err = scans.CurrentScan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if current.Import.Status != domain.ImportDone || current.Import.Settled != 2 {
		t.Fatalf("すべて済んだあと = %+v, want 2本のうち2本・done", current.Import)
	}
	if !current.Import.SettledAt.After(current.FinishedAt) {
		t.Fatalf("完了の時刻 = %v, want 走査の終了 %v より後", current.Import.SettledAt, current.FinishedAt)
	}
}

// 開始の応答も、取り込みの状態を組み立てて返す。対象を数える前は finding である。
func TestStartScanReturnsFindingImport(t *testing.T) {
	release := make(chan struct{})
	scanner := &fakeScanner{result: domain.ScanResult{Total: 1, Processed: 1}, release: release}
	scans, store, _ := newTestScans(t, context.Background(), scanner)
	scan, _, err := scans.StartScan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if scan.Import.Status != domain.ImportFinding || scan.Import.Counted {
		t.Fatalf("開始直後 = %+v, want finding・本数なし", scan.Import)
	}
	close(release)
	store.waitFinished(t)
	scans.Wait()
}

// 走査が報告したファイルの失敗は問題として記録され、変化として知らせる。登録できなかった
// ファイルは分母と済みの本数に入り、失敗の問題があるので取り込みは partial になる
// （specs/024-import-progress/research.md R-5）。
func TestReportedFileIssueMakesImportPartial(t *testing.T) {
	scanner := &fakeScanner{result: domain.ScanResult{Total: 2, Processed: 1, Added: 1, Failed: 1}}
	scans, store, publisher := newTestScans(t, context.Background(), scanner)
	if _, _, err := scans.StartScan(context.Background()); err != nil {
		t.Fatal(err)
	}
	store.waitFinished(t)
	scans.Wait()
	store.setVideos(1, 1, time.Now())

	before := len(publisher.published())
	if err := scans.ReportScanIssue(context.Background(), domain.ScanFileIssue{
		Path: "/media/broken.mp4", Kind: domain.IssueUnreadable,
	}); err != nil {
		t.Fatal(err)
	}
	if events := publisher.published(); len(events) != before+1 || events[len(events)-1] != (domain.ScanChanged{}) {
		t.Fatalf("知らせ = %v, want ScanChanged を1回", events[before:])
	}

	current, err := scans.CurrentScan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if current.Import.Status != domain.ImportPartial || current.Import.Total != 2 || current.Import.Settled != 2 {
		t.Fatalf("取り込み = %+v, want 2本のうち2本・partial", current.Import)
	}
	if current.Issues.Failed != 1 || current.Issues.Unregistered != 1 {
		t.Fatalf("本数 = %+v, want 失敗1・未登録1", current.Issues)
	}
}

// 問題の一覧は直近の走査の id と一緒に返り、一度も走査していなければ ErrNotFound である。
func TestListScanIssues(t *testing.T) {
	scans, store, _ := newTestScans(t, context.Background(), &fakeScanner{})
	if _, err := scans.ListScanIssues(context.Background(), "", 50); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("走査前: err = %v, want ErrNotFound", err)
	}
	if _, _, err := scans.StartScan(context.Background()); err != nil {
		t.Fatal(err)
	}
	store.waitFinished(t)
	scans.Wait()
	for _, path := range []string{"/media/b.mp4", "/media/a.mp4"} {
		if err := scans.ReportScanIssue(context.Background(), domain.ScanFileIssue{Path: path, Kind: domain.IssueUnreadable}); err != nil {
			t.Fatal(err)
		}
	}
	page, err := scans.ListScanIssues(context.Background(), "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if page.ScanID != 1 || len(page.Items) != 1 || page.NextCursor == "" {
		t.Fatalf("1ページ目 = %+v", page)
	}
	if _, err := scans.ListScanIssues(context.Background(), "not a cursor", 1); !errors.Is(err, domain.ErrInvalidCursor) {
		t.Fatalf("不正なカーソル: err = %v, want ErrInvalidCursor", err)
	}
}
