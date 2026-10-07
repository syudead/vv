package app

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// このファイルは、取り込みの開始・張り直し・停止が重なったときの順序を確かめる。

// gatedProbe は、更新が止まったかの確認をゲートが開くまで止める ChangeProbe である。
// 確認は AutoImport の状態の外で走るので、その間に切り替えが起きうる。
type gatedProbe struct {
	entered chan struct{}
	gate    chan struct{}
}

func newGatedProbe() *gatedProbe {
	return &gatedProbe{entered: make(chan struct{}, 8), gate: make(chan struct{})}
}

func (p *gatedProbe) probe(domain.DirtyDirectory) (time.Time, error) {
	p.entered <- struct{}{}
	<-p.gate
	return time.Time{}, nil
}

// racingAutoImport は gatedProbe つきの AutoImport を起動して、監視が張られるまで待つ。
func racingAutoImport(t *testing.T) (*AutoImport, *fakeWatchScans, *stepClock, *gatedProbe) {
	t.Helper()
	probe := newGatedProbe()
	scans := &fakeWatchScans{}
	clock := newStepClock()
	auto := NewAutoImport(AutoImportOptions{
		Store:   &fakeAutoImportStore{enabled: true},
		Folders: fakeActivityFolders{roots: []domain.MediaFolder{{ID: 1, Path: fixturePath("/media")}}},
		Watcher: &fakeFolderWatcher{},
		Scans:   scans,
		Probe:   probe.probe,
		Clock:   clock,
		Logger:  discardLogger(),
	})
	if err := auto.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(auto.Stop)
	rig := &autoImportRig{auto: auto}
	rig.waitState(t, domain.FolderWatchActive)
	return auto, scans, clock, probe
}

// fireBlockedInProbe は変更を 1 つ報告し、取り込みの判断が確認の途中で止まるところまで進める。
// 返す関数は、確認を再開して判断が終わるまで待つ。
func fireBlockedInProbe(t *testing.T, auto *AutoImport, clock *stepClock, probe *gatedProbe) (resume func()) {
	t.Helper()
	auto.Changed(dirtyDir(fixturePath("/media/a"), false))
	advanced := make(chan struct{})
	go func() {
		defer close(advanced)
		clock.Advance(2 * time.Second)
	}()
	select {
	case <-probe.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("取り込みの判断が確認まで進まなかった")
	}
	return func() {
		close(probe.gate)
		select {
		case <-advanced:
		case <-time.After(5 * time.Second):
			t.Fatal("取り込みの判断が終わらなかった")
		}
	}
}

// 取り込みの判断が確認の途中にある間に切にしたら、そのあとで走査を始めてはならない。
func TestAutoImportDisableDuringSettleCheckStartsNoScan(t *testing.T) {
	auto, scans, clock, probe := racingAutoImport(t)
	resume := fireBlockedInProbe(t, auto, clock, probe)

	status, err := auto.SetEnabled(context.Background(), false)
	if err != nil || status.Enabled {
		t.Fatalf("SetEnabled = %+v, %v", status, err)
	}
	resume()
	if got := scans.started(); len(got) != 0 {
		t.Fatalf("切にしたあとに走査を始めた: %v", got)
	}
	auto.Changed(dirtyDir(fixturePath("/media/b"), false))
	clock.Advance(time.Minute)
	if got := scans.started(); len(got) != 0 {
		t.Fatalf("切のあとの変更で走査を始めた: %v", got)
	}
}

// 停止のあとで走査を始めてはならない。
func TestAutoImportStopDuringSettleCheckStartsNoScan(t *testing.T) {
	auto, scans, clock, probe := racingAutoImport(t)
	resume := fireBlockedInProbe(t, auto, clock, probe)

	auto.Stop()
	resume()
	if got := scans.started(); len(got) != 0 {
		t.Fatalf("停止のあとに走査を始めた: %v", got)
	}
}

// フォルダの変更の途中に走査を始めてはならない。始めずに待たせたものは、変更が終わったあとで取り込む。
func TestAutoImportFolderChangeDuringSettleCheckHoldsBatch(t *testing.T) {
	auto, scans, clock, probe := racingAutoImport(t)
	resume := fireBlockedInProbe(t, auto, clock, probe)

	auto.FoldersChanging(context.Background())
	resume()
	if got := scans.started(); len(got) != 0 {
		t.Fatalf("フォルダの変更の途中に走査を始めた: %v", got)
	}

	auto.FoldersChanged(context.Background())
	(&autoImportRig{auto: auto}).waitState(t, domain.FolderWatchActive)
	// 確認はゲートが開いたままなので、すぐ戻る。
	clock.Advance(2 * time.Second)
	got := scans.started()
	want := []domain.DirtyDirectory{dirtyDir(fixturePath("/media/a"), false)}
	if len(got) != 1 || !slices.Equal(got[0], want) {
		t.Fatalf("始めた走査 = %v, want [%v]", got, want)
	}
}

// foldersByCall は呼び出しごとに決まった一覧を返す。gates[i] が nil でなければ、i 回目の呼び出しは
// それが閉じるまで戻らない。
type foldersByCall struct {
	mu    sync.Mutex
	calls int
	lists [][]domain.MediaFolder
	gates []chan struct{}
	seen  chan int
}

