package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/jobs"
	"github.com/syudead/vv/internal/store"
)

// missingSourceFixture は所在のファイルが無い動画を1本取り込む。
func missingSourceFixture(t *testing.T) (context.Context, string, *store.DB, int64) {
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
	if err := db.EnqueueJob(ctx, kind, videoID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL().Exec(`update jobs set attempts = ? where kind = ? and video_id = ?`,
		domain.MaxJobAttempts-1, string(kind), videoID); err != nil {
		t.Fatal(err)
	}
	job, err := db.ClaimJob(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return job
}

// ファイルの確認で上限まで失敗した読み取り・サムネイルのジョブは、ハンドラ自身は
// 状態を書かず、ジョブの失敗と同じ取引で動画側が failed になる。失敗が記録される
// までは pending のままで、その間の読み取りのやり直しは断られる。
func TestProcessingHandlersLeaveTerminalFailureToFailClaimedJob(t *testing.T) {
	cases := []struct {
		kind    domain.JobKind
		handler func(Config, *store.DB) jobs.Handler
		state   func(domain.Video) string
	}{
		{kind: domain.JobProbe, handler: func(_ Config, db *store.DB) jobs.Handler { return probeHandler(db) },
			state: func(v domain.Video) string { return string(v.ProbeState) }},
		{kind: domain.JobThumbnail, handler: thumbnailHandler,
			state: func(v domain.Video) string { return string(v.ThumbnailState) }},
	}
	for _, tc := range cases {
		t.Run(string(tc.kind), func(t *testing.T) {
			ctx, dataDir, db, videoID := missingSourceFixture(t)
			job := claimLastAttempt(t, ctx, db, tc.kind, videoID)

			handleErr := tc.handler(Config{DataDir: dataDir}, db)(ctx, job)
			if handleErr == nil {
				t.Fatal("missing source unexpectedly succeeded")
			}
			video, err := db.GetVideo(ctx, videoID)
			if err != nil {
				t.Fatal(err)
			}
			if got := tc.state(video); got != "pending" {
				t.Fatalf("handler が状態を書いた: %q, want pending", got)
			}
			if err := db.RetryProbe(ctx, videoID, true); !errors.Is(err, domain.ErrProbeNotFailed) {
				t.Fatal("失敗の記録前に読み取りのやり直しを受け付けた")
			}

			if err := db.FailClaimedJob(ctx, job, handleErr.Error()); err != nil {
				t.Fatal(err)
			}
			video, err = db.GetVideo(ctx, videoID)
			if err != nil {
				t.Fatal(err)
			}
			if got := tc.state(video); got != "failed" {
				t.Fatalf("state = %q, want failed", got)
			}
			if tc.kind == domain.JobProbe && video.ProbeError != handleErr.Error() {
				t.Fatalf("probeError = %q, want %q", video.ProbeError, handleErr.Error())
			}
		})
	}
}

// 起動時の整合で、pending のまま終端の失敗ジョブを持つ動画が failed になる。
func TestReconcileProcessingFailuresAtStartup(t *testing.T) {
	ctx, dataDir, db, videoID := missingSourceFixture(t)
	job := claimLastAttempt(t, ctx, db, domain.JobProbe, videoID)
	if _, err := db.SQL().Exec(`update jobs set state = 'failed', last_error = 'legacy' where id = ?`, job.ID); err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	newLibrary(Config{DataDir: dataDir}, db, logger).reconcileProcessingFailures(ctx)

	video, err := db.GetVideo(ctx, videoID)
	if err != nil {
		t.Fatal(err)
	}
	if video.ProbeState != domain.ProbeStateFailed || video.ProbeError != "legacy" {
		t.Fatalf("probe = %q (%q), want failed", video.ProbeState, video.ProbeError)
	}
}
