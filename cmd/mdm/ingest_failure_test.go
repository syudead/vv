package main

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/syudead/vv/internal/app"
	"github.com/syudead/vv/internal/artifacts"
	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/media"
	"github.com/syudead/vv/internal/store"
)

// Use the real store and artifacts to verify ingestion behavior across SQLite transactions.
func newTestIngest(db *store.DB, dataDir string) *app.Ingest {
	return app.NewIngest(app.IngestOptions{
		Store:     db.Ingest(),
		Generator: media.NewAssets(),
		Artifacts: artifacts.New(Config{DataDir: dataDir}.ThumbnailsDir()),
		Logger:    slog.New(slog.DiscardHandler),
	})
}

func missingSourceFixture(t *testing.T) (context.Context, string, *store.DB, int64) {
	t.Helper()
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
	video, err := db.ScanIndex().UpsertVideo(ctx, domain.VideoFile{
		Path: filepath.Join(mediaDir, "missing.mp4"), Title: "missing", ContentKey: "missing-source", SizeBytes: 1,
		MTime: time.Unix(1, 0), Container: "mp4",
	})
	if err != nil {
		t.Fatal(err)
	}
	return ctx, dataDir, db, video.ID
}

func claimLastAttempt(t *testing.T, ctx context.Context, db *store.DB, kind domain.JobKind, videoID int64) domain.Job {
	t.Helper()
	if kind == domain.JobThumbnail {
		// Thumbnails are claimable only after probing.
		probe := domain.Probe{DurationMs: 1000, VideoCodec: "h264", AudioCodec: "aac"}
		if err := db.Ingest().ApplyProbe(ctx, videoID, probe, domain.EvaluatePlayability("mp4", probe)); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Ingest().EnqueueJob(ctx, kind, videoID, domain.ActivityByRule); err != nil {
		t.Fatal(err)
	}
	if err := store.SetJobAttemptsForTest(ctx, db, kind, videoID, domain.MaxJobAttempts-1); err != nil {
		t.Fatal(err)
	}
	job, err := db.Ingest().ClaimJob(ctx, kind)
	if err != nil {
		t.Fatal(err)
	}
	return job
}

// Video failure is committed with exhausted job failure; until then it stays pending and rejects
// probe retries.
func TestIngestHandlersLeaveTerminalFailureToFailClaimedJob(t *testing.T) {
	cases := []struct {
		kind  domain.JobKind
		state func(domain.Video) string
	}{
		{kind: domain.JobProbe, state: func(v domain.Video) string { return string(v.ProbeState) }},
		{kind: domain.JobThumbnail, state: func(v domain.Video) string { return string(v.ThumbnailState) }},
	}
	for _, tc := range cases {
		t.Run(string(tc.kind), func(t *testing.T) {
			ctx, dataDir, db, videoID := missingSourceFixture(t)
			job := claimLastAttempt(t, ctx, db, tc.kind, videoID)

			handleErr := newTestIngest(db, dataDir).Handler(tc.kind)(ctx, job)
			if handleErr == nil {
				t.Fatal("missing source unexpectedly succeeded")
			}
			video, err := db.Library().GetVideo(ctx, domain.AudienceOwner, videoID)
			if err != nil {
				t.Fatal(err)
			}
			if got := tc.state(video); got != "pending" {
				t.Fatalf("handler が状態を書いた: %q, want pending", got)
			}
			if err := db.Ingest().RetryProbe(ctx, videoID); !errors.Is(err, domain.ErrProbeNotFailed) {
				t.Fatal("失敗の記録前に読み取りのやり直しを受け付けた")
			}

			if err := db.Ingest().FailClaimedJob(ctx, job, handleErr); err != nil {
				t.Fatal(err)
			}
			video, err = db.Library().GetVideo(ctx, domain.AudienceOwner, videoID)
			if err != nil {
				t.Fatal(err)
			}
			if got := tc.state(video); got != "failed" {
				t.Fatalf("state = %q, want failed", got)
			}
			if tc.kind == domain.JobProbe && video.ProbeError != handleErr.Error() {
				t.Fatalf("probeError = %q, want %q", video.ProbeError, handleErr.Error())
			}
			if tc.kind == domain.JobProbe && video.ProbeErrorCode != domain.ProbeErrorFileUnavailable {
				t.Fatalf("probeErrorCode = %q, want file_unavailable", video.ProbeErrorCode)
			}
		})
	}
}
