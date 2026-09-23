package store

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// jobsFixture は動画を1本入れた状態を返す。ジョブは videos を参照する。
func jobsFixture(t *testing.T) (*DB, int64) {
	t.Helper()

	db := migratedDB(t)
	added, err := db.UpsertVideo(context.Background(),
		sampleFile("/media/a.mp4", "a", "key-a", 1024, 0))
	if err != nil {
		t.Fatal(err)
	}
	return db, added.ID
}

// 状態遷移 queued → running → done。
func TestJobLifecycle(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()

	if err := db.EnqueueJob(ctx, JobProbe, videoID); err != nil {
		t.Fatal(err)
	}

	job, err := db.ClaimJob(ctx)
	if err != nil {
		t.Fatalf("ジョブを取り出せない: %v", err)
	}
	if job.Kind != JobProbe || job.VideoID != videoID {
		t.Errorf("取り出したジョブ = %+v", job)
	}

	// 専有したので、2 度目の取り出しでは何も返らない。
	if _, err := db.ClaimJob(ctx); !errors.Is(err, ErrNoJob) {
		t.Errorf("同じジョブが二重に取り出せた: %v", err)
	}

	if err := db.CompleteJob(ctx, job.ID); err != nil {
		t.Fatal(err)
	}
	if got := jobState(t, db, job.ID); got != "done" {
		t.Errorf("state = %q, want done", got)
	}
}

// 待ち行列が空なら ErrNoJob。ワーカーはこれを見て待機に入る。
func TestClaimJobOnEmptyQueue(t *testing.T) {
	db := migratedDB(t)

	if _, err := db.ClaimJob(context.Background()); !errors.Is(err, ErrNoJob) {
		t.Errorf("err = %v, want ErrNoJob", err)
	}
}

// 失敗は attempts を +1 して queued に戻し、3 回で failed にして止める。
// 止めないと、壊れたファイル1つがワーカーを永久に占有する。
func TestFailJobRetriesThenGivesUp(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()

	if err := db.EnqueueJob(ctx, JobProbe, videoID); err != nil {
		t.Fatal(err)
	}

	for attempt := 1; attempt <= MaxJobAttempts; attempt++ {
		job, err := db.ClaimJob(ctx)
		if err != nil {
			t.Fatalf("%d 回目の取り出しに失敗した: %v", attempt, err)
		}
		if job.Attempts != attempt {
			t.Errorf("%d 回目: Attempts = %d, want %d", attempt, job.Attempts, attempt)
		}
		if err := db.FailJob(ctx, job.ID, "ffprobe が失敗しました"); err != nil {
			t.Fatal(err)
		}

		want := "queued"
		if attempt == MaxJobAttempts {
			want = "failed"
		}
		if got := jobState(t, db, job.ID); got != want {
			t.Errorf("%d 回目のあと state = %q, want %q", attempt, got, want)
		}
	}

	// 諦めたジョブは二度と取り出されない。
	if _, err := db.ClaimJob(ctx); !errors.Is(err, ErrNoJob) {
		t.Errorf("failed なジョブが取り出せた: %v", err)
	}

	if MaxJobAttempts != 3 {
		t.Errorf("MaxJobAttempts = %d, want 3", MaxJobAttempts)
	}
}

// 失敗の理由は残す。何が起きたかを後から追えるようにする。
func TestFailJobRecordsReason(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()

	if err := db.EnqueueJob(ctx, JobProbe, videoID); err != nil {
		t.Fatal(err)
	}
	job, err := db.ClaimJob(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.FailJob(ctx, job.ID, "壊れた JSON"); err != nil {
		t.Fatal(err)
	}

	var reason string
	if err := db.SQL().QueryRow(`select last_error from jobs where id = ?`, job.ID).
		Scan(&reason); err != nil {
		t.Fatal(err)
	}
	if reason != "壊れた JSON" {
		t.Errorf("last_error = %q", reason)
	}
}

// 同じ (kind, video_id) の未完了ジョブは1件だけ。再スキャンを繰り返しても
// 待ち行列が膨らまない。
func TestEnqueueJobIsIdempotentWhilePending(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		if err := db.EnqueueJob(ctx, JobProbe, videoID); err != nil {
			t.Fatalf("%d 回目の投入で失敗した: %v", i, err)
		}
	}

	if got := countJobs(t, db); got != 1 {
		t.Errorf("ジョブ = %d 件, want 1", got)
	}

	// 種類が違えば別のジョブである。
	if err := db.EnqueueJob(ctx, JobThumbnail, videoID); err != nil {
		t.Fatal(err)
	}
	if got := countJobs(t, db); got != 2 {
		t.Errorf("種類違いを含めて %d 件, want 2", got)
	}
}

