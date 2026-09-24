package httpapi

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// fakeRelated は同じフォルダの動画と追加日時の近い動画を決め打ちで返す。
type fakeRelated struct {
	siblings  map[string][]domain.RelatedSibling
	neighbors []domain.RelatedNeighbor
	videos    map[int64]domain.Video
	err       error
	// lastDir は最後に問い合わせたディレクトリ。
	lastDir string
}

func (f *fakeRelated) DirectVideoPaths(_ context.Context, dir string) ([]domain.RelatedSibling, error) {
	f.lastDir = dir
	return f.siblings[dir], f.err
}

func (f *fakeRelated) VideosAddedNear(context.Context, int64, time.Time, int) ([]domain.RelatedNeighbor, error) {
	return f.neighbors, nil
}

func (f *fakeRelated) VideosByIDs(_ context.Context, ids []int64) ([]domain.Video, error) {
	videos := []domain.Video{}
	for _, id := range ids {
		if video, ok := f.videos[id]; ok {
			videos = append(videos, video)
		}
	}
	return videos, nil
}

func relatedFixture() (*fakeLibrary, *fakeRelated) {
	videos := map[int64]domain.Video{}
	for id, name := range map[int64]string{1: "ep 2", 2: "ep 10", 3: "ep 9", 4: "other"} {
		video := sampleVideo(id, name)
		video.Path = "/media/show/" + name + ".mp4"
		video.ContentKey = name + ":1"
		videos[id] = video
	}
	videos[4] = func(v domain.Video) domain.Video { v.Path = "/media/other/other.mp4"; return v }(videos[4])
	related := &fakeRelated{
		siblings: map[string][]domain.RelatedSibling{"/media/show": {
			{VideoID: 1, Path: "/media/show/ep 2.mp4"},
			{VideoID: 2, Path: "/media/show/ep 10.mp4"},
			{VideoID: 3, Path: "/media/show/ep 9.mp4"},
		}},
		neighbors: []domain.RelatedNeighbor{{VideoID: 4, AddedAt: time.Unix(1_757_000_001, 0)}},
		videos:    videos,
	}
	return &fakeLibrary{videos: videos}, related
}

// 代表の所在のディレクトリで並べ、nextId は後続の先頭を指す。一覧と同じく
// progress を付け、所在は載せない。
func TestGetRelatedVideos(t *testing.T) {
	library, related := relatedFixture()
	playback := newFakePlayback()
	playback.saved["ep 2:1"] = domain.Progress{PositionMs: 1000}
	handler := newTestServer(t, Options{Videos: library, Related: related, Playback: playback})

	rec := do(t, handler, http.MethodGet, "/api/videos/3/related")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	got := decode[gen.RelatedVideos](t, rec)
	if related.lastDir != "/media/show" {
		t.Errorf("dir = %q, want /media/show", related.lastDir)
	}
	ids := make([]int64, 0, len(got.Items))
	for _, item := range got.Items {
		ids = append(ids, item.Id)
		if item.Location != nil || item.SeekThumbnailState != nil {
			t.Errorf("関連動画 %d に詳細だけの項目が載っている", item.Id)
		}
	}
	if want := []int64{2, 1, 4}; !slices.Equal(ids, want) {
		t.Fatalf("ids = %v, want %v", ids, want)
	}
	if got.NextId == nil || *got.NextId != 2 {
		t.Fatalf("nextId = %v, want 2", got.NextId)
	}
	if got.Items[1].Progress == nil || got.Items[1].Progress.PositionMs != 1000 {
		t.Errorf("progress = %+v, want 1000", got.Items[1].Progress)
	}
	if rec.Header().Get("Cache-Control") != cacheNoStore {
		t.Errorf("Cache-Control = %q", rec.Header().Get("Cache-Control"))
	}
}

// 同じフォルダの後続が無いとき nextId は省かれる。
func TestGetRelatedVideosOmitsNextAtFolderEnd(t *testing.T) {
	library, related := relatedFixture()
	handler := newTestServer(t, Options{Videos: library, Related: related})

	got := decode[gen.RelatedVideos](t, do(t, handler, http.MethodGet, "/api/videos/2/related"))
	if got.NextId != nil {
		t.Fatalf("nextId = %d, want 省略", *got.NextId)
	}
	if len(got.Items) == 0 || got.Items[0].Id != 1 {
		t.Fatalf("items = %+v, want 先行の先頭が ep 2", got.Items)
	}
}

func TestGetRelatedVideosErrors(t *testing.T) {
	library, related := relatedFixture()
	handler := newTestServer(t, Options{Videos: library, Related: related})
	if rec := do(t, handler, http.MethodGet, "/api/videos/99/related"); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown id: status = %d, want 404", rec.Code)
	}

	related.err = errors.New("disk I/O error")
	if rec := do(t, handler, http.MethodGet, "/api/videos/1/related"); rec.Code != http.StatusInternalServerError {
		t.Fatalf("store failure: status = %d, want 500", rec.Code)
	}
}
