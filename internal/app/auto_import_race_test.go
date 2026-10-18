package app

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// The settling probe runs outside the state lock, allowing settings changes while blocked.
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

// The returned release function resumes probing and waits for the decision to finish.
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

func TestAutoImportStopDuringSettleCheckStartsNoScan(t *testing.T) {
	auto, scans, clock, probe := racingAutoImport(t)
	resume := fireBlockedInProbe(t, auto, clock, probe)

	auto.Stop()
	resume()
	if got := scans.started(); len(got) != 0 {
		t.Fatalf("停止のあとに走査を始めた: %v", got)
	}
}

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
	// The probe gate remains open, so the next check returns immediately.
	clock.Advance(2 * time.Second)
	got := scans.started()
	want := []domain.DirtyDirectory{dirtyDir(fixturePath("/media/a"), false)}
	if len(got) != 1 || !slices.Equal(got[0], want) {
		t.Fatalf("始めた走査 = %v, want [%v]", got, want)
	}
}

// Each non-nil gate blocks that indexed folder-list call until closure.
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
	<-folders.seen

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
