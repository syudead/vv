package store

import (
	"context"
	"errors"
	"testing"
	"time"
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

// 失敗は attempts を +1 して queued に戻し、3 回で failed にして止める（R-106）。
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

// 起動時に running のまま残っている行は queued へ戻す。取り込み中に止めても
// 次の起動で再開でき、重複も生まない（R-106 / spec のエッジケース）。
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

// 完了した行は 7 日で掃除する。取り込み直後に最大 2万行になるため
// （data-model.md 4 節）。
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
