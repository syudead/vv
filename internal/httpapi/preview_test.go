package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// previewUrl は、状態が done で、アプリケーション層がファイルがあると答えた
// ときだけ出す。
func TestVideoResponsesExposePreviewStateAndOnlyServeableDoneURL(t *testing.T) {
	video := sampleVideo(1, "movie")
	video.PreviewState = domain.PreviewStateDone
	fake := &fakeLibrary{videos: map[int64]domain.Video{video.ID: video}, page: domain.VideoPage{Items: []domain.Video{video}}}
	catalog := &fakeCatalog{previews: map[string]bool{}}
	handler := newTestServer(t, Options{Videos: fake, Catalog: catalog})

	for _, target := range []string{"/api/videos", "/api/videos/1"} {
		rec := do(t, handler, http.MethodGet, target)
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"previewState":"done"`) {
			t.Fatalf("GET %s response = %d %s", target, rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "previewUrl") {
			t.Fatalf("GET %s exposed unavailable preview URL: %s", target, rec.Body.String())
		}
	}

	catalog.previews[video.ContentKey] = true
	rec := do(t, handler, http.MethodGet, "/api/videos")
	if !strings.Contains(rec.Body.String(), `"previewUrl":"/api/videos/1/preview?v=`+video.ContentKey+`"`) {
		t.Fatalf("done preview URL missing: %s", rec.Body.String())
	}

	// 状態が done でなければ、ファイルがあっても URL は出さない。
	video.PreviewState = domain.PreviewStatePending
	fake.videos[video.ID] = video
	if rec := do(t, handler, http.MethodGet, "/api/videos/1"); strings.Contains(rec.Body.String(), "previewUrl") {
		t.Fatalf("pending preview exposed URL: %s", rec.Body.String())
	}
}

func TestGetVideoPreviewServesRangesAndCachePolicies(t *testing.T) {
	video := sampleVideo(1, "movie")
	video.PreviewState = domain.PreviewStateDone
	path := filepath.Join(t.TempDir(), "preview.mp4")
	if err := os.WriteFile(path, []byte("preview-data"), 0o644); err != nil {
		t.Fatal(err)
	}
	artifacts := &fakeArtifacts{previews: map[string]string{video.ContentKey: path}}
	handler := newTestServer(t, Options{Videos: &fakeLibrary{videos: map[int64]domain.Video{video.ID: video}}, Artifacts: artifacts})

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
	if got := full.Header().Get("Cache-Control"); got != cacheRevalidate {
		t.Errorf("versioned Cache-Control = %q", got)
	}
	if full.Header().Get("ETag") == "" {
		t.Error("ETag が無い")
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

	// 版の無い要求・古い版の要求も、同じく使うたびに確かめさせる（guest-api.md §5）。
	for _, target := range []string{"/api/videos/1/preview?v=stale", "/api/videos/1/preview"} {
		rec := do(t, handler, http.MethodGet, target)
		if rec.Code != 200 || rec.Header().Get("Cache-Control") != cacheRevalidate {
			t.Fatalf("unversioned response = %d cache=%q", rec.Code, rec.Header().Get("Cache-Control"))
		}
		if got := rec.Header().Get("ETag"); got != full.Header().Get("ETag") {
			t.Errorf("%s: ETag = %q, want %q", target, got, full.Header().Get("ETag"))
		}
	}

	// ETag が一致すれば 304 で、本文を送らない。
	conditional := httptest.NewRequest(http.MethodGet, "/api/videos/1/preview?v="+video.ContentKey, nil)
	conditional.Header.Set("If-None-Match", full.Header().Get("ETag"))
	notModified := httptest.NewRecorder()
	handler.ServeHTTP(notModified, conditional)
	if notModified.Code != http.StatusNotModified || notModified.Body.Len() != 0 {
		t.Errorf("If-None-Match: status = %d, body = %d バイト", notModified.Code, notModified.Body.Len())
	}
}

// 置き場が「無い」と答えたもの（無い・空・manifest と合わない・生成途中）は、
// no-store の 404 にする。置き場が無い場合も同じである。
func TestGetVideoPreviewReturnsNoStore404ForUnavailableAssets(t *testing.T) {
	video := sampleVideo(1, "movie")
	video.PreviewState = domain.PreviewStateDone

	for name, artifacts := range map[string]ArtifactReader{"無い": &fakeArtifacts{}, "置き場が無い": nil} {
		handler := newTestServer(t, Options{Videos: &fakeLibrary{videos: map[int64]domain.Video{video.ID: video}}, Artifacts: artifacts})
		rec := do(t, handler, http.MethodGet, "/api/videos/1/preview?v="+video.ContentKey)
		if rec.Code != 404 || rec.Header().Get("Cache-Control") != cacheNoStore {
			t.Fatalf("%s: response = %d cache=%q", name, rec.Code, rec.Header().Get("Cache-Control"))
		}
	}
}

func TestGetVideoPreviewRejectsNonDoneStatesWithoutFallback(t *testing.T) {
	transcoder := &fakeTranscoder{body: "must not be used"}

	for _, state := range []domain.PreviewState{domain.PreviewStatePending, domain.PreviewStateFailed} {
		video := sampleVideo(1, "movie")
		video.Path = filepath.Join(t.TempDir(), "source-must-not-be-opened.mp4")
		video.PreviewState = state
		handler := newTestServer(t, Options{
			Videos:     &fakeLibrary{videos: map[int64]domain.Video{video.ID: video}},
			Artifacts:  &fakeArtifacts{},
			Transcoder: transcoder,
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

// アプリケーション層が作り直しを積んで準備中と読み替えた動画は、一覧・詳細・
// 関連動画のどれでも previewState が pending になり、URL は出ない。
func TestRequeuedPreviewIsReportedAsPending(t *testing.T) {
	video := sampleVideo(1, "movie")
	video.PreviewState = domain.PreviewStateDone
	fake := &fakeLibrary{videos: map[int64]domain.Video{video.ID: video}, page: domain.VideoPage{Items: []domain.Video{video}}}
	catalog := &fakeCatalog{requeue: true, related: domain.RelatedVideos{Items: []domain.Video{video}}}
	handler := newTestServer(t, Options{Videos: fake, Catalog: catalog})

	for _, target := range []string{"/api/videos", "/api/videos/1", "/api/videos/1/related"} {
		rec := do(t, handler, http.MethodGet, target)
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"previewState":"pending"`) {
			t.Fatalf("GET %s response = %d %s", target, rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "previewUrl") {
			t.Fatalf("GET %s exposed missing preview URL: %s", target, rec.Body.String())
		}
	}
}

