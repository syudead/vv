package httpapi

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

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

// 動画1件の応答は代表の所在の絶対パスを返す。一覧の項目には所在も
// シーク用プレビューの状態も載らない。
func TestGetVideoReturnsLocationOnlyForSingleVideo(t *testing.T) {
	video := sampleVideo(1, "海辺の散歩")
	video.Path = "/media/旅行/海辺の散歩.mp4"
	library := &fakeLibrary{
		videos: map[int64]domain.Video{1: video},
		page:   domain.VideoPage{Items: []domain.Video{video}, Total: 1},
	}
	handler := newTestServer(t, Options{Videos: library, Catalog: &fakeCatalog{}})

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

// 動画1件の応答にもタグが載り、タグが無ければ空配列になる（#267）。
func TestGetVideoIncludesTags(t *testing.T) {
	video := sampleVideo(1, "海辺の散歩")
	library := &fakeLibrary{
		videos: map[int64]domain.Video{1: video},
		page:   domain.VideoPage{Items: []domain.Video{video}, Total: 1},
	}
	tags := &fakeTags{byContentKey: map[string][]domain.TagRef{
		video.ContentKey: {{ID: 3, Name: "海"}},
	}}
	handler := newTestServer(t, Options{Videos: library, Catalog: &fakeCatalog{}, Tags: tags})

	got := decode[gen.Video](t, do(t, handler, http.MethodGet, "/api/videos/1"))
	if len(got.Tags) != 1 || got.Tags[0].Id != 3 || got.Tags[0].Name != "海" {
		t.Fatalf("tags = %+v", got.Tags)
	}
}

// アプリケーション層が導いた状態を、そのまま契約の値へ写す。導き方は
// internal/app の Catalog.SeekThumbnailState で検証する。
func TestGetVideoSeekThumbnailState(t *testing.T) {
	cases := []struct {
		state domain.SeekThumbnailState
		want  gen.VideoSeekThumbnailState
	}{
		{state: domain.SeekThumbnailDone, want: gen.VideoSeekThumbnailStateDone},
		{state: domain.SeekThumbnailPending, want: gen.VideoSeekThumbnailStatePending},
		{state: domain.SeekThumbnailFailed, want: gen.VideoSeekThumbnailStateFailed},
	}
	for _, tc := range cases {
		t.Run(string(tc.state), func(t *testing.T) {
			video := sampleVideo(1, "動画")
			handler := newTestServer(t, Options{
				Videos:  &fakeLibrary{videos: map[int64]domain.Video{1: video}},
				Catalog: &fakeCatalog{seekStates: map[int64]domain.SeekThumbnailState{1: tc.state}},
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
		Videos:  &fakeLibrary{videos: map[int64]domain.Video{1: pending, 2: audioOnly}},
		Catalog: &fakeCatalog{},
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
		Videos:  &fakeLibrary{videos: map[int64]domain.Video{1: video}},
		Catalog: &fakeCatalog{seekErr: errors.New("disk I/O error")},
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
