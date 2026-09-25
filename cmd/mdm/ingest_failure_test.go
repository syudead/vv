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

// newTestIngest は本物の保存層と生成物の置き場で取り込みを組み立てる。
// アプリケーション層の単体テスト（internal/app）はどちらも差し替えるので、
// SQLite の取引と組み合わせたときの振る舞いはここで確かめる。
func newTestIngest(db *store.DB, dataDir string) *app.Ingest {
	return app.NewIngest(app.IngestOptions{
		Store:     db.Ingest(),
		Generator: media.NewAssets(),
		Artifacts: artifacts.New(Config{DataDir: dataDir}.ThumbnailsDir()),
		Logger:    slog.New(slog.DiscardHandler),
	})
}

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
		// サムネイルは解析の後に取り出すので、解析は済ませておく。
		probe := domain.Probe{DurationMs: 1000, VideoCodec: "h264", AudioCodec: "aac"}
		if err := db.Ingest().ApplyProbe(ctx, videoID, probe, domain.EvaluatePlayability("mp4", probe)); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Ingest().EnqueueJob(ctx, kind, videoID); err != nil {
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

// ファイルの確認で上限まで失敗した読み取り・サムネイルのジョブは、ハンドラ自身は
// 状態を書かず、ジョブの失敗と同じ取引で動画側が failed になる。失敗が記録される
// までは pending のままで、その間の読み取りのやり直しは断られる。
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
			if err := db.Ingest().RetryProbe(ctx, videoID, true); !errors.Is(err, domain.ErrProbeNotFailed) {
				t.Fatal("失敗の記録前に読み取りのやり直しを受け付けた")
			}

			if err := db.Ingest().FailClaimedJob(ctx, job, handleErr.Error()); err != nil {
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
		})
	}
}
