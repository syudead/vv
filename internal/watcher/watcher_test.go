package watcher

import (
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

const waitFor = 5 * time.Second

// recorder collects what a Watcher reports.
type recorder struct {
	mu       sync.Mutex
	changes  []Change
	problems []Problem
	notify   chan struct{}
}

func newRecorder() *recorder { return &recorder{notify: make(chan struct{}, 1024)} }

func (r *recorder) handler() Handler {
	return Handler{
		Change: func(c Change) {
			r.mu.Lock()
			r.changes = append(r.changes, c)
			r.mu.Unlock()
			r.ping()
		},
		Problem: func(p Problem) {
			r.mu.Lock()
			r.problems = append(r.problems, p)
			r.mu.Unlock()
			r.ping()
		},
	}
}

func (r *recorder) ping() {
	select {
	case r.notify <- struct{}{}:
	default:
	}
}

func (r *recorder) hasChange(c Change) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, got := range r.changes {
		if got == c {
			return true
		}
	}
	return false
}

func (r *recorder) hasProblem(kind ProblemKind, path string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, got := range r.problems {
		if got.Kind == kind && got.Path == path {
			return true
		}
	}
	return false
}

// waitChange waits until c has been reported.
func waitChange(t *testing.T, r *recorder, c Change) {
	t.Helper()
	deadline := time.After(waitFor)
	for !r.hasChange(c) {
		select {
		case <-r.notify:
		case <-deadline:
			t.Fatalf("change %+v not reported; got %+v", c, r.changes)
		}
	}
}

