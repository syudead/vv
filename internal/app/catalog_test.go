package app

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// fakeCatalogStore は動画の応答を組み立てるときの問い合わせ先の偽物である。
// 消えたプレビューの作り直しは、保存層と同じく状態が done の間の1回だけ積む。
type fakeCatalogStore struct {
	mu sync.Mutex

	previewStates map[int64]domain.PreviewState
	requeued      []int64
	requeueErr    error

	thumbnailJobActive map[int64]bool
	thumbnailJobErr    error

	retried []bool

	siblings  map[string][]domain.RelatedSibling
	neighbors []domain.RelatedNeighbor
	videos    map[int64]domain.Video
	lastDir   string
}

func (f *fakeCatalogStore) RequeueMissingPreview(_ context.Context, id int64, _ string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.requeueErr != nil {
		return false, f.requeueErr
	}
	if f.previewStates[id] != domain.PreviewStateDone {
		return false, nil
	}
	f.previewStates[id] = domain.PreviewStatePending
	f.requeued = append(f.requeued, id)
	return true, nil
}

func (f *fakeCatalogStore) ThumbnailJobActive(_ context.Context, id int64) (bool, error) {
	return f.thumbnailJobActive[id], f.thumbnailJobErr
}

func (f *fakeCatalogStore) RetryProbe(_ context.Context, _ int64, seekThumbnailMissing bool) error {
	f.retried = append(f.retried, seekThumbnailMissing)
	return nil
}

func (f *fakeCatalogStore) DirectVideoPaths(_ context.Context, dir string) ([]domain.RelatedSibling, error) {
	f.lastDir = dir
	return f.siblings[dir], nil
}

func (f *fakeCatalogStore) VideosAddedNear(context.Context, int64, time.Time, int) ([]domain.RelatedNeighbor, error) {
	return f.neighbors, nil
}

func (f *fakeCatalogStore) VideosByIDs(_ context.Context, ids []int64) ([]domain.Video, error) {
	out := []domain.Video{}
	for _, id := range ids {
		if video, ok := f.videos[id]; ok {
			out = append(out, video)
		}
	}
	return out, nil
}

// fakeArtifactFiles は生成物のファイルの有無を決め打ちで答える。
type fakeArtifactFiles struct {
	previews map[string]bool
	seek     map[string]bool
}

func (f fakeArtifactFiles) PreviewAvailable(key string) bool        { return f.previews[key] }
func (f fakeArtifactFiles) SeekThumbnailsAvailable(key string) bool { return f.seek[key] }

func donePreviewVideo() domain.Video {
	video := probedVideo(1, "a")
	video.PreviewState = domain.PreviewStateDone
	return video
}

// ファイルがあれば URL を出せると答え、何も書かない。
func TestPresentVideosWithPreviewFile(t *testing.T) {
	video := donePreviewVideo()
	store := &fakeCatalogStore{previewStates: map[int64]domain.PreviewState{1: domain.PreviewStateDone}}
	catalog := NewCatalog(CatalogOptions{Index: store, Ingest: store, Files: fakeArtifactFiles{previews: map[string]bool{"a": true}}})

	views := catalog.PresentVideos(context.Background(), []domain.Video{video})
	if len(views) != 1 || !views[0].PreviewAvailable || views[0].Video.PreviewState != domain.PreviewStateDone {
		t.Fatalf("views = %+v", views)
	}
	if len(store.requeued) != 0 {
		t.Fatalf("ファイルがあるのに作り直しを積んだ: %v", store.requeued)
	}
}

// 作り終えた記録があるのにファイルが無ければ、作り直しを積み、準備中として返す。
// 同じ欠損を並行して見つけても、積むのは1回だけである。
func TestPresentVideosRequeuesMissingPreviewOnce(t *testing.T) {
	video := donePreviewVideo()
	store := &fakeCatalogStore{previewStates: map[int64]domain.PreviewState{1: domain.PreviewStateDone}}
	catalog := NewCatalog(CatalogOptions{Index: store, Ingest: store, Files: fakeArtifactFiles{}, Logger: discardLogger()})

	var wg sync.WaitGroup
	results := make([]domain.VideoView, 8)
	for i := range results {
		wg.Go(func() {
			results[i] = catalog.PresentVideos(context.Background(), []domain.Video{video})[0]
		})
	}
	wg.Wait()

	if len(store.requeued) != 1 {
		t.Fatalf("作り直しの予約 = %v, want 1回", store.requeued)
	}
	pending := 0
	for _, view := range results {
		if view.PreviewAvailable {
			t.Fatal("ファイルが無いのに URL を出せると答えた")
		}
		if view.Video.PreviewState == domain.PreviewStatePending {
			pending++
		}
	}
	if pending != 1 {
		t.Fatalf("準備中と読み替えた応答 = %d, want 積んだ1回だけ", pending)
	}
}

// 作り直しを積めなくても応答は返す。URL は出さず、状態は書き換えない。
func TestPresentVideosKeepsStateWhenRequeueFails(t *testing.T) {
	video := donePreviewVideo()
	store := &fakeCatalogStore{requeueErr: errors.New("database is locked")}
	catalog := NewCatalog(CatalogOptions{Index: store, Ingest: store, Files: fakeArtifactFiles{}, Logger: discardLogger()})

	view := catalog.PresentVideos(context.Background(), []domain.Video{video})[0]
	if view.PreviewAvailable || view.Video.PreviewState != domain.PreviewStateDone {
		t.Fatalf("view = %+v", view)
	}
}

