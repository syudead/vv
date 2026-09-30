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

// 作り直したサムネイルが前と同じ大きさ・更新時刻でも、内容が違えば ETag が
// 変わり、古い ETag での確認には新しい画像を返す。
func TestThumbnailETagFollowsContent(t *testing.T) {
	artifacts, video := thumbnailFixture(t)
	handler := newTestServer(t, Options{
		Videos:    &fakeLibrary{videos: map[int64]domain.Video{1: video}},
		Artifacts: artifacts,
	})
	const target = "/api/videos/1/thumbnail?v=abcdef012345"
	path := artifacts.thumbnails[video.ContentKey]
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	before := do(t, handler, http.MethodGet, target).Header().Get("ETag")

	replaced := []byte{0xff, 0xd8, 0xff, 0xe1, 0x00, 0x10}
	if err := os.WriteFile(path, replaced, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.Header.Set("If-None-Match", before)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != string(replaced) {
		t.Fatalf("古い ETag での確認 = %d %x、新しい画像を返すべき", rec.Code, rec.Body.Bytes())
	}
	if after := rec.Header().Get("ETag"); after == before {
		t.Errorf("内容が変わっても ETag = %q のまま", after)
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

// If-Modified-Since だけの要求には、更新時刻が同じでも新しい画像を返す。更新時刻で
// 304 にすると、同じ秒に作り直した画像が古いまま残る（Devin の指摘、PR 292）。
func TestThumbnailIgnoresIfModifiedSince(t *testing.T) {
	artifacts, video := thumbnailFixture(t)
	handler := newTestServer(t, Options{
		Videos:    &fakeLibrary{videos: map[int64]domain.Video{1: video}},
		Artifacts: artifacts,
	})
	const target = "/api/videos/1/thumbnail?v=abcdef012345"
	path := artifacts.thumbnails[video.ContentKey]
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := do(t, handler, http.MethodGet, target).Header().Get("Last-Modified"); got != "" {
		t.Errorf("Last-Modified = %q, 更新時刻で確かめさせない", got)
	}

	replaced := []byte{0xff, 0xd8, 0xff, 0xe1, 0x00, 0x10}
	if err := os.WriteFile(path, replaced, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.Header.Set("If-Modified-Since", info.ModTime().UTC().Format(http.TimeFormat))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != string(replaced) {
		t.Fatalf("If-Modified-Since だけの要求 = %d %x、新しい画像を返すべき", rec.Code, rec.Body.Bytes())
	}
}

// 位置が指定されている動画の thumbnailUrl の版は `<内容鍵の先頭>-r<改版番号>` で、位置の値を
// 含まない。未指定なら今までどおり内容鍵の先頭だけ（specs/029-video-overrides/research.md R-6）。
func TestThumbnailURLVersionIncludesRevision(t *testing.T) {
	position := int64(123_456)
	for _, tc := range []struct {
		name  string
		video domain.Video
		want  string
	}{
		{name: "未指定", video: domain.Video{ID: 1, ContentKey: "abcdef0123456789"},
			want: "/api/videos/1/thumbnail?v=abcdef012345"},
		{name: "指定", video: domain.Video{ID: 1, ContentKey: "abcdef0123456789",
			ThumbnailPositionMs: &position, ThumbnailRevision: 1_790_000_000_001},
			want: "/api/videos/1/thumbnail?v=abcdef012345-r1790000000001"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := thumbnailURL(tc.video); got != tc.want {
				t.Fatalf("thumbnailURL = %q, want %q", got, tc.want)
			}
		})
	}
}
