package store

import (
	"context"
	"errors"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// jobCounts は種類と状態ごとのジョブの数を返す。
func jobCounts(t *testing.T, db *DB, videoID int64) map[string]int {
	t.Helper()
	rows, err := db.SQL().Query(`select kind || ':' || state, count(*) from jobs where video_id = ? group by kind, state`, videoID)
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

// claimAtLastAttempt は指定の種類のジョブを、最後の試行として専有する。
func claimAtLastAttempt(t *testing.T, db *DB, kind domain.JobKind, videoID int64) domain.Job {
	t.Helper()
	ctx := context.Background()
	if err := db.Ingest().EnqueueJob(ctx, kind, videoID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL().Exec(`update jobs set attempts = ? where kind = ? and video_id = ?`, domain.MaxJobAttempts-1, string(kind), videoID); err != nil {
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

// failedVideoFixture は読み取り・サムネイル・プレビューがすべて終端失敗した
// 動画を、ジョブの失敗を経て作る。
func failedVideoFixture(t *testing.T) (*DB, int64) {
	t.Helper()
	db, videoID := jobsFixture(t)
	ctx := context.Background()
	for _, kind := range []domain.JobKind{domain.JobProbe, domain.JobThumbnail} {
		job := claimAtLastAttempt(t, db, kind, videoID)
		if err := db.Ingest().FailClaimedJob(ctx, job, string(kind)+" failed"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.SQL().Exec(`update videos set preview_state = 'failed' where id = ?`, videoID); err != nil {
		t.Fatal(err)
	}
	return db, videoID
}

// 失敗した動画を読み取り直すと、状態が pending に戻り、読み取りとサムネイルの
// ジョブがちょうど1件ずつ積まれる。続けて送ると 2 回目は ErrProbeNotFailed で、
// ジョブは増えない。
func TestRetryProbeRequeuesFailedVideoOnce(t *testing.T) {
	db, videoID := failedVideoFixture(t)
	ctx := context.Background()

	if err := db.Ingest().RetryProbe(ctx, videoID, true); err != nil {
		t.Fatal(err)
	}
	video, err := db.Library().GetVideo(ctx, videoID)
	if err != nil {
		t.Fatal(err)
	}
	if video.ProbeState != domain.ProbeStatePending || video.ProbeError != "" {
		t.Fatalf("probe = %q (%q), want pending without error", video.ProbeState, video.ProbeError)
	}
	if video.ThumbnailState != domain.ThumbnailStatePending || video.PreviewState != domain.PreviewStatePending {
		t.Fatalf("thumbnail = %q, preview = %q, want pending", video.ThumbnailState, video.PreviewState)
	}
	want := map[string]int{"probe:queued": 1, "thumbnail:queued": 1}
	if got := jobCounts(t, db, videoID); !equalCounts(got, want) {
		t.Fatalf("jobs = %v, want %v", got, want)
	}

	if err := db.Ingest().RetryProbe(ctx, videoID, true); !errors.Is(err, domain.ErrProbeNotFailed) {
		t.Fatalf("2 回目: err = %v, want ErrProbeNotFailed", err)
	}
	if got := jobCounts(t, db, videoID); !equalCounts(got, want) {
		t.Fatalf("2 回目のあと jobs = %v, want %v", got, want)
	}
}

// サムネイルが完成していれば状態は done のままにする。シーク用プレビューの
// 置き場が無いときだけサムネイルのジョブを積む。
func TestRetryProbeKeepsCompletedThumbnail(t *testing.T) {
	for _, tc := range []struct {
		name        string
		seekMissing bool
		want        map[string]int
	}{
		{name: "seek missing", seekMissing: true, want: map[string]int{"probe:queued": 1, "thumbnail:queued": 1}},
		{name: "seek present", seekMissing: false, want: map[string]int{"probe:queued": 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, videoID := jobsFixture(t)
			ctx := context.Background()
			if _, err := db.SQL().Exec(`update videos set probe_state = 'failed', probe_error = 'broken',
				thumbnail_state = 'done' where id = ?`, videoID); err != nil {
				t.Fatal(err)
			}
			if err := db.Ingest().RetryProbe(ctx, videoID, tc.seekMissing); err != nil {
				t.Fatal(err)
			}
			video, err := db.Library().GetVideo(ctx, videoID)
			if err != nil {
				t.Fatal(err)
			}
			if video.ThumbnailState != domain.ThumbnailStateDone {
				t.Fatalf("thumbnail = %q, want done", video.ThumbnailState)
			}
			if got := jobCounts(t, db, videoID); !equalCounts(got, tc.want) {
				t.Fatalf("jobs = %v, want %v", got, tc.want)
			}
		})
	}
}

// 読み取り中・読み取り済みの動画は ErrProbeNotFailed、無い動画は ErrNotFound。
func TestRetryProbeRejectsNonFailedAndMissing(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()
	if err := db.Ingest().RetryProbe(ctx, videoID, true); !errors.Is(err, domain.ErrProbeNotFailed) {
		t.Fatalf("pending: err = %v, want ErrProbeNotFailed", err)
	}
	probe := domain.Probe{DurationMs: 1_000, VideoCodec: "h264", AudioCodec: "aac"}
	if err := db.Ingest().ApplyProbe(ctx, videoID, probe, domain.EvaluatePlayability("mp4", probe)); err != nil {
		t.Fatal(err)
	}
	if err := db.Ingest().RetryProbe(ctx, videoID, true); !errors.Is(err, domain.ErrProbeNotFailed) {
		t.Fatalf("done: err = %v, want ErrProbeNotFailed", err)
	}
	if err := db.Ingest().RetryProbe(ctx, videoID+100, true); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing: err = %v, want domain.ErrNotFound", err)
	}
	if count := countJobs(t, db); count != 0 {
		t.Fatalf("jobs = %d, want 0", count)
	}
}

// ファイルの確認などで上限まで失敗した読み取り・サムネイルのジョブは、ジョブを
// failed にするのと同じ取引で動画側を failed にする。
func TestFailClaimedJobRecordsProbeAndThumbnailFailure(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()

	probeJob := claimAtLastAttempt(t, db, domain.JobProbe, videoID)
	if err := db.Ingest().FailClaimedJob(ctx, probeJob, "open /media/a.mp4: no such file or directory"); err != nil {
		t.Fatal(err)
	}
	thumbnailJob := claimAtLastAttempt(t, db, domain.JobThumbnail, videoID)
	if err := db.Ingest().FailClaimedJob(ctx, thumbnailJob, "open /media/a.mp4: no such file or directory"); err != nil {
		t.Fatal(err)
	}

	video, err := db.Library().GetVideo(ctx, videoID)
	if err != nil {
		t.Fatal(err)
	}
	if video.ProbeState != domain.ProbeStateFailed || video.ProbeError != "open /media/a.mp4: no such file or directory" {
		t.Fatalf("probe = %q (%q), want failed with reason", video.ProbeState, video.ProbeError)
	}
	if video.ThumbnailState != domain.ThumbnailStateFailed {
		t.Fatalf("thumbnail = %q, want failed", video.ThumbnailState)
	}
}

// 失敗の記録が拒まれたら、ジョブの失敗も記録しない（同じ取引である）。
func TestFailClaimedProbeRollsBackJobWhenStateUpdateFails(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()
	job := claimAtLastAttempt(t, db, domain.JobProbe, videoID)
	if _, err := db.SQL().Exec(`create trigger reject_probe_failure before update of probe_state on videos
		when new.probe_state = 'failed' begin select raise(abort, 'reject probe failure'); end`); err != nil {
		t.Fatal(err)
	}
	if err := db.Ingest().FailClaimedJob(ctx, job, "probe failed"); err == nil {
		t.Fatal("FailClaimedJob succeeded despite rejected probe state update")
	}
	if got := jobState(t, db, job.ID); got != "running" {
		t.Fatalf("job state = %q, want running after rollback", got)
	}
}

// 再試行が残っている間は、動画側は pending のままにする。
func TestFailClaimedProbeKeepsPendingWhileRetrying(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()
	if err := db.Ingest().EnqueueJob(ctx, domain.JobProbe, videoID); err != nil {
		t.Fatal(err)
	}
	job, err := db.Ingest().ClaimJob(ctx, domain.JobProbe)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ingest().FailClaimedJob(ctx, job, "retry"); err != nil {
		t.Fatal(err)
	}
	video, err := db.Library().GetVideo(ctx, videoID)
	if err != nil {
		t.Fatal(err)
	}
	if video.ProbeState != domain.ProbeStatePending {
		t.Fatalf("probe = %q, want pending", video.ProbeState)
	}
}

// 読み取りの結果を保存したあと、プレビューのジョブを積むところで上限まで失敗
// しても、probe_state は done のまま変わらない。代表サムネイルの後でシーク用
// プレビューだけが失敗したときも、thumbnail_state は done のまま残る。
func TestFailClaimedJobKeepsCompletedState(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()

	probeJob := claimAtLastAttempt(t, db, domain.JobProbe, videoID)
	probe := domain.Probe{DurationMs: 1_000, VideoCodec: "h264", AudioCodec: "aac"}
	applied, err := db.Ingest().ApplyProbeForJob(ctx, probeJob, probe, domain.EvaluatePlayability("mp4", probe))
	if err != nil || !applied {
		t.Fatalf("apply = %v, %v", applied, err)
	}
	if err := db.Ingest().FailClaimedJob(ctx, probeJob, "プレビューのジョブを積めません"); err != nil {
		t.Fatal(err)
	}

	thumbnailJob := claimAtLastAttempt(t, db, domain.JobThumbnail, videoID)
	if written, err := db.Ingest().SetThumbnailStateForJob(ctx, thumbnailJob, domain.ThumbnailStateDone); err != nil || !written {
		t.Fatalf("thumbnail done = %v, %v", written, err)
	}
	if err := db.Ingest().FailClaimedJob(ctx, thumbnailJob, "seek thumbnails failed"); err != nil {
		t.Fatal(err)
	}

	video, err := db.Library().GetVideo(ctx, videoID)
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

// 最後の試行が失敗した直後（ジョブの失敗が記録される前）には、動画はまだ failed に
// ならない。この間の要求は ErrProbeNotFailed になり、ジョブの無い pending は
// 生まれない。失敗が記録されたあとの要求は、新しいジョブをちょうど1件積む。
func TestRetryProbeDuringFinalAttemptWindow(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()
	job := claimAtLastAttempt(t, db, domain.JobProbe, videoID)

	// ハンドラは失敗を返したが、ワーカーはまだ FailClaimedJob を呼んでいない。
	if err := db.Ingest().RetryProbe(ctx, videoID, false); !errors.Is(err, domain.ErrProbeNotFailed) {
		t.Fatalf("window: err = %v, want ErrProbeNotFailed", err)
	}
	if err := db.Ingest().FailClaimedJob(ctx, job, "ffprobe failed"); err != nil {
		t.Fatal(err)
	}
	if err := db.Ingest().RetryProbe(ctx, videoID, false); err != nil {
		t.Fatal(err)
	}
	// サムネイルは完成していないので、読み取りと同じ組で積む。
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
