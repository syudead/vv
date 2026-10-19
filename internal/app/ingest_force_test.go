package app

import (
	"bytes"
	"context"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func forcedJob(kind domain.JobKind, video domain.Video) domain.Job {
	job := jobFor(kind, video)
	job.Force = true
	return job
}

// A forced thumbnail job rebuilds a done thumbnail from the applied image, else the chosen position, else the
// automatic frame; an unforced job leaves a done thumbnail alone.
func TestIngestForcedThumbnailRebuildsDoneThumbnail(t *testing.T) {
	for _, tc := range []struct {
		name      string
		image     []byte
		position  *int64
		force     bool
		wantCalls []string
	}{
		{name: "差し替えた画像", image: []byte("fitted:edited"), position: int64Ptr(7_000), force: true, wantCalls: []string{"check"}},
		{name: "選んだ位置", position: int64Ptr(7_000), force: true, wantCalls: []string{"check", "thumbnail-at"}},
		{name: "自動", force: true, wantCalls: []string{"check", "thumbnail"}},
		{name: "強制しない", force: false, wantCalls: []string{"check"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			video := probedVideo(1, "a")
			video.ThumbnailState = domain.ThumbnailStateDone
			video.ThumbnailPositionMs = tc.position
			store := newFakeIngestStore(video)
			if tc.image != nil {
				store.images["a"] = tc.image
			}
			generator := &fakeGenerator{thumbnailDir: t.TempDir()}
			ingest, _ := newTestIngest(store, generator)
			job := jobFor(domain.JobThumbnail, video)
			job.Force = tc.force

			if err := ingest.Thumbnail(context.Background(), job); err != nil {
				t.Fatal(err)
			}
			calls, _ := generator.snapshot()
			if !slices.Equal(calls, tc.wantCalls) {
				t.Fatalf("呼び出し = %v, want %v", calls, tc.wantCalls)
			}
			if tc.position != nil && tc.image == nil && !slices.Equal(generator.positions, []int64{7_000}) {
				t.Fatalf("ThumbnailAt の位置 = %v, want [7000]", generator.positions)
			}
			if tc.image != nil && !bytes.Equal(generator.published["a"], tc.image) {
				t.Fatalf("公開 = %q, want %q", generator.published["a"], tc.image)
			}
			if _, published := generator.published["a"]; published != tc.force {
				t.Fatalf("公開した = %v, want %v", published, tc.force)
			}
			if recorded := len(store.thumbnailSubstitutions) == 1; recorded != tc.force {
				t.Fatalf("完了の記録 = %v, want %v", store.thumbnailSubstitutions, tc.force)
			}
		})
	}
}

// Forced seek and preview jobs hand their force to the artifact store, which then does not adopt what is on disk.
func TestIngestForcedJobsPublishWithForce(t *testing.T) {
	video := probedVideo(1, "a")
	video.ThumbnailState = domain.ThumbnailStateDone
	for _, kind := range []domain.JobKind{domain.JobSeekThumbnail, domain.JobPreview} {
		t.Run(string(kind), func(t *testing.T) {
			store := newFakeIngestStore(video)
			generator := &fakeGenerator{seekPublished: true}
			ingest, _ := newTestIngest(store, generator)

			if err := ingest.Handler(kind)(context.Background(), forcedJob(kind, video)); err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(generator.publishForces, []bool{true}) {
				t.Fatalf("公開の force = %v, want [true]", generator.publishForces)
			}
		})
	}
}