// done でないプレビューは、ファイルを確かめず、積みもしない。
func TestPresentVideosIgnoresUnfinishedPreview(t *testing.T) {
	video := probedVideo(1, "a")
	store := &fakeCatalogStore{previewStates: map[int64]domain.PreviewState{}}
	catalog := NewCatalog(CatalogOptions{Index: store, Ingest: store, Files: fakeArtifactFiles{previews: map[string]bool{"a": true}}})

	view := catalog.PresentVideos(context.Background(), []domain.Video{video})[0]
	if view.PreviewAvailable || len(store.requeued) != 0 {
		t.Fatalf("view = %+v, requeued = %v", view, store.requeued)
	}
}

func TestSeekThumbnailState(t *testing.T) {
	cases := []struct {
		name      string
		dir       bool
		thumbnail domain.ThumbnailState
		active    bool
		want      domain.SeekThumbnailState
	}{
		{name: "置き場あり", dir: true, thumbnail: domain.ThumbnailStateDone, want: domain.SeekThumbnailDone},
		{name: "サムネイル未作成", thumbnail: domain.ThumbnailStatePending, want: domain.SeekThumbnailPending},
		{name: "ジョブが queued・running", thumbnail: domain.ThumbnailStateDone, active: true, want: domain.SeekThumbnailPending},
		{name: "ジョブが failed", thumbnail: domain.ThumbnailStateFailed, want: domain.SeekThumbnailFailed},
		// 失敗したジョブの行が保持期間を過ぎて消えても pending と読まない。
		{name: "ジョブの行が消えた", thumbnail: domain.ThumbnailStateDone, want: domain.SeekThumbnailFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			video := probedVideo(1, "a")
			video.ThumbnailState = tc.thumbnail
			store := &fakeCatalogStore{thumbnailJobActive: map[int64]bool{1: tc.active}}
			catalog := NewCatalog(CatalogOptions{
				Index: store, Ingest: store,
				Files: fakeArtifactFiles{seek: map[string]bool{"a": tc.dir}},
			})
			got, err := catalog.SeekThumbnailState(context.Background(), video)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("state = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSeekThumbnailStateReportsStoreFailure(t *testing.T) {
	video := probedVideo(1, "a")
	video.ThumbnailState = domain.ThumbnailStateDone
	store := &fakeCatalogStore{thumbnailJobErr: errors.New("disk I/O error")}
	catalog := NewCatalog(CatalogOptions{
		Index: store, Ingest: store, Files: fakeArtifactFiles{},
	})
	if _, err := catalog.SeekThumbnailState(context.Background(), video); err == nil {
		t.Fatal("失敗が返らない")
	}
}

// 代表の所在のディレクトリで並べ、nextId は後続の先頭を指す。
func TestRelatedVideos(t *testing.T) {
	videos := map[int64]domain.Video{}
	for id, name := range map[int64]string{1: "ep 2", 2: "ep 10", 3: "ep 9", 4: "other"} {
		video := probedVideo(id, name)
		video.Path = "/media/show/" + name + ".mp4"
		videos[id] = video
	}
	videos[4] = func(v domain.Video) domain.Video { v.Path = "/media/other/other.mp4"; return v }(videos[4])
	store := &fakeCatalogStore{
		siblings: map[string][]domain.RelatedSibling{"/media/show": {
			{VideoID: 1, Path: "/media/show/ep 2.mp4"},
			{VideoID: 2, Path: "/media/show/ep 10.mp4"},
			{VideoID: 3, Path: "/media/show/ep 9.mp4"},
		}},
		neighbors: []domain.RelatedNeighbor{{VideoID: 4, AddedAt: time.Unix(1_757_000_001, 0)}},
		videos:    videos,
	}
	catalog := NewCatalog(CatalogOptions{Index: store, Ingest: store, Files: fakeArtifactFiles{}})

	got, err := catalog.RelatedVideos(context.Background(), videos[3])
	if err != nil {
		t.Fatal(err)
	}
	if store.lastDir != "/media/show" {
		t.Errorf("dir = %q, want /media/show", store.lastDir)
	}
	ids := make([]int64, 0, len(got.Items))
	for _, item := range got.Items {
		ids = append(ids, item.ID)
	}
	if want := []int64{2, 1, 4}; !slices.Equal(ids, want) {
		t.Fatalf("ids = %v, want %v", ids, want)
	}
	if got.NextID != 2 {
		t.Fatalf("nextId = %d, want 2", got.NextID)
	}

	// 同じフォルダの後続が無いとき NextID は 0 で、先行の先頭から並ぶ。
	got, err = catalog.RelatedVideos(context.Background(), videos[2])
	if err != nil {
		t.Fatal(err)
	}
	if got.NextID != 0 || len(got.Items) == 0 || got.Items[0].ID != 1 {
		t.Fatalf("フォルダの末尾: next = %d, items = %+v", got.NextID, got.Items)
	}
}

// 読み取りのやり直しには、シーク用プレビューの置き場の有無を確かめて渡す。
func TestRetryProbePassesSeekThumbnailPresence(t *testing.T) {
	store := &fakeCatalogStore{}
	catalog := NewCatalog(CatalogOptions{Index: store, Ingest: store, Files: fakeArtifactFiles{seek: map[string]bool{"present": true}}})

	for _, key := range []string{"missing", "present"} {
		if err := catalog.RetryProbe(context.Background(), probedVideo(1, key)); err != nil {
			t.Fatal(err)
		}
	}
	if want := []bool{true, false}; !slices.Equal(store.retried, want) {
		t.Fatalf("seekThumbnailMissing = %v, want %v", store.retried, want)
	}
}
