package store

import (
	"context"
	"errors"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func jobCounts(t *testing.T, db *DB, videoID int64) map[string]int {
	t.Helper()
	rows, err := db.sql.Query(`select kind || ':' || state, count(*) from jobs where video_id = ? group by kind, state`, videoID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	counts := map[string]int{}
	for rows.Next() {
		var key string
		var count int
		if err := rows.Scan(&key, &count); err != nil {
			t.Fatal(err)
		}
		counts[key] = count
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return counts
}

func claimAtLastAttempt(t *testing.T, db *DB, kind domain.JobKind, videoID int64) domain.Job {
	t.Helper()
	ctx := context.Background()
	if err := db.Ingest().EnqueueJob(ctx, kind, videoID, domain.ActivityByRule); err != nil {
		t.Fatal(err)
	}
	if _, err := db.sql.Exec(`update jobs set attempts = ? where kind = ? and video_id = ?`, domain.MaxJobAttempts-1, string(kind), videoID); err != nil {
		t.Fatal(err)
	}
	job, err := db.Ingest().ClaimJob(ctx, kind)
	if err != nil {
		t.Fatal(err)
	}
	if job.Kind != kind || job.Attempts != domain.MaxJobAttempts || !job.LastLocation {
		t.Fatalf("claimed = %+v", job)
	}
	return job
}

func failedVideoFixture(t *testing.T) (*DB, int64) {
	t.Helper()
	db, videoID := jobsFixture(t)
	ctx := context.Background()
	for _, kind := range []domain.JobKind{domain.JobProbe, domain.JobThumbnail} {
		job := claimAtLastAttempt(t, db, kind, videoID)
		if err := db.Ingest().FailClaimedJob(ctx, job, errors.New(string(kind)+" failed")); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.sql.Exec(`update videos set preview_state = 'failed' where id = ?`, videoID); err != nil {
		t.Fatal(err)
	}
	return db, videoID
}

func TestRetryProbeRequeuesFailedVideoOnce(t *testing.T) {
	db, videoID := failedVideoFixture(t)
	ctx := context.Background()

	if err := db.Ingest().RetryProbe(ctx, videoID); err != nil {
		t.Fatal(err)
	}
	video, err := db.Library().GetVideo(ctx, domain.AudienceOwner, videoID)
	if err != nil {
		t.Fatal(err)
	}
	if video.ProbeState != domain.ProbeStatePending || video.ProbeError != "" || video.ProbeErrorCode != "" {
		t.Fatalf("probe = %q (%q, %q), want pending without error", video.ProbeState, video.ProbeError, video.ProbeErrorCode)
	}
	if video.ThumbnailState != domain.ThumbnailStatePending || video.PreviewState != domain.PreviewStatePending {
		t.Fatalf("thumbnail = %q, preview = %q, want pending", video.ThumbnailState, video.PreviewState)
	}
	want := map[string]int{"probe:queued": 1, "thumbnail:queued": 1}
	if got := jobCounts(t, db, videoID); !equalCounts(got, want) {
		t.Fatalf("jobs = %v, want %v", got, want)
	}

	if err := db.Ingest().RetryProbe(ctx, videoID); !errors.Is(err, domain.ErrProbeNotFailed) {
		t.Fatalf("2 回目: err = %v, want ErrProbeNotFailed", err)
	}
	if got := jobCounts(t, db, videoID); !equalCounts(got, want) {
		t.Fatalf("2 回目のあと jobs = %v, want %v", got, want)
	}
}

func TestRetryProbeKeepsCompletedThumbnail(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()
	if _, err := db.sql.Exec(`update videos set probe_state = 'failed', probe_error = 'broken',
		thumbnail_state = 'done', seek_thumbnail_state = 'done' where id = ?`, videoID); err != nil {
		t.Fatal(err)
	}
	if err := db.Ingest().RetryProbe(ctx, videoID); err != nil {
		t.Fatal(err)
	}
	video, err := db.Library().GetVideo(ctx, domain.AudienceOwner, videoID)
	if err != nil {
		t.Fatal(err)
	}
	if video.ThumbnailState != domain.ThumbnailStateDone || video.SeekThumbnailState != domain.SeekThumbnailDone {
		t.Fatalf("thumbnail = %q, seek = %q, want done", video.ThumbnailState, video.SeekThumbnailState)
	}
	if got, want := jobCounts(t, db, videoID), map[string]int{"probe:queued": 1}; !equalCounts(got, want) {
		t.Fatalf("jobs = %v, want %v", got, want)
	}
}

func TestRetryProbeRejectsNonFailedAndMissing(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()
	if err := db.Ingest().RetryProbe(ctx, videoID); !errors.Is(err, domain.ErrProbeNotFailed) {
		t.Fatalf("pending: err = %v, want ErrProbeNotFailed", err)
	}
	probe := domain.Probe{DurationMs: 1_000, VideoCodec: "h264", AudioCodec: "aac"}
	if err := db.Ingest().ApplyProbe(ctx, videoID, probe, domain.EvaluatePlayability("mp4", probe)); err != nil {
		t.Fatal(err)
	}
	if err := db.Ingest().RetryProbe(ctx, videoID); !errors.Is(err, domain.ErrProbeNotFailed) {
		t.Fatalf("done: err = %v, want ErrProbeNotFailed", err)
	}
	if err := db.Ingest().RetryProbe(ctx, videoID+100); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing: err = %v, want domain.ErrNotFound", err)
	}
	if count := countJobs(t, db); count != 0 {
		t.Fatalf("jobs = %d, want 0", count)
	}
}

// Terminal job failure and video failure must be atomic.
func TestFailClaimedJobRecordsProbeAndThumbnailFailure(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()

	probeJob := claimAtLastAttempt(t, db, domain.JobProbe, videoID)
	probeCause := domain.NewProbeFailure(domain.ProbeErrorFileUnavailable, errors.New("open /media/a.mp4: no such file or directory"))
	if err := db.Ingest().FailClaimedJob(ctx, probeJob, probeCause); err != nil {
		t.Fatal(err)
	}
	thumbnailJob := claimAtLastAttempt(t, db, domain.JobThumbnail, videoID)
	if err := db.Ingest().FailClaimedJob(ctx, thumbnailJob, errors.New("open /media/a.mp4: no such file or directory")); err != nil {
		t.Fatal(err)
	}

	video, err := db.Library().GetVideo(ctx, domain.AudienceOwner, videoID)
	if err != nil {
		t.Fatal(err)
	}
	if video.ProbeState != domain.ProbeStateFailed || video.ProbeError != "open /media/a.mp4: no such file or directory" {
		t.Fatalf("probe = %q (%q), want failed with reason", video.ProbeState, video.ProbeError)
	}
	if video.ProbeErrorCode != domain.ProbeErrorFileUnavailable {
		t.Fatalf("probe error code = %q, want file_unavailable", video.ProbeErrorCode)
	}
	if video.ThumbnailState != domain.ThumbnailStateFailed {
		t.Fatalf("thumbnail = %q, want failed", video.ThumbnailState)
	}
}

func TestFailClaimedProbeRollsBackJobWhenStateUpdateFails(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()
	job := claimAtLastAttempt(t, db, domain.JobProbe, videoID)
	if _, err := db.sql.Exec(`create trigger reject_probe_failure before update of probe_state on videos
		when new.probe_state = 'failed' begin select raise(abort, 'reject probe failure'); end`); err != nil {
		t.Fatal(err)
	}
	if err := db.Ingest().FailClaimedJob(ctx, job, errors.New("probe failed")); err == nil {
		t.Fatal("FailClaimedJob succeeded despite rejected probe state update")
	}
	if got := jobState(t, db, job.ID); got != "running" {
		t.Fatalf("job state = %q, want running after rollback", got)
	}
}

func TestFailClaimedProbeKeepsPendingWhileRetrying(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()
	if err := db.Ingest().EnqueueJob(ctx, domain.JobProbe, videoID, domain.ActivityByRule); err != nil {
		t.Fatal(err)
	}
	job, err := db.Ingest().ClaimJob(ctx, domain.JobProbe)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ingest().FailClaimedJob(ctx, job, errors.New("retry")); err != nil {
		t.Fatal(err)
	}
	video, err := db.Library().GetVideo(ctx, domain.AudienceOwner, videoID)
	if err != nil {
		t.Fatal(err)
	}
	if video.ProbeState != domain.ProbeStatePending {
		t.Fatalf("probe = %q, want pending", video.ProbeState)
	}
}

// A later job failure must not overwrite probe or representative-thumbnail results already saved successfully.
func TestFailClaimedJobKeepsCompletedState(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()

	probeJob := claimAtLastAttempt(t, db, domain.JobProbe, videoID)
	probe := domain.Probe{DurationMs: 1_000, VideoCodec: "h264", AudioCodec: "aac"}
	applied, err := db.Ingest().ApplyProbeForJob(ctx, probeJob, probe, domain.EvaluatePlayability("mp4", probe))
	if err != nil || !applied {
		t.Fatalf("apply = %v, %v", applied, err)
	}
	if err := db.Ingest().FailClaimedJob(ctx, probeJob, errors.New("プレビューのジョブを積めません")); err != nil {
		t.Fatal(err)
	}

	thumbnailJob := claimAtLastAttempt(t, db, domain.JobThumbnail, videoID)
	if written, err := db.Ingest().SetThumbnailStateForJob(ctx, thumbnailJob, domain.ThumbnailStateDone, domain.SubstitutionNone); err != nil || !written {
		t.Fatalf("thumbnail done = %v, %v", written, err)
	}
	if err := db.Ingest().FailClaimedJob(ctx, thumbnailJob, errors.New("seek thumbnails failed")); err != nil {
		t.Fatal(err)
	}

	video, err := db.Library().GetVideo(ctx, domain.AudienceOwner, videoID)
	if err != nil {
		t.Fatal(err)
	}
	if video.ProbeState != domain.ProbeStateDone || video.ProbeError != "" || !video.Playable {
		t.Fatalf("probe = %q (%q, playable=%v), want done", video.ProbeState, video.ProbeError, video.Playable)
	}
	if video.ThumbnailState != domain.ThumbnailStateDone {
		t.Fatalf("thumbnail = %q, want done", video.ThumbnailState)
	}
}

// Before terminal failure commits, retry must reject rather than create pending state without a job.
func TestRetryProbeDuringFinalAttemptWindow(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()
	job := claimAtLastAttempt(t, db, domain.JobProbe, videoID)

	// Simulate a handler failure before the worker records it through FailClaimedJob.
	if err := db.Ingest().RetryProbe(ctx, videoID); !errors.Is(err, domain.ErrProbeNotFailed) {
		t.Fatalf("window: err = %v, want ErrProbeNotFailed", err)
	}
	if err := db.Ingest().FailClaimedJob(ctx, job, errors.New("ffprobe failed")); err != nil {
		t.Fatal(err)
	}
	if err := db.Ingest().RetryProbe(ctx, videoID); err != nil {
		t.Fatal(err)
	}
	if got, want := jobCounts(t, db, videoID), map[string]int{"probe:queued": 1, "thumbnail:queued": 1}; !equalCounts(got, want) {
		t.Fatalf("jobs = %v, want %v", got, want)
	}
}

func equalCounts(got, want map[string]int) bool {
	if len(got) != len(want) {
		return false
	}
	for key, count := range want {
		if got[key] != count {
			return false
		}
	}
	return true
}
