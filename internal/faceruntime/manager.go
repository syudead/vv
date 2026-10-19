package faceruntime

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// downloadTimeout bounds one whole download; the largest model is 174 MB.
const downloadTimeout = 2 * time.Hour

// Options configure a Manager.
type Options struct {
	// Dir is <data>/face; nothing under it is created until a download starts.
	Dir     string
	Catalog Catalog
	// Client downloads the files; nil uses the process's proxy environment (HTTPS_PROXY), never the plugin proxy.
	Client *http.Client
	// Launch starts the helper, normally an ExecLauncher.
	Launch Launcher
	// Threads is onnxruntime's thread count; 0 uses half of the CPU cores, at least one.
	Threads int
	// Nil uses slog.Default.
	Logger *slog.Logger
}

// Manager keeps the runtime and model files under Dir: it reads their state at startup, downloads what a
// recognizer needs, and starts the helper. The state is never stored; the files and their hashes are the state.
type Manager struct {
	opts   Options
	logger *slog.Logger
	check  sync.Once

	mu sync.Mutex
	// files holds the verified state of each item by its key.
	files map[string]fileState
	run   *download
	// failed is the last download's or start's failure, for the recognizer it was for.
	failed *recognizerFailure
}

type download struct {
	recognizer domain.FaceRecognizer
	cancel     context.CancelFunc
	finished   chan struct{}
	done       atomic.Int64
	total      int64
}

type recognizerFailure struct {
	recognizer domain.FaceRecognizer
	reason     domain.FaceRuntimeFailure
}

// item is one thing a recognizer needs: the runtime archive or a model.
type item struct {
	key string
	pin Pin
	// model is the model's file name, empty for the runtime.
	model string
}

const runtimeKey = "runtime"

