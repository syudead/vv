package app

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// The poster is the frame the thumbnail shows: the chosen position, else the automatic frame.
func TestPosterExtractsTheThumbnailFrame(t *testing.T) {
	withPosition := probedVideo(1, "a")
	withPosition.ThumbnailState = domain.ThumbnailStateDone
	withPosition.ThumbnailPositionMs = int64Ptr(7_000)
	automatic := probedVideo(1, "a")
	automatic.ThumbnailState = domain.ThumbnailStateDone
	for _, tc := range []struct {
		name      string
		video     domain.Video
		wantCalls []string
	}{
		{name: "位置あり", video: withPosition, wantCalls: []string{"poster-at", "publish-poster"}},
		{name: "自動", video: automatic, wantCalls: []string{"poster", "publish-poster"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			generator := &fakeGenerator{}
			ingest, _ := newTestIngest(newFakeIngestStore(tc.video), generator)

			if err := ingest.Poster(context.Background(), tc.video.ID, tc.video.Path, "/real/a.mp4"); err != nil {
				t.Fatal(err)
			}
			calls, _ := generator.snapshot()
			if !slices.Equal(calls, tc.wantCalls) {
				t.Fatalf("呼び出し = %v, want %v", calls, tc.wantCalls)
			}
			if tc.video.ThumbnailPositionMs != nil && !slices.Equal(generator.positions, []int64{7_000}) {
				t.Fatalf("PosterAt の位置 = %v", generator.positions)
			}
		})
	}
}

func TestPosterSkipsWhatItCannotOrNeedNotExtract(t *testing.T) {
	video := probedVideo(1, "a")
	video.ThumbnailState = domain.ThumbnailStateDone
	t.Run("適用した画像", func(t *testing.T) {
		store := newFakeIngestStore(video)
		store.images["a"] = []byte("fitted:edited")
		generator := &fakeGenerator{}
		ingest, _ := newTestIngest(store, generator)

		err := ingest.Poster(context.Background(), video.ID, video.Path, "/real/a.mp4")
		if !errors.Is(err, domain.ErrThumbnailFrameUnavailable) {
			t.Fatalf("err = %v, want ErrThumbnailFrameUnavailable", err)
		}
		if calls, _ := generator.snapshot(); len(calls) != 0 {
			t.Fatalf("呼び出し = %v", calls)
		}
	})
	t.Run("作成済み", func(t *testing.T) {
		generator := &fakeGenerator{posters: map[string]bool{"a": true}}
		ingest, _ := newTestIngest(newFakeIngestStore(video), generator)

		if err := ingest.Poster(context.Background(), video.ID, video.Path, "/real/a.mp4"); err != nil {
			t.Fatal(err)
		}
		if calls, _ := generator.snapshot(); len(calls) != 0 {
			t.Fatalf("呼び出し = %v", calls)
		}
	})
}
