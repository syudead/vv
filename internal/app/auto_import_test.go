package app

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// このファイルは AutoImport を、偽の watcher・時計・走査で確かめる。ファイルシステムも
// SQLite も使わない（specs/042-folder-watch-import/plan.md の Implementation Work）。

// stepClock は Advance で進める時計である。予約した関数は、時刻が来たら Advance の中で順に呼ぶ。
type stepClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []*stepTimer
}

type stepTimer struct {
	at      time.Time
	f       func()
	stopped bool
	fired   bool
	clock   *stepClock
}

func (t *stepTimer) Stop() bool {
	t.clock.mu.Lock()
	defer t.clock.mu.Unlock()
	was := !t.stopped && !t.fired
	t.stopped = true
	return was
}

func newStepClock() *stepClock {
	return &stepClock{now: time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)}
}

func (c *stepClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *stepClock) AfterFunc(d time.Duration, f func()) ClockTimer {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &stepTimer{at: c.now.Add(d), f: f, clock: c}
	c.timers = append(c.timers, t)
	return t
}

// Advance は d だけ進め、その間に来た予約を、時刻の順に呼ぶ。呼んだ関数が足した予約も、
// 時刻が来ていれば呼ぶ。
func (c *stepClock) Advance(d time.Duration) {
	c.mu.Lock()
	target := c.now.Add(d)
	c.mu.Unlock()
	for {
		c.mu.Lock()
		var next *stepTimer
		for _, t := range c.timers {
			if t.stopped || t.fired || t.at.After(target) {
				continue
			}
			if next == nil || t.at.Before(next.at) {
				next = t
			}
		}
		if next == nil {
			c.now = target
			c.mu.Unlock()
			return
		}
		next.fired = true
		if next.at.After(c.now) {
			c.now = next.at
		}
		c.mu.Unlock()
		next.f()
	}
}

// pending は、まだ来ていない予約の数である。
func (c *stepClock) pending() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, t := range c.timers {
		if !t.stopped && !t.fired {
			n++
		}
	}
	return n
}

// fakeFolderWatcher は張った roots を記録する。block が nil でなければ、Arm はそれが閉じるまで戻らない。
type fakeFolderWatcher struct {
	mu      sync.Mutex
	arms    [][]string
	disarms int
	block   chan struct{}
	// disarmBlock が nil でなければ、Disarm はそれが閉じるまで戻らない。
	disarmBlock chan struct{}
}

func (f *fakeFolderWatcher) Arm(roots []string) error {
	f.mu.Lock()
	f.arms = append(f.arms, slices.Clone(roots))
	block := f.block
	f.mu.Unlock()
	if block != nil {
		<-block
	}
	return nil
}

func (f *fakeFolderWatcher) Disarm() {
	f.mu.Lock()
	f.disarms++
	block := f.disarmBlock
	f.mu.Unlock()
	if block != nil {
		<-block
	}
}

func (f *fakeFolderWatcher) armed() [][]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.arms)
}

// fakeWatchScans は監視の走査の始め方と止め方の偽物である。running が空でなければ、その主体の走査が走っている。
type fakeWatchScans struct {
	mu        sync.Mutex
	running   domain.ScanOrigin
	starts    [][]domain.DirtyDirectory
	supersede int
}

func (f *fakeWatchScans) StartWatchScan(_ context.Context, dirs []domain.DirtyDirectory) (domain.Scan, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.running != "" {
		return domain.Scan{Origin: f.running, State: domain.ScanRunning}, false, nil
	}
	f.running = domain.ScanOriginWatch
	f.starts = append(f.starts, slices.Clone(dirs))
	return domain.Scan{Origin: domain.ScanOriginWatch, State: domain.ScanRunning}, true, nil
}

func (f *fakeWatchScans) SupersedeWatchScan(context.Context) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.running != domain.ScanOriginWatch {
		return false
	}
	f.running = ""
	f.supersede++
	return true
}

