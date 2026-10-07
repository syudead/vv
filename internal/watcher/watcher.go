package watcher

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/fsnotify/fsnotify"
)

// Change is one changed directory. Recursive means every file and directory
// below Dir may have changed too (a directory was created, removed or renamed).
type Change struct {
	Dir       string
	Recursive bool
}

// ProblemKind says why the watcher cannot promise to see every change.
type ProblemKind string

const (
	// ProblemOverflow means the OS dropped notifications. Only a full read can
	// say what changed.
	ProblemOverflow ProblemKind = "overflow"
	// ProblemWatchLimit means a watch could not be added because the OS limit
	// on watches or watch instances is reached.
	ProblemWatchLimit ProblemKind = "watch_limit"
	// ProblemPermission means a directory cannot be read or watched.
	ProblemPermission ProblemKind = "permission"
	// ProblemUnreachable means a media folder is missing or is not a directory.
	ProblemUnreachable ProblemKind = "unreachable"
)

// Problem is a reported watch problem. Path is empty for ProblemOverflow.
type Problem struct {
	Kind ProblemKind
	Path string
	Err  error
}

// Handler receives what the watcher sees. Both callbacks run on the watcher's
// own goroutine (Problem may also run inside Arm), and must return quickly.
// They must not call Arm, Disarm or Close.
type Handler struct {
	Change  func(Change)
	Problem func(Problem)
}

// Watcher watches a set of media folders. The zero value is not usable; use New.
type Watcher struct {
	handler Handler

	// readDir and newBackend are replaced by tests.
	readDir    func(string) ([]fs.DirEntry, error)
	newBackend func() (backend, error)

	mu      sync.Mutex
	current *session
}

// backend is the part of fsnotify.Watcher a session uses.
type backend interface {
	Add(name string) error
	Close() error
	Events() <-chan fsnotify.Event
	Errors() <-chan error
}

type fsnotifyBackend struct{ *fsnotify.Watcher }

func (b fsnotifyBackend) Events() <-chan fsnotify.Event { return b.Watcher.Events }
func (b fsnotifyBackend) Errors() <-chan error          { return b.Watcher.Errors }

// New returns a Watcher that watches nothing until Arm is called.
func New(handler Handler) *Watcher {
	return &Watcher{
		handler: handler,
		readDir: os.ReadDir,
		newBackend: func() (backend, error) {
			w, err := fsnotify.NewWatcher()
			if err != nil {
				return nil, err
			}
			return fsnotifyBackend{w}, nil
		},
	}
}

// Arm replaces every watch with watches for the given media folders and every
// directory below them. It reads directory entries only. Problems found while
// arming are reported through the handler, so a media folder that cannot be
// watched does not stop the others. The returned error is set only when no
// watch instance can be created at all; it has been reported as a problem too.
func (w *Watcher) Arm(roots []string) error {
	w.Disarm()

	b, err := w.newBackend()
	if err != nil {
		if w.handler.Problem != nil {
			w.handler.Problem(classify("", err))
		}
		return fmt.Errorf("create watcher: %w", err)
	}
	s := &session{
		w:    w,
		b:    b,
		dirs: map[string]struct{}{},
		done: make(chan struct{}),
	}
	for _, root := range roots {
		s.armRoot(root)
	}

	w.mu.Lock()
	w.current = s
	w.mu.Unlock()
	go s.run()
	return nil
}

// Disarm removes every watch and waits for the event goroutine to finish. No
// callback runs after it returns.
func (w *Watcher) Disarm() {
	w.mu.Lock()
	s := w.current
	w.current = nil
	w.mu.Unlock()
	if s != nil {
		s.stop()
	}
}

// Close is Disarm; the Watcher can be armed again afterwards.
func (w *Watcher) Close() error {
	w.Disarm()
	return nil
}

// session is one arming: its own backend and its own set of watched
// directories.
type session struct {
	w *Watcher
	b backend

	mu      sync.Mutex
	dirs    map[string]struct{}
	stopped bool
	limit   bool // a watch limit was reported; stop adding and stop repeating it

	done chan struct{}
}

func (s *session) stop() {
	s.mu.Lock()
	s.stopped = true
	s.mu.Unlock()
	_ = s.b.Close()
	<-s.done
}

func (s *session) isStopped() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stopped
}

func (s *session) change(c Change) {
	if s.w.handler.Change != nil && !s.isStopped() {
		s.w.handler.Change(c)
	}
}

func (s *session) problem(p Problem) {
	if s.w.handler.Problem != nil && !s.isStopped() {
		s.w.handler.Problem(p)
	}
}

