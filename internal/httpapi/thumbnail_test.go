package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// thumbnailFixture は生成済みのサムネイルを1枚置いた状態を返す。
func thumbnailFixture(t *testing.T) (*fakeArtifacts, domain.Video) {
	t.Helper()

	video := sampleVideo(1, "海辺の散歩")
	path := filepath.Join(t.TempDir(), "thumbnail.jpg")
	// JPEG の先頭バイト列。ServeContent が中身を読む。
	if err := os.WriteFile(path, []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10}, 0o600); err != nil {
		t.Fatal(err)
	}
	return &fakeArtifacts{thumbnails: map[string]string{video.ContentKey: path}}, video
}

// 生成済みのサムネイルを image/jpeg で返す。
func TestGetThumbnail(t *testing.T) {
	artifacts, video := thumbnailFixture(t)
	handler := newTestServer(t, Options{
		Videos:    &fakeLibrary{videos: map[int64]domain.Video{1: video}},
		Artifacts: artifacts,
	})

	rec := do(t, handler, http.MethodGet, "/api/videos/1/thumbnail?v=abcdef012345")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("Content-Type"); got != "image/jpeg" {
		t.Errorf("Content-Type = %q, want image/jpeg", got)
	}
}

// 版の有無によらず、使うたびに確かめさせる（specs/016-single-account-auth/
// contracts/guest-api.md §5）。ETag が一致すれば 304 になる。
func TestThumbnailCacheControlRevalidates(t *testing.T) {
	artifacts, video := thumbnailFixture(t)
	handler := newTestServer(t, Options{
		Videos:    &fakeLibrary{videos: map[int64]domain.Video{1: video}},
		Artifacts: artifacts,
	})

	for _, target := range []string{"/api/videos/1/thumbnail?v=abcdef012345", "/api/videos/1/thumbnail"} {
		rec := do(t, handler, http.MethodGet, target)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d", target, rec.Code)
		}
		if got := rec.Header().Get("Cache-Control"); got != cacheRevalidate {
			t.Errorf("%s: Cache-Control = %q, want %q", target, got, cacheRevalidate)
		}
		etag := rec.Header().Get("ETag")
		if etag == "" {
			t.Fatalf("%s: ETag が無い", target)
		}
		req := httptest.NewRequest(http.MethodGet, target, nil)
		req.Header.Set("If-None-Match", etag)
		again := httptest.NewRecorder()
		handler.ServeHTTP(again, req)
		if again.Code != http.StatusNotModified || again.Body.Len() != 0 {
			t.Errorf("%s: If-None-Match の status = %d, body = %d バイト", target, again.Code, again.Body.Len())
		}
		if got := again.Header().Get("Cache-Control"); got != cacheRevalidate {
			t.Errorf("%s: 304 の Cache-Control = %q", target, got)
		}
	}
}

// 成功の応答は ETag で確かめさせるが、失敗応答にキャッシュの指示が残ると壊れた
// 結果が再利用される。416 では no-store に戻ることを固定する。
func TestThumbnailUnsatisfiableRangeIsNotCached(t *testing.T) {
	artifacts, video := thumbnailFixture(t)
	handler := newTestServer(t, Options{
		Videos:    &fakeLibrary{videos: map[int64]domain.Video{1: video}},
		Artifacts: artifacts,
	})

	rec := rangeRequest(t, handler, "/api/videos/1/thumbnail?v=abcdef012345", "bytes=99999999999-")
	if rec.Code != http.StatusRequestedRangeNotSatisfiable {
		t.Fatalf("status = %d, want 416", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got != cacheNoStore {
		t.Errorf("416 の Cache-Control = %q, want %q", got, cacheNoStore)
	}
}

// 未生成なら 404。クライアントは枠だけを描く。
func TestThumbnailNotGenerated(t *testing.T) {
	pending := sampleVideo(2, "解析前")
	pending.ThumbnailState = domain.ThumbnailStatePending

	handler := newTestServer(t, Options{
		Videos:    &fakeLibrary{videos: map[int64]domain.Video{2: pending}},
		Artifacts: &fakeArtifacts{},
	})

	rec := do(t, handler, http.MethodGet, "/api/videos/2/thumbnail?v=abcdef012345")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rec.Code, rec.Body)
	}
}

// 動画そのものが無ければ 404。
func TestThumbnailForMissingVideo(t *testing.T) {
	handler := newTestServer(t, Options{
		Videos: &fakeLibrary{}, Artifacts: &fakeArtifacts{},
	})

	rec := do(t, handler, http.MethodGet, "/api/videos/999/thumbnail")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
	if got := decode[gen.Error](t, rec); got.Code != codeNotFound {
		t.Errorf("code = %q, want %s", got.Code, codeNotFound)
	}
}

// 状態が done でも実体が無ければ 404 にする。壊れた応答を返すより、
// 未生成と同じ扱いにして枠を描かせる方が利用者の損失が小さい。
func TestThumbnailMissingFileOnDisk(t *testing.T) {
	handler := newTestServer(t, Options{
		Videos:    &fakeLibrary{videos: map[int64]domain.Video{1: sampleVideo(1, "a")}},
		Artifacts: &fakeArtifacts{},
	})

	rec := do(t, handler, http.MethodGet, "/api/videos/1/thumbnail?v=abcdef012345")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}
