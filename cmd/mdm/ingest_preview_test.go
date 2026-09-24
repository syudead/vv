package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/store"
)

func TestIngestPreviewMarksOnlyPreviewFailedAtRetryLimit(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	db, err := store.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := store.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	mediaDir := t.TempDir()
	if _, err := db.Settings().AddMediaFolder(ctx, mediaDir); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(mediaDir, "movie.mp4")
	video, err := db.ScanIndex().UpsertVideo(ctx, domain.VideoFile{
		Path: path, Title: "movie", ContentKey: "zero-duration", SizeBytes: 1,
		MTime: time.Unix(1, 0), Container: "mp4",
	})
	if err != nil {
		t.Fatal(err)
	}
	probe := domain.Probe{VideoCodec: "h264", AudioCodec: "aac"}
	if err := db.Ingest().ApplyProbe(ctx, video.ID, probe, domain.EvaluatePlayability("mp4", probe)); err != nil {
		t.Fatal(err)
	}
	if err := db.Ingest().EnqueueJob(ctx, domain.JobPreview, video.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.SetJobAttemptsForTest(ctx, db, domain.JobPreview, video.ID, domain.MaxJobAttempts-1); err != nil {
		t.Fatal(err)
	}
	job, err := db.Ingest().ClaimJob(ctx, domain.JobPreview)
	if err != nil {
		t.Fatal(err)
	}
	handleErr := newTestIngest(db, dataDir).Preview(ctx, job)
	if handleErr == nil {
		t.Fatal("zero-duration preview unexpectedly succeeded")
	}
	if err := db.Ingest().FailClaimedJob(ctx, job, handleErr.Error()); err != nil {
		t.Fatal(err)
	}
	got, err := db.Library().GetVideo(ctx, video.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.PreviewState != domain.PreviewStateFailed || got.ProbeState != domain.ProbeStateDone || got.ThumbnailState != domain.ThumbnailStatePending {
		t.Fatalf("video states after preview failure: probe=%q thumbnail=%q preview=%q", got.ProbeState, got.ThumbnailState, got.PreviewState)
	}
}

func TestIngestPreviewUnreadableSourceMarksFailedAtRetryLimit(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	db, err := store.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := store.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	mediaDir := t.TempDir()
	if _, err := db.Settings().AddMediaFolder(ctx, mediaDir); err != nil {
		t.Fatal(err)
	}
	video, err := db.ScanIndex().UpsertVideo(ctx, domain.VideoFile{
		Path: filepath.Join(mediaDir, "missing.mp4"), Title: "missing", ContentKey: "missing-source", SizeBytes: 1,
		MTime: time.Unix(1, 0), Container: "mp4",
	})
	if err != nil {
		t.Fatal(err)
	}
	probe := domain.Probe{DurationMs: 1_000, VideoCodec: "h264", AudioCodec: "aac"}
	if err := db.Ingest().ApplyProbe(ctx, video.ID, probe, domain.EvaluatePlayability("mp4", probe)); err != nil {
		t.Fatal(err)
	}
	if err := db.Ingest().EnqueueJob(ctx, domain.JobPreview, video.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.SetJobAttemptsForTest(ctx, db, domain.JobPreview, video.ID, domain.MaxJobAttempts-1); err != nil {
		t.Fatal(err)
	}
	job, err := db.Ingest().ClaimJob(ctx, domain.JobPreview)
	if err != nil {
		t.Fatal(err)
	}
	handleErr := newTestIngest(db, dataDir).Preview(ctx, job)
	if handleErr == nil {
		t.Fatal("preview with missing source unexpectedly succeeded")
	}
	if err := db.Ingest().FailClaimedJob(ctx, job, handleErr.Error()); err != nil {
		t.Fatal(err)
	}
	got, err := db.Library().GetVideo(ctx, video.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.PreviewState != domain.PreviewStateFailed {
		t.Fatalf("preview state = %q, want failed", got.PreviewState)
	}
}