func armed(t *testing.T, roots ...string) (*Watcher, *recorder) {
	t.Helper()
	r := newRecorder()
	w := New(r.handler())
	if err := w.Arm(roots); err != nil {
		t.Fatalf("Arm: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })
	return w, r
}

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func write(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFileCreateRemoveRenameReportParent(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	mkdir(t, sub)
	_, r := armed(t, root)

	write(t, filepath.Join(sub, "a.mp4"))
	waitChange(t, r, Change{Dir: sub})

	r.mu.Lock()
	r.changes = nil
	r.mu.Unlock()
	if err := os.Rename(filepath.Join(sub, "a.mp4"), filepath.Join(sub, "b.mp4")); err != nil {
		t.Fatal(err)
	}
	waitChange(t, r, Change{Dir: sub})

	r.mu.Lock()
	r.changes = nil
	r.mu.Unlock()
	if err := os.Remove(filepath.Join(sub, "b.mp4")); err != nil {
		t.Fatal(err)
	}
	waitChange(t, r, Change{Dir: sub})

	if r.hasChange(Change{Dir: sub, Recursive: true}) {
		t.Fatalf("a file event reported its parent as recursive: %+v", r.changes)
	}
}

func TestDirectoryCreateAndRemoveReportItselfRecursive(t *testing.T) {
	root := t.TempDir()
	_, r := armed(t, root)

	dir := filepath.Join(root, "new")
	mkdir(t, dir)
	waitChange(t, r, Change{Dir: dir, Recursive: true})

	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	// The dir was watched after its create event, so its removal is a directory.
	deadline := time.After(waitFor)
	for {
		r.mu.Lock()
		n := 0
		for _, c := range r.changes {
			if c == (Change{Dir: dir, Recursive: true}) {
				n++
			}
		}
		r.mu.Unlock()
		if n >= 2 {
			break
		}
		select {
		case <-r.notify:
		case <-deadline:
			t.Fatalf("directory removal not reported recursively: %+v", r.changes)
		}
	}
}

func TestFileWrittenBeforeSubdirectoryWatchIsCoveredByRecursiveReport(t *testing.T) {
	root := t.TempDir()
	_, r := armed(t, root)

	// The file lands in a nested directory right after it is created, before
	// the watcher can have added a watch for it.
	nested := filepath.Join(root, "a", "b")
	mkdir(t, nested)
	file := filepath.Join(nested, "early.mp4")
	write(t, file)

	// The topmost created directory is reported recursively; it covers the
	// nested one and the file, whichever order the events arrived in.
	waitChange(t, r, Change{Dir: filepath.Join(root, "a"), Recursive: true})
	if _, err := os.Stat(file); err != nil {
		t.Fatal(err)
	}

	// The nested watches exist by then: a later write below reports its parent.
	write(t, filepath.Join(nested, "late.mp4"))
	waitChange(t, r, Change{Dir: nested})
}

func TestArmWatchesExistingTreeAndSkipsExcludedDirectoriesAndSymlinks(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "a", "b")
	mkdir(t, deep)
	hidden := filepath.Join(root, ".hidden")
	mkdir(t, hidden)
	mkdir(t, filepath.Join(root, "@eaDir"))
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skipf("symlink: %v", err)
	}

	w, r := armed(t, root)
	w.mu.Lock()
	s := w.current
	w.mu.Unlock()
	s.mu.Lock()
	got := map[string]bool{}
	for d := range s.dirs {
		got[d] = true
	}
	s.mu.Unlock()

	for _, want := range []string{root, filepath.Join(root, "a"), deep} {
		if !got[want] {
			t.Errorf("%s is not watched; watched %v", want, got)
		}
	}
	for _, skip := range []string{hidden, filepath.Join(root, "@eaDir"), filepath.Join(root, "link")} {
		if got[skip] {
			t.Errorf("%s is watched but must be skipped", skip)
		}
	}

	write(t, filepath.Join(deep, "x.mp4"))
	waitChange(t, r, Change{Dir: deep})
}

func TestExcludedDirectoryNamesMatchTheScanner(t *testing.T) {
	for _, name := range []string{"@eaDir", "#recycle", "lost+found", ".git", ".x"} {
		if !isExcludedDir(name) {
			t.Errorf("%s is not excluded", name)
		}
	}
	for _, name := range []string{"movies", "a.b", "eaDir"} {
		if isExcludedDir(name) {
			t.Errorf("%s is excluded", name)
		}
	}
}

func TestMissingAndNonDirectoryRootsAreUnreachable(t *testing.T) {
	root := t.TempDir()
	missing := filepath.Join(root, "gone")
	file := filepath.Join(root, "file")
	write(t, file)
	good := filepath.Join(root, "good")
	mkdir(t, good)

	_, r := armed(t, missing, file, good)
	if !r.hasProblem(ProblemUnreachable, missing) {
		t.Errorf("missing root not reported: %+v", r.problems)
	}
	if !r.hasProblem(ProblemUnreachable, file) {
		t.Errorf("file root not reported: %+v", r.problems)
	}

	write(t, filepath.Join(good, "x.mp4"))
	waitChange(t, r, Change{Dir: good})
}

func TestUnreadableDirectoryIsReportedAsPermissionProblem(t *testing.T) {
	root := t.TempDir()
	locked := filepath.Join(root, "locked")
	mkdir(t, locked)

	r := newRecorder()
	w := New(r.handler())
	real := w.readDir
	w.readDir = func(name string) ([]fs.DirEntry, error) {
		if name == locked {
			return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrPermission}
		}
		return real(name)
	}
	if err := w.Arm([]string{root}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close() })

	if !r.hasProblem(ProblemPermission, locked) {
		t.Fatalf("unreadable directory not reported: %+v", r.problems)
	}
}

// fakeBackend lets a test inject what the OS would send.
type fakeBackend struct {
	events chan fsnotify.Event
	errs   chan error
	addErr map[string]error
	once   sync.Once

	mu      sync.Mutex
	removed []string
}

func newFakeBackend() *fakeBackend {
	return &fakeBackend{
		events: make(chan fsnotify.Event),
		errs:   make(chan error),
		addErr: map[string]error{},
	}
}

func (f *fakeBackend) Add(name string) error {
	return f.addErr[name]
}
func (f *fakeBackend) Remove(name string) error {
	f.mu.Lock()
	f.removed = append(f.removed, name)
	f.mu.Unlock()
	return nil
}
func (f *fakeBackend) Close() error {
	f.once.Do(func() { close(f.events); close(f.errs) })
	return nil
}
func (f *fakeBackend) Events() <-chan fsnotify.Event { return f.events }
func (f *fakeBackend) Errors() <-chan error          { return f.errs }

func armedFake(t *testing.T, fb *fakeBackend, roots ...string) *recorder {
	t.Helper()
	r := newRecorder()
	w := New(r.handler())
	w.newBackend = func() (backend, error) { return fb, nil }
	if err := w.Arm(roots); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close() })
	return r
}

