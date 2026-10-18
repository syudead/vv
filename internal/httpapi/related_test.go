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
		video.ContentKey = name + ":1"
		video.UserKey = video.ContentKey
		video.ContentKey = name + ":1"
		videos[id] = video
	}
	videos[4] = func(v domain.Video) domain.Video { v.Path = "/media/other/other.mp4"; return v }(videos[4])
	catalog := &fakeCatalog{related: domain.RelatedVideos{
		Items:  []domain.Video{videos[2], videos[1], videos[4]},
		NextID: 2,
		PrevID: 1,
	}}
	return &fakeLibrary{videos: videos}, catalog
}

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
	if got.PrevId == nil || *got.PrevId != 1 {
		t.Fatalf("prevId = %v, want 1", got.PrevId)
	}
	if got.Items[1].Progress == nil || got.Items[1].Progress.PositionMs != 1000 {
		t.Errorf("progress = %+v, want 1000", got.Items[1].Progress)
	}
	if rec.Header().Get("Cache-Control") != cacheNoStore {
		t.Errorf("Cache-Control = %q", rec.Header().Get("Cache-Control"))
	}
}

func TestGetRelatedVideosPages(t *testing.T) {
	library, catalog := relatedFixture()
	cursor := domain.RelatedCursor{Score: 25_000_000, AddedAt: 1_757_000_000, VideoID: 4}
	catalog.related.NextCursor = &cursor
	handler := newTestServer(t, Options{Videos: library, Catalog: catalog})

	got := decode[gen.RelatedVideos](t, do(t, handler, http.MethodGet, "/api/videos/3/related"))
	if got.NextCursor == nil || *got.NextCursor != cursor.String() {
		t.Fatalf("nextCursor = %v, want %q", got.NextCursor, cursor.String())
	}
	rec := do(t, handler, http.MethodGet, "/api/videos/3/related?limit=5&cursor="+cursor.String())
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	want := []domain.RelatedPage{
		{Limit: domain.DefaultRelatedPageSize},
		{After: &cursor, Limit: 5},
	}
	if len(catalog.relatedPages) != 2 || catalog.relatedPages[0] != want[0] ||
		catalog.relatedPages[1].Limit != 5 || *catalog.relatedPages[1].After != cursor {
		t.Fatalf("pages = %+v, want %+v", catalog.relatedPages, want)
	}

	for _, query := range []string{"limit=0", "limit=101", "cursor=bogus", "cursor=1.2"} {
		if rec := do(t, handler, http.MethodGet, "/api/videos/3/related?"+query); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", query, rec.Code)
		}
	}
}

func TestGetRelatedVideosOmitsMissingNeighbors(t *testing.T) {
	library, catalog := relatedFixture()
	catalog.related.NextID = 0
	catalog.related.PrevID = 0
	handler := newTestServer(t, Options{Videos: library, Catalog: catalog})

	got := decode[gen.RelatedVideos](t, do(t, handler, http.MethodGet, "/api/videos/2/related"))
	if got.NextId != nil {
		t.Fatalf("nextId = %d, want 省略", *got.NextId)
	}
	if got.PrevId != nil {
		t.Fatalf("prevId = %d, want 省略", *got.PrevId)
	}
}

func TestGetRelatedVideosIncludesTags(t *testing.T) {
	library, catalog := relatedFixture()
	tags := &fakeTags{byContentKey: map[string][]domain.VideoTag{
		"ep 10:1": {manualTag(1, "旅行")},
	}}
	handler := newTestServer(t, Options{Videos: library, Catalog: catalog, Tags: tags})

	got := decode[gen.RelatedVideos](t, do(t, handler, http.MethodGet, "/api/videos/3/related"))
	// items[0] corresponds to videos[2], ep 10 with content key ep 10:1.
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
