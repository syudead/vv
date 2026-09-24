package store

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
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

	job, err := db.ClaimJob(ctx, JobProbe)
	if err != nil {
		t.Fatalf("ジョブを取り出せない: %v", err)
	}
	if job.Kind != JobProbe || job.VideoID != videoID {
		t.Errorf("取り出したジョブ = %+v", job)
	}

	// 専有したので、2 度目の取り出しでは何も返らない。
	if _, err := db.ClaimJob(ctx, JobProbe); !errors.Is(err, ErrNoJob) {
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

	if _, err := db.ClaimJob(context.Background(), JobProbe); !errors.Is(err, ErrNoJob) {
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
		job, err := db.ClaimJob(ctx, JobProbe)
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
	if _, err := db.ClaimJob(ctx, JobProbe); !errors.Is(err, ErrNoJob) {
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
	job, err := db.ClaimJob(ctx, JobProbe)
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
		job, err := db.ClaimJob(ctx, JobProbe)
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
	if _, err := db.ClaimJob(ctx, JobProbe); err != nil {
		t.Errorf("再投入したジョブを取り出せない: %v", err)
	}
	// 諦めた行は積み上げない。
	if got := countJobs(t, db); got != 1 {
		t.Errorf("ジョブ = %d 件, want 1（諦めた行を残さない）", got)
	}
}

// 終端の失敗は動画の状態に記録されるので、積み直さない。
func TestEnsureJobDoesNotReviveTerminalFailure(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()
	if err := db.EnqueueJob(ctx, JobProbe, videoID); err != nil {
		t.Fatal(err)
	}
	for range MaxJobAttempts {
		job, err := db.ClaimJob(ctx, JobProbe)
		if err != nil {
			t.Fatal(err)
		}
		if err := db.FailClaimedJob(ctx, job, "unreadable location"); err != nil {
			t.Fatal(err)
		}
	}
	var probeState string
	if err := db.SQL().QueryRow(`select probe_state from videos where id = ?`, videoID).Scan(&probeState); err != nil {
		t.Fatal(err)
	}
	if probeState != "failed" {
		t.Fatalf("probe_state = %s, want failed", probeState)
	}
	if err := db.EnsureJob(ctx, JobProbe, videoID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ClaimJob(ctx, JobProbe); !errors.Is(err, ErrNoJob) {
		t.Fatalf("terminal failure was revived: %v", err)
	}
}

// 旧版は失敗を行にだけ記録し、動画を pending のまま残すことがあった。走査が
// その動画を見つけたら、失敗の行を捨てて積み直す。
func TestEnsureJobRequeuesLegacyFailureOfPendingVideo(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()
	if err := db.EnqueueJob(ctx, JobProbe, videoID); err != nil {
		t.Fatal(err)
	}
	for range MaxJobAttempts {
		job, err := db.ClaimJob(ctx, JobProbe)
		if err != nil {
			t.Fatal(err)
		}
		// 旧版と同じく、行にだけ失敗を記録する。
		if err := db.FailJob(ctx, job.ID, "unreadable location"); err != nil {
			t.Fatal(err)
		}
	}
	recorder := &queuedRecorder{}
	db.OnJobsChanged(recorder.record)
	if err := db.EnsureJob(ctx, JobProbe, videoID); err != nil {
		t.Fatal(err)
	}
	if got := recorder.take(); fmt.Sprint(got) != "[probe]" {
		t.Errorf("知らせ = %v, want [probe]", got)
	}
	job, err := db.ClaimJob(ctx, JobProbe)
	if err != nil {
		t.Fatalf("積み直されていない: %v", err)
	}
	if job.Attempts != 1 {
		t.Errorf("Attempts = %d, want 1（新しい行から数え直す）", job.Attempts)
	}
	if got := countJobs(t, db); got != 1 {
		t.Errorf("ジョブ = %d 件, want 1（旧版の失敗の行を残さない）", got)
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
	job, err := db.ClaimJob(ctx, JobProbe)
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

	if _, err := db.ClaimJob(ctx, JobProbe); err != nil {
		t.Errorf("巻き戻したジョブを取り出せない: %v", err)
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
		job, err := db.ClaimJob(ctx, JobProbe)
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
	if _, err := db.ClaimJob(ctx, JobProbe); !errors.Is(err, ErrNoJob) {
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
	// サムネイルは解析の後に取り出すので、解析は済ませておく。
	probeDone(t, db, video.ID)
	if err := db.EnqueueJob(ctx, JobThumbnail, video.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ClaimJob(ctx, JobThumbnail); !errors.Is(err, ErrNoJob) {
		t.Fatalf("unregistered ClaimJob error = %v, want ErrNoJob", err)
	}
	if got := jobState(t, db, 1); got != "queued" {
		t.Fatalf("unregistered job state = %s, want queued", got)
	}
	recorder := &queuedRecorder{}
	db.OnJobsChanged(recorder.record)
	if _, err := db.AddMediaFolder(ctx, root); err != nil {
		t.Fatal(err)
	}
	// 眠っているワーカーを起こす知らせが出ること。登録で取り出せるようになった
	// 待ちの仕事は、次に仕事が積まれるのを待たずに処理される。
	if got := recorder.take(); !slices.Contains(got, JobThumbnail) {
		t.Fatalf("フォルダ登録の知らせ = %v, want thumbnail を含む", got)
	}
	job, err := db.ClaimJob(ctx, JobThumbnail)
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
	job, err := db.ClaimJob(ctx, JobProbe)
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
	retried, err := db.ClaimJob(ctx, JobProbe)
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
	job, err := db.ClaimJob(ctx, JobPreview)
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

func TestFailClaimedPreviewAtomicallyMarksTerminalState(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()
	if err := db.EnqueueJob(ctx, JobPreview, videoID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL().Exec(`update jobs set attempts = ? where kind = 'preview' and video_id = ?`, MaxJobAttempts-1, videoID); err != nil {
		t.Fatal(err)
	}
	job, err := db.ClaimJob(ctx, JobPreview)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.FailClaimedJob(ctx, job, "preview failed"); err != nil {
		t.Fatal(err)
	}
	if got := jobState(t, db, job.ID); got != "failed" {
		t.Fatalf("job state = %q, want failed", got)
	}
	video, err := db.GetVideo(ctx, videoID)
	if err != nil {
		t.Fatal(err)
	}
	if video.PreviewState != domain.PreviewStateFailed {
		t.Fatalf("preview state = %q, want failed", video.PreviewState)
	}
}

func TestFailClaimedPreviewKeepsPendingWhileRetrying(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()
	if err := db.EnqueueJob(ctx, JobPreview, videoID); err != nil {
		t.Fatal(err)
	}
	job, err := db.ClaimJob(ctx, JobPreview)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.FailClaimedJob(ctx, job, "retry preview"); err != nil {
		t.Fatal(err)
	}
	if got := jobState(t, db, job.ID); got != "queued" {
		t.Fatalf("job state = %q, want queued", got)
	}
	video, err := db.GetVideo(ctx, videoID)
	if err != nil {
		t.Fatal(err)
	}
	if video.PreviewState != domain.PreviewStatePending {
		t.Fatalf("preview state = %q, want pending", video.PreviewState)
	}
}

func TestFailClaimedPreviewRollsBackJobWhenStateUpdateFails(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()
	if err := db.EnqueueJob(ctx, JobPreview, videoID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL().Exec(`update jobs set attempts = ? where kind = 'preview' and video_id = ?`, MaxJobAttempts-1, videoID); err != nil {
		t.Fatal(err)
	}
	job, err := db.ClaimJob(ctx, JobPreview)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL().Exec(`create trigger reject_preview_failure before update of preview_state on videos
		when new.preview_state = 'failed' begin select raise(abort, 'reject preview failure'); end`); err != nil {
		t.Fatal(err)
	}
	if err := db.FailClaimedJob(ctx, job, "preview failed"); err == nil {
		t.Fatal("FailClaimedJob succeeded despite rejected preview state update")
	}
	if got := jobState(t, db, job.ID); got != "running" {
		t.Fatalf("job state = %q, want running after rollback", got)
	}
	video, err := db.GetVideo(ctx, videoID)
	if err != nil {
		t.Fatal(err)
	}
	if video.PreviewState != domain.PreviewStatePending {
		t.Fatalf("preview state = %q, want pending after rollback", video.PreviewState)
	}
}

func TestPreviewRunningJobIsRequeuedAfterRestart(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()
	if err := db.EnqueueJob(ctx, JobPreview, videoID); err != nil {
		t.Fatal(err)
	}
	claimed, err := db.ClaimJob(ctx, JobPreview)
	if err != nil {
		t.Fatal(err)
	}
	if claimed.Kind != JobPreview {
		t.Fatalf("kind = %q", claimed.Kind)
	}
	if restored, err := db.RequeueRunningJobs(ctx); err != nil || restored != 1 {
		t.Fatalf("RequeueRunningJobs() = %d, %v", restored, err)
	}
	retried, err := db.ClaimJob(ctx, JobPreview)
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
	job, err := db.ClaimJob(ctx, JobPreview)
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
	job, err := db.ClaimJob(ctx, JobPreview)
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
	job, err := db.ClaimJob(ctx, JobPreview)
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
	job, err := db.ClaimJob(ctx, JobPreview)
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
	job, err := db.ClaimJob(ctx, JobProbe)
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

// ClaimJob は指定した段階の仕事だけを取り出す。段階ごとのワーカーが、別の
// 段階の仕事を横取りしない。
func TestClaimJobTakesOnlyTheRequestedKind(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()
	probeDone(t, db, videoID)
	if err := db.EnqueueJob(ctx, JobThumbnail, videoID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ClaimJob(ctx, JobProbe); !errors.Is(err, ErrNoJob) {
		t.Fatalf("別の段階の ClaimJob error = %v, want ErrNoJob", err)
	}
	job, err := db.ClaimJob(ctx, JobThumbnail)
	if err != nil {
		t.Fatal(err)
	}
	if job.Kind != JobThumbnail {
		t.Fatalf("Kind = %s, want thumbnail", job.Kind)
	}
}

// queuedRecorder は OnJobsChanged の知らせを記録する。
type queuedRecorder struct {
	kinds []JobKind
	calls int
}

func (r *queuedRecorder) record(kinds []JobKind) {
	r.kinds = append(r.kinds, kinds...)
	r.calls++
}

func (r *queuedRecorder) take() []JobKind {
	kinds := r.kinds
	r.kinds = nil
	return kinds
}

// 仕事を積んだら、確定したあとでその段階を知らせる。ワーカーはこれで起きる。
// 何も積まなかったときは知らせない。
func TestJobsQueuedNotification(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()
	recorder := &queuedRecorder{}
	db.OnJobsChanged(func(kinds []JobKind) {
		// 知らせを受けた時点で、積んだ行が別の接続から見えていること。
		var queued int
		if err := db.SQL().QueryRow(`select count(*) from jobs where state = 'queued'`).Scan(&queued); err != nil {
			t.Error(err)
		}
		if queued == 0 {
			t.Error("確定前に知らせた")
		}
		recorder.record(kinds)
	})

	if err := db.EnqueueJob(ctx, JobProbe, videoID); err != nil {
		t.Fatal(err)
	}
	if got := recorder.take(); fmt.Sprint(got) != "[probe]" {
		t.Errorf("EnqueueJob の知らせ = %v, want [probe]", got)
	}

	if err := db.EnsureJob(ctx, JobThumbnail, videoID); err != nil {
		t.Fatal(err)
	}
	if got := recorder.take(); fmt.Sprint(got) != "[thumbnail]" {
		t.Errorf("EnsureJob の知らせ = %v, want [thumbnail]", got)
	}
	// 既にあるので何も積まない。
	if err := db.EnsureJob(ctx, JobThumbnail, videoID); err != nil {
		t.Fatal(err)
	}
	if got := recorder.take(); len(got) != 0 {
		t.Errorf("積まなかった EnsureJob が知らせた: %v", got)
	}

	if _, err := db.ClaimJob(ctx, JobProbe); err != nil {
		t.Fatal(err)
	}
	restored, err := db.RequeueRunningJobs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if restored != 1 {
		t.Fatalf("戻した数 = %d, want 1", restored)
	}
	if got := recorder.take(); len(got) == 0 {
		t.Error("RequeueRunningJobs が知らせない")
	}
	if _, err := db.RequeueRunningJobs(ctx); err != nil {
		t.Fatal(err)
	}
	if got := recorder.take(); len(got) != 0 {
		t.Errorf("何も戻さなかった RequeueRunningJobs が知らせた: %v", got)
	}
}

// 段階ごとの残りは queued と running を数え、終わった仕事と、登録外の所在しか
// ない動画の仕事は数えない。
func TestProcessingCountsRemainingWorkPerStage(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()
	other, err := db.UpsertVideo(ctx, sampleFile("/media/b.mp4", "b", "key-b", 2048, 0))
	if err != nil {
		t.Fatal(err)
	}
	outside, err := db.UpsertVideo(ctx, sampleFile("/elsewhere/c.mp4", "c", "key-c", 4096, 0))
	if err != nil {
		t.Fatal(err)
	}

	for _, job := range []struct {
		kind JobKind
		id   int64
	}{
		{JobProbe, videoID}, {JobProbe, other.ID}, {JobThumbnail, videoID},
		{JobPreview, other.ID}, {JobProbe, outside.ID},
	} {
		if err := db.EnqueueJob(ctx, job.kind, job.id); err != nil {
			t.Fatal(err)
		}
	}
	// 処理中も残りに数える（サムネイルは解析の後に取り出すので、解析は済ませておく）。
	probeDone(t, db, videoID)
	if _, err := db.ClaimJob(ctx, JobThumbnail); err != nil {
		t.Fatal(err)
	}
	// 終わった仕事は数えない。
	preview, err := db.ClaimJob(ctx, JobPreview)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.CompleteJob(ctx, preview.ID); err != nil {
		t.Fatal(err)
	}

	got, err := db.Processing(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := domain.Processing{Probe: 2, Thumbnail: 1, Preview: 0}
	if got != want {
		t.Errorf("Processing = %+v, want %+v", got, want)
	}
	if got.Remaining() != 3 {
		t.Errorf("Remaining = %d, want 3", got.Remaining())
	}
}

// サムネイルは解析が終わるまで取り出さない。段階ごとのワーカーは並行して動くので、
// 解析より先に作ると、動画の長さが分からないまま抽出位置が決まってしまう。
func TestClaimThumbnailWaitsForProbe(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()
	if err := db.EnqueueJob(ctx, JobThumbnail, videoID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ClaimJob(ctx, JobThumbnail); !errors.Is(err, ErrNoJob) {
		t.Fatalf("解析前の ClaimJob error = %v, want ErrNoJob", err)
	}

	probe := domain.Probe{DurationMs: 100_000, VideoCodec: "h264", AudioCodec: "aac"}
	if err := db.ApplyProbe(ctx, videoID, probe, domain.EvaluatePlayability("mp4", probe)); err != nil {
		t.Fatal(err)
	}
	job, err := db.ClaimJob(ctx, JobThumbnail)
	if err != nil {
		t.Fatalf("解析後も取り出せない: %v", err)
	}
	if job.VideoID != videoID {
		t.Fatalf("VideoID = %d, want %d", job.VideoID, videoID)
	}
}

// probeDone は解析を済ませた状態にする。サムネイルは解析の後に取り出す。
func probeDone(t *testing.T, db *DB, videoID int64) {
	t.Helper()
	probe := domain.Probe{DurationMs: 100_000, VideoCodec: "h264", AudioCodec: "aac"}
	if err := db.ApplyProbe(context.Background(), videoID, probe, domain.EvaluatePlayability("mp4", probe)); err != nil {
		t.Fatal(err)
	}
}

// 登録を外しても、登録外の所在が残る動画の行は消えない。それでもその仕事は
// 残りとして数えなくなるので、変わったことを知らせる。画面の残りの数が古いまま
// 残らない。
func TestDeleteMediaFolderNotifiesJobsChangedWithoutDeletingVideos(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	video, err := db.UpsertVideo(ctx, sampleFile("/media/a.mp4", "a", "key-a", 1024, 0))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertVideo(ctx, sampleFile("/legacy/a.mp4", "a", "key-a", 1024, 0)); err != nil {
		t.Fatal(err)
	}
	if err := db.EnqueueJob(ctx, JobProbe, video.ID); err != nil {
		t.Fatal(err)
	}
	if got, err := db.Processing(ctx); err != nil || got.Probe != 1 {
		t.Fatalf("Processing = %+v, %v, want probe 1", got, err)
	}
	var folderID, version int64
	if err := db.SQL().QueryRow(`select id, version from media_folders where path = '/media'`).Scan(&folderID, &version); err != nil {
		t.Fatal(err)
	}

	recorder := &queuedRecorder{}
	db.OnJobsChanged(recorder.record)
	deleted := 0
	db.OnVideosDeleted(func(videos []DeletedVideo) { deleted += len(videos) })
	if err := db.DeleteMediaFolder(ctx, folderID, version); err != nil {
		t.Fatal(err)
	}

	if deleted != 0 {
		t.Fatalf("消えた動画 = %d, want 0（登録外の所在が残る）", deleted)
	}
	if recorder.calls != 1 {
		t.Errorf("知らせ = %d 回, want 1", recorder.calls)
	}
	if got, err := db.Processing(ctx); err != nil || got.Probe != 0 {
		t.Errorf("Processing = %+v, %v, want probe 0", got, err)
	}
}

// 作り終えたプレビューのファイルが無い動画は、状態を戻して作り直しを1回だけ
// 積む。何度見つけても重ねて積まない。
func TestRequeueMissingPreview(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()
	probeDone(t, db, videoID)
	if err := db.SetPreviewState(ctx, videoID, domain.PreviewStateDone); err != nil {
		t.Fatal(err)
	}
	recorder := &queuedRecorder{}
	db.OnJobsChanged(recorder.record)

	if requeued, err := db.RequeueMissingPreview(ctx, videoID, "other-key"); err != nil || requeued {
		t.Fatalf("内容の違う要求 = %v, %v, want false", requeued, err)
	}
	requeued, err := db.RequeueMissingPreview(ctx, videoID, "key-a")
	if err != nil || !requeued {
		t.Fatalf("RequeueMissingPreview = %v, %v, want true", requeued, err)
	}
	if got := recorder.take(); fmt.Sprint(got) != "[preview]" {
		t.Errorf("知らせ = %v, want [preview]", got)
	}
	var state string
	if err := db.SQL().QueryRow(`select preview_state from videos where id = ?`, videoID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "pending" {
		t.Errorf("preview_state = %s, want pending", state)
	}

	if requeued, err := db.RequeueMissingPreview(ctx, videoID, "key-a"); err != nil || requeued {
		t.Fatalf("2度目 = %v, %v, want false", requeued, err)
	}
	job, err := db.ClaimJob(ctx, JobPreview)
	if err != nil {
		t.Fatalf("作り直しのジョブが無い: %v", err)
	}
	if job.VideoID != videoID {
		t.Errorf("VideoID = %d, want %d", job.VideoID, videoID)
	}
	if _, err := db.ClaimJob(ctx, JobPreview); !errors.Is(err, ErrNoJob) {
		t.Errorf("作り直しを重ねて積んだ: %v", err)
	}
}
