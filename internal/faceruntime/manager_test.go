package faceruntime

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/faceproto"
)

// fileServer serves fixed files and counts the requests it gets.
type fileServer struct {
	*httptest.Server
	requests atomic.Int64
}

func newFileServer(t *testing.T, files map[string][]byte) *fileServer {
	t.Helper()
	s := &fileServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.requests.Add(1)
		body, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(s.Close)
	return s
}

func pinOf(url string, body []byte) Pin {
	sum := sha256.Sum256(body)
	return Pin{URL: url, SHA256: hex.EncodeToString(sum[:]), Size: int64(len(body))}
}

// runtimeArchive is a Linux runtime archive whose helper and loader are placeholders, and the pins of its files;
// the launcher is faked.
func runtimeArchive(t *testing.T) ([]byte, map[string]string) {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	files := map[string]string{}
	for name, mode := range map[string]int64{
		"mdm-face":                 0o755,
		"lib/ld-linux-x86-64.so.2": 0o755,
		"lib/libonnxruntime.so":    0o644,
		"licenses/onnxruntime.txt": 0o644,
	} {
		body := []byte("content of " + name)
		files[name] = pinOf("", body).SHA256
		header := &tar.Header{Name: "face-runtime-linux-amd64/" + name, Mode: mode, Size: int64(len(body)), Typeflag: tar.TypeReg}
		if err := tw.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes(), files
}

// fixture is a catalog served by an httptest server and a launcher that fakes the helper.
type fixture struct {
	dir     string
	server  *fileServer
	catalog Catalog
	model   []byte
	// hello is what the fake helper greets with.
	hello faceproto.Message

	mu       sync.Mutex
	commands []HelperCommand
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	archive, files := runtimeArchive(t)
	detector, model := []byte("yunet model"), []byte("sface model")
	f := &fixture{
		dir:   filepath.Join(t.TempDir(), "face"),
		model: model,
		hello: faceproto.Hello{Version: faceproto.Version, Detector: "yunet_2023mar", Recognizer: "sface_int8", Dim: 128},
	}
	f.server = newFileServer(t, map[string][]byte{"/runtime.tar.gz": archive, "/yunet.onnx": detector, "/sface.onnx": model})
	f.catalog = Catalog{
		Version: 1,
		Platform: &Platform{
			Name: "linux-amd64", Loader: "ld-linux-x86-64.so.2",
			Archive: pinOf(f.server.URL+"/runtime.tar.gz", archive), Files: files,
		},
		Detector: Model{File: "yunet.onnx", Pin: pinOf(f.server.URL+"/yunet.onnx", detector)},
		Recognizers: map[domain.FaceRecognizer]Model{
			domain.FaceRecognizerSFaceInt8: {File: "sface.onnx", Pin: pinOf(f.server.URL+"/sface.onnx", model)},
		},
	}
	return f
}

func (f *fixture) manager() *Manager {
	return New(Options{Dir: f.dir, Catalog: f.catalog, Client: f.server.Client(), Launch: f.launch, Threads: 3})
}

// launch fakes a helper that greets with f.hello and then reads its input until it closes.
func (f *fixture) launch(cmd HelperCommand) (Process, error) {
	f.mu.Lock()
	f.commands = append(f.commands, cmd)
	f.mu.Unlock()
	return pipeProcess(func(in io.Reader, out io.Writer) {
		if faceproto.Write(out, f.hello) == nil {
			_, _ = io.Copy(io.Discard, in)
		}
	}), nil
}

// pipeProcess runs serve as a helper over pipes; Stop closes them, which ends serve.
func pipeProcess(serve func(in io.Reader, out io.Writer)) Process {
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		serve(inR, outW)
		_ = outW.Close()
	}()
	stop := sync.OnceValue(func() error {
		_ = inW.Close()
		_ = inR.Close()
		_ = outR.Close()
		<-done
		return nil
	})
	return Process{In: inW, Out: outR, Stop: stop}
}

func waitDownload(m *Manager) {
	m.mu.Lock()
	d := m.run
	m.mu.Unlock()
	if d != nil {
		<-d.finished
	}
}

// regularFiles lists the files under dir, relative to it.
func regularFiles(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	slices.Sort(out)
	return out
}

func TestStatusWithoutStartMakesNoRequestAndCreatesNothing(t *testing.T) {
	f := newFixture(t)
	m := f.manager()

	m.Check()
	if got := m.Status(domain.FaceRecognizerSFaceInt8); got.State != domain.FaceRuntimeAbsent {
		t.Fatalf("Status = %+v, want absent", got)
	}
	if n := f.server.requests.Load(); n != 0 {
		t.Fatalf("requests = %d, want 0", n)
	}
	if _, err := os.Stat(f.dir); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Stat(%s) = %v, want not exist", f.dir, err)
	}
}

