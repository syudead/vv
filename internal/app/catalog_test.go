package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// The fake mirrors storage's once-per-done-state repair enqueueing.
type fakeCatalogStore struct {
	mu sync.Mutex

	previewStates map[int64]domain.PreviewState
	requeued      []int64
	requeueErr    error

	seekStates     map[int64]domain.SeekThumbnailState
	seekRequeued   []int64
	seekRequeueErr error

	retried []int64

	candidates []domain.RelatedCandidate
	tagNames   map[int64]string
	videos     map[int64]domain.Video
	groups     map[int64]domain.VideoGroup
	windows    []domain.GroupWindow
	contexts   []int64
	audiences  []domain.Audience
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

func (f *fakeCatalogStore) RequeueMissingSeekThumbnails(_ context.Context, id int64, _ string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.seekRequeueErr != nil {
		return false, f.seekRequeueErr
	}
	if f.seekStates[id] != domain.SeekThumbnailDone {
		return false, nil
	}
	f.seekStates[id] = domain.SeekThumbnailPending
	f.seekRequeued = append(f.seekRequeued, id)
	return true, nil
}

func (f *fakeCatalogStore) RetryProbe(_ context.Context, id int64) error {
	f.retried = append(f.retried, id)
	return nil
}

func (f *fakeCatalogStore) RelatedCandidates(_ context.Context, audience domain.Audience, _ int64) ([]domain.RelatedCandidate, error) {
	f.audiences = append(f.audiences, audience)
	return f.candidates, nil
}

func (f *fakeCatalogStore) VideosByIDs(_ context.Context, audience domain.Audience, ids []int64) ([]domain.Video, error) {
	f.audiences = append(f.audiences, audience)
	out := []domain.Video{}
	for _, id := range ids {
		if video, ok := f.videos[id]; ok {
			out = append(out, video)
		}
	}
	return out, nil
}

func (f *fakeCatalogStore) RelatedTagNames(_ context.Context, ids []int64) (map[int64]string, error) {
	names := map[int64]string{}
	for _, id := range ids {
		if name, ok := f.tagNames[id]; ok {
			names[id] = name
		}
	}
	return names, nil
}

func (f *fakeCatalogStore) RelatedTagIDs(_ context.Context, names []string) (map[string]int64, error) {
	ids := map[string]int64{}
	for id, name := range f.tagNames {
		if slices.Contains(names, name) {
			ids[name] = id
		}
	}
	return ids, nil
}

// Return IDs from all members but details only within the requested window, matching storage.
func (f *fakeCatalogStore) VideoGroup(_ context.Context, audience domain.Audience, id int64, opened domain.VideoContext, window domain.GroupWindow) (domain.VideoGroup, bool, error) {
	f.audiences = append(f.audiences, audience)
	f.contexts = append(f.contexts, opened.GroupID)
	f.windows = append(f.windows, window)
	group, ok := f.groups[id]
	if !ok {
		return domain.VideoGroup{}, false, nil
	}
	all := group.Members
	group.MemberIDs = make([]int64, 0, len(all))
	for _, member := range all {
		group.MemberIDs = append(group.MemberIDs, member.ID)
	}
	group.Members = nil
	if window.Limit > 0 {
		group.Offset = window.Start(len(all), group.Position(id))
		group.Members = all[group.Offset:min(group.Offset+window.Limit, len(all))]
	}
	return group, true, nil
}

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

func TestPresentVideosKeepsStateWhenRequeueFails(t *testing.T) {
	video := donePreviewVideo()
	store := &fakeCatalogStore{requeueErr: errors.New("database is locked")}
	catalog := NewCatalog(CatalogOptions{Index: store, Ingest: store, Files: fakeArtifactFiles{}, Logger: discardLogger()})

	view := catalog.PresentVideos(context.Background(), []domain.Video{video})[0]
	if view.PreviewAvailable || view.Video.PreviewState != domain.PreviewStateDone {
		t.Fatalf("view = %+v", view)
	}
}

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
		name   string
		dir    bool
		stored domain.SeekThumbnailState
		want   domain.SeekThumbnailState
	}{
		{name: "置き場あり", dir: true, stored: domain.SeekThumbnailDone, want: domain.SeekThumbnailDone},
		{name: "置き場があれば列が pending でも done", dir: true, stored: domain.SeekThumbnailPending, want: domain.SeekThumbnailDone},
		{name: "生成待ち", stored: domain.SeekThumbnailPending, want: domain.SeekThumbnailPending},
		{name: "失敗", stored: domain.SeekThumbnailFailed, want: domain.SeekThumbnailFailed},
		// A forced rebuild that gave up keeps the published sprite; the failure must not read as done.
		{name: "置き場があっても失敗は失敗", dir: true, stored: domain.SeekThumbnailFailed, want: domain.SeekThumbnailFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			video := probedVideo(1, "a")
			video.ThumbnailState = domain.ThumbnailStateDone
			video.SeekThumbnailState = tc.stored
			store := &fakeCatalogStore{seekStates: map[int64]domain.SeekThumbnailState{1: tc.stored}}
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
			if len(store.seekRequeued) != 0 {
				t.Fatalf("requeued = %v, want none", store.seekRequeued)
			}
		})
	}
}

