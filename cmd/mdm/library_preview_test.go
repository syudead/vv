package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/media"
	"github.com/syudead/vv/internal/store"
)

func TestReconcilePreviewsRequeuesMissingCompletedAsset(t *testing.T) {
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
		Path:       filepath.Join(mediaDir, "movie.mp4"),
		Title:      "movie",
		ContentKey: "content-key",
		SizeBytes:  1,
		MTime:      time.Unix(1, 0),
		Container:  "mp4",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetPreviewState(ctx, video.ID, domain.PreviewStateDone); err != nil {
		t.Fatal(err)
	}
	if err := db.EnqueueJob(ctx, domain.JobPreview, video.ID); err != nil {
		t.Fatal(err)
	}
	job, err := db.ClaimJob(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.CompleteJob(ctx, job.ID); err != nil {
		t.Fatal(err)
	}
	broken := media.PreviewPath(Config{DataDir: dataDir}.ThumbnailsDir(), "content-key")
	if err := os.MkdirAll(filepath.Dir(broken), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(broken, []byte("broken"), 0o600); err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	lib := newLibrary(Config{DataDir: dataDir}, db, logger)
	lib.reconcilePreviews(ctx)

	got, err := db.GetVideo(ctx, video.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.PreviewState != domain.PreviewStatePending {
		t.Fatalf("preview state = %q, want pending", got.PreviewState)
	}
	if _, err := os.Stat(broken); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("broken preview was not removed: %v", err)
	}
	repair, err := db.ClaimJob(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if repair.Kind != domain.JobPreview || repair.VideoID != video.ID {
		t.Fatalf("repair job = %+v", repair)
	}
}

func TestReconcilePreviewsRepairsEveryIntegrityFailure(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T, video, manifest string)
	}{
		{name: "missing mp4", mutate: func(t *testing.T, video, _ string) {
			t.Helper()
			if err := os.Remove(video); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "missing manifest", mutate: func(t *testing.T, _, manifest string) {
			t.Helper()
			if err := os.Remove(manifest); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "truncated mp4", mutate: func(t *testing.T, video, _ string) {
			t.Helper()
			if err := os.Truncate(video, 3); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "bit corruption", mutate: func(t *testing.T, video, _ string) {
			t.Helper()
			if err := os.WriteFile(video, []byte("valid-previex"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, dataDir, db, videoID := completedPreviewFixture(t)
			path := media.PreviewPath(Config{DataDir: dataDir}.ThumbnailsDir(), "content-key")
			manifest := media.PreviewManifestPath(Config{DataDir: dataDir}.ThumbnailsDir(), "content-key")
			writePreviewPair(t, path, manifest, []byte("valid-preview"))
			tt.mutate(t, path, manifest)

			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			newLibrary(Config{DataDir: dataDir}, db, logger).reconcilePreviews(ctx)
			got, err := db.GetVideo(ctx, videoID)
			if err != nil {
				t.Fatal(err)
			}
			if got.PreviewState != domain.PreviewStatePending {
				t.Fatalf("preview state = %q", got.PreviewState)
			}
			for _, removed := range []string{path, manifest} {
				if _, err := os.Stat(removed); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("invalid asset remains at %s: %v", removed, err)
				}
			}
			repair, err := db.ClaimJob(ctx)
			if err != nil || repair.Kind != domain.JobPreview || repair.VideoID != videoID {
				t.Fatalf("repair job = %+v, %v", repair, err)
			}
		})
	}
}

func TestReconcilePreviewsRepairsLegacyPendingTerminalFailure(t *testing.T) {
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
		Path: filepath.Join(mediaDir, "movie.mp4"), Title: "movie", ContentKey: "legacy-failure", SizeBytes: 1,
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
	job, err := db.ClaimJob(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL().Exec(`update jobs set state = 'failed', attempts = ? where id = ?`, domain.MaxJobAttempts, job.ID); err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	newLibrary(Config{DataDir: dataDir}, db, logger).reconcilePreviews(ctx)
	got, err := db.GetVideo(ctx, video.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.PreviewState != domain.PreviewStateFailed {
		t.Fatalf("preview state = %q, want failed", got.PreviewState)
	}
}

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
	job, err := db.ClaimJob(ctx)
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
	job, err := db.ClaimJob(ctx)
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

func completedPreviewFixture(t *testing.T) (context.Context, string, *store.DB, int64) {
	t.Helper()
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
		Path: filepath.Join(mediaDir, "movie.mp4"), Title: "movie", ContentKey: "content-key",
		SizeBytes: 1, MTime: time.Unix(1, 0), Container: "mp4",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetPreviewState(ctx, video.ID, domain.PreviewStateDone); err != nil {
		t.Fatal(err)
	}
	if err := db.EnqueueJob(ctx, domain.JobPreview, video.ID); err != nil {
		t.Fatal(err)
	}
	job, err := db.ClaimJob(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.CompleteJob(ctx, job.ID); err != nil {
		t.Fatal(err)
	}
	return ctx, dataDir, db, video.ID
}

func writePreviewPair(t *testing.T, video, manifest string, payload []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(video), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(video, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(payload)
	data, err := json.Marshal(map[string]any{
		"version": 1,
		"size":    len(payload),
		"sha256":  hex.EncodeToString(digest[:]),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