// 一度諦めたジョブは、再投入で取り直せる。内容が変わった動画を解析し直せ
// なければ、差し替えたファイルが永久に未解析のままになる。
func TestEnqueueJobRetriesAfterFailure(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()

	if err := db.EnqueueJob(ctx, JobProbe, videoID); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < MaxJobAttempts; i++ {
		job, err := db.ClaimJob(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := db.FailJob(ctx, job.ID, "失敗"); err != nil {
			t.Fatal(err)
		}
	}

	if err := db.EnqueueJob(ctx, JobProbe, videoID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ClaimJob(ctx); err != nil {
		t.Errorf("再投入したジョブを取り出せない: %v", err)
	}
	// 諦めた行は積み上げない。
	if got := countJobs(t, db); got != 1 {
		t.Errorf("ジョブ = %d 件, want 1（諦めた行を残さない）", got)
	}
}

func TestEnsureJobDoesNotReviveTerminalFailure(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()
	if err := db.EnqueueJob(ctx, JobProbe, videoID); err != nil {
		t.Fatal(err)
	}
	for range MaxJobAttempts {
		job, err := db.ClaimJob(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := db.FailJob(ctx, job.ID, "unreadable location"); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.EnsureJob(ctx, JobProbe, videoID); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-8 * 24 * time.Hour).Unix()
	if _, err := db.SQL().Exec(`update jobs set updated_at = ? where video_id = ?`, old, videoID); err != nil {
		t.Fatal(err)
	}
	removed, err := db.DeleteFinishedJobsBefore(ctx, time.Now().Add(-JobRetention))
	if err != nil {
		t.Fatal(err)
	}
	if removed != 0 {
		t.Fatalf("pending state retry suppression was garbage-collected: %d", removed)
	}
	if _, err := db.ClaimJob(ctx); !errors.Is(err, ErrNoJob) {
		t.Fatalf("terminal failure was revived: %v", err)
	}
}

// 起動時に running のまま残っている行は queued へ戻す。取り込み中に止めても
// 次の起動で再開でき、重複も生まない。
func TestRequeueRunningJobs(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()

	if err := db.EnqueueJob(ctx, JobProbe, videoID); err != nil {
		t.Fatal(err)
	}
	job, err := db.ClaimJob(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// ここでプロセスが落ちた状況を作る（running のまま放置）。

	restored, err := db.RequeueRunningJobs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if restored != 1 {
		t.Errorf("巻き戻した数 = %d, want 1", restored)
	}
	if got := jobState(t, db, job.ID); got != "queued" {
		t.Errorf("state = %q, want queued", got)
	}

	if _, err := db.ClaimJob(ctx); err != nil {
		t.Errorf("巻き戻したジョブを取り出せない: %v", err)
	}
}

// 完了した行は 7 日で掃除する。取り込み直後に最大 2万行になるため。
func TestDeleteFinishedJobsBefore(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()

	if err := db.EnqueueJob(ctx, JobProbe, videoID); err != nil {
		t.Fatal(err)
	}
	job, err := db.ClaimJob(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.CompleteJob(ctx, job.ID); err != nil {
		t.Fatal(err)
	}

	// まだ新しいので消えない。
	removed, err := db.DeleteFinishedJobsBefore(ctx, time.Now().Add(-JobRetention))
	if err != nil {
		t.Fatal(err)
	}
	if removed != 0 {
		t.Errorf("新しい完了行が消えた: %d 件", removed)
	}

	// 8 日前に完了したことにする。
	old := time.Now().Add(-8 * 24 * time.Hour).Unix()
	if _, err := db.SQL().Exec(`update jobs set updated_at = ? where id = ?`, old, job.ID); err != nil {
		t.Fatal(err)
	}

	removed, err = db.DeleteFinishedJobsBefore(ctx, time.Now().Add(-JobRetention))
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 {
		t.Errorf("古い完了行が消えなかった: %d 件, want 1", removed)
	}

	if JobRetention != 7*24*time.Hour {
		t.Errorf("JobRetention = %v, want 168h", JobRetention)
	}
}

// 未完了のジョブは掃除の対象にしない。
func TestDeleteFinishedJobsKeepsPending(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()

	if err := db.EnqueueJob(ctx, JobProbe, videoID); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-30 * 24 * time.Hour).Unix()
	if _, err := db.SQL().Exec(`update jobs set created_at = ?, updated_at = ?`, old, old); err != nil {
		t.Fatal(err)
	}

	removed, err := db.DeleteFinishedJobsBefore(ctx, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if removed != 0 {
		t.Errorf("未完了のジョブが消えた: %d 件", removed)
	}
}

func TestClaimJobTriesEveryLocationBeforeConsumingAnotherAttempt(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	var videoID int64
	for i := range 4 {
		result, err := db.UpsertVideo(ctx, sampleFile(fmt.Sprintf("/media/%d/movie.mp4", i), "movie", "shared", 1, 0))
		if err != nil {
			t.Fatal(err)
		}
		videoID = result.ID
	}
	if err := db.EnqueueJob(ctx, JobProbe, videoID); err != nil {
		t.Fatal(err)
	}
	for i := range 4 * MaxJobAttempts {
		job, err := db.ClaimJob(ctx)
		if err != nil {
			t.Fatal(err)
		}
		wantAttempt := i/4 + 1
		if job.Attempts != wantAttempt {
			t.Fatalf("location %d attempts = %d, want %d", i, job.Attempts, wantAttempt)
		}
		if job.LastLocation != (i%4 == 3) {
			t.Fatalf("location %d LastLocation = %v", i, job.LastLocation)
		}
		if err := db.FailClaimedJob(ctx, job, "location unavailable"); err != nil {
			t.Fatal(err)
		}
		wantState := "queued"
		if i == 4*MaxJobAttempts-1 {
			wantState = "failed"
		}
		if got := jobState(t, db, job.ID); got != wantState {
			t.Fatalf("location %d state = %s, want %s", i, got, wantState)
		}
	}
	if _, err := db.ClaimJob(ctx); !errors.Is(err, ErrNoJob) {
		t.Fatalf("ClaimJob after final cycle error = %v, want ErrNoJob", err)
	}
}

func TestClaimJobWaitsForMigratedLocationToBeRegistered(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	root := t.TempDir()
	video, err := db.UpsertVideo(ctx, sampleFile(filepath.Join(root, "movie.mp4"), "movie", "migrated", 1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL().Exec(`delete from media_folders`); err != nil {
		t.Fatal(err)
	}
	if err := db.EnqueueJob(ctx, JobThumbnail, video.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ClaimJob(ctx); !errors.Is(err, ErrNoJob) {
		t.Fatalf("unregistered ClaimJob error = %v, want ErrNoJob", err)
	}
	if got := jobState(t, db, 1); got != "queued" {
		t.Fatalf("unregistered job state = %s, want queued", got)
	}
	if _, err := db.AddMediaFolder(ctx, root); err != nil {
		t.Fatal(err)
	}
	job, err := db.ClaimJob(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if job.LocationPath != filepath.Join(root, "movie.mp4") {
		t.Fatalf("LocationPath = %q", job.LocationPath)
	}
}

func TestClaimedJobBecomesStaleWhenLocationIsAdded(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()
	if err := db.EnqueueJob(ctx, JobProbe, videoID); err != nil {
		t.Fatal(err)
	}
	job, err := db.ClaimJob(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertVideo(ctx, sampleFile("/media/z.mp4", "z", "key-a", 1024, 0)); err != nil {
		t.Fatal(err)
	}

	written, err := db.ApplyProbeForJob(ctx, job, domain.Probe{VideoCodec: "h264", AudioCodec: "aac"}, domain.Playability{Playable: true})
	if err != nil {
		t.Fatal(err)
	}
	if written {
		t.Fatal("job claimed before a location was added wrote a stale result")
	}
	written, err = db.SetThumbnailStateForJob(ctx, job, domain.ThumbnailStateDone)
	if err != nil {
		t.Fatal(err)
	}
	if written {
		t.Fatal("thumbnail job claimed before a location was added wrote a stale result")
	}
	if err := db.FailClaimedJob(ctx, job, "old location failed"); err != nil {
		t.Fatal(err)
	}
	if got := jobState(t, db, job.ID); got != "queued" {
		t.Fatalf("state = %q, want queued", got)
	}
	retried, err := db.ClaimJob(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if retried.LocationPath != "/media/z.mp4" {
		t.Fatalf("retry path = %q, want /media/z.mp4", retried.LocationPath)
	}
}

func TestPreviewStateUsesContentIdentityForSuccessAndClaimIdentityForFailure(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()
	probe := domain.Probe{DurationMs: 1000, VideoCodec: "h264", AudioCodec: "aac"}
	if err := db.ApplyProbe(ctx, videoID, probe, domain.EvaluatePlayability(domain.ContainerFromPath("/media/a.mp4"), probe)); err != nil {
		t.Fatal(err)
	}
	if err := db.EnqueueJob(ctx, JobPreview, videoID); err != nil {
		t.Fatal(err)
	}
	job, err := db.ClaimJob(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertVideo(ctx, sampleFile("/media/z.mp4", "z", "key-a", 1024, 0)); err != nil {
		t.Fatal(err)
	}

	written, err := db.SetPreviewStateForContent(ctx, job, domain.PreviewStateDone)
	if err != nil || !written {
		t.Fatalf("content-key completion = %v, %v", written, err)
	}
	if err := db.SetPreviewState(ctx, videoID, domain.PreviewStatePending); err != nil {
		t.Fatal(err)
	}
	written, err = db.SetPreviewStateForJob(ctx, job, domain.PreviewStateFailed)
	if err != nil {
		t.Fatal(err)
	}
	if written {
		t.Fatal("stale location claim marked preview failed")
	}
	video, err := db.GetVideo(ctx, videoID)
	if err != nil {
		t.Fatal(err)
	}
	if video.PreviewState != domain.PreviewStatePending {
		t.Fatalf("preview state = %q, want pending", video.PreviewState)
	}
}

func TestDeleteFinishedJobsRemovesTerminalPreviewFailure(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()
	if err := db.EnqueueJob(ctx, JobPreview, videoID); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-8 * 24 * time.Hour).Unix()
	if _, err := db.SQL().Exec(`update videos set preview_state = 'failed' where id = ?`, videoID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL().Exec(`update jobs set state = 'failed', attempts = ?, updated_at = ? where video_id = ? and kind = 'preview'`, MaxJobAttempts, old, videoID); err != nil {
		t.Fatal(err)
	}
	removed, err := db.DeleteFinishedJobsBefore(ctx, time.Now().Add(-JobRetention))
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 || countJobs(t, db) != 0 {
		t.Fatalf("removed = %d, jobs = %d", removed, countJobs(t, db))
	}
}

func TestPreviewRunningJobIsRequeuedAfterRestart(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()
	if err := db.EnqueueJob(ctx, JobPreview, videoID); err != nil {
		t.Fatal(err)
	}
	claimed, err := db.ClaimJob(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if claimed.Kind != JobPreview {
		t.Fatalf("kind = %q", claimed.Kind)
	}
	if restored, err := db.RequeueRunningJobs(ctx); err != nil || restored != 1 {
		t.Fatalf("RequeueRunningJobs() = %d, %v", restored, err)
	}
	retried, err := db.ClaimJob(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if retried.ID != claimed.ID || retried.Kind != JobPreview {
		t.Fatalf("retried job = %+v, want id %d preview", retried, claimed.ID)
	}
}

func TestPreviewCompletionRejectsChangedContent(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()
	if err := db.EnqueueJob(ctx, JobPreview, videoID); err != nil {
		t.Fatal(err)
	}
	job, err := db.ClaimJob(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertVideo(ctx, sampleFile("/media/a.mp4", "replacement", "key-b", 2048, time.Second)); err != nil {
		t.Fatal(err)
	}
	current, err := db.ContentKeyCurrent(ctx, job.VideoID, job.ContentKey)
	if err != nil {
		t.Fatal(err)
	}
	if current {
		t.Fatal("changed content remained current")
	}
	written, err := db.SetPreviewStateForContent(ctx, job, domain.PreviewStateDone)
	if err != nil {
		t.Fatal(err)
	}
	if written {
		t.Fatal("changed content accepted stale preview completion")
	}
}

func TestPreviewSourceRejectsReassignedClaimedPathWithOriginalContentRemaining(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	video, err := db.UpsertVideo(ctx, sampleFile("/media/a.mp4", "a", "key-a", 1024, 0))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertVideo(ctx, sampleFile("/media/b.mp4", "b", "key-a", 1024, 0)); err != nil {
		t.Fatal(err)
	}
	if err := db.EnqueueJob(ctx, JobPreview, video.ID); err != nil {
		t.Fatal(err)
	}
	job, err := db.ClaimJob(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if job.LocationPath != "/media/a.mp4" {
		t.Fatalf("claimed path = %q", job.LocationPath)
	}
	if _, err := db.UpsertVideo(ctx, sampleFile("/media/a.mp4", "replacement", "key-b", 2048, time.Second)); err != nil {
		t.Fatal(err)
	}
	contentCurrent, err := db.ContentKeyCurrent(ctx, job.VideoID, job.ContentKey)
	if err != nil || !contentCurrent {
		t.Fatalf("original content should remain through b.mp4: %v, %v", contentCurrent, err)
	}
	sourceCurrent, err := db.PreviewSourceCurrent(ctx, job)
	if err != nil {
		t.Fatal(err)
	}
	if sourceCurrent {
		t.Fatal("claimed path reassigned to different content was accepted")
	}
}

func TestPreviewSourceAcceptsLocationOnlyChange(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()
	if err := db.EnqueueJob(ctx, JobPreview, videoID); err != nil {
		t.Fatal(err)
	}
	job, err := db.ClaimJob(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertVideo(ctx, sampleFile("/media/z.mp4", "z", job.ContentKey, 1024, 0)); err != nil {
		t.Fatal(err)
	}
	current, err := db.PreviewSourceCurrent(ctx, job)
	if err != nil || !current {
		t.Fatalf("location-only change = %v, %v; want current", current, err)
	}
}

func TestCompletePreviewAtomicallyFinishesAssetAndJobAfterCancellation(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()
	if err := db.EnqueueJob(ctx, JobPreview, videoID); err != nil {
		t.Fatal(err)
	}
	job, err := db.ClaimJob(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertVideo(ctx, sampleFile("/media/z.mp4", "z", job.ContentKey, 1024, 0)); err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	applied, err := db.CompletePreviewForContent(context.WithoutCancel(cancelled), job)
	if err != nil || !applied {
		t.Fatalf("CompletePreviewForContent() = %v, %v", applied, err)
	}
	if got := jobState(t, db, job.ID); got != "done" {
		t.Fatalf("job state = %q, want done", got)
	}
	video, err := db.GetVideo(ctx, videoID)
	if err != nil {
		t.Fatal(err)
	}
	if video.PreviewState != domain.PreviewStateDone {
		t.Fatalf("preview state = %q, want done", video.PreviewState)
	}
	if err := db.CompleteClaimedJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	if got := jobState(t, db, job.ID); got != "done" {
		t.Fatalf("generic completion rewrote atomic preview completion to %q", got)
	}
}

func TestClaimedJobBecomesStaleWhenLowerIDLocationIsReassigned(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	lower, err := db.UpsertVideo(ctx, sampleFile("/media/a.mp4", "a", "key-a", 1024, 0))
	if err != nil {
		t.Fatal(err)
	}
	target, err := db.UpsertVideo(ctx, sampleFile("/media/b.mp4", "b", "key-b", 1024, 0))
	if err != nil {
		t.Fatal(err)
	}
	if lower.ID >= target.ID {
		t.Fatalf("fixture IDs = %d, %d; want lower source ID", lower.ID, target.ID)
	}
	if err := db.EnqueueJob(ctx, JobProbe, target.ID); err != nil {
		t.Fatal(err)
	}
	job, err := db.ClaimJob(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Reusing the earlier location for the target content changes membership
	// without creating an ID larger than the claimed target location.
	if _, err := db.UpsertVideo(ctx, sampleFile("/media/a.mp4", "a", "key-b", 2048, time.Second)); err != nil {
		t.Fatal(err)
	}
	current, err := db.JobIdentityCurrent(ctx, job)
	if err != nil {
		t.Fatal(err)
	}
	if current {
		t.Fatal("job identity remained current after a lower-ID location was reassigned")
	}
	written, err := db.ApplyProbeForJob(ctx, job, domain.Probe{VideoCodec: "h264"}, domain.Playability{Playable: true})
	if err != nil {
		t.Fatal(err)
	}
	if written {
		t.Fatal("job wrote a result after a lower-ID location was reassigned")
	}
	if err := db.CompleteClaimedJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	if got := jobState(t, db, job.ID); got != "queued" {
		t.Fatalf("state = %q, want queued", got)
	}
}

func jobState(t *testing.T, db *DB, id int64) string {
	t.Helper()

	var state string
	if err := db.SQL().QueryRow(`select state from jobs where id = ?`, id).Scan(&state); err != nil {
		t.Fatal(err)
	}
	return state
}

func countJobs(t *testing.T, db *DB) int {
	t.Helper()

	var count int
	if err := db.SQL().QueryRow(`select count(*) from jobs`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}