func (f *foldersByCall) ListMediaFolders(context.Context) ([]domain.MediaFolder, error) {
	f.mu.Lock()
	i := f.calls
	f.calls++
	f.mu.Unlock()
	f.seen <- i
	if f.gates[i] != nil {
		<-f.gates[i]
	}
	return f.lists[i], nil
}

// 古い張り直しが一覧の取得で遅れても、そのあとで最新の監視を置き換えてはならない。
func TestAutoImportStaleArmDoesNotReplaceLatestWatchSet(t *testing.T) {
	gate := make(chan struct{})
	folders := &foldersByCall{
		lists: [][]domain.MediaFolder{
			{{ID: 1, Path: fixturePath("/old")}},
			{{ID: 2, Path: fixturePath("/new")}},
		},
		gates: []chan struct{}{gate, nil},
		seen:  make(chan int, 4),
	}
	watcher := &fakeFolderWatcher{}
	auto := NewAutoImport(AutoImportOptions{
		Store:   &fakeAutoImportStore{enabled: true},
		Folders: folders,
		Watcher: watcher,
		Scans:   &fakeWatchScans{},
		Clock:   newStepClock(),
		Logger:  discardLogger(),
	})
	if err := auto.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(auto.Stop)
	<-folders.seen // 1 つ目の張り込みが一覧の取得で止まった。

	auto.FoldersChanging(context.Background())
	auto.FoldersChanged(context.Background())
	(&autoImportRig{auto: auto}).waitState(t, domain.FolderWatchActive)

	close(gate)
	auto.arms.Wait()
	got := watcher.armed()
	if len(got) == 0 || !slices.Equal(got[len(got)-1], []string{fixturePath("/new")}) {
		t.Fatalf("最後に張った roots = %v, want [%s]", got, fixturePath("/new"))
	}
	if len(got) != 1 {
		t.Fatalf("古い世代が張った: %v", got)
	}
}

// 張り込みが終わらなくても、Stop は上限のあとで戻る。
func TestAutoImportStopDoesNotWaitForeverForArming(t *testing.T) {
	watcher := &fakeFolderWatcher{block: make(chan struct{})}
	t.Cleanup(func() { close(watcher.block) })
	auto := NewAutoImport(AutoImportOptions{
		Store:    &fakeAutoImportStore{enabled: true},
		Folders:  fakeActivityFolders{roots: []domain.MediaFolder{{ID: 1, Path: fixturePath("/media")}}},
		Watcher:  watcher,
		Scans:    &fakeWatchScans{},
		Clock:    newStepClock(),
		Logger:   discardLogger(),
		StopWait: 20 * time.Millisecond,
	})
	if err := auto.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	for len(watcher.armed()) == 0 {
		time.Sleep(time.Millisecond)
	}
	stopped := make(chan struct{})
	go func() {
		auto.Stop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop が張り込みを待ち続けた")
	}
}

// 張っている間に届いた問題は、張り終えたあとの active で上書きされない。
func TestAutoImportProblemDuringArmStaysLimited(t *testing.T) {
	t.Run("Arm の途中で届いた問題", func(t *testing.T) {
		var auto *AutoImport
		watcher := &problemWatcher{report: func() {
			auto.WatchProblem(domain.FolderWatchProblem{Kind: domain.FolderWatchProblemLimit, Path: fixturePath("/media/x")})
		}}
		auto = NewAutoImport(AutoImportOptions{
			Store:   &fakeAutoImportStore{enabled: true},
			Folders: fakeActivityFolders{roots: []domain.MediaFolder{{ID: 1, Path: fixturePath("/media")}}},
			Watcher: watcher,
			Scans:   &fakeWatchScans{},
			Clock:   newStepClock(),
			Logger:  discardLogger(),
		})
		if err := auto.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(auto.Stop)
		auto.arms.Wait()
		if status := auto.Status(); status.State != domain.FolderWatchLimited || status.Problem == nil {
			t.Fatalf("状態 = %+v, want limited", status)
		}
	})

	t.Run("問題を報告せずに Arm が失敗した", func(t *testing.T) {
		auto := NewAutoImport(AutoImportOptions{
			Store:   &fakeAutoImportStore{enabled: true},
			Folders: fakeActivityFolders{roots: []domain.MediaFolder{{ID: 1, Path: fixturePath("/media")}}},
			Watcher: &problemWatcher{err: context.DeadlineExceeded},
			Scans:   &fakeWatchScans{},
			Clock:   newStepClock(),
			Logger:  discardLogger(),
		})
		if err := auto.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(auto.Stop)
		auto.arms.Wait()
		if status := auto.Status(); status.State != domain.FolderWatchLimited || status.Problem == nil {
			t.Fatalf("状態 = %+v, want limited", status)
		}
	})
}

type problemWatcher struct {
	report func()
	err    error
}

func (w *problemWatcher) Arm([]string) error {
	if w.report != nil {
		w.report()
	}
	return w.err
}

func (w *problemWatcher) Disarm() {}