// New does not touch the files until Check, Status or Start.
func New(opts Options) *Manager {
	if opts.Client == nil {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.Proxy = http.ProxyFromEnvironment
		opts.Client = &http.Client{Transport: transport, Timeout: downloadTimeout}
	}
	if opts.Threads <= 0 {
		opts.Threads = max(1, runtime.NumCPU()/2)
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Manager{opts: opts, logger: logger.With(slog.String("component", "face-runtime"))}
}

// Supported reports whether a runtime is published for this platform.
func (m *Manager) Supported() bool { return m.opts.Catalog.Platform != nil }

// Recognizers lists what each recognizer downloads, in the domain table's order.
func (m *Manager) Recognizers() []domain.FaceRecognizerDownload {
	var out []domain.FaceRecognizerDownload
	for _, r := range []domain.FaceRecognizer{domain.FaceRecognizerSFaceInt8, domain.FaceRecognizerArcFaceR50} {
		model, ok := m.opts.Catalog.Recognizers[r]
		if !ok {
			continue
		}
		out = append(out, domain.FaceRecognizerDownload{
			Recognizer: r, DownloadBytes: model.Size, License: model.License, NonCommercial: model.NonCommercial,
		})
	}
	return out
}

func (m *Manager) runtimeDir() string {
	return filepath.Join(m.opts.Dir, "runtime-v"+strconv.Itoa(m.opts.Catalog.Version))
}

func (m *Manager) modelsDir() string { return filepath.Join(m.opts.Dir, "models") }

func (m *Manager) tmpDir() string { return filepath.Join(m.opts.Dir, ".tmp") }

// items lists what recognizer r needs, the runtime first.
func (m *Manager) items(r domain.FaceRecognizer) []item {
	c := m.opts.Catalog
	out := []item{{key: runtimeKey, pin: c.Platform.Archive}, {key: c.Detector.File, pin: c.Detector.Pin, model: c.Detector.File}}
	if model, ok := c.Recognizers[r]; ok {
		out = append(out, item{key: model.File, pin: model.Pin, model: model.File})
	}
	return out
}

// Check verifies every file the catalog knows by its hash, once; it reads but never creates files.
func (m *Manager) Check() {
	m.check.Do(func() {
		if !m.Supported() {
			return
		}
		files := map[string]fileState{
			runtimeKey: verifyRuntime(m.runtimeDir(), m.opts.Catalog.Platform.Files),
		}
		models := append([]Model{m.opts.Catalog.Detector}, mapValues(m.opts.Catalog.Recognizers)...)
		for _, model := range models {
			files[model.File] = verifyModel(filepath.Join(m.modelsDir(), model.File), model.SHA256)
		}
		m.mu.Lock()
		m.files = files
		m.mu.Unlock()
	})
}

func mapValues(models map[domain.FaceRecognizer]Model) []Model {
	out := make([]Model, 0, len(models))
	for _, model := range models {
		out = append(out, model)
	}
	return out
}

// Status is the state of what recognizer r needs.
func (m *Manager) Status(r domain.FaceRecognizer) domain.FaceRuntimeStatus {
	if !m.Supported() {
		return domain.FaceRuntimeStatus{State: domain.FaceRuntimeUnsupported}
	}
	m.Check()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.run != nil && m.run.recognizer == r {
		return domain.FaceRuntimeStatus{
			State: domain.FaceRuntimeDownloading, BytesDone: m.run.done.Load(), BytesTotal: m.run.total,
		}
	}
	if m.failed != nil && m.failed.recognizer == r {
		return domain.FaceRuntimeStatus{State: domain.FaceRuntimeFailed, Reason: m.failed.reason}
	}
	state := domain.FaceRuntimeStatus{State: domain.FaceRuntimeReady}
	for _, it := range m.items(r) {
		switch m.files[it.key] {
		case fileBad:
			return domain.FaceRuntimeStatus{State: domain.FaceRuntimeFailed, Reason: domain.FaceRuntimeHashMismatch}
		case fileAbsent:
			state.State = domain.FaceRuntimeAbsent
		}
	}
	return state
}

// Start downloads in the background whatever recognizer r needs that is absent or does not match its hash, then
// starts the helper once to check its handshake. A download for another recognizer is cancelled first; one for r
// keeps running.
func (m *Manager) Start(r domain.FaceRecognizer) error {
	if !m.Supported() {
		return domain.ErrFaceRecognitionUnsupported
	}
	m.Check()
	m.mu.Lock()
	if m.run != nil && m.run.recognizer == r {
		m.mu.Unlock()
		return nil
	}
	m.mu.Unlock()
	m.Stop()

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.run != nil {
		// Another Start won the race; the app serializes its calls, so this one is for the same recognizer.
		return nil
	}
	// A retry after a failure for r probes the helper again even when every file is in place, since a helper_start
	// failure leaves the files verified.
	retry := m.failed != nil && m.failed.recognizer == r
	m.failed = nil
	var missing []item
	for _, it := range m.items(r) {
		if m.files[it.key] != fileOK {
			missing = append(missing, it)
		}
	}
	if len(missing) == 0 && !retry {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	d := &download{recognizer: r, cancel: cancel, finished: make(chan struct{})}
	for _, it := range missing {
		d.total += it.pin.Size
	}
	m.run = d
	go m.download(ctx, d, missing)
	return nil
}

// Stop cancels a running download and waits for it; the files already verified stay.
func (m *Manager) Stop() {
	m.mu.Lock()
	d := m.run
	m.mu.Unlock()
	if d == nil {
		return
	}
	d.cancel()
	<-d.finished
}

func (m *Manager) download(ctx context.Context, d *download, missing []item) {
	defer close(d.finished)
	defer d.cancel()
	err := m.fetchAll(ctx, d, missing)
	if err == nil {
		err = m.probe(ctx, d.recognizer)
	}
	if rmErr := os.RemoveAll(m.tmpDir()); rmErr != nil {
		m.logger.Warn("could not remove the face runtime's temporary files", slog.Any("error", rmErr))
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.run = nil
	switch {
	case ctx.Err() != nil:
	case err != nil:
		m.logger.Warn("the face runtime download failed", slog.Any("error", err))
		m.failed = &recognizerFailure{recognizer: d.recognizer, reason: reasonOf(err)}
	default:
		m.logger.Info("the face runtime is ready", slog.String("recognizer", string(d.recognizer)))
	}
}

func (m *Manager) fetchAll(ctx context.Context, d *download, missing []item) error {
	if err := os.MkdirAll(m.tmpDir(), 0o755); err != nil {
		return fail(domain.FaceRuntimeDisk, err)
	}
	for _, it := range missing {
		if err := m.fetchItem(ctx, d, it); err != nil {
			return err
		}
		m.mu.Lock()
		m.files[it.key] = fileOK
		m.mu.Unlock()
	}
	return nil
}

// fetchItem downloads one item under .tmp and moves it into place once its hash matches.
func (m *Manager) fetchItem(ctx context.Context, d *download, it item) error {
	tmp := filepath.Join(m.tmpDir(), it.key+".download")
	if err := fetch(ctx, m.opts.Client, it.pin, tmp, func(n int64) { d.done.Add(n) }); err != nil {
		return errors.Join(err, os.Remove(tmp))
	}
	if it.model != "" {
		if err := os.MkdirAll(m.modelsDir(), 0o755); err != nil {
			return fail(domain.FaceRuntimeDisk, err)
		}
		return diskErr(os.Rename(tmp, filepath.Join(m.modelsDir(), it.model)))
	}
	staged := filepath.Join(m.tmpDir(), filepath.Base(m.runtimeDir()))
	root := "face-runtime-" + m.opts.Catalog.Platform.Name
	if err := unpack(tmp, m.opts.Catalog.Platform.zipped(), root, staged); err != nil {
		return fail(domain.FaceRuntimeDisk, err)
	}
	if verifyRuntime(staged, m.opts.Catalog.Platform.Files) != fileOK {
		return fail(domain.FaceRuntimeHashMismatch, errors.New("the unpacked runtime does not match its pinned files"))
	}
	if err := os.RemoveAll(m.runtimeDir()); err != nil {
		return fail(domain.FaceRuntimeDisk, err)
	}
	return diskErr(os.Rename(staged, m.runtimeDir()))
}

func diskErr(err error) error {
	if err == nil {
		return nil
	}
	return fail(domain.FaceRuntimeDisk, err)
}

// probe starts the helper once, so a runtime of another protocol version fails the download, not an analysis.
func (m *Manager) probe(ctx context.Context, r domain.FaceRecognizer) error {
	h := m.newHelper(r, nil)
	if err := h.Start(ctx); err != nil {
		return fail(domain.FaceRuntimeHelperStart, err)
	}
	if err := h.Close(); err != nil {
		m.logger.Warn("the face helper did not stop cleanly", slog.Any("error", err))
	}
	return nil
}

// Helper returns a client of recognizer r's helper; a start that fails marks the runtime failed.
func (m *Manager) Helper(r domain.FaceRecognizer) *Helper {
	return m.newHelper(r, func(err error) {
		m.logger.Warn("the face helper did not start", slog.Any("error", err))
		m.mu.Lock()
		defer m.mu.Unlock()
		m.failed = &recognizerFailure{recognizer: r, reason: domain.FaceRuntimeHelperStart}
	})
}

func (m *Manager) newHelper(r domain.FaceRecognizer, startFailed func(error)) *Helper {
	return &Helper{command: m.helperCommand(r), launch: m.opts.Launch, recognizer: r, startFailed: startFailed}
}

// helperCommand starts mdm-face through the bundled loader on Linux, so it runs under the runtime's glibc on any
// host, Alpine included; on Windows it starts mdm-face.exe.
func (m *Manager) helperCommand(r domain.FaceRecognizer) HelperCommand {
	p := m.opts.Catalog.Platform
	rt := m.runtimeDir()
	args := []string{
		"-detector", filepath.Join(m.modelsDir(), m.opts.Catalog.Detector.File),
		"-recognizer", filepath.Join(m.modelsDir(), m.opts.Catalog.Recognizers[r].File),
		"-model", string(r),
		"-threads", strconv.Itoa(m.opts.Threads),
	}
	if p.windows() {
		return HelperCommand{
			Path: filepath.Join(rt, "mdm-face.exe"),
			Args: append([]string{"-ort", filepath.Join(rt, "onnxruntime.dll")}, args...),
		}
	}
	lib := filepath.Join(rt, "lib")
	return HelperCommand{
		Path: filepath.Join(lib, p.Loader),
		Args: slices.Concat([]string{"--library-path", lib, filepath.Join(rt, "mdm-face"),
			"-ort", filepath.Join(lib, "libonnxruntime.so")}, args),
	}
}