// armRoot watches root and everything below it. A missing or non-directory
// root is reported unreachable.
func (s *session) armRoot(root string) {
	info, err := os.Lstat(root)
	if err != nil {
		s.problem(Problem{Kind: kindFor(err, ProblemUnreachable), Path: root, Err: err})
		return
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		s.problem(Problem{Kind: ProblemUnreachable, Path: root,
			Err: fmt.Errorf("%s is not a directory", root)})
		return
	}
	s.addTree(root)
}

// addTree adds a watch for dir and every directory below it, skipping excluded
// directories and symbolic links. A directory's watch is added before its
// entries are listed, so nothing created after the watch exists is missed.
func (s *session) addTree(dir string) {
	s.mu.Lock()
	stop := s.limit || s.stopped
	s.mu.Unlock()
	if stop {
		return
	}
	if err := s.b.Add(dir); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return // removed while arming; its parent's event covers it
		}
		p := classify(dir, err)
		if p.Kind == ProblemWatchLimit {
			s.mu.Lock()
			already := s.limit
			s.limit = true
			s.mu.Unlock()
			if already {
				return
			}
		}
		s.problem(p)
		return
	}
	s.mu.Lock()
	s.dirs[dir] = struct{}{}
	s.mu.Unlock()

	entries, err := s.w.readDir(dir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		s.problem(classify(dir, err))
	}
	for _, e := range entries {
		if !e.IsDir() || e.Type()&os.ModeSymlink != 0 || isExcludedDir(e.Name()) {
			continue
		}
		s.addTree(filepath.Join(dir, e.Name()))
	}
}

func (s *session) run() {
	defer close(s.done)
	events, errs := s.b.Events(), s.b.Errors()
	for events != nil || errs != nil {
		select {
		case ev, ok := <-events:
			if !ok {
				events = nil
				continue
			}
			s.handleEvent(ev)
		case err, ok := <-errs:
			if !ok {
				errs = nil
				continue
			}
			s.handleError(err)
		}
	}
}

func (s *session) handleError(err error) {
	s.problem(classify("", err))
}

// handleEvent maps one event to a changed directory (R-2). A file event marks
// its parent; an event on a directory marks that directory recursively. A
// created directory is watched first and reported after, so a file written
// before its watch existed is covered by the recursive report.
func (s *session) handleEvent(ev fsnotify.Event) {
	path := filepath.Clean(ev.Name)
	switch {
	case ev.Has(fsnotify.Create):
		info, err := os.Lstat(path)
		if err == nil && info.IsDir() {
			if isExcludedDir(filepath.Base(path)) {
				return
			}
			s.addTree(path)
			s.change(Change{Dir: path, Recursive: true})
			return
		}
		s.change(Change{Dir: filepath.Dir(path)})
	case ev.Has(fsnotify.Remove) || ev.Has(fsnotify.Rename):
		if s.forget(path) {
			s.change(Change{Dir: path, Recursive: true})
			return
		}
		s.change(Change{Dir: filepath.Dir(path)})
	case ev.Has(fsnotify.Write):
		s.change(Change{Dir: filepath.Dir(path)})
	}
}

// forget drops path and every watched directory below it. It reports whether
// path was a watched directory.
func (s *session) forget(path string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, was := s.dirs[path]
	prefix := path + string(filepath.Separator)
	for d := range s.dirs {
		if d == path || strings.HasPrefix(d, prefix) {
			delete(s.dirs, d)
		}
	}
	return was
}

// classify turns an error into a problem for path.
func classify(path string, err error) Problem {
	return Problem{Kind: kindFor(err, ProblemUnreachable), Path: path, Err: err}
}

// kindFor picks the problem kind for err; other is used when err names no
// known cause.
func kindFor(err error, other ProblemKind) ProblemKind {
	switch {
	case errors.Is(err, fsnotify.ErrEventOverflow):
		return ProblemOverflow
	case errors.Is(err, syscall.ENOSPC), errors.Is(err, syscall.EMFILE), errors.Is(err, syscall.ENFILE):
		return ProblemWatchLimit
	case errors.Is(err, fs.ErrPermission):
		return ProblemPermission
	default:
		return other
	}
}

// excludedNames mirrors the directories the scanner skips (internal/scanner
// excludedNames). Adapters may not import each other, so the list is kept in
// step by hand and the test pins the names.
var excludedNames = map[string]struct{}{
	"@eaDir":     {},
	"#recycle":   {},
	"lost+found": {},
}

// isExcludedDir reports whether a directory is skipped, as the scanner does.
func isExcludedDir(name string) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	_, ok := excludedNames[name]
	return ok
}
