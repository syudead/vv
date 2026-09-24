package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func TestVideoResponsesExposePreviewStateAndOnlyServeableDoneURL(t *testing.T) {
	dir := t.TempDir()
	video := sampleVideo(1, "movie")
	video.PreviewState = domain.PreviewStateDone
	fake := &fakeLibrary{videos: map[int64]domain.Video{video.ID: video}, page: domain.VideoPage{Items: []domain.Video{video}}}
	handler := newTestServer(t, Options{Videos: fake, ThumbnailsDir: dir})

	for _, target := range []string{"/api/videos", "/api/videos/1"} {
		rec := do(t, handler, http.MethodGet, target)
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"previewState":"done"`) {
			t.Fatalf("GET %s response = %d %s", target, rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "previewUrl") {
			t.Fatalf("GET %s exposed unavailable preview URL: %s", target, rec.Body.String())
		}
	}

	path := previewFilePath(dir, video.ContentKey)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("preview-data"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := do(t, handler, http.MethodGet, "/api/videos")
	if !strings.Contains(rec.Body.String(), `"previewUrl":"/api/videos/1/preview?v=`+video.ContentKey+`"`) {
		t.Fatalf("done preview URL missing: %s", rec.Body.String())
	}
}

func TestGetVideoPreviewServesRangesAndCachePolicies(t *testing.T) {
	dir := t.TempDir()
	video := sampleVideo(1, "movie")
	video.PreviewState = domain.PreviewStateDone
	path := previewFilePath(dir, video.ContentKey)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("preview-data"), 0o644); err != nil {
		t.Fatal(err)
	}
	handler := newTestServer(t, Options{Videos: &fakeLibrary{videos: map[int64]domain.Video{video.ID: video}}, ThumbnailsDir: dir})

	full := do(t, handler, http.MethodGet, "/api/videos/1/preview?v="+video.ContentKey)
	if full.Code != 200 || full.Body.String() != "preview-data" {
		t.Fatalf("full response = %d %q", full.Code, full.Body.String())
	}
	if got := full.Header().Get("Content-Type"); got != "video/mp4" {
		t.Errorf("Content-Type = %q", got)
	}
	if got := full.Header().Get("Accept-Ranges"); got != "bytes" {
		t.Errorf("Accept-Ranges = %q", got)
	}
	if got := full.Header().Get("Cache-Control"); got != cacheImmutable {
		t.Errorf("versioned Cache-Control = %q", got)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/videos/1/preview?v="+video.ContentKey, nil)
	req.Header.Set("Range", "bytes=0-6")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 206 || rec.Body.String() != "preview" || rec.Header().Get("Content-Range") != "bytes 0-6/12" {
		t.Fatalf("range response = %d %q range=%q", rec.Code, rec.Body.String(), rec.Header().Get("Content-Range"))
	}
	if got := rec.Header().Get("Content-Length"); got != "7" {
		t.Errorf("range Content-Length = %q, want 7", got)
	}

	badRangeReq := httptest.NewRequest(http.MethodGet, "/api/videos/1/preview?v="+video.ContentKey, nil)
	badRangeReq.Header.Set("Range", "bytes=99-100")
	badRangeRec := httptest.NewRecorder()
	handler.ServeHTTP(badRangeRec, badRangeReq)
	if badRangeRec.Code != http.StatusRequestedRangeNotSatisfiable || badRangeRec.Header().Get("Cache-Control") != cacheNoStore {
		t.Fatalf("unsatisfiable range response = %d cache=%q", badRangeRec.Code, badRangeRec.Header().Get("Cache-Control"))
	}
	if got := badRangeRec.Header().Get("Content-Range"); got != "bytes */12" {
		t.Errorf("unsatisfiable Content-Range = %q, want %q", got, "bytes */12")
	}

	for _, target := range []string{"/api/videos/1/preview?v=stale", "/api/videos/1/preview"} {
		rec := do(t, handler, http.MethodGet, target)
		if rec.Code != 200 || rec.Header().Get("Cache-Control") != cacheNoStore {
			t.Fatalf("unversioned response = %d cache=%q", rec.Code, rec.Header().Get("Cache-Control"))
		}
	}
}

func TestGetVideoPreviewReturnsNoStore404ForUnavailableAssets(t *testing.T) {
	dir := t.TempDir()
	video := sampleVideo(1, "movie")
	video.PreviewState = domain.PreviewStateDone
	handler := newTestServer(t, Options{Videos: &fakeLibrary{videos: map[int64]domain.Video{video.ID: video}}, ThumbnailsDir: dir})

	for _, zero := range []bool{false, true} {
		if zero {
			path := previewFilePath(dir, video.ContentKey)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, nil, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		rec := do(t, handler, http.MethodGet, "/api/videos/1/preview?v="+video.ContentKey)
		if rec.Code != 404 || rec.Header().Get("Cache-Control") != cacheNoStore {
			t.Fatalf("zero=%t response = %d cache=%q", zero, rec.Code, rec.Header().Get("Cache-Control"))
		}
		if zero {
			_ = os.Remove(previewFilePath(dir, video.ContentKey))
		}
	}
}

func TestGetVideoPreviewRejectsNonDoneStatesWithoutFallback(t *testing.T) {
	dir := t.TempDir()
	transcoder := &fakeTranscoder{body: "must not be used"}

	for _, state := range []domain.PreviewState{domain.PreviewStatePending, domain.PreviewStateFailed} {
		video := sampleVideo(1, "movie")
		video.Path = filepath.Join(t.TempDir(), "source-must-not-be-opened.mp4")
		video.PreviewState = state
		handler := newTestServer(t, Options{
			Videos:        &fakeLibrary{videos: map[int64]domain.Video{video.ID: video}},
			ThumbnailsDir: dir,
			Transcoder:    transcoder,
		})

		rec := do(t, handler, http.MethodGet, "/api/videos/1/preview?v="+video.ContentKey)
		if rec.Code != http.StatusNotFound || rec.Header().Get("Cache-Control") != cacheNoStore {
			t.Fatalf("state=%s response = %d cache=%q", state, rec.Code, rec.Header().Get("Cache-Control"))
		}
		if location := rec.Header().Get("Location"); location != "" {
			t.Errorf("state=%s redirected to %q", state, location)
		}
	}

	if transcoder.starts != 0 {
		t.Fatalf("preview fallback started transcoder %d times", transcoder.starts)
	}
}

// fakePreviewRepair は作り直しの要求を記録する。
type fakePreviewRepair struct {
	mu    sync.Mutex
	calls []int64
}

func (f *fakePreviewRepair) RequeueMissingPreview(_ context.Context, id int64, _ string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, id)
	return true, nil
}

// 作り終えた記録があるのにファイルが無ければ、作り直しを積み、応答では準備中と
// して返す。ファイルがあれば積まない。
func TestMissingPreviewIsRequeuedWhenFound(t *testing.T) {
	dir := t.TempDir()
	video := sampleVideo(1, "movie")
	video.PreviewState = domain.PreviewStateDone
	fake := &fakeLibrary{videos: map[int64]domain.Video{video.ID: video}, page: domain.VideoPage{Items: []domain.Video{video}}}
	repair := &fakePreviewRepair{}
	handler := newTestServer(t, Options{Videos: fake, ThumbnailsDir: dir, PreviewRepair: repair})

	for _, target := range []string{"/api/videos", "/api/videos/1"} {
		rec := do(t, handler, http.MethodGet, target)
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"previewState":"pending"`) {
			t.Fatalf("GET %s response = %d %s", target, rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "previewUrl") {
			t.Fatalf("GET %s exposed missing preview URL: %s", target, rec.Body.String())
		}
	}
	if len(repair.calls) != 2 || repair.calls[0] != 1 {
		t.Fatalf("作り直しの要求 = %v, want [1 1]", repair.calls)
	}

	path := previewFilePath(dir, video.ContentKey)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("preview-data"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := do(t, handler, http.MethodGet, "/api/videos")
	if !strings.Contains(rec.Body.String(), `"previewUrl"`) {
		t.Fatalf("preview URL missing: %s", rec.Body.String())
	}
	if len(repair.calls) != 2 {
		t.Fatalf("ファイルがあるのに作り直しを積んだ: %v", repair.calls)
	}
}