func TestStartEndsReadyWithFilesInPlaceAndStartsTheHelperThroughTheLoader(t *testing.T) {
	f := newFixture(t)
	m := f.manager()

	if err := m.Start(domain.FaceRecognizerSFaceInt8); err != nil {
		t.Fatal(err)
	}
	waitDownload(m)

	if got := m.Status(domain.FaceRecognizerSFaceInt8); got.State != domain.FaceRuntimeReady {
		t.Fatalf("Status = %+v, want ready", got)
	}
	want := []string{
		"models/sface.onnx", "models/yunet.onnx",
		"runtime-v1/lib/ld-linux-x86-64.so.2", "runtime-v1/lib/libonnxruntime.so",
		"runtime-v1/licenses/onnxruntime.txt", "runtime-v1/mdm-face",
	}
	if got := regularFiles(t, f.dir); !slices.Equal(got, want) {
		t.Fatalf("files = %v, want %v", got, want)
	}
	if info, err := os.Stat(filepath.Join(f.dir, "runtime-v1", "mdm-face")); err != nil || info.Mode().Perm()&0o100 == 0 {
		t.Fatalf("mdm-face = %v, %v; want an executable file", info, err)
	}
	rt, models := filepath.Join(f.dir, "runtime-v1"), filepath.Join(f.dir, "models")
	wantCmd := HelperCommand{
		Path: filepath.Join(rt, "lib", "ld-linux-x86-64.so.2"),
		Args: []string{"--library-path", filepath.Join(rt, "lib"), filepath.Join(rt, "mdm-face"),
			"-ort", filepath.Join(rt, "lib", "libonnxruntime.so"),
			"-detector", filepath.Join(models, "yunet.onnx"), "-recognizer", filepath.Join(models, "sface.onnx"),
			"-model", "sface_int8", "-threads", "3"},
	}
	if len(f.commands) != 1 || f.commands[0].Path != wantCmd.Path || !slices.Equal(f.commands[0].Args, wantCmd.Args) {
		t.Fatalf("helper commands = %+v, want one %+v", f.commands, wantCmd)
	}
	// A restart reads the same state from the files alone.
	if got := f.manager().Status(domain.FaceRecognizerSFaceInt8); got.State != domain.FaceRuntimeReady {
		t.Fatalf("Status after a restart = %+v, want ready", got)
	}
}

func TestWrongHashEndsFailedAndLeavesNoFile(t *testing.T) {
	f := newFixture(t)
	f.catalog.Platform.Archive.SHA256 = hex.EncodeToString(make([]byte, sha256.Size))
	m := f.manager()

	if err := m.Start(domain.FaceRecognizerSFaceInt8); err != nil {
		t.Fatal(err)
	}
	waitDownload(m)

	got := m.Status(domain.FaceRecognizerSFaceInt8)
	if got.State != domain.FaceRuntimeFailed || got.Reason != domain.FaceRuntimeHashMismatch {
		t.Fatalf("Status = %+v, want failed with hash_mismatch", got)
	}
	if files := regularFiles(t, f.dir); len(files) != 0 {
		t.Fatalf("files = %v, want none", files)
	}
}

func TestStartupReadsATruncatedModelAsFailed(t *testing.T) {
	f := newFixture(t)
	m := f.manager()
	if err := m.Start(domain.FaceRecognizerSFaceInt8); err != nil {
		t.Fatal(err)
	}
	waitDownload(m)
	model := filepath.Join(f.dir, "models", "sface.onnx")
	if err := os.WriteFile(model, f.model[:3], 0o644); err != nil {
		t.Fatal(err)
	}

	got := f.manager().Status(domain.FaceRecognizerSFaceInt8)
	if got.State != domain.FaceRuntimeFailed || got.Reason != domain.FaceRuntimeHashMismatch {
		t.Fatalf("Status = %+v, want failed with hash_mismatch", got)
	}
}

func TestHelperOfAnotherProtocolVersionFailsTheDownload(t *testing.T) {
	f := newFixture(t)
	f.hello = faceproto.Hello{Version: faceproto.Version + 1, Recognizer: "sface_int8"}
	m := f.manager()

	if err := m.Start(domain.FaceRecognizerSFaceInt8); err != nil {
		t.Fatal(err)
	}
	waitDownload(m)

	got := m.Status(domain.FaceRecognizerSFaceInt8)
	if got.State != domain.FaceRuntimeFailed || got.Reason != domain.FaceRuntimeHelperStart {
		t.Fatalf("Status = %+v, want failed with helper_start", got)
	}
}

func TestRetryAfterHelperStartFailureProbesTheHelperAgain(t *testing.T) {
	f := newFixture(t)
	f.hello = faceproto.Hello{Version: faceproto.Version + 1, Recognizer: "sface_int8"}
	m := f.manager()
	if err := m.Start(domain.FaceRecognizerSFaceInt8); err != nil {
		t.Fatal(err)
	}
	waitDownload(m)
	requests := f.server.requests.Load()

	if err := m.Start(domain.FaceRecognizerSFaceInt8); err != nil {
		t.Fatal(err)
	}
	waitDownload(m)

	got := m.Status(domain.FaceRecognizerSFaceInt8)
	if got.State != domain.FaceRuntimeFailed || got.Reason != domain.FaceRuntimeHelperStart {
		t.Fatalf("Status after a retry = %+v, want failed with helper_start", got)
	}
	if len(f.commands) != 2 {
		t.Fatalf("helper starts = %d, want 2", len(f.commands))
	}
	if n := f.server.requests.Load(); n != requests {
		t.Fatalf("requests after a retry = %d, want %d", n, requests)
	}
}

func TestStartupReadsATamperedRuntimeAsFailed(t *testing.T) {
	for name, tamper := range map[string]func(rt string) error{
		"replaced helper": func(rt string) error {
			return os.WriteFile(filepath.Join(rt, "mdm-face"), []byte("another helper"), 0o755)
		},
		"added library": func(rt string) error {
			return os.WriteFile(filepath.Join(rt, "lib", "libextra.so"), []byte("injected"), 0o644)
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			m := f.manager()
			if err := m.Start(domain.FaceRecognizerSFaceInt8); err != nil {
				t.Fatal(err)
			}
			waitDownload(m)
			if err := tamper(filepath.Join(f.dir, "runtime-v1")); err != nil {
				t.Fatal(err)
			}

			got := f.manager().Status(domain.FaceRecognizerSFaceInt8)
			if got.State != domain.FaceRuntimeFailed || got.Reason != domain.FaceRuntimeHashMismatch {
				t.Fatalf("Status = %+v, want failed with hash_mismatch", got)
			}
		})
	}
}