func (f *fakeWatchScans) started() [][]domain.DirtyDirectory {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.starts)
}

// begin は手動の走査が走り始めたことにする。
func (f *fakeWatchScans) begin(origin domain.ScanOrigin) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.running = origin
}

// end は走っている走査が閉じたことにして、その主体を返す。
func (f *fakeWatchScans) end() domain.ScanOrigin {
	f.mu.Lock()
	defer f.mu.Unlock()
	origin := f.running
	f.running = ""
	return origin
}

type fakeAutoImportStore struct {
	mu      sync.Mutex
	enabled bool
	saved   []bool
}

func (f *fakeAutoImportStore) AutoImport(context.Context) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.enabled, nil
}

func (f *fakeAutoImportStore) SaveAutoImport(_ context.Context, enabled bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.enabled = enabled
	f.saved = append(f.saved, enabled)
	return nil
}

// autoImportRig は AutoImport と、それを囲む偽物である。
type autoImportRig struct {
	auto    *AutoImport
	clock   *stepClock
	watcher *fakeFolderWatcher
	scans   *fakeWatchScans
	store   *fakeAutoImportStore
	// newest は probe が返す、ディレクトリごとのいちばん新しい更新の時刻である。無ければゼロ値（止まっている）。
	newest map[string]time.Time
}

func newAutoImportRig(t *testing.T, enabled bool, roots ...string) *autoImportRig {
	t.Helper()
	rig := &autoImportRig{
		clock:   newStepClock(),
		watcher: &fakeFolderWatcher{},
		scans:   &fakeWatchScans{},
		store:   &fakeAutoImportStore{enabled: enabled},
		newest:  map[string]time.Time{},
	}
	var folders []domain.MediaFolder
	for i, root := range roots {
		folders = append(folders, domain.MediaFolder{ID: int64(i + 1), Path: root})
	}
	rig.auto = NewAutoImport(AutoImportOptions{
		Store:   rig.store,
		Folders: fakeActivityFolders{roots: folders},
		Watcher: rig.watcher,
		Scans:   rig.scans,
		Probe: func(dir domain.DirtyDirectory) (time.Time, error) {
			return rig.newest[dir.Path], nil
		},
		Clock:  rig.clock,
		Logger: discardLogger(),
	})
	if err := rig.auto.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rig.auto.Stop)
	return rig
}

// waitState は監視の状態が want になるまで待つ。
func (r *autoImportRig) waitState(t *testing.T, want domain.FolderWatchState) domain.AutoImportStatus {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		status := r.auto.Status()
		if status.State == want {
			return status
		}
		if time.Now().After(deadline) {
			t.Fatalf("監視の状態 = %q, want %q", status.State, want)
		}
		time.Sleep(time.Millisecond)
	}
}

func dirtyDir(path string, recursive bool) domain.DirtyDirectory {
	return domain.DirtyDirectory{Path: fixturePath(path), Recursive: recursive}
}

// イベントが 2 秒以内に続く間は走査せず、静止したあとに、変わったディレクトリだけを
// 1 回の監視の走査にする。
func TestAutoImportWaitsForQuietThenScansOnlyDirtyDirectories(t *testing.T) {
	rig := newAutoImportRig(t, true, fixturePath("/media"))
	rig.waitState(t, domain.FolderWatchActive)

	rig.auto.Changed(dirtyDir("/media/a", false))
	rig.clock.Advance(1900 * time.Millisecond)
	rig.auto.Changed(dirtyDir("/media/b", true))
	rig.clock.Advance(1900 * time.Millisecond)
	rig.auto.Changed(dirtyDir("/media/a", false))
	rig.clock.Advance(1900 * time.Millisecond)
	if got := rig.scans.started(); len(got) != 0 {
		t.Fatalf("イベントが続いている間に走査が始まった: %v", got)
	}

	rig.clock.Advance(200 * time.Millisecond)
	got := rig.scans.started()
	want := domain.NormalizeDirtyDirectories([]domain.DirtyDirectory{dirtyDir("/media/a", false), dirtyDir("/media/b", true)})
	if len(got) != 1 || !slices.Equal(got[0], want) {
		t.Fatalf("始めた走査 = %v, want [%v]", got, want)
	}

	// 閉じたあとに新しい変更が無ければ、もう走査しない。
	origin := rig.scans.end()
	rig.auto.ScanFinished(origin, domain.ScanDone)
	rig.clock.Advance(time.Minute)
	if got := rig.scans.started(); len(got) != 1 {
		t.Fatalf("変更が無いのに走査が増えた: %v", got)
	}
}

