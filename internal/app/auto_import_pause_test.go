package app

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// fakeHeldWatchStore keeps the held rows in a set.
type fakeHeldWatchStore struct {
	mu   sync.Mutex
	rows map[domain.DirtyDirectory]struct{}
}

func newFakeHeldWatchStore(dirs ...domain.DirtyDirectory) *fakeHeldWatchStore {
	f := &fakeHeldWatchStore{rows: map[domain.DirtyDirectory]struct{}{}}
	for _, dir := range dirs {
		f.rows[dir] = struct{}{}
	}
	return f
}

func (f *fakeHeldWatchStore) HeldWatchDirectories(context.Context) ([]domain.DirtyDirectory, error) {
	return f.held(), nil
}

func (f *fakeHeldWatchStore) HoldWatchDirectories(_ context.Context, dirs []domain.DirtyDirectory) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, dir := range dirs {
		f.rows[dir] = struct{}{}
	}
	return nil
}

func (f *fakeHeldWatchStore) ReleaseWatchDirectories(_ context.Context, dirs []domain.DirtyDirectory) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, dir := range dirs {
		delete(f.rows, dir)
	}
	return nil
}

func (f *fakeHeldWatchStore) held() []domain.DirtyDirectory {
	f.mu.Lock()
	defer f.mu.Unlock()
	dirs := make([]domain.DirtyDirectory, 0, len(f.rows))
	for dir := range f.rows {
		dirs = append(dirs, dir)
	}
	return domain.NormalizeDirtyDirectories(dirs)
}

// mutableFolders lets a test delete a media folder between arms.
type mutableFolders struct {
	mu    sync.Mutex
	roots []domain.MediaFolder
}

func (f *mutableFolders) ListMediaFolders(context.Context) ([]domain.MediaFolder, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.roots), nil
}

func newPausableRig(t *testing.T, held *fakeHeldWatchStore, paused bool, roots ...string) *autoImportRig {
	t.Helper()
	rig := newAutoImportRigWith(t, true, roots, func(o *AutoImportOptions) {
		o.Held, o.Paused = held, paused
	})
	rig.waitState(t, domain.FolderWatchActive)
	return rig
}

func dirtyDirs(dirs ...domain.DirtyDirectory) []domain.DirtyDirectory {
	return domain.NormalizeDirtyDirectories(dirs)
}

func TestAutoImportHoldsChangesReportedWhilePausedUntilResume(t *testing.T) {
	held := newFakeHeldWatchStore()
	rig := newPausableRig(t, held, false, fixturePath("/media"))
	ctx := context.Background()

	if err := rig.auto.PauseBackground(ctx); err != nil {
		t.Fatal(err)
	}
	rig.auto.Changed(dirtyDir("/media/a", false))
	rig.clock.Advance(time.Minute)
	if got := rig.scans.started(); len(got) != 0 {
		t.Fatalf("停止中に走査を始めた: %v", got)
	}
	if got, want := held.held(), dirtyDirs(dirtyDir("/media/a", false)); !slices.Equal(got, want) {
		t.Fatalf("保留の行 = %v, want %v", got, want)
	}

	if err := rig.auto.ResumeBackground(ctx); err != nil {
		t.Fatal(err)
	}
	rig.clock.Advance(autoImportQuiet)
	got := rig.scans.started()
	if want := dirtyDirs(dirtyDir("/media/a", false)); len(got) != 1 || !slices.Equal(got[0], want) {
		t.Fatalf("再開後の走査 = %v, want [%v]", got, want)
	}
	if rows := held.held(); len(rows) != 0 {
		t.Fatalf("取ったバッチの行が残った: %v", rows)
	}
}

func TestAutoImportPauseSupersedesBatchAndScansItAfterResume(t *testing.T) {
	held := newFakeHeldWatchStore()
	rig := newPausableRig(t, held, false, fixturePath("/media"))
	ctx := context.Background()
	rig.auto.Changed(dirtyDir("/media/a", true))
	rig.clock.Advance(autoImportQuiet)
	if got := rig.scans.started(); len(got) != 1 {
		t.Fatalf("始めた走査 = %v, want 1 回", got)
	}

	if err := rig.auto.PauseBackground(ctx); err != nil {
		t.Fatal(err)
	}
	if rig.scans.supersede != 1 {
		t.Fatalf("止めた回数 = %d, want 1", rig.scans.supersede)
	}
	if got, want := held.held(), dirtyDirs(dirtyDir("/media/a", true)); !slices.Equal(got, want) {
		t.Fatalf("保留の行 = %v, want %v", got, want)
	}
	rig.clock.Advance(time.Minute)
	if got := rig.scans.started(); len(got) != 1 {
		t.Fatalf("停止中に走査を始めた: %v", got)
	}

	if err := rig.auto.ResumeBackground(ctx); err != nil {
		t.Fatal(err)
	}
	rig.clock.Advance(autoImportQuiet)
	got := rig.scans.started()
	if want := dirtyDirs(dirtyDir("/media/a", true)); len(got) != 2 || !slices.Equal(got[1], want) {
		t.Fatalf("始めた走査 = %v, want 2 回目が %v", got, want)
	}
}

