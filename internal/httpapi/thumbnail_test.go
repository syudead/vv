package httpapi

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// thumbnailFixture は生成済みのサムネイルを1枚置いた状態を返す。
func thumbnailFixture(t *testing.T) (string, domain.Video) {
	t.Helper()

	dir := t.TempDir()
	video := sampleVideo(1, "海辺の散歩")

	// internal/media の ThumbnailPath と同じ規則で置く。
	name := "abcdef0123456789abcdef_1024"
	path := filepath.Join(dir, name[:2], name+".jpg")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	// JPEG の先頭バイト列。ServeContent が中身を読む。
	if err := os.WriteFile(path, []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10}, 0o600); err != nil {
		t.Fatal(err)
	}
	return dir, video
}

// 生成済みのサムネイルを image/jpeg で返す。
func TestGetThumbnail(t *testing.T) {
	dir, video := thumbnailFixture(t)
	handler := newTestServer(t, Options{
		Videos:        &fakeLibrary{videos: map[int64]domain.Video{1: video}},
		ThumbnailsDir: dir,
	})

	rec := do(t, handler, http.MethodGet, "/api/videos/1/thumbnail?v=abcdef012345")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("Content-Type"); got != "image/jpeg" {
		t.Errorf("Content-Type = %q, want image/jpeg", got)
	}
}

// v 付きの要求には長期キャッシュを付ける。内容が変われば content_key が
// 変わり URL も変わるので、古い画像が残らない。
func TestThumbnailCacheControlWithVersion(t *testing.T) {
	dir, video := thumbnailFixture(t)
	handler := newTestServer(t, Options{
		Videos:        &fakeLibrary{videos: map[int64]domain.Video{1: video}},
		ThumbnailsDir: dir,
	})

	rec := do(t, handler, http.MethodGet, "/api/videos/1/thumbnail?v=abcdef012345")
	if got := rec.Header().Get("Cache-Control"); got != cacheImmutable {
		t.Errorf("Cache-Control = %q, want %q", got, cacheImmutable)
	}
}

// 版付きサムネイルは1年の immutable で配信するが、失敗応答にそれが残ると
// 壊れた結果が1年キャッシュされる。416 では no-store に戻ることを固定する。
func TestThumbnailUnsatisfiableRangeIsNotCached(t *testing.T) {
	dir, video := thumbnailFixture(t)
	handler := newTestServer(t, Options{
		Videos:        &fakeLibrary{videos: map[int64]domain.Video{1: video}},
		ThumbnailsDir: dir,
	})

	rec := rangeRequest(t, handler, "/api/videos/1/thumbnail?v=abcdef012345", "bytes=99999999999-")
	if rec.Code != http.StatusRequestedRangeNotSatisfiable {
		t.Fatalf("status = %d, want 416", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got != cacheNoStore {
		t.Errorf("416 の Cache-Control = %q, want %q", got, cacheNoStore)
	}
}

// v の無い要求には長期キャッシュを付けない。版が分からないものを1年
// 抱えさせると、差し替えても古い画像が残る。
func TestThumbnailCacheControlWithoutVersion(t *testing.T) {
	dir, video := thumbnailFixture(t)
	handler := newTestServer(t, Options{
		Videos:        &fakeLibrary{videos: map[int64]domain.Video{1: video}},
		ThumbnailsDir: dir,
	})

	rec := do(t, handler, http.MethodGet, "/api/videos/1/thumbnail")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got == cacheImmutable {
		t.Errorf("v の無い要求に長期キャッシュが付いた: %q", got)
	}
}

// 未生成なら 404。クライアントは枠だけを描く。
func TestThumbnailNotGenerated(t *testing.T) {
	pending := sampleVideo(2, "解析前")
	pending.ThumbnailState = domain.ThumbnailStatePending

	handler := newTestServer(t, Options{
		Videos:        &fakeLibrary{videos: map[int64]domain.Video{2: pending}},
		ThumbnailsDir: t.TempDir(),
	})

	rec := do(t, handler, http.MethodGet, "/api/videos/2/thumbnail?v=abcdef012345")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rec.Code, rec.Body)
	}
}

// 動画そのものが無ければ 404。
func TestThumbnailForMissingVideo(t *testing.T) {
	handler := newTestServer(t, Options{
		Videos: &fakeLibrary{}, ThumbnailsDir: t.TempDir(),
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
		Videos:        &fakeLibrary{videos: map[int64]domain.Video{1: sampleVideo(1, "a")}},
		ThumbnailsDir: t.TempDir(),
	})

	rec := do(t, handler, http.MethodGet, "/api/videos/1/thumbnail?v=abcdef012345")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}