// 更新が止まっていないファイルのあるディレクトリは変わったまま残り、止まってから取り込む。
// 止まったディレクトリは待たせない。
func TestAutoImportKeepsUnsettledDirectoryDirtyUntilSettled(t *testing.T) {
	rig := newAutoImportRig(t, true, fixturePath("/media"))
	rig.waitState(t, domain.FolderWatchActive)

	copying := fixturePath("/media/copying")
	rig.newest[copying] = rig.clock.Now().Add(-time.Second)
	rig.auto.Changed(dirtyDir("/media/copying", false))
	rig.auto.Changed(dirtyDir("/media/done", false))
	rig.clock.Advance(2 * time.Second)

	got := rig.scans.started()
	if len(got) != 1 || !slices.Equal(got[0], []domain.DirtyDirectory{dirtyDir("/media/done", false)}) {
		t.Fatalf("始めた走査 = %v, want 止まった done だけ", got)
	}
	origin := rig.scans.end()
	rig.auto.ScanFinished(origin, domain.ScanDone)

	// まだ 10 秒たっていない間は、残ったままである。
	rig.clock.Advance(5 * time.Second)
	if got := rig.scans.started(); len(got) != 1 {
		t.Fatalf("更新が止まる前に取り込んだ: %v", got)
	}

	rig.clock.Advance(5 * time.Second)
	got = rig.scans.started()
	if len(got) != 2 || !slices.Equal(got[1], []domain.DirtyDirectory{dirtyDir("/media/copying", false)}) {
		t.Fatalf("始めた走査 = %v, want 2 回目が copying", got)
	}
}

// 走査が走っている間（手動の走査）に起きた変更は、その走査が閉じたあとで取り込む。
func TestAutoImportImportsChangesMadeDuringManualScanAfterIt(t *testing.T) {
	rig := newAutoImportRig(t, true, fixturePath("/media"))
	rig.waitState(t, domain.FolderWatchActive)

	rig.scans.begin(domain.ScanOriginManual)
	rig.auto.Changed(dirtyDir("/media/a", false))
	rig.clock.Advance(time.Minute)
	if got := rig.scans.started(); len(got) != 0 {
		t.Fatalf("手動の走査の間に始めた: %v", got)
	}
	if rig.clock.pending() != 0 {
		t.Fatal("手動の走査の間に予約が残って空回りしている")
	}

	origin := rig.scans.end()
	rig.auto.ScanFinished(origin, domain.ScanDone)
	rig.clock.Advance(2 * time.Second)
	got := rig.scans.started()
	if len(got) != 1 || !slices.Equal(got[0], []domain.DirtyDirectory{dirtyDir("/media/a", false)}) {
		t.Fatalf("始めた走査 = %v", got)
	}
}

