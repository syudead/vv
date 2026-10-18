package app

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func int64Ptr(value int64) *int64 { return &value }

// Publish the selected or automatic frame before recording its position.
func TestSetThumbnailPositionPublishesThenRecords(t *testing.T) {
	for _, tc := range []struct {
		name      string
		position  *int64
		wantCalls []string
	}{
		{name: "指定", position: int64Ptr(12_000),
			wantCalls: []string{"thumbnail-at", "publish-thumbnail", "set-position"}},
		{name: "解除", position: nil,
			wantCalls: []string{"thumbnail", "publish-thumbnail", "set-position"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			video := probedVideo(1, "a")
			store := newFakeIngestStore(video)
			generator := &fakeGenerator{recordPublish: true}
			store.order = generator
			ingest, _ := newTestIngest(store, generator)

			saved, err := ingest.SetThumbnailPosition(context.Background(), video.ID, video.Path, "/media/a.mp4", tc.position)
			if err != nil {
				t.Fatal(err)
			}
			calls, removed := generator.snapshot()
			if !slices.Equal(calls, tc.wantCalls) {
				t.Fatalf("呼び出し = %v, want %v", calls, tc.wantCalls)
			}
			if tc.position != nil && !slices.Equal(generator.positions, []int64{*tc.position}) {
				t.Fatalf("ThumbnailAt の位置 = %v", generator.positions)
			}
			if len(store.positions) != 1 || saved.ThumbnailState != domain.ThumbnailStateDone {
				t.Fatalf("記録 = %v, 返した動画 = %+v", store.positions, saved)
			}
			if len(removed) != 0 {
				t.Fatalf("参照のある生成物を消した: %v", removed)
			}
		})
	}
}

func TestSetThumbnailPositionGenerationFailure(t *testing.T) {
	for _, tc := range []struct {
		name      string
		position  *int64
		generator *fakeGenerator
	}{
		{name: "指定", position: int64Ptr(1_000), generator: &fakeGenerator{thumbnailAtErr: errors.New("no frame")}},
		{name: "解除", position: nil, generator: &fakeGenerator{thumbnailErr: errors.New("no frame")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			video := probedVideo(1, "a")
			store := newFakeIngestStore(video)
			ingest, _ := newTestIngest(store, tc.generator)

			_, err := ingest.SetThumbnailPosition(context.Background(), video.ID, video.Path, "/media/a.mp4", tc.position)
			if !errors.Is(err, domain.ErrThumbnailFrameUnavailable) {
				t.Fatalf("err = %v, want ErrThumbnailFrameUnavailable", err)
			}
			if len(store.positions) != 0 {
				t.Fatalf("失敗なのに記録した: %v", store.positions)
			}
		})
	}
}

func TestSetThumbnailPositionRejectsBeforeGenerating(t *testing.T) {
	pending := probedVideo(1, "a")
	pending.ProbeState = domain.ProbeStatePending
	pending.DurationMs = nil
	probed := probedVideo(2, "b")
	for _, tc := range []struct {
		name     string
		video    domain.Video
		position int64
		want     error
	}{
		{name: "解析前", video: pending, position: 0, want: domain.ErrDurationUnknown},
		{name: "尺ちょうど", video: probed, position: *probed.DurationMs, want: domain.ErrThumbnailPositionOutOfRange},
		{name: "負", video: probed, position: -1, want: domain.ErrThumbnailPositionOutOfRange},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newFakeIngestStore(tc.video)
			generator := &fakeGenerator{recordPublish: true}
			ingest, _ := newTestIngest(store, generator)

			_, err := ingest.SetThumbnailPosition(context.Background(), tc.video.ID, tc.video.Path, "/media/x.mp4", &tc.position)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if calls, _ := generator.snapshot(); len(calls) != 0 || len(store.positions) != 0 {
				t.Fatalf("呼び出し = %v, 記録 = %v", calls, store.positions)
			}
		})
	}
}

