package httpapi

import (
	"errors"
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

// openFixture は実在するファイルを代表の所在に持つ動画を返す。
func openFixture(t *testing.T) (domain.Video, *fakeLibrary) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "movie.mp4")
	if err := os.WriteFile(path, []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}
	video := sampleVideo(1, "movie")
	video.Path = path
	return video, &fakeLibrary{videos: map[int64]domain.Video{1: video}}
}

func openRequest(target, remote, host string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(`{"path":"/etc/passwd"}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = remote
	req.Host = host
	return req
}

func assertErrorCode(t *testing.T, rec *httptest.ResponseRecorder, status int, code gen.ErrorCode) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d: %s", rec.Code, status, rec.Body)
	}
	if got := decode[gen.Error](t, rec); got.Code != code {
		t.Fatalf("code = %q, want %q", got.Code, code)
	}
}

// ループバックから、ループバックの Host で来た要求だけが 204 を得る。opener には、
// 要求の内容に関わらず代表の所在だけが渡る。
func TestOpenVideoFileOpensRepresentativeLocation(t *testing.T) {
	video, library := openFixture(t)
	opener := &fakeOpener{available: true}
	handler := newTestServer(t, Options{Videos: library, Opener: opener})

	for _, req := range []*http.Request{
		openRequest("/api/videos/1/open?path=/etc/passwd", "127.0.0.1:50000", "localhost:8080"),
		openRequest("/api/videos/1/open", "[::1]:50000", "[::1]:8080"),
		openRequest("/api/videos/1/open", "127.0.0.1:50000", "127.0.0.1"),
	} {
		rec := serve(handler, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("%s %s: status = %d, want 204: %s", req.RemoteAddr, req.Host, rec.Code, rec.Body)
		}
	}
	if want := []string{video.Path, video.Path, video.Path}; !slices.Equal(opener.opened, want) {
		t.Fatalf("opened = %q, want 代表の所在だけ %q", opener.opened, want)
	}
}

// ループバック以外の要求と、Host がループバックの名前でない要求は 403 になる。
func TestOpenVideoFileRejectsNonLoopback(t *testing.T) {
	_, library := openFixture(t)
	opener := &fakeOpener{available: true}
	handler := newTestServer(t, Options{Videos: library, Opener: opener})

	for _, tc := range []struct{ remote, host string }{
		{remote: "192.168.1.20:50000", host: "192.168.1.10:8080"},
		{remote: "192.168.1.20:50000", host: "localhost:8080"},
		// DNS rebinding で 127.0.0.1 を指させた他サイトのページ。
		{remote: "127.0.0.1:50000", host: "evil.example:8080"},
		{remote: "127.0.0.1:50000", host: "192.168.1.10:8080"},
	} {
		rec := serve(handler, openRequest("/api/videos/1/open", tc.remote, tc.host))
		assertErrorCode(t, rec, http.StatusForbidden, codeForbidden)
	}
	if len(opener.opened) != 0 {
		t.Fatalf("opened = %q, want none", opener.opened)
	}
}

// 開けない環境では、要求元を問わず 409 open_unavailable になる（403 より先）。
func TestOpenVideoFileUnavailableRegardlessOfRequester(t *testing.T) {
	_, library := openFixture(t)
	for _, opts := range []Options{
		{Videos: library, Opener: &fakeOpener{available: false}},
		{Videos: library},
	} {
		handler := newTestServer(t, opts)
		for _, remote := range []string{"127.0.0.1:1", "192.168.1.20:1"} {
			rec := serve(handler, openRequest("/api/videos/1/open", remote, "localhost:8080"))
			assertErrorCode(t, rec, http.StatusConflict, codeOpenUnavailable)
		}
	}
}

// ファイルが無いときは 409 file_missing になる。要求元の判定（403）の後である。
func TestOpenVideoFileReportsMissingFile(t *testing.T) {
	video, library := openFixture(t)
	if err := os.Remove(video.Path); err != nil {
		t.Fatal(err)
	}
	opener := &fakeOpener{available: true}
	handler := newTestServer(t, Options{Videos: library, Opener: opener})

	rec := serve(handler, openRequest("/api/videos/1/open", "127.0.0.1:1", "localhost"))
	assertErrorCode(t, rec, http.StatusConflict, codeFileMissing)

	rec = serve(handler, openRequest("/api/videos/1/open", "192.168.1.20:1", "localhost"))
	assertErrorCode(t, rec, http.StatusForbidden, codeForbidden)
	if len(opener.opened) != 0 {
		t.Fatalf("opened = %q, want none", opener.opened)
	}
}

// 知らない id は、開けない環境でも要求元を問わず 404 になる。
func TestOpenVideoFileUnknownVideo(t *testing.T) {
	_, library := openFixture(t)
	for _, opener := range []*fakeOpener{{available: true}, {available: false}} {
		handler := newTestServer(t, Options{Videos: library, Opener: opener})
		rec := serve(handler, openRequest("/api/videos/99/open", "192.168.1.20:1", "example.com"))
		assertErrorCode(t, rec, http.StatusNotFound, codeNotFound)
	}
}

func TestOpenVideoFileReportsStartFailure(t *testing.T) {
	_, library := openFixture(t)
	handler := newTestServer(t, Options{Videos: library, Opener: &fakeOpener{available: true, err: errors.New("fork failed")}})
	rec := serve(handler, openRequest("/api/videos/1/open", "127.0.0.1:1", "localhost"))
	assertErrorCode(t, rec, http.StatusInternalServerError, codeInternal)
}

// 既存の同一オリジン確認は、この経路にもそのまま掛かる。
func TestOpenVideoFileKeepsSameOriginCheck(t *testing.T) {
	_, library := openFixture(t)
	opener := &fakeOpener{available: true}
	handler := newTestServer(t, Options{Videos: library, Opener: opener})
	req := openRequest("/api/videos/1/open", "127.0.0.1:1", "localhost:8080")
	req.Header.Set("Origin", "http://evil.example")
	assertErrorCode(t, serve(handler, req), http.StatusForbidden, codeForbidden)
	if len(opener.opened) != 0 {
		t.Fatalf("opened = %q, want none", opener.opened)
	}
}