// メディアフォルダの変更は走っているバッチを止め、そのディレクトリを変わったものとして戻し、
// 監視を張り直したあとで取り込む。変更の間は新しい走査を始めない。
func TestAutoImportFolderChangeSupersedesBatchAndRequeuesIt(t *testing.T) {
	rig := newAutoImportRig(t, true, fixturePath("/media"))
	rig.waitState(t, domain.FolderWatchActive)

	rig.auto.Changed(dirtyDir("/media/a", true))
	rig.clock.Advance(2 * time.Second)
	if got := rig.scans.started(); len(got) != 1 {
		t.Fatalf("始めた走査 = %v", got)
	}

	rig.auto.FoldersChanging(context.Background())
	if rig.scans.supersede != 1 {
		t.Fatalf("止めた回数 = %d, want 1", rig.scans.supersede)
	}
	// 変更の間に起きた変更では、走査を始めない。
	rig.auto.Changed(dirtyDir("/media/b", false))
	rig.clock.Advance(time.Minute)
	if got := rig.scans.started(); len(got) != 1 {
		t.Fatalf("フォルダの変更の間に走査を始めた: %v", got)
	}

	rig.auto.FoldersChanged(context.Background())
	rig.waitState(t, domain.FolderWatchActive)
	if got := rig.watcher.armed(); len(got) != 2 {
		t.Fatalf("張った回数 = %d, want 2（起動と張り直し）", len(got))
	}
	rig.clock.Advance(2 * time.Second)
	got := rig.scans.started()
	want := domain.NormalizeDirtyDirectories([]domain.DirtyDirectory{dirtyDir("/media/a", true), dirtyDir("/media/b", false)})
	if len(got) != 2 || !slices.Equal(got[1], want) {
		t.Fatalf("始めた走査 = %v, want 2 回目が %v", got, want)
	}
}

// 走っているのが手動の走査なら、フォルダの変更は何も止めず、戻すものも無い。
func TestAutoImportFolderChangeLeavesManualScanAlone(t *testing.T) {
	rig := newAutoImportRig(t, true, fixturePath("/media"))
	rig.waitState(t, domain.FolderWatchActive)
	rig.scans.begin(domain.ScanOriginManual)

	rig.auto.FoldersChanging(context.Background())
	rig.auto.FoldersChanged(context.Background())
	if rig.scans.supersede != 0 {
		t.Fatalf("止めた回数 = %d, want 0", rig.scans.supersede)
	}
	rig.clock.Advance(time.Minute)
	if got := rig.scans.started(); len(got) != 0 {
		t.Fatalf("走査を始めた: %v", got)
	}
}

