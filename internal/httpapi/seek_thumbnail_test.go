package httpapi

import (
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

func seekThumbnailServer(t *testing.T, reader ArtifactReader) (http.Handler, domain.Video) {
	t.Helper()
	mediaDir, video, _ := streamFixture(t, "a.mp4", 128)
	return newTestServer(t, Options{
		Videos:    &fakeLibrary{videos: map[int64]domain.Video{video.ID: video}, roots: []string{mediaDir}},
		Artifacts: reader,
	}), video
}

// assertRevalidated は、成功の応答が private, no-cache と ETag を持ち、
// If-None-Match が一致すれば本文なしの 304 になることを確かめる。
func assertRevalidated(t *testing.T, handler http.Handler, target string, rec *httptest.ResponseRecorder) {
	t.Helper()
	if got := rec.Header().Get("Cache-Control"); got != cacheRevalidate {
		t.Errorf("Cache-Control = %q", got)
	}
	etag := rec.Header().Get("ETag")
	if etag == "" {
		t.Fatal("ETag が無い")
	}
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.Header.Set("If-None-Match", `W/"other", `+etag)
	again := httptest.NewRecorder()
	handler.ServeHTTP(again, req)
	if again.Code != http.StatusNotModified || again.Body.Len() != 0 {
		t.Errorf("If-None-Match: status = %d, body = %d バイト", again.Code, again.Body.Len())
	}
	if again.Header().Get("Cache-Control") != cacheRevalidate || again.Header().Get("ETag") != etag {
		t.Errorf("304 のヘッダー = %v", again.Header())
	}
}

// 配置情報は契約の形（contracts/seek-sprite-api.md §2）で、シートの版付き URL を
// 番号順に並べる。
func TestSeekThumbnailReturnsSpriteDescription(t *testing.T) {
	reader := &fakeArtifacts{sprite: testSprite()}
	handler, video := seekThumbnailServer(t, reader)
	const target = "/api/videos/1/seek-thumbnail?v=abcdef012345"
	rec := do(t, handler, http.MethodGet, target)

	if rec.Code != http.StatusOK {
		t.Fatalf("response = %d %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != contentTypeJSON {
		t.Errorf("Content-Type = %q", got)
	}
	got := decode[gen.SeekThumbnailSprite](t, rec)
	version := thumbnailVersion(video.ContentKey)
	want := gen.SeekThumbnailSprite{
		IntervalMs: 5000, FrameCount: 150, Columns: 10, Rows: 10, FrameWidth: 320, FrameHeight: 180,
		Sheets: []string{
			"/api/videos/1/seek-thumbnail/0?v=" + version,
			"/api/videos/1/seek-thumbnail/1?v=" + version,
		},
	}
	if got.IntervalMs != want.IntervalMs || got.FrameCount != want.FrameCount || got.Columns != want.Columns ||
		got.Rows != want.Rows || got.FrameWidth != want.FrameWidth || got.FrameHeight != want.FrameHeight ||
		!slices.Equal(got.Sheets, want.Sheets) {
		t.Fatalf("配置情報 = %+v, want %+v", got, want)
	}
	if reader.contentKey != video.ContentKey {
		t.Errorf("読んだ内容 = %q", reader.contentKey)
	}
	assertRevalidated(t, handler, target, rec)

	// 版が無くても、使うたびに確かめさせる。
	bare := do(t, handler, http.MethodGet, "/api/videos/1/seek-thumbnail")
	if bare.Code != http.StatusOK || bare.Header().Get("Cache-Control") != cacheRevalidate {
		t.Errorf("版なし: status=%d cache=%q", bare.Code, bare.Header().Get("Cache-Control"))
	}
}

// シートは JPEG を返し、private, no-cache と ETag で確かめさせる。
func TestSeekThumbnailSheetReturnsJPEG(t *testing.T) {
	reader := &fakeArtifacts{sprite: testSprite(), image: []byte{0xff, 0xd8, 0xff, 0xd9}}
	handler, video := seekThumbnailServer(t, reader)
	const target = "/api/videos/1/seek-thumbnail/1?v=abcdef012345"
	rec := do(t, handler, http.MethodGet, target)

	if rec.Code != http.StatusOK || rec.Body.String() != string(reader.image) {
		t.Fatalf("response = %d %x", rec.Code, rec.Body.Bytes())
	}
	if got := rec.Header().Get("Content-Type"); got != "image/jpeg" {
		t.Errorf("Content-Type = %q", got)
	}
	if reader.sheet != 1 || reader.contentKey != video.ContentKey {
		t.Errorf("reader = %+v", reader)
	}
	assertRevalidated(t, handler, target, rec)
}

// シートの番号は整数で、枚数以上なら 404 になる。
func TestSeekThumbnailSheetRejectsInvalidAndOutOfRange(t *testing.T) {
	handler, _ := seekThumbnailServer(t, &fakeArtifacts{sprite: testSprite(), image: []byte("jpeg")})
	for target, want := range map[string]int{
		"/api/videos/1/seek-thumbnail/-1":   http.StatusBadRequest,
		"/api/videos/1/seek-thumbnail/nope": http.StatusBadRequest,
		"/api/videos/1/seek-thumbnail/2":    http.StatusNotFound,
		"/api/videos/1/seek-thumbnail/6":    http.StatusNotFound,
	} {
		rec := do(t, handler, http.MethodGet, target)
		if rec.Code != want {
			t.Errorf("%s: status=%d want=%d body=%s", target, rec.Code, want, rec.Body.String())
		}
		if rec.Header().Get("Cache-Control") != cacheNoStore {
			t.Errorf("%s: error cache=%q", target, rec.Header().Get("Cache-Control"))
		}
	}
}

// 解析が終わっていない・映像が無い動画は、どちらの経路も 409 になる。
func TestSeekThumbnailRequiresProbe(t *testing.T) {
	reader := &fakeArtifacts{sprite: testSprite(), image: []byte("jpeg")}
	_, video := seekThumbnailServer(t, reader)
	mediaDir := t.TempDir()
	for name, mutate := range map[string]func(*domain.Video){
		"解析待ち":  func(v *domain.Video) { v.ProbeState = domain.ProbeStatePending },
		"映像が無い": func(v *domain.Video) { v.VideoCodec = "" },
	} {
		changed := video
		mutate(&changed)
		handler := newTestServer(t, Options{
			Videos:    &fakeLibrary{videos: map[int64]domain.Video{1: changed}, roots: []string{mediaDir}},
			Artifacts: reader,
		})
		for _, target := range []string{"/api/videos/1/seek-thumbnail", "/api/videos/1/seek-thumbnail/0"} {
			rec := do(t, handler, http.MethodGet, target)
			assertErrorBody(t, name+" "+target, rec.Code, rec.Body.Bytes(),
				wantError{status: http.StatusConflict, code: gen.ErrorCodeConflict, reason: reasonProbeInfoMissing})
		}
	}
}

// 生成中（配置情報が無い）は 409、配置情報やシートを読めなければ 500 になる。
func TestSeekThumbnailMapsFileFailures(t *testing.T) {
	tests := []struct {
		name   string
		reader *fakeArtifacts
		target string
		want   int
	}{
		{"生成中の配置情報", &fakeArtifacts{}, "/api/videos/1/seek-thumbnail?v=version", http.StatusConflict},
		{"生成中のシート", &fakeArtifacts{image: []byte("jpeg")}, "/api/videos/1/seek-thumbnail/0?v=version", http.StatusConflict},
		{"形の違う配置情報", &fakeArtifacts{spriteErr: errors.New("bad sprite")}, "/api/videos/1/seek-thumbnail", http.StatusInternalServerError},
		{"シートを読めない", &fakeArtifacts{sprite: testSprite(), err: errors.New("read failed")}, "/api/videos/1/seek-thumbnail/0", http.StatusInternalServerError},
		{"シートが無い", &fakeArtifacts{sprite: testSprite(), err: fs.ErrNotExist}, "/api/videos/1/seek-thumbnail/0", http.StatusInternalServerError},
		{"空のシート", &fakeArtifacts{sprite: testSprite()}, "/api/videos/1/seek-thumbnail/0", http.StatusInternalServerError},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			handler, _ := seekThumbnailServer(t, tc.reader)
			rec := do(t, handler, http.MethodGet, tc.target)
			if rec.Code != tc.want {
				t.Fatalf("status=%d want=%d body=%s", rec.Code, tc.want, rec.Body.String())
			}
			if rec.Header().Get("Cache-Control") != cacheNoStore {
				t.Errorf("error cache=%q", rec.Header().Get("Cache-Control"))
			}
			if tc.want == http.StatusConflict {
				assertErrorBody(t, tc.name, rec.Code, rec.Body.Bytes(),
					wantError{status: http.StatusConflict, code: gen.ErrorCodeConflict, reason: reasonSeekPreviewGenerating})
			}
		})
	}
}

func TestSeekThumbnailForMissingVideo(t *testing.T) {
	handler := newTestServer(t, Options{Videos: &fakeLibrary{}, Artifacts: &fakeArtifacts{sprite: testSprite()}})
	for _, target := range []string{"/api/videos/999/seek-thumbnail", "/api/videos/999/seek-thumbnail/0"} {
		rec := do(t, handler, http.MethodGet, target)
		if rec.Code != http.StatusNotFound || decode[gen.Error](t, rec).Code != codeNotFound {
			t.Errorf("%s: status=%d body=%s", target, rec.Code, rec.Body.String())
		}
	}
}