func TestSeekThumbnailStateRequeuesMissingOnce(t *testing.T) {
	video := probedVideo(1, "a")
	video.SeekThumbnailState = domain.SeekThumbnailDone
	store := &fakeCatalogStore{seekStates: map[int64]domain.SeekThumbnailState{1: domain.SeekThumbnailDone}}
	catalog := NewCatalog(CatalogOptions{Index: store, Ingest: store, Files: fakeArtifactFiles{}})

	for range 2 {
		got, err := catalog.SeekThumbnailState(context.Background(), video)
		if err != nil {
			t.Fatal(err)
		}
		if got != domain.SeekThumbnailPending {
			t.Fatalf("state = %q, want pending", got)
		}
	}
	if want := []int64{1}; !slices.Equal(store.seekRequeued, want) {
		t.Fatalf("requeued = %v, want %v", store.seekRequeued, want)
	}
}

func TestSeekThumbnailStateIgnoresRequeueFailure(t *testing.T) {
	video := probedVideo(1, "a")
	video.SeekThumbnailState = domain.SeekThumbnailDone
	store := &fakeCatalogStore{seekRequeueErr: errors.New("disk I/O error")}
	catalog := NewCatalog(CatalogOptions{Index: store, Ingest: store, Files: fakeArtifactFiles{}})
	got, err := catalog.SeekThumbnailState(context.Background(), video)
	if err != nil || got != domain.SeekThumbnailPending {
		t.Fatalf("state = %q, err = %v, want pending", got, err)
	}
}

func relatedFixture(entries ...domain.RelatedCandidate) *fakeCatalogStore {
	store := &fakeCatalogStore{videos: map[int64]domain.Video{}}
	for _, entry := range entries {
		video := probedVideo(entry.VideoID, entry.Title)
		video.Path = entry.Path
		video.AddedAt = entry.AddedAt
		store.videos[entry.VideoID] = video
		store.candidates = append(store.candidates, entry)
	}
	return store
}

var relatedBase = time.Unix(1_757_000_000, 0)

func relatedEntry(id int64, path string, tags ...int64) domain.RelatedCandidate {
	return domain.RelatedCandidate{VideoID: id, Path: fixturePath(path), Title: "v", AddedAt: relatedBase, TagIDs: tags}
}

func videoIDs(videos []domain.Video) []int64 {
	ids := make([]int64, 0, len(videos))
	for _, video := range videos {
		ids = append(ids, video.ID)
	}
	return ids
}