// A directory reported just before the pause is in neither the batch nor the reports made while paused.
func TestAutoImportPauseWritesDirectoriesStillInTheirQuietTime(t *testing.T) {
	held := newFakeHeldWatchStore()
	rig := newPausableRig(t, held, false, fixturePath("/media"))
	rig.auto.Changed(dirtyDir("/media/a", false))
	rig.clock.Advance(autoImportQuiet / 2)

	if err := rig.auto.PauseBackground(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got, want := held.held(), dirtyDirs(dirtyDir("/media/a", false)); !slices.Equal(got, want) {
		t.Fatalf("保留の行 = %v, want %v", got, want)
	}
}

// The restart of a paused server keeps the held directories and scans them after the resume.
func TestAutoImportStartedPausedScansHeldRowsAfterResume(t *testing.T) {
	held := newFakeHeldWatchStore(dirtyDir("/media/a", false), dirtyDir("/media/b", true))
	rig := newPausableRig(t, held, true, fixturePath("/media"))
	ctx := context.Background()
	rig.clock.Advance(time.Minute)
	if got := rig.scans.started(); len(got) != 0 {
		t.Fatalf("停止中の起動で走査を始めた: %v", got)
	}

	if err := rig.auto.ResumeBackground(ctx); err != nil {
		t.Fatal(err)
	}
	rig.clock.Advance(autoImportQuiet)
	got := rig.scans.started()
	want := dirtyDirs(dirtyDir("/media/a", false), dirtyDir("/media/b", true))
	if len(got) != 1 || !slices.Equal(got[0], want) {
		t.Fatalf("再開後の走査 = %v, want [%v]", got, want)
	}
	if rows := held.held(); len(rows) != 0 {
		t.Fatalf("取ったバッチの行が残った: %v", rows)
	}
}

func TestAutoImportDisablingDeletesHeldRows(t *testing.T) {
	held := newFakeHeldWatchStore()
	rig := newPausableRig(t, held, true, fixturePath("/media"))
	rig.auto.Changed(dirtyDir("/media/a", false))
	if len(held.held()) != 1 {
		t.Fatalf("保留の行 = %v, want 1 行", held.held())
	}

	if _, err := rig.auto.SetEnabled(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if rows := held.held(); len(rows) != 0 {
		t.Fatalf("切にしても行が残った: %v", rows)
	}
}

// The rearm after a folder change drops the directories of a deleted folder from the dirty set and the held rows.
func TestAutoImportFolderChangeDeletesHeldRowsOutsideTheFolders(t *testing.T) {
	held := newFakeHeldWatchStore()
	folders := &mutableFolders{roots: []domain.MediaFolder{
		{ID: 1, Path: fixturePath("/media")}, {ID: 2, Path: fixturePath("/other")},
	}}
	rig := newAutoImportRigWith(t, true, nil, func(o *AutoImportOptions) {
		o.Held, o.Paused, o.Folders = held, true, folders
	})
	rig.waitState(t, domain.FolderWatchActive)
	ctx := context.Background()
	rig.auto.Changed(dirtyDir("/media/a", false))
	rig.auto.Changed(dirtyDir("/other/b", false))

	rig.auto.FoldersChanging(ctx)
	folders.mu.Lock()
	folders.roots = folders.roots[:1]
	folders.mu.Unlock()
	rig.auto.FoldersChanged(ctx)
	rig.waitState(t, domain.FolderWatchActive)
	if got, want := held.held(), dirtyDirs(dirtyDir("/media/a", false)); !slices.Equal(got, want) {
		t.Fatalf("保留の行 = %v, want %v", got, want)
	}

	if err := rig.auto.ResumeBackground(ctx); err != nil {
		t.Fatal(err)
	}
	rig.clock.Advance(autoImportQuiet)
	got := rig.scans.started()
	if want := dirtyDirs(dirtyDir("/media/a", false)); len(got) != 1 || !slices.Equal(got[0], want) {
		t.Fatalf("再開後の走査 = %v, want [%v]", got, want)
	}
}

// A batch whose settle probes are running is in neither the dirty set nor a scan and its rows are already deleted,
// so the pause must write it; the scan that later takes it deletes the rows again.
func TestAutoImportPauseWritesTheBatchStillSettling(t *testing.T) {
	held := newFakeHeldWatchStore()
	probing, release := make(chan struct{}), make(chan struct{})
	rig := newAutoImportRigWith(t, true, []string{fixturePath("/media")}, func(o *AutoImportOptions) {
		o.Held = held
		o.Probe = func(domain.DirtyDirectory) (time.Time, error) {
			close(probing)
			<-release
			return time.Time{}, nil
		}
	})
	rig.waitState(t, domain.FolderWatchActive)
	ctx := context.Background()
	rig.auto.Changed(dirtyDir("/media/a", false))
	fired := make(chan struct{})
	go func() {
		defer close(fired)
		rig.clock.Advance(autoImportQuiet)
	}()
	<-probing

	if err := rig.auto.PauseBackground(ctx); err != nil {
		t.Fatal(err)
	}
	want := dirtyDirs(dirtyDir("/media/a", false))
	if got := held.held(); !slices.Equal(got, want) {
		t.Fatalf("保留の行 = %v, want %v", got, want)
	}

	if err := rig.auto.ResumeBackground(ctx); err != nil {
		t.Fatal(err)
	}
	close(release)
	<-fired
	if got := rig.scans.started(); len(got) != 1 || !slices.Equal(got[0], want) {
		t.Fatalf("整定後の走査 = %v, want [%v]", got, want)
	}
	if rows := held.held(); len(rows) != 0 {
		t.Fatalf("走査が取った行が残った: %v", rows)
	}
}