// 自動の取り込みを入にしても、走査は始めない。保存して監視を張るだけである。
func TestAutoImportEnablingStartsNoScan(t *testing.T) {
	rig := newAutoImportRig(t, false, fixturePath("/media"))
	if status := rig.auto.Status(); status.Enabled || status.State != domain.FolderWatchOff {
		t.Fatalf("起動直後の状態 = %+v, want 切の off", status)
	}
	if got := rig.watcher.armed(); len(got) != 0 {
		t.Fatalf("切のまま張った: %v", got)
	}

	status, err := rig.auto.SetEnabled(context.Background(), true)
	if err != nil || !status.Enabled {
		t.Fatalf("SetEnabled = %+v, %v", status, err)
	}
	rig.waitState(t, domain.FolderWatchActive)
	rig.clock.Advance(time.Hour)
	if got := rig.scans.started(); len(got) != 0 {
		t.Fatalf("入にしただけで走査を始めた: %v", got)
	}
	if !slices.Equal(rig.store.saved, []bool{true}) {
		t.Fatalf("保存 = %v, want [true]", rig.store.saved)
	}
	if got := rig.watcher.armed(); len(got) != 1 {
		t.Fatalf("張った回数 = %d, want 1", len(got))
	}

	// 同じ値は何もしない。
	if _, err := rig.auto.SetEnabled(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if len(rig.store.saved) != 1 || len(rig.watcher.armed()) != 1 {
		t.Fatalf("同じ値で保存か張り直しをした: %v, %d", rig.store.saved, len(rig.watcher.armed()))
	}
}

// 切にすると、監視を外し、変わったディレクトリの集合を捨て、走っている取り込みを止める。
func TestAutoImportDisablingDisarmsAndDropsDirtyDirectories(t *testing.T) {
	rig := newAutoImportRig(t, true, fixturePath("/media"))
	rig.waitState(t, domain.FolderWatchActive)
	rig.auto.Changed(dirtyDir("/media/a", false))

	status, err := rig.auto.SetEnabled(context.Background(), false)
	if err != nil || status.Enabled || status.State != domain.FolderWatchOff {
		t.Fatalf("SetEnabled = %+v, %v", status, err)
	}
	if rig.watcher.disarms == 0 {
		t.Fatal("監視を外していない")
	}
	rig.clock.Advance(time.Minute)
	if got := rig.scans.started(); len(got) != 0 {
		t.Fatalf("切にしたのに走査を始めた: %v", got)
	}
	rig.auto.Changed(dirtyDir("/media/b", false))
	rig.clock.Advance(time.Minute)
	if got := rig.scans.started(); len(got) != 0 {
		t.Fatalf("切のあとの変更で走査を始めた: %v", got)
	}
}

// 監視の状態は、張っている間は starting、張り終えれば active、問題があれば limited、
// メディアフォルダが無ければ off である。
func TestAutoImportStatus(t *testing.T) {
	t.Run("starting から active へ", func(t *testing.T) {
		rig := &autoImportRig{}
		watcher := &fakeFolderWatcher{block: make(chan struct{})}
		auto := NewAutoImport(AutoImportOptions{
			Store:   &fakeAutoImportStore{enabled: true},
			Folders: fakeActivityFolders{roots: []domain.MediaFolder{{ID: 1, Path: fixturePath("/media")}}},
			Watcher: watcher,
			Scans:   &fakeWatchScans{},
			Clock:   newStepClock(),
			Logger:  discardLogger(),
		})
		rig.auto = auto
		if err := auto.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(auto.Stop)
		if status := auto.Status(); !status.Enabled || status.State != domain.FolderWatchStarting {
			t.Fatalf("張っている間の状態 = %+v, want starting", status)
		}
		close(watcher.block)
		rig.waitState(t, domain.FolderWatchActive)
	})

	t.Run("問題は limited と問題を返し、手動の走査が done で閉じれば events_lost だけ解ける", func(t *testing.T) {
		rig := newAutoImportRig(t, true, fixturePath("/media"))
		rig.waitState(t, domain.FolderWatchActive)

		rig.auto.WatchProblem(domain.FolderWatchProblem{Kind: domain.FolderWatchProblemEventsLost})
		status := rig.auto.Status()
		if status.State != domain.FolderWatchLimited || status.Problem == nil ||
			status.Problem.Kind != domain.FolderWatchProblemEventsLost {
			t.Fatalf("状態 = %+v, want limited の events_lost", status)
		}
		rig.auto.ScanStarted(domain.ScanOriginManual)
		rig.auto.ScanFinished(domain.ScanOriginWatch, domain.ScanDone)
		if status := rig.auto.Status(); status.State != domain.FolderWatchLimited {
			t.Fatalf("監視の走査が閉じて問題が解けた: %+v", status)
		}
		rig.auto.ScanFinished(domain.ScanOriginManual, domain.ScanFailed)
		if status := rig.auto.Status(); status.State != domain.FolderWatchLimited {
			t.Fatalf("失敗した手動の走査で問題が解けた: %+v", status)
		}
		rig.auto.ScanFinished(domain.ScanOriginManual, domain.ScanDone)
		if status := rig.auto.Status(); status.State != domain.FolderWatchActive || status.Problem != nil {
			t.Fatalf("状態 = %+v, want active", status)
		}

		rig.auto.WatchProblem(domain.FolderWatchProblem{Kind: domain.FolderWatchProblemLimit, Path: fixturePath("/media/x")})
		rig.auto.ScanStarted(domain.ScanOriginManual)
		rig.auto.ScanFinished(domain.ScanOriginManual, domain.ScanDone)
		if status := rig.auto.Status(); status.State != domain.FolderWatchLimited ||
			status.Problem == nil || status.Problem.Path != fixturePath("/media/x") {
			t.Fatalf("watch_limit は手動の走査では解けない: %+v", status)
		}
		// 入れ直すと解ける。
		if _, err := rig.auto.SetEnabled(context.Background(), false); err != nil {
			t.Fatal(err)
		}
		if _, err := rig.auto.SetEnabled(context.Background(), true); err != nil {
			t.Fatal(err)
		}
		rig.waitState(t, domain.FolderWatchActive)
	})

	t.Run("メディアフォルダが無ければ off", func(t *testing.T) {
		rig := newAutoImportRig(t, true)
		rig.waitState(t, domain.FolderWatchOff)
		if status := rig.auto.Status(); !status.Enabled {
			t.Fatalf("状態 = %+v, want 入のまま", status)
		}
	})
}

// 手動の走査が走っている間に報告された取りこぼしは、その走査が閉じても解けない。走査が読み終えた
// ディレクトリのものかもしれないからである。次の手動の走査が全体を読めば解ける。
func TestAutoImportKeepsEventsLostReportedDuringManualScan(t *testing.T) {
	rig := newAutoImportRig(t, true, fixturePath("/media"))
	rig.waitState(t, domain.FolderWatchActive)

	rig.auto.ScanStarted(domain.ScanOriginManual)
	rig.auto.WatchProblem(domain.FolderWatchProblem{Kind: domain.FolderWatchProblemEventsLost})
	rig.auto.ScanFinished(domain.ScanOriginManual, domain.ScanDone)
	if status := rig.auto.Status(); status.State != domain.FolderWatchLimited || status.Problem == nil ||
		status.Problem.Kind != domain.FolderWatchProblemEventsLost {
		t.Fatalf("走査の間の取りこぼしが解けた: %+v", status)
	}

	rig.auto.ScanStarted(domain.ScanOriginManual)
	rig.auto.ScanFinished(domain.ScanOriginManual, domain.ScanDone)
	if status := rig.auto.Status(); status.State != domain.FolderWatchActive || status.Problem != nil {
		t.Fatalf("次の手動の走査で解けない: %+v", status)
	}
}

// 同じディレクトリの変更の報告が続いても、変わったディレクトリの集合は報告の数では増えない。
func TestAutoImportDirtySetGrowsPerDirectoryNotPerNotification(t *testing.T) {
	rig := newAutoImportRig(t, true, fixturePath("/media"))
	rig.waitState(t, domain.FolderWatchActive)

	for range 1000 {
		rig.auto.Changed(dirtyDir("/media/a", false))
		rig.auto.Changed(dirtyDir("/media/b", false))
		rig.auto.Changed(dirtyDir("/media/b", true))
	}
	rig.auto.mu.Lock()
	got := len(rig.auto.dirty)
	rig.auto.mu.Unlock()
	if got != 3 {
		t.Fatalf("変わったディレクトリの数 = %d, want 3", got)
	}
	rig.clock.Advance(autoImportQuiet)
	started := rig.scans.started()
	if len(started) != 1 || len(started[0]) != 2 {
		t.Fatalf("始めた走査 = %v, want a と b（再帰）の 1 回", started)
	}
}

// Disarm が応答しないマウントを読む goroutine を待っていても、Stop は上限で戻る。
func TestAutoImportStopDoesNotWaitForeverOnDisarm(t *testing.T) {
	watcher := &fakeFolderWatcher{disarmBlock: make(chan struct{})}
	t.Cleanup(func() { close(watcher.disarmBlock) })
	auto := NewAutoImport(AutoImportOptions{
		Store:    &fakeAutoImportStore{enabled: true},
		Folders:  fakeActivityFolders{roots: []domain.MediaFolder{{ID: 1, Path: fixturePath("/media")}}},
		Watcher:  watcher,
		Scans:    &fakeWatchScans{},
		Clock:    newStepClock(),
		Logger:   discardLogger(),
		StopWait: 50 * time.Millisecond,
	})
	if err := auto.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	stopped := make(chan struct{})
	go func() {
		auto.Stop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("Disarm が戻らないと Stop が戻らない")
	}
}
