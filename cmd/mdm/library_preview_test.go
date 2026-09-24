package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/store"
)

func TestPreviewHandlerMarksOnlyPreviewFailedAtRetryLimit(t *testing.T) {
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
	if _, err := db.AddMediaFolder(ctx, mediaDir); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(mediaDir, "movie.mp4")
	video, err := db.UpsertVideo(ctx, domain.VideoFile{
		Path: path, Title: "movie", ContentKey: "zero-duration", SizeBytes: 1,
		MTime: time.Unix(1, 0), Container: "mp4",
	})
	if err != nil {
		t.Fatal(err)
	}
	probe := domain.Probe{VideoCodec: "h264", AudioCodec: "aac"}
	if err := db.ApplyProbe(ctx, video.ID, probe, domain.EvaluatePlayability("mp4", probe)); err != nil {
		t.Fatal(err)
	}
	if err := db.EnqueueJob(ctx, domain.JobPreview, video.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL().Exec(`update jobs set attempts = ? where kind = 'preview' and video_id = ?`, domain.MaxJobAttempts-1, video.ID); err != nil {
		t.Fatal(err)
	}
	job, err := db.ClaimJob(ctx, domain.JobPreview)
	if err != nil {
		t.Fatal(err)
	}
	handleErr := previewHandler(Config{DataDir: dataDir}, db)(ctx, job)
	if handleErr == nil {
		t.Fatal("zero-duration preview unexpectedly succeeded")
	}
	if err := db.FailClaimedJob(ctx, job, handleErr.Error()); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetVideo(ctx, video.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.PreviewState != domain.PreviewStateFailed || got.ProbeState != domain.ProbeStateDone || got.ThumbnailState != domain.ThumbnailStatePending {
		t.Fatalf("video states after preview failure: probe=%q thumbnail=%q preview=%q", got.ProbeState, got.ThumbnailState, got.PreviewState)
	}
}

func TestPreviewHandlerUnreadableSourceMarksFailedAtRetryLimit(t *testing.T) {
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
	if _, err := db.AddMediaFolder(ctx, mediaDir); err != nil {
		t.Fatal(err)
	}
	video, err := db.UpsertVideo(ctx, domain.VideoFile{
		Path: filepath.Join(mediaDir, "missing.mp4"), Title: "missing", ContentKey: "missing-source", SizeBytes: 1,
		MTime: time.Unix(1, 0), Container: "mp4",
	})
	if err != nil {
		t.Fatal(err)
	}
	probe := domain.Probe{DurationMs: 1_000, VideoCodec: "h264", AudioCodec: "aac"}
	if err := db.ApplyProbe(ctx, video.ID, probe, domain.EvaluatePlayability("mp4", probe)); err != nil {
		t.Fatal(err)
	}
	if err := db.EnqueueJob(ctx, domain.JobPreview, video.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL().Exec(`update jobs set attempts = ? where kind = 'preview' and video_id = ?`, domain.MaxJobAttempts-1, video.ID); err != nil {
		t.Fatal(err)
	}
	job, err := db.ClaimJob(ctx, domain.JobPreview)
	if err != nil {
		t.Fatal(err)
	}
	handleErr := previewHandler(Config{DataDir: dataDir}, db)(ctx, job)
	if handleErr == nil {
		t.Fatal("preview with missing source unexpectedly succeeded")
	}
	if err := db.FailClaimedJob(ctx, job, handleErr.Error()); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetVideo(ctx, video.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.PreviewState != domain.PreviewStateFailed {
		t.Fatalf("preview state = %q, want failed", got.PreviewState)
	}
}
