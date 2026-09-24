package httpapi

import (
	"errors"
	"net/http"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

func relatedFixture() (*fakeLibrary, *fakeCatalog) {
	videos := map[int64]domain.Video{}
	for id, name := range map[int64]string{1: "ep 2", 2: "ep 10", 3: "ep 9", 4: "other"} {
		video := sampleVideo(id, name)
		video.Path = "/media/show/" + name + ".mp4"
		video.ContentKey = name + ":1"
		videos[id] = video
	}
	videos[4] = func(v domain.Video) domain.Video { v.Path = "/media/other/other.mp4"; return v }(videos[4])
	catalog := &fakeCatalog{related: domain.RelatedVideos{
		Items:  []domain.Video{videos[2], videos[1], videos[4]},
		NextID: 2,
	}}
	return &fakeLibrary{videos: videos}, catalog
}

// アプリケーション層が並べた順に返し、nextId を写す。一覧と同じく progress を
// 付け、所在は載せない。並べ方は internal/app の Catalog.RelatedVideos で検証する。
func TestGetRelatedVideos(t *testing.T) {
	library, catalog := relatedFixture()
	playback := newFakePlayback()
	playback.saved["ep 2:1"] = domain.Progress{PositionMs: 1000}
	handler := newTestServer(t, Options{Videos: library, Catalog: catalog, Playback: playback})

	rec := do(t, handler, http.MethodGet, "/api/videos/3/related")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	got := decode[gen.RelatedVideos](t, rec)
	if !slices.Equal(catalog.relatedAsked, []int64{3}) {
		t.Errorf("基準の動画 = %v, want [3]", catalog.relatedAsked)
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
	library, catalog := relatedFixture()
	catalog.related.NextID = 0
	handler := newTestServer(t, Options{Videos: library, Catalog: catalog})

	got := decode[gen.RelatedVideos](t, do(t, handler, http.MethodGet, "/api/videos/2/related"))
	if got.NextId != nil {
		t.Fatalf("nextId = %d, want 省略", *got.NextId)
	}
}

// 関連動画の応答にもタグが載り、タグの無い動画は空配列になる（#267）。
func TestGetRelatedVideosIncludesTags(t *testing.T) {
	library, catalog := relatedFixture()
	tags := &fakeTags{byContentKey: map[string][]domain.TagRef{
		"ep 10:1": {{ID: 1, Name: "旅行"}},
	}}
	handler := newTestServer(t, Options{Videos: library, Catalog: catalog, Tags: tags})

	got := decode[gen.RelatedVideos](t, do(t, handler, http.MethodGet, "/api/videos/3/related"))
	// items[0] は videos[2]（"ep 10"、content key "ep 10:1"）である（relatedFixture 参照）。
	if len(got.Items[0].Tags) != 1 || got.Items[0].Tags[0].Name != "旅行" {
		t.Fatalf("tags = %+v", got.Items[0].Tags)
	}
	if got.Items[1].Tags == nil || len(got.Items[1].Tags) != 0 {
		t.Fatalf("tags = %+v, want 空配列", got.Items[1].Tags)
	}
}

func TestGetRelatedVideosErrors(t *testing.T) {
	library, catalog := relatedFixture()
	handler := newTestServer(t, Options{Videos: library, Catalog: catalog})
	if rec := do(t, handler, http.MethodGet, "/api/videos/99/related"); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown id: status = %d, want 404", rec.Code)
	}

	catalog.relatedErr = errors.New("disk I/O error")
	if rec := do(t, handler, http.MethodGet, "/api/videos/1/related"); rec.Code != http.StatusInternalServerError {
		t.Fatalf("store failure: status = %d, want 500", rec.Code)
	}
}
