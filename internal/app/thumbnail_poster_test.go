package app

import (
	"context"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// A frame's poster comes from the same extraction, written where the store publishes it.
func TestThumbnailFramesWriteThePoster(t *testing.T) {
	video := probedVideo(1, "a")
	for _, tc := range []struct {
		name string
		run  func(*Ingest) error
	}{
		{name: "ジョブ", run: func(ingest *Ingest) error {
			return ingest.Handler(domain.JobThumbnail)(context.Background(), jobFor(domain.JobThumbnail, video))
		}},
		{name: "位置の指定", run: func(ingest *Ingest) error {
			_, err := ingest.SetThumbnailPosition(context.Background(), video.ID, video.Path, "/real/a.mp4", int64Ptr(5_000))
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			generator := &fakeGenerator{}
			ingest, _ := newTestIngest(newFakeIngestStore(video), generator)

			if err := tc.run(ingest); err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(generator.posterOutputs, []string{"tmp/poster/a"}) {
				t.Fatalf("poster の出力先 = %v", generator.posterOutputs)
			}
		})
	}
}

// An applied image's stored source is its poster; a row applied before sources were stored has none.
func TestThumbnailImagesPublishTheirSourceAsPoster(t *testing.T) {
	t.Run("適用", func(t *testing.T) {
		video := sizedVideo(1, "a", 1920, 1080)
		generator := &fakeGenerator{thumbnailDir: t.TempDir()}
		ingest, _ := newTestIngest(newFakeIngestStore(video), generator)

		if _, err := ingest.ApplyThumbnailImage(context.Background(), video.ID, "a", []byte("edited"), domain.ThumbnailImageUploaded); err != nil {
			t.Fatal(err)
		}
		if got := string(generator.posters["a"]); got != "source:edited" {
			t.Fatalf("poster = %q, want source:edited", got)
		}
	})
	for _, tc := range []struct {
		name   string
		source []byte
		want   []byte
	}{
		{name: "ジョブで元画像あり", source: []byte("source"), want: []byte("source")},
		{name: "ジョブで元画像なし", source: nil, want: nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			video := probedVideo(1, "a")
			store := newFakeIngestStore(video)
			store.images["a"] = []byte("fitted")
			store.sources = map[string][]byte{"a": tc.source}
			generator := &fakeGenerator{thumbnailDir: t.TempDir()}
			ingest, _ := newTestIngest(store, generator)

			if err := ingest.Handler(domain.JobThumbnail)(context.Background(), jobFor(domain.JobThumbnail, video)); err != nil {
				t.Fatal(err)
			}
			if got, ok := generator.posters["a"]; string(got) != string(tc.want) || ok != (tc.want != nil) {
				t.Fatalf("poster = %q (%v), want %q", got, ok, tc.want)
			}
		})
	}
}