// Generation and recording must both exclude other thumbnail writes and deletion.
func TestSetThumbnailPositionHoldsGenerationLock(t *testing.T) {
	video := probedVideo(1, "a")
	store := newFakeIngestStore(video)
	generator := &fakeGenerator{}
	ingest, _ := newTestIngest(store, generator)
	var phases []string
	check := func(phase string) {
		assertThumbnailLocksHeld(t, ingest.artifacts, "a")
		phases = append(phases, phase)
	}
	generator.gate = check
	ingest.store = &thumbnailLockStore{IngestStore: store, check: check}
	if _, err := ingest.SetThumbnailPosition(context.Background(), video.ID, video.Path, video.Path, int64Ptr(5_000)); err != nil {
		t.Fatal(err)
	}
	if want := []string{"thumbnail-at", "record-position"}; !slices.Equal(phases, want) {
		t.Fatalf("phases = %v, want %v", phases, want)
	}
}

// Selected positions never fall back to the first frame.
func TestIngestThumbnailUsesRecordedPosition(t *testing.T) {
	withPosition := probedVideo(1, "a")
	withPosition.ThumbnailPositionMs = int64Ptr(7_000)
	without := probedVideo(2, "b")
	for _, tc := range []struct {
		name      string
		video     domain.Video
		wantCalls []string
		want      domain.Substitution
	}{
		{name: "位置あり", video: withPosition, wantCalls: []string{"check", "thumbnail-at"}, want: domain.SubstitutionNone},
		{name: "位置なし", video: without, wantCalls: []string{"check", "thumbnail"}, want: domain.SubstitutionNone},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newFakeIngestStore(tc.video)
			generator := &fakeGenerator{}
			ingest, _ := newTestIngest(store, generator)

			if err := ingest.Thumbnail(context.Background(), jobFor(domain.JobThumbnail, tc.video)); err != nil {
				t.Fatal(err)
			}
			calls, _ := generator.snapshot()
			if !slices.Equal(calls, tc.wantCalls) {
				t.Fatalf("呼び出し = %v, want %v", calls, tc.wantCalls)
			}
			if tc.video.ThumbnailPositionMs != nil && !slices.Equal(generator.positions, []int64{7_000}) {
				t.Fatalf("ThumbnailAt の位置 = %v", generator.positions)
			}
			if !slices.Equal(store.thumbnailSubstitutions, []domain.Substitution{tc.want}) {
				t.Fatalf("代用 = %v, want [%v]", store.thumbnailSubstitutions, tc.want)
			}
		})
	}
}

// Failed selected-frame extraction leaves the position override intact; the queue owns retries and
// failure state.
func TestIngestThumbnailAtPositionFailureKeepsPosition(t *testing.T) {
	video := probedVideo(1, "a")
	video.ThumbnailPositionMs = int64Ptr(7_000)
	store := newFakeIngestStore(video)
	generator := &fakeGenerator{thumbnailAtErr: errors.New("no frame")}
	ingest, _ := newTestIngest(store, generator)

	if err := ingest.Thumbnail(context.Background(), jobFor(domain.JobThumbnail, video)); err == nil {
		t.Fatal("失敗が返らない")
	}
	if calls, _ := generator.snapshot(); slices.Contains(calls, "thumbnail") {
		t.Fatalf("先頭のコマで作り直した: %v", calls)
	}
	if len(store.thumbnailStates) != 0 || len(store.positions) != 0 {
		t.Fatalf("状態 = %v, 位置の記録 = %v", store.thumbnailStates, store.positions)
	}
	if got, _ := store.GetVideo(context.Background(), video.ID); got.ThumbnailPositionMs == nil {
		t.Fatal("位置が消えた")
	}
}

// The initial read sees pending; the read under the lock sees the user's completed
// selection. The job must not generate over that newer thumbnail.
func TestIngestThumbnailRereadsInsideLock(t *testing.T) {
	video := probedVideo(1, "a")
	store := newFakeIngestStore(video)
	generator := &fakeGenerator{}
	ingest, _ := newTestIngest(store, generator)
	store.afterRead = func(reads int) {
		if reads == 1 {
			current := video
			current.ThumbnailPositionMs = int64Ptr(9_000)
			current.ThumbnailState = domain.ThumbnailStateDone
			store.videos[video.ID] = current
		} else {
			assertThumbnailLocksHeld(t, ingest.artifacts, "a")
		}
	}
	if err := ingest.Thumbnail(context.Background(), jobFor(domain.JobThumbnail, video)); err != nil {
		t.Fatal(err)
	}
	if store.reads != 2 {
		t.Fatalf("video reads = %d, want 2", store.reads)
	}
	if calls, _ := generator.snapshot(); !slices.Equal(calls, []string{"check"}) {
		t.Fatalf("generation after completed thumbnail: %v", calls)
	}
	if len(store.thumbnailStates) != 0 {
		t.Fatalf("job recorded state: %v", store.thumbnailStates)
	}
}

