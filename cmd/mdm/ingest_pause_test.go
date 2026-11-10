package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/syudead/vv/internal/app"
	"github.com/syudead/vv/internal/artifacts"
	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/media"
	"github.com/syudead/vv/internal/mediafs"
	"github.com/syudead/vv/internal/store"
)

// pausedPreview writes part of the preview and is then stopped by a pause, as an encode cancelled mid-run is.
type pausedPreview struct {
	*media.Assets
	pause context.CancelCauseFunc
}

func (g pausedPreview) Preview(ctx context.Context, _, output string, _ int64) error {
	if err := os.WriteFile(output, []byte("partial"), 0o644); err != nil {
		return err
	}
	g.pause(domain.ErrJobPaused)
	<-ctx.Done()
	return ctx.Err()
}

// A preview stage stopped by a pause leaves nothing of its partial output in the artifact store's temporary
// directory.
func TestIngestPreviewCancelledMidRunLeavesNoTemporaryFile(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	db, err := store.OpenMigratedForTest(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	mediaDir := t.TempDir()
	if _, err := db.Settings().AddMediaFolder(ctx, mediaDir); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(mediaDir, "movie.mp4")
	if err := os.WriteFile(path, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	video, err := db.ScanIndex().UpsertVideo(ctx, domain.VideoFile{
		Path: path, Title: "movie", ContentKey: "paused-preview", SizeBytes: 5, MTime: time.Unix(1, 0), Container: "mp4",
	})
	if err != nil {
		t.Fatal(err)
	}
	probe := domain.Probe{DurationMs: 10_000, VideoCodec: "h264", AudioCodec: "aac"}
	if err := db.Ingest().ApplyProbe(ctx, video.ID, probe, domain.EvaluatePlayability("mp4", probe)); err != nil {
		t.Fatal(err)
	}
	if err := db.Ingest().EnqueueJob(ctx, domain.JobPreview, video.ID, domain.ActivityByRule); err != nil {
		t.Fatal(err)
	}
	job, err := db.Ingest().ClaimJob(ctx, domain.JobPreview)
	if err != nil {
		t.Fatal(err)
	}
	jobCtx, pause := context.WithCancelCause(ctx)
	thumbnails := Config{DataDir: dataDir}.ThumbnailsDir()
	ingest := app.NewIngest(app.IngestOptions{
		Store: db.Ingest(), Generator: pausedPreview{Assets: media.NewAssets(), pause: pause},
		Artifacts: artifacts.New(thumbnails),
		Sources:   app.MediaFiles{Folders: db.Settings(), Resolver: mediafs.New()},
		Logger:    slog.New(slog.DiscardHandler),
	})

	if err := ingest.Preview(jobCtx, job); !errors.Is(err, context.Canceled) {
		t.Fatalf("Preview = %v, want the cancellation", err)
	}
	entries, err := os.ReadDir(filepath.Join(thumbnails, ".tmp"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("temporary entries after the pause = %v, want none", entries)
	}
}