func TestRelatedVideos(t *testing.T) {
	store := relatedFixture(
		relatedEntry(1, "/media/show/a.mp4", 7, 8),
		relatedEntry(2, "/media/show/b.mp4"),
		relatedEntry(3, "/media/other/c.mp4", 7, 8),
		relatedEntry(4, "/media/else/d.mp4", 7, 9),
		relatedEntry(5, "/media/far/e.mp4"),
	)
	catalog := NewCatalog(CatalogOptions{Index: store, Ingest: store, Files: fakeArtifactFiles{}})
	ctx := context.Background()

	got, err := catalog.RelatedVideos(ctx, domain.AudienceOwner, store.videos[1], domain.RelatedPage{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if want := []int64{3, 2}; !slices.Equal(videoIDs(got.Items), want) {
		t.Fatalf("ids = %v, want %v", videoIDs(got.Items), want)
	}
	if got.NextID != 3 || got.PrevID != 0 || got.Group != nil || got.NextCursor == nil {
		t.Fatalf("next = %d, prev = %d, group = %+v, cursor = %v", got.NextID, got.PrevID, got.Group, got.NextCursor)
	}

	rest, err := catalog.RelatedVideos(ctx, domain.AudienceOwner, store.videos[1], domain.RelatedPage{After: got.NextCursor, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if want := []int64{4, 5}; !slices.Equal(videoIDs(rest.Items), want) {
		t.Fatalf("rest = %v, want %v", videoIDs(rest.Items), want)
	}
	if rest.NextID != 0 || rest.NextCursor != nil || rest.Group != nil {
		t.Fatalf("rest next = %d, cursor = %v, group = %+v", rest.NextID, rest.NextCursor, rest.Group)
	}
	if last := store.windows[len(store.windows)-1]; last.Limit != 0 {
		t.Errorf("続きのページがグループの詳細を読んだ: %+v", last)
	}
}

func TestRelatedVideosEmpty(t *testing.T) {
	store := relatedFixture(relatedEntry(1, "/media/a.mp4"))
	catalog := NewCatalog(CatalogOptions{Index: store, Ingest: store, Files: fakeArtifactFiles{}})
	got, err := catalog.RelatedVideos(context.Background(), domain.AudienceOwner, store.videos[1], domain.RelatedPage{Limit: 10})
	if err != nil || len(got.Items) != 0 || got.NextID != 0 || got.NextCursor != nil {
		t.Fatalf("only self = %+v, %v", got, err)
	}

	gone := relatedFixture(relatedEntry(2, "/media/b.mp4"))
	catalog = NewCatalog(CatalogOptions{Index: gone, Ingest: gone, Files: fakeArtifactFiles{}})
	got, err = catalog.RelatedVideos(context.Background(), domain.AudienceOwner, probedVideo(1, "a"), domain.RelatedPage{Limit: 10})
	if err != nil || len(got.Items) != 0 || got.NextID != 0 {
		t.Fatalf("self gone = %+v, %v", got, err)
	}
}

type reverseIDScorer struct{}

func (reverseIDScorer) For(domain.RelatedCandidate) func(domain.RelatedCandidate) domain.RelatedScore {
	return func(other domain.RelatedCandidate) domain.RelatedScore { return domain.RelatedScore(-other.VideoID) }
}

func TestRelatedVideosUsesTheGivenScorer(t *testing.T) {
	store := relatedFixture(
		relatedEntry(1, "/media/show/a.mp4", 7),
		relatedEntry(2, "/media/x/b.mp4"),
		relatedEntry(3, "/media/show/c.mp4", 7),
	)
	catalog := NewCatalog(CatalogOptions{Index: store, Ingest: store, Files: fakeArtifactFiles{}, Scorer: reverseIDScorer{}})
	got, err := catalog.RelatedVideos(context.Background(), domain.AudienceOwner, store.videos[1], domain.RelatedPage{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if want := []int64{2, 3}; !slices.Equal(videoIDs(got.Items), want) {
		t.Fatalf("ids = %v, want %v", videoIDs(got.Items), want)
	}
}

// Preserve audience across all three reads so guests cannot see private related videos.
func TestRelatedVideosPassesAudience(t *testing.T) {
	for _, audience := range []domain.Audience{domain.AudienceGuest, domain.AudienceOwner} {
		store := relatedFixture(relatedEntry(1, "/media/show/a.mp4"), relatedEntry(2, "/media/show/b.mp4"))
		catalog := NewCatalog(CatalogOptions{Index: store, Ingest: store, Files: fakeArtifactFiles{}})
		if _, err := catalog.RelatedVideos(context.Background(), audience, store.videos[1], domain.RelatedPage{Limit: 10}); err != nil {
			t.Fatal(err)
		}
		if want := []domain.Audience{audience, audience, audience}; !slices.Equal(store.audiences, want) {
			t.Errorf("%s: audiences = %v, want %v", audience, store.audiences, want)
		}
	}
}

func TestRetryProbePassesVideoID(t *testing.T) {
	store := &fakeCatalogStore{}
	catalog := NewCatalog(CatalogOptions{Index: store, Ingest: store, Files: fakeArtifactFiles{}})
	if err := catalog.RetryProbe(context.Background(), probedVideo(7, "a")); err != nil {
		t.Fatal(err)
	}
	if want := []int64{7}; !slices.Equal(store.retried, want) {
		t.Fatalf("retried = %v, want %v", store.retried, want)
	}
}

// Exclude group members before ranking so large groups cannot crowd out related videos.
func TestRelatedVideosForGroupMember(t *testing.T) {
	const size = 25
	var entries []domain.RelatedCandidate
	for i := range size {
		entries = append(entries, relatedEntry(int64(i+1), fmt.Sprintf("/media/big/%02d.mp4", i+1), 7))
	}
	entries = append(entries, relatedEntry(100, "/media/big/zz.mp4", 7), relatedEntry(200, "/media/far/far.mp4"))
	store := relatedFixture(entries...)
	var members []domain.Video
	for i := range size {
		members = append(members, store.videos[int64(i+1)])
	}
	group := domain.VideoGroup{ID: 7, Folder: &domain.VideoFolder{RootID: 1, Path: "big"}, Name: "big", Members: members}
	groups := map[int64]domain.VideoGroup{}
	for _, member := range members {
		groups[member.ID] = group
	}
	store.groups = groups

	for _, tc := range []struct {
		name       string
		self       int64
		prev, next int64
	}{
		{name: "最初", self: 1, prev: 0, next: 2},
		{name: "途中", self: 10, prev: 9, next: 11},
		{name: "最後", self: size, prev: size - 1, next: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store.windows = nil
			catalog := NewCatalog(CatalogOptions{Index: store, Ingest: store, Files: fakeArtifactFiles{}})
			got, err := catalog.RelatedVideos(context.Background(), domain.AudienceOwner, store.videos[tc.self], domain.RelatedPage{Limit: 10})
			if err != nil {
				t.Fatal(err)
			}
			if got.Group == nil || got.Group.Total() != size || len(got.Group.Members) != size {
				t.Fatalf("group = %+v, want %d 本の全メンバー", got.Group, size)
			}
			if want := (domain.GroupWindow{Limit: domain.GroupMemberWindow, Around: true}); store.windows[0] != want {
				t.Errorf("window = %+v, want %+v", store.windows[0], want)
			}
			if got.PrevID != tc.prev || got.NextID != tc.next {
				t.Errorf("前後 = %d・%d, want %d・%d", got.PrevID, got.NextID, tc.prev, tc.next)
			}
			if want := []int64{100, 200}; !slices.Equal(videoIDs(got.Items), want) {
				t.Errorf("関連動画 = %v, want グループの外の %v", videoIDs(got.Items), want)
			}
		})
	}
}

func TestVideoGroupPassesAudience(t *testing.T) {
	group := domain.VideoGroup{Name: "g", Members: []domain.Video{probedVideo(1, "a"), probedVideo(2, "b")}}
	store := &fakeCatalogStore{groups: map[int64]domain.VideoGroup{1: group}}
	catalog := NewCatalog(CatalogOptions{Index: store, Ingest: store, Files: fakeArtifactFiles{}})
	window := domain.GroupWindow{Offset: 1, Limit: 5}
	got, ok, err := catalog.VideoGroup(context.Background(), domain.AudienceGuest, probedVideo(1, "a"), domain.VideoContext{GroupID: 4}, window)
	if err != nil || !ok || got.Name != "g" {
		t.Fatalf("VideoGroup = %+v・%v・%v", got, ok, err)
	}
	if !slices.Equal(store.audiences, []domain.Audience{domain.AudienceGuest}) {
		t.Errorf("audiences = %v", store.audiences)
	}
	if !slices.Equal(store.windows, []domain.GroupWindow{window}) {
		t.Errorf("windows = %v", store.windows)
	}
	if !slices.Equal(store.contexts, []int64{4}) {
		t.Errorf("contexts = %v, want [4]", store.contexts)
	}
	if _, ok, _ := catalog.VideoGroup(context.Background(), domain.AudienceGuest, probedVideo(3, "c"), domain.VideoContext{}, window); ok {
		t.Error("メンバーでない動画にグループがある")
	}
}

// Group windows limit details while counts and neighbors use the full membership.
func TestRelatedVideosForLargeGroupCarriesAWindow(t *testing.T) {
	const size = domain.GroupMemberWindow*2 + 50
	videos := map[int64]domain.Video{}
	var members []domain.Video
	for i := range size {
		id := int64(i + 1)
		video := probedVideo(id, "m")
		video.Path = fmt.Sprintf(fixturePath("/media/big/%03d.mp4"), id)
		videos[id] = video
		members = append(members, video)
	}
	group := domain.VideoGroup{ID: 7, Folder: &domain.VideoFolder{RootID: 1, Path: "big"}, Name: "big", Members: members}
	store := &fakeCatalogStore{videos: videos, groups: map[int64]domain.VideoGroup{200: group}}
	catalog := NewCatalog(CatalogOptions{Index: store, Ingest: store, Files: fakeArtifactFiles{}})
	got, err := catalog.RelatedVideos(context.Background(), domain.AudienceOwner, videos[200], domain.RelatedPage{Limit: 10, GroupID: 7})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(store.contexts, []int64{7}) {
		t.Errorf("contexts = %v, want [7]", store.contexts)
	}
	if got.Group == nil || got.Group.Total() != size || len(got.Group.Members) != domain.GroupMemberWindow {
		t.Fatalf("group = %+v", got.Group)
	}
	if want := 200 - 1 - domain.GroupMemberWindow/2; got.Group.Offset != want || got.Group.Members[0].ID != int64(want+1) {
		t.Errorf("offset = %d (first %d), want %d", got.Group.Offset, got.Group.Members[0].ID, want)
	}
	if got.PrevID != 199 || got.NextID != 201 {
		t.Errorf("前後 = %d・%d, want 199・201", got.PrevID, got.NextID)
	}
}