func TestForcedOverflowIsReported(t *testing.T) {
	fb := newFakeBackend()
	r := armedFake(t, fb, t.TempDir())

	fb.errs <- fsnotify.ErrEventOverflow

	deadline := time.After(waitFor)
	for !r.hasProblem(ProblemOverflow, "") {
		select {
		case <-r.notify:
		case <-deadline:
			t.Fatalf("overflow not reported: %+v", r.problems)
		}
	}
}

func TestWatchLimitIsReportedOnceAndStopsArming(t *testing.T) {
	root := t.TempDir()
	mkdir(t, filepath.Join(root, "a"))
	mkdir(t, filepath.Join(root, "b"))
	fb := newFakeBackend()
	fb.addErr[filepath.Join(root, "a")] = syscall.ENOSPC
	fb.addErr[filepath.Join(root, "b")] = syscall.ENOSPC
	r := armedFake(t, fb, root)

	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, p := range r.problems {
		if p.Kind == ProblemWatchLimit {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("watch limit reported %d times, want 1: %+v", n, r.problems)
	}
}

func TestNoCallbackAfterDisarm(t *testing.T) {
	root := t.TempDir()
	r := newRecorder()
	w := New(r.handler())
	if err := w.Arm([]string{root}); err != nil {
		t.Fatal(err)
	}
	w.Disarm()

	write(t, filepath.Join(root, "a.mp4"))
	time.Sleep(100 * time.Millisecond)
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.changes) != 0 {
		t.Fatalf("change reported after Disarm: %+v", r.changes)
	}
}

func TestArmReplacesEarlierWatches(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	r := newRecorder()
	w := New(r.handler())
	t.Cleanup(func() { _ = w.Close() })
	if err := w.Arm([]string{first}); err != nil {
		t.Fatal(err)
	}
	if err := w.Arm([]string{second}); err != nil {
		t.Fatal(err)
	}

	write(t, filepath.Join(first, "a.mp4"))
	write(t, filepath.Join(second, "b.mp4"))
	waitChange(t, r, Change{Dir: second})
	if r.hasChange(Change{Dir: first}) {
		t.Fatalf("a replaced media folder is still watched: %+v", r.changes)
	}
}

func TestRemovedRootIsReportedUnreachable(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "videos")
	sub := filepath.Join(root, "sub")
	mkdir(t, sub)
	_, r := armed(t, root)

	if err := os.Rename(root, filepath.Join(parent, "old")); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(waitFor)
	for !r.hasProblem(ProblemUnreachable, root) {
		select {
		case <-r.notify:
		case <-deadline:
			t.Fatalf("removed media folder not reported: %+v", r.problems)
		}
	}
	waitChange(t, r, Change{Dir: root, Recursive: true})
}

func TestRenamedDirectoryIsRemovedFromTheBackend(t *testing.T) {
	root := t.TempDir()
	films := filepath.Join(root, "films")
	deep := filepath.Join(films, "deep")
	mkdir(t, deep)
	fb := newFakeBackend()
	r := armedFake(t, fb, root)

	fb.events <- fsnotify.Event{Name: films, Op: fsnotify.Rename}
	waitChange(t, r, Change{Dir: films, Recursive: true})

	fb.mu.Lock()
	defer fb.mu.Unlock()
	got := map[string]bool{}
	for _, p := range fb.removed {
		got[p] = true
	}
	if !got[films] || !got[deep] || got[root] {
		t.Fatalf("backend removals = %v, want films and deep only", fb.removed)
	}
}

func TestDisarmDuringArmLeavesNothingWatching(t *testing.T) {
	root := t.TempDir()
	r := newRecorder()
	w := New(r.handler())
	release := make(chan struct{})
	entered := make(chan struct{})
	real := w.readDir
	var once sync.Once
	w.readDir = func(name string) ([]fs.DirEntry, error) {
		once.Do(func() { close(entered); <-release })
		return real(name)
	}
	done := make(chan struct{})
	go func() { _ = w.Arm([]string{root}); close(done) }()
	<-entered
	w.Disarm()
	close(release)
	<-done

	write(t, filepath.Join(root, "a.mp4"))
	time.Sleep(100 * time.Millisecond)
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.changes) != 0 {
		t.Fatalf("a change was reported after Disarm: %+v", r.changes)
	}
}