// Restore the previous image when persistence fails after publication so file, position and
// revision stay consistent.
func TestSetThumbnailPositionRestoresPreviousImageWhenRecordingFails(t *testing.T) {
	video := probedVideo(1, "a")
	store := newFakeIngestStore(video)
	store.setPositionErr = errors.New("database is locked")
	generator := &fakeGenerator{recordPublish: true}
	ingest, _ := newTestIngest(store, generator)

	_, err := ingest.SetThumbnailPosition(context.Background(), video.ID, video.Path, video.Path, int64Ptr(4_000))
	if err == nil || !errors.Is(err, store.setPositionErr) {
		t.Fatalf("err = %v, want 記録の失敗", err)
	}
	if got := generator.stashCalls(); !slices.Equal(got, []string{"stash", "restore"}) {
		t.Fatalf("写しの操作 = %v, want [stash restore]", got)
	}

	store.setPositionErr = nil
	generator.stashes = nil
	if _, err := ingest.SetThumbnailPosition(context.Background(), video.ID, video.Path, video.Path, int64Ptr(4_000)); err != nil {
		t.Fatal(err)
	}
	if got := generator.stashCalls(); !slices.Equal(got, []string{"stash", "discard"}) {
		t.Fatalf("写しの操作 = %v, want [stash discard]", got)
	}
}

// Reject location reassignment before generation, before publication, or before persistence; never
// expose another video's frame. If reassigned after publication, restore the previous image and
// return ErrMediaFileUnavailable.
func TestSetThumbnailPositionRevalidatesSource(t *testing.T) {
	for _, tc := range []struct {
		name        string
		staleAt     int
		wantCalls   []string
		wantStashes []string
	}{
		{name: "生成の前", staleAt: 1, wantCalls: nil, wantStashes: nil},
		{name: "生成の間", staleAt: 2,
			wantCalls: []string{"thumbnail-at"}, wantStashes: []string{"stash", "discard"}},
		{name: "公開と記録の間", staleAt: 3,
			wantCalls: []string{"thumbnail-at", "publish-thumbnail"}, wantStashes: []string{"stash", "restore"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			video := probedVideo(1, "a")
			store := newFakeIngestStore(video)
			store.staleSourceAt = tc.staleAt
			generator := &fakeGenerator{recordPublish: true}
			ingest, _ := newTestIngest(store, generator)

			_, err := ingest.SetThumbnailPosition(context.Background(), video.ID, "/media/a.mp4", "/real/a.mp4", int64Ptr(4_000))
			if !errors.Is(err, domain.ErrMediaFileUnavailable) {
				t.Fatalf("err = %v, want ErrMediaFileUnavailable", err)
			}
			calls, _ := generator.snapshot()
			if !slices.Equal(calls, tc.wantCalls) {
				t.Fatalf("呼び出し = %v, want %v", calls, tc.wantCalls)
			}
			if got := generator.stashCalls(); !slices.Equal(got, tc.wantStashes) {
				t.Fatalf("写しの操作 = %v, want %v", got, tc.wantStashes)
			}
			if len(store.positions) != 0 {
				t.Fatalf("付け替わった所在で記録した: %v", store.positions)
			}
			for _, location := range store.sourceLocations {
				if location != "/media/a.mp4" {
					t.Fatalf("確かめ直した所在 = %v, want 辿る前の所在", store.sourceLocations)
				}
			}
		})
	}
}

// Deletion while waiting for the generation lock returns ErrNotFound before generation.
func TestSetThumbnailPositionVideoGoneBeforeGenerating(t *testing.T) {
	video := probedVideo(1, "a")
	store := newFakeIngestStore(video)
	store.staleSourceAt = 1

	store.afterRead = func(reads int) {
		if reads == 1 {
			delete(store.videos, video.ID)
		}
	}
	generator := &fakeGenerator{recordPublish: true}
	ingest, _ := newTestIngest(store, generator)

	_, err := ingest.SetThumbnailPosition(context.Background(), video.ID, video.Path, video.Path, int64Ptr(4_000))
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if calls, _ := generator.snapshot(); len(calls) != 0 {
		t.Fatalf("消えた動画で生成した: %v", calls)
	}
}