// If-Modified-Since だけの要求には、更新時刻が同じでも本文を返す。確かめは内容の
// ダイジェストの ETag だけで行う（Devin の指摘、PR 292）。
func TestGetVideoPreviewIgnoresIfModifiedSince(t *testing.T) {
	video := sampleVideo(1, "movie")
	video.PreviewState = domain.PreviewStateDone
	path := filepath.Join(t.TempDir(), "preview.mp4")
	if err := os.WriteFile(path, []byte("preview-data"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	artifacts := &fakeArtifacts{previews: map[string]string{video.ContentKey: path}}
	handler := newTestServer(t, Options{Videos: &fakeLibrary{videos: map[int64]domain.Video{video.ID: video}}, Artifacts: artifacts})
	full := do(t, handler, http.MethodGet, "/api/videos/1/preview?v="+video.ContentKey)
	if got := full.Header().Get("Last-Modified"); got != "" {
		t.Errorf("Last-Modified = %q, 更新時刻で確かめさせない", got)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/videos/1/preview?v="+video.ContentKey, nil)
	req.Header.Set("If-Modified-Since", info.ModTime().UTC().Format(http.TimeFormat))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != "preview-data" {
		t.Fatalf("If-Modified-Since だけの要求 = %d %q、本文を返すべき", rec.Code, rec.Body.String())
	}
}
