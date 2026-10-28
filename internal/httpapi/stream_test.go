package httpapi

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

func streamFixture(t *testing.T, name string, size int) (string, domain.Video, []byte) {
	t.Helper()

	mediaDir := t.TempDir()
	path := filepath.Join(mediaDir, name)

	content := make([]byte, size)
	for i := range content {
		content[i] = byte(i % 251)
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	video := sampleVideo(1, "海辺の散歩")
	video.Path = path
	video.SizeBytes = int64(size)
	video.Container = domain.ContainerFromPath(path)

	return mediaDir, video, content
}

func streamServer(t *testing.T, mediaDir string, video domain.Video) http.Handler {
	t.Helper()

	return newTestServer(t, Options{
		Videos: &fakeLibrary{videos: map[int64]domain.Video{video.ID: video}, roots: []string{mediaDir}},
	})
}

func rangeRequest(t *testing.T, handler http.Handler, target, rangeHeader string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, target, nil)
	if rangeHeader != "" {
		req.Header.Set("Range", rangeHeader)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

// Test that the handler passes the right seeker and modification time; standard-library Range semantics are not reimplemented here.
func TestStreamServesRanges(t *testing.T) {
	const size = 4096
	mediaDir, video, content := streamFixture(t, "a.mp4", size)
	handler := streamServer(t, mediaDir, video)

	tests := []struct {
		name      string
		header    string
		wantStart int
		wantEnd   int
	}{
		{"先頭", "bytes=0-1023", 0, 1023},
		{"途中", "bytes=1024-2047", 1024, 2047},
		{"末尾", fmt.Sprintf("bytes=%d-", size-512), size - 512, size - 1},
		{"末尾からの相対指定", "bytes=-512", size - 512, size - 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := rangeRequest(t, handler, "/api/videos/1/stream", tc.header)

			if rec.Code != http.StatusPartialContent {
				t.Fatalf("status = %d, want 206: %s", rec.Code, rec.Body)
			}
			wantRange := fmt.Sprintf("bytes %d-%d/%d", tc.wantStart, tc.wantEnd, size)
			if got := rec.Header().Get("Content-Range"); got != wantRange {
				t.Errorf("Content-Range = %q, want %q", got, wantRange)
			}
			if got := rec.Body.Bytes(); string(got) != string(content[tc.wantStart:tc.wantEnd+1]) {
				t.Errorf("本文が要求した範囲と違う（%d バイト）", len(got))
			}
		})
	}
}

func TestStreamServesWholeFile(t *testing.T) {
	const size = 2048
	mediaDir, video, content := streamFixture(t, "a.mp4", size)
	handler := streamServer(t, mediaDir, video)

	rec := rangeRequest(t, handler, "/api/videos/1/stream", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("Accept-Ranges"); got != "bytes" {
		t.Errorf("Accept-Ranges = %q, want bytes", got)
	}
	if rec.Body.Len() != len(content) {
		t.Errorf("本文 = %d バイト, want %d", rec.Body.Len(), len(content))
	}
}

// foregroundLog records foreground reads and response writes in the order they happen.
type foregroundLog struct {
	events []string
}

func (l *foregroundLog) ReadInForeground(path string) func() {
	l.events = append(l.events, "open "+path)
	return func() { l.events = append(l.events, "end") }
}

type loggingRecorder struct {
	*httptest.ResponseRecorder
	log *foregroundLog
}

func (r loggingRecorder) Write(p []byte) (int, error) {
	r.log.events = append(r.log.events, "write")
	return r.ResponseRecorder.Write(p)
}

// The location links to a file in a nested folder, which may sit on another disk, so the held path is the link target.
func TestStreamHoldsItsDiskWhileServing(t *testing.T) {
	mediaDir, video, _ := streamFixture(t, "a.mp4", 1024)
	target := filepath.Join(mediaDir, "slow", "a.mp4")
	if err := os.Mkdir(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(video.Path, target); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, video.Path); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	resolved, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}
	log := &foregroundLog{}
	handler := newTestServer(t, Options{
		Videos:     &fakeLibrary{videos: map[int64]domain.Video{video.ID: video}, roots: []string{mediaDir}},
		Foreground: log,
	})

	rec := loggingRecorder{ResponseRecorder: httptest.NewRecorder(), log: log}
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/videos/1/stream", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}
	if want := []string{"open " + resolved, "write", "end"}; !slices.Equal(log.events, want) {
		t.Fatalf("events = %q, want %q", log.events, want)
	}
}

func TestStreamRejectsUnsatisfiableRange(t *testing.T) {
	mediaDir, video, _ := streamFixture(t, "a.mp4", 1024)
	handler := streamServer(t, mediaDir, video)

	rec := rangeRequest(t, handler, "/api/videos/1/stream", "bytes=99999999999-")
	if rec.Code != http.StatusRequestedRangeNotSatisfiable {
		t.Errorf("status = %d, want 416", rec.Code)
	}
	// ServeContent can replace success with 416, so success cache headers must not survive.
	if got := rec.Header().Get("Cache-Control"); got != cacheNoStore {
		t.Errorf("416 の Cache-Control = %q, want %q", got, cacheNoStore)
	}
}

// ReaderFrom is the real HTTP response's sendfile entry point.
type readerFromRecorder struct {
	*httptest.ResponseRecorder
	readFromCalls int
}

func (r *readerFromRecorder) ReadFrom(src io.Reader) (int64, error) {
	r.readFromCalls++
	return io.Copy(r.ResponseRecorder, src)
}

// Ensure wrappers preserve ReaderFrom to avoid userspace copies.
func TestStreamPassesReadFromToUnderlyingWriter(t *testing.T) {
	const size = 64 * 1024
	mediaDir, video, content := streamFixture(t, "a.mp4", size)
	handler := streamServer(t, mediaDir, video)

	for _, rangeHeader := range []string{"", "bytes=1024-4095"} {
		rec := &readerFromRecorder{ResponseRecorder: httptest.NewRecorder()}
		req := httptest.NewRequest(http.MethodGet, "/api/videos/1/stream", nil)
		if rangeHeader != "" {
			req.Header.Set("Range", rangeHeader)
		}
		handler.ServeHTTP(rec, req)

		if rec.readFromCalls == 0 {
			t.Errorf("Range %q: 下の ResponseWriter の ReadFrom が呼ばれていない", rangeHeader)
		}
		want := content
		if rangeHeader != "" {
			want = content[1024:4096]
		}
		if !bytes.Equal(rec.Body.Bytes(), want) {
			t.Errorf("Range %q: 本文 = %d バイト, want %d", rangeHeader, rec.Body.Len(), len(want))
		}
	}
}

func TestErrorCacheWriterReadFromWithoutReaderFrom(t *testing.T) {
	rec := httptest.NewRecorder()
	writer := &errorCacheWriter{ResponseWriter: struct{ http.ResponseWriter }{rec}}
	n, err := writer.ReadFrom(strings.NewReader("hello"))
	if err != nil || n != 5 || rec.Body.String() != "hello" {
		t.Fatalf("ReadFrom = %d, %v; body %q", n, err, rec.Body.String())
	}
}

func TestStreamHonorsIfRange(t *testing.T) {
	const size = 2048
	mediaDir, video, _ := streamFixture(t, "a.mp4", size)
	handler := streamServer(t, mediaDir, video)

	first := rangeRequest(t, handler, "/api/videos/1/stream", "")
	lastModified := first.Header().Get("Last-Modified")
	if lastModified == "" {
		t.Fatal("Last-Modified が付いていない（ModTime を渡していない）")
	}

	req := httptest.NewRequest(http.MethodGet, "/api/videos/1/stream", nil)
	req.Header.Set("Range", "bytes=0-99")
	req.Header.Set("If-Range", lastModified)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusPartialContent {
		t.Errorf("一致する If-Range で status = %d, want 206", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/videos/1/stream", nil)
	req.Header.Set("Range", "bytes=0-99")
	req.Header.Set("If-Range", "Sun, 01 Jan 2006 00:00:00 GMT")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("一致しない If-Range で status = %d, want 200（全体）", rec.Code)
	}
}

func TestStreamContentType(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"a.mp4", "video/mp4"},
		{"a.m4v", "video/mp4"},
		{"a.webm", "video/webm"},
		{"a.mkv", "application/octet-stream"},
		{"a.avi", "application/octet-stream"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mediaDir, video, _ := streamFixture(t, tc.name, 512)
			handler := streamServer(t, mediaDir, video)

			rec := rangeRequest(t, handler, "/api/videos/1/stream", "")
			if got := rec.Header().Get("Content-Type"); got != tc.want {
				t.Errorf("Content-Type = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestStreamCacheControl(t *testing.T) {
	mediaDir, video, _ := streamFixture(t, "a.mp4", 512)
	handler := streamServer(t, mediaDir, video)

	rec := rangeRequest(t, handler, "/api/videos/1/stream", "")
	if got := rec.Header().Get("Cache-Control"); got != cacheStream {
		t.Errorf("Cache-Control = %q, want %q", got, cacheStream)
	}
}

func TestStreamServesUnplayableFormats(t *testing.T) {
	mediaDir, video, _ := streamFixture(t, "a.mkv", 512)
	video.Playable = false
	video.UnplayableReason = domain.ReasonContainer
	handler := streamServer(t, mediaDir, video)

	rec := rangeRequest(t, handler, "/api/videos/1/stream", "")
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200（再生可否と配信は別）", rec.Code)
	}
}

func TestStreamRefusesPathsOutsideMediaDir(t *testing.T) {
	mediaDir, video, _ := streamFixture(t, "a.mp4", 512)

	outside := filepath.Join(t.TempDir(), "秘密.mp4")
	if err := os.WriteFile(outside, []byte("外のファイル"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		path string
	}{
		{"絶対パスで外を指す", outside},
		{"相対で外へ出る", filepath.Join(mediaDir, "..", filepath.Base(outside))},
		{"接頭辞が似ているだけの別ディレクトリ", mediaDir + "-other/a.mp4"},
		{"MediaDir そのもの", mediaDir},
		{"空", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			outsideVideo := video
			outsideVideo.Path = tc.path
			handler := streamServer(t, mediaDir, outsideVideo)

			rec := rangeRequest(t, handler, "/api/videos/1/stream", "")
			if rec.Code != http.StatusNotFound {
				t.Errorf("status = %d, want 404（存在を漏らさない）", rec.Code)
			}
			if rec.Code == http.StatusForbidden {
				t.Error("403 は存在を漏らす")
			}
		})
	}
}

func TestStreamRefusesSymlinkEscape(t *testing.T) {
	mediaDir, video, _ := streamFixture(t, "a.mp4", 512)

	outsideDir := t.TempDir()
	outside := filepath.Join(outsideDir, "秘密.mp4")
	if err := os.WriteFile(outside, []byte("外のファイル"), 0o600); err != nil {
		t.Fatal(err)
	}

	link := filepath.Join(mediaDir, "link.mp4")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("シンボリックリンクを作れない: %v", err)
	}

	video.Path = link
	handler := streamServer(t, mediaDir, video)

	rec := rangeRequest(t, handler, "/api/videos/1/stream", "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestStreamAllowsSymlinkInsideMediaDir(t *testing.T) {
	mediaDir, video, _ := streamFixture(t, "a.mp4", 512)

	link := filepath.Join(mediaDir, "別名.mp4")
	if err := os.Symlink(filepath.Join(mediaDir, "a.mp4"), link); err != nil {
		t.Skipf("シンボリックリンクを作れない: %v", err)
	}

	video.Path = link
	handler := streamServer(t, mediaDir, video)

	if rec := rangeRequest(t, handler, "/api/videos/1/stream", ""); rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestStreamRefusesNonRegularFiles(t *testing.T) {
	mediaDir, video, _ := streamFixture(t, "a.mp4", 512)

	dir := filepath.Join(mediaDir, "ディレクトリ.mp4")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	video.Path = dir
	handler := streamServer(t, mediaDir, video)

	if rec := rangeRequest(t, handler, "/api/videos/1/stream", ""); rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestStreamHandlesMissingFile(t *testing.T) {
	mediaDir, video, _ := streamFixture(t, "a.mp4", 512)
	if err := os.Remove(video.Path); err != nil {
		t.Fatal(err)
	}
	handler := streamServer(t, mediaDir, video)

	rec := rangeRequest(t, handler, "/api/videos/1/stream", "")
	assertErrorBody(t, "消えた実体", rec.Code, rec.Body.Bytes(),
		wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound, reason: reasonFileUnavailable})
}

func TestStreamForMissingVideo(t *testing.T) {
	handler := newTestServer(t, Options{Videos: &fakeLibrary{roots: []string{t.TempDir()}}})

	if rec := rangeRequest(t, handler, "/api/videos/999/stream", ""); rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestStreamWithoutMediaDir(t *testing.T) {
	_, video, _ := streamFixture(t, "a.mp4", 512)
	handler := newTestServer(t, Options{
		Videos: &fakeLibrary{videos: map[int64]domain.Video{1: video}},
	})

	rec := rangeRequest(t, handler, "/api/videos/1/stream", "")
	if rec.Code == http.StatusOK {
		t.Error("MediaDir が無いのに配信した")
	}
}

func TestStreamPathStaysUnderAPI(t *testing.T) {
	mediaDir, video, _ := streamFixture(t, "a.mp4", 512)
	handler := streamServer(t, mediaDir, video)

	rec := rangeRequest(t, handler, "/api/videos/1/stream/extra", "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "<html") {
		t.Error("/api/ 配下で HTML が返った")
	}
}

func TestStreamWithoutMediaFilesIsInternalError(t *testing.T) {
	mediaDir, video, _ := streamFixture(t, "movie.mp4", 16)
	handler := NewRouter(Options{
		Videos: &fakeLibrary{videos: map[int64]domain.Video{video.ID: video}, roots: []string{mediaDir}},
		Assets: emptyAssets{},
		Auth:   ownerAuth{},
	})
	if rec := do(t, handler, http.MethodGet, "/api/videos/1/stream"); rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500: %s", rec.Code, rec.Body)
	}
}
