package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// fakeThumbnailJobs はサムネイルのジョブが進行中の動画を決め打ちで返す。
type fakeThumbnailJobs struct {
	active map[int64]bool
	err    error
}

func (f *fakeThumbnailJobs) ThumbnailJobActive(_ context.Context, videoID int64) (bool, error) {
	return f.active[videoID], f.err
}

// fakeOpener は開いたパスを記録する。
type fakeOpener struct {
	available bool
	opened    []string
	err       error
}

func (f *fakeOpener) Available() bool { return f.available }

func (f *fakeOpener) Open(path string) error {
	f.opened = append(f.opened, path)
	return f.err
}

func serve(handler http.Handler, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

// makeSeekThumbnailDir はシーク用プレビューの置き場を作る。
func makeSeekThumbnailDir(t *testing.T, thumbnailsDir, contentKey string) {
	t.Helper()
	if err := os.MkdirAll(seekThumbnailDirPath(thumbnailsDir, contentKey), 0o755); err != nil {
		t.Fatal(err)
	}
}

// 動画1件の応答は代表の所在の絶対パスを返す。一覧の項目には所在も
// シーク用プレビューの状態も載らない。
func TestGetVideoReturnsLocationOnlyForSingleVideo(t *testing.T) {
	video := sampleVideo(1, "海辺の散歩")
	video.Path = "/media/旅行/海辺の散歩.mp4"
	library := &fakeLibrary{
		videos: map[int64]domain.Video{1: video},
		page:   domain.VideoPage{Items: []domain.Video{video}, Total: 1},
	}
	handler := newTestServer(t, Options{Videos: library, ThumbnailsDir: t.TempDir()})

	got := decode[gen.Video](t, do(t, handler, http.MethodGet, "/api/videos/1"))
	if got.Location == nil || got.Location.Path != "/media/旅行/海辺の散歩.mp4" {
		t.Fatalf("location = %+v, want 代表の所在", got.Location)
	}
	if got.SeekThumbnailState == nil {
		t.Fatal("seekThumbnailState が入っていない")
	}

	rec := do(t, handler, http.MethodGet, "/api/videos")
	if strings.Contains(rec.Body.String(), `"location"`) || strings.Contains(rec.Body.String(), `"seekThumbnailState"`) {
		t.Fatalf("一覧に詳細だけの項目が載っている: %s", rec.Body)
	}
}

func TestGetVideoSeekThumbnailState(t *testing.T) {
	cases := []struct {
		name      string
		dir       bool
		thumbnail domain.ThumbnailState
		active    bool
		want      gen.VideoSeekThumbnailState
	}{
		{name: "置き場あり", dir: true, thumbnail: domain.ThumbnailStateDone, want: gen.VideoSeekThumbnailStateDone},
		{name: "サムネイル未作成", thumbnail: domain.ThumbnailStatePending, want: gen.VideoSeekThumbnailStatePending},
		{name: "ジョブが queued・running", thumbnail: domain.ThumbnailStateDone, active: true, want: gen.VideoSeekThumbnailStatePending},
		{name: "ジョブが failed", thumbnail: domain.ThumbnailStateFailed, want: gen.VideoSeekThumbnailStateFailed},
		// 失敗したジョブの行が保持期間を過ぎて消えても pending と読まない。
		{name: "ジョブの行が消えた", thumbnail: domain.ThumbnailStateDone, want: gen.VideoSeekThumbnailStateFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			thumbnailsDir := t.TempDir()
			video := sampleVideo(1, "動画")
			video.ThumbnailState = tc.thumbnail
			if tc.dir {
				makeSeekThumbnailDir(t, thumbnailsDir, video.ContentKey)
			}
			handler := newTestServer(t, Options{
				Videos:        &fakeLibrary{videos: map[int64]domain.Video{1: video}},
				ThumbnailJobs: &fakeThumbnailJobs{active: map[int64]bool{1: tc.active}},
				ThumbnailsDir: thumbnailsDir,
			})

			got := decode[gen.Video](t, do(t, handler, http.MethodGet, "/api/videos/1"))
			if got.SeekThumbnailState == nil || *got.SeekThumbnailState != tc.want {
				t.Fatalf("seekThumbnailState = %v, want %s", got.SeekThumbnailState, tc.want)
			}
		})
	}
}

// 読み取り前の動画と映像の無い動画には seekThumbnailState が無い。
func TestGetVideoOmitsSeekThumbnailStateWithoutSeekThumbnail(t *testing.T) {
	pending := sampleVideo(1, "読み取り前")
	pending.ProbeState = domain.ProbeStatePending
	pending.DurationMs = nil
	pending.VideoCodec = ""
	audioOnly := sampleVideo(2, "音声のみ")
	audioOnly.VideoCodec = ""
	handler := newTestServer(t, Options{
		Videos:        &fakeLibrary{videos: map[int64]domain.Video{1: pending, 2: audioOnly}},
		ThumbnailJobs: &fakeThumbnailJobs{},
		ThumbnailsDir: t.TempDir(),
	})

	for _, target := range []string{"/api/videos/1", "/api/videos/2"} {
		got := decode[gen.Video](t, do(t, handler, http.MethodGet, target))
		if got.SeekThumbnailState != nil {
			t.Errorf("%s: seekThumbnailState = %s, want 省略", target, *got.SeekThumbnailState)
		}
		if got.Location == nil {
			t.Errorf("%s: location が入っていない", target)
		}
	}
}

func TestGetVideoReportsThumbnailJobFailure(t *testing.T) {
	video := sampleVideo(1, "動画")
	handler := newTestServer(t, Options{
		Videos:        &fakeLibrary{videos: map[int64]domain.Video{1: video}},
		ThumbnailJobs: &fakeThumbnailJobs{err: errors.New("disk I/O error")},
		ThumbnailsDir: t.TempDir(),
	})
	if rec := do(t, handler, http.MethodGet, "/api/videos/1"); rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

// openable は、ループバックから、ループバックの Host で来た要求で、開ける環境の
// ときだけ真になる。
func TestGetVideoOpenable(t *testing.T) {
	cases := []struct {
		name      string
		remote    string
		host      string
		available bool
		want      bool
	}{
		{name: "IPv4 loopback", remote: "127.0.0.1:1", host: "127.0.0.1:8080", available: true, want: true},
		{name: "localhost", remote: "127.0.0.1:1", host: "localhost", available: true, want: true},
		{name: "IPv6 loopback", remote: "[::1]:1", host: "[::1]:8080", available: true, want: true},
		{name: "unavailable", remote: "127.0.0.1:1", host: "localhost:8080", available: false, want: false},
		{name: "LAN client", remote: "192.168.1.20:1", host: "192.168.1.10:8080", available: true, want: false},
		{name: "DNS rebinding", remote: "127.0.0.1:1", host: "evil.example:8080", available: true, want: false},
		{name: "LAN address from same PC", remote: "127.0.0.1:1", host: "192.168.1.10:8080", available: true, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := newTestServer(t, Options{
				Videos: &fakeLibrary{videos: map[int64]domain.Video{1: sampleVideo(1, "動画")}},
				Opener: &fakeOpener{available: tc.available},
			})
			req := httptest.NewRequest(http.MethodGet, "/api/videos/1", nil)
			req.RemoteAddr = tc.remote
			req.Host = tc.host
			got := decode[gen.Video](t, serve(handler, req))
			if got.Location == nil || got.Location.Openable != tc.want {
				t.Fatalf("location = %+v, want openable %v", got.Location, tc.want)
			}
		})
	}
}
