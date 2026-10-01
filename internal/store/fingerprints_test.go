package store

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"testing"

	"github.com/pressly/goose/v3"

	"github.com/syudead/vv/internal/domain"
)

// seekDone はシーク用サムネイルの仕事を専有して完了を記録する。
func seekDone(t *testing.T, db *DB, videoID int64) {
	t.Helper()
	ctx := context.Background()
	if err := db.Ingest().EnqueueJob(ctx, domain.JobSeekThumbnail, videoID); err != nil {
		t.Fatal(err)
	}
	job, err := db.Ingest().ClaimJob(ctx, domain.JobSeekThumbnail)
	if err != nil {
		t.Fatal(err)
	}
	if applied, err := db.Ingest().SetSeekThumbnailStateForJob(ctx, job, domain.SeekThumbnailDone, domain.SubstitutionNone); err != nil || !applied {
		t.Fatalf("SetSeekThumbnailStateForJob = %v, %v", applied, err)
	}
	if err := db.Ingest().CompleteClaimedJob(ctx, job); err != nil {
		t.Fatal(err)
	}
}

func sampleFingerprint() domain.Fingerprint {
	return domain.Fingerprint{Version: domain.FingerprintVersion, IntervalMs: 5000, Frames: []domain.FrameHash{
		{Hash: 1}, {Hash: 2, Flat: true}, {Hash: 3},
	}}
}

// storedFingerprint は内容の指紋の行を返す。無ければ ok が偽。
func storedFingerprint(t *testing.T, db *DB, key string) (domain.Fingerprint, bool) {
	t.Helper()
	var version int
	var interval int64
	var hashes []byte
	err := db.sql.QueryRow(`select version, interval_ms, hashes from video_fingerprints where content_key = ?`, key).
		Scan(&version, &interval, &hashes)
	if err != nil {
		return domain.Fingerprint{}, false
	}
	fingerprint, err := domain.DecodeFingerprint(hashes, version, interval)
	if err != nil {
		t.Fatal(err)
	}
	return fingerprint, true
}

// シーク用サムネイルの完了を書く取引で指紋の仕事を積み、知らせる。その仕事は走査の
// 残りの仕事に数え、動画を直近の走査の対象に加える。
func TestSeekThumbnailDoneQueuesFingerprint(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()
	probeDone(t, db, videoID)
	if _, _, err := db.Scans().StartScan(ctx); err != nil {
		t.Fatal(err)
	}
	recorder := &queuedRecorder{}
	db.PublishTo(recorder)

	seekDone(t, db, videoID)
	if got := jobCounts(t, db, videoID); got["fingerprint:queued"] != 1 {
		t.Fatalf("jobs = %v, want one queued fingerprint", got)
	}
	if fmt.Sprint(recorder.kinds) != "[seek_thumbnail fingerprint]" {
		t.Errorf("知らせ = %v, want [seek_thumbnail fingerprint]", recorder.kinds)
	}
	if got := remainingByKind(t, db); got[domain.JobFingerprint] != 1 {
		t.Fatalf("残り = %v, want one fingerprint", got)
	}
	var inScan int
	if err := db.sql.QueryRow(`select count(*) from scan_videos where video_id = ?`, videoID).Scan(&inScan); err != nil {
		t.Fatal(err)
	}
	if inScan != 1 {
		t.Fatalf("scan_videos = %d, want 1", inScan)
	}

	// 失敗や作り直しの pending では積まない。
	other, err := db.ScanIndex().UpsertVideo(ctx, sampleFile(fixturePath("/media/b.mp4"), "b", "key-b", 2048, 0))
	if err != nil {
		t.Fatal(err)
	}
	probeDone(t, db, other.ID)
	job := claimAtLastAttempt(t, db, domain.JobSeekThumbnail, other.ID)
	if err := db.Ingest().FailClaimedJob(ctx, job, errors.New("ffmpeg failed")); err != nil {
		t.Fatal(err)
	}
	if got := jobCounts(t, db, other.ID); got["fingerprint:queued"] != 0 {
		t.Fatalf("失敗したシーク用サムネイルの jobs = %v", got)
	}
}

// 指紋の仕事はシーク用サムネイルが完成するまで取り出さない。
func TestClaimFingerprintWaitsForSeekThumbnail(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()
	probeDone(t, db, videoID)
	if err := db.Ingest().EnqueueJob(ctx, domain.JobFingerprint, videoID); err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{"pending", "failed"} {
		if _, err := db.sql.Exec(`update videos set seek_thumbnail_state = ? where id = ?`, state, videoID); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Ingest().ClaimJob(ctx, domain.JobFingerprint); !errors.Is(err, domain.ErrNoJob) {
			t.Fatalf("seek %s: ClaimJob error = %v, want domain.ErrNoJob", state, err)
		}
	}
	if _, err := db.sql.Exec(`update videos set seek_thumbnail_state = 'done' where id = ?`, videoID); err != nil {
		t.Fatal(err)
	}
	job, err := db.Ingest().ClaimJob(ctx, domain.JobFingerprint)
	if err != nil {
		t.Fatal(err)
	}
	if job.VideoID != videoID || job.Kind != domain.JobFingerprint {
		t.Fatalf("claimed = %+v", job)
	}
}

// 指紋は専有した時点の内容と所在が今も同じときだけ置き換え、内容の参照が無くなると消える。
func TestApplyFingerprintForJob(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()
	probeDone(t, db, videoID)
	seekDone(t, db, videoID)
	job, err := db.Ingest().ClaimJob(ctx, domain.JobFingerprint)
	if err != nil {
		t.Fatal(err)
	}

	stale := job
	stale.LocationGeneration++
	if applied, err := db.Ingest().ApplyFingerprintForJob(ctx, stale, sampleFingerprint()); err != nil || applied {
		t.Fatalf("古い世代の記録 = %v, %v, want false", applied, err)
	}
	if _, ok := storedFingerprint(t, db, "key-a"); ok {
		t.Fatal("古い世代で指紋を書いた")
	}
	if applied, err := db.Ingest().ApplyFingerprintForJob(ctx, job, sampleFingerprint()); err != nil || !applied {
		t.Fatalf("ApplyFingerprintForJob = %v, %v", applied, err)
	}
	replaced := sampleFingerprint()
	replaced.IntervalMs = 6000
	replaced.Frames = replaced.Frames[:1]
	if applied, err := db.Ingest().ApplyFingerprintForJob(ctx, job, replaced); err != nil || !applied {
		t.Fatalf("置き換え = %v, %v", applied, err)
	}
	got, ok := storedFingerprint(t, db, "key-a")
	if !ok || got.IntervalMs != 6000 || len(got.Frames) != 1 || got.Frames[0].Hash != 1 ||
		!bytes.Equal(got.Encode(), replaced.Encode()) {
		t.Fatalf("指紋 = %+v, %v", got, ok)
	}

	if err := db.ScanIndex().DeleteVideos(ctx, []int64{videoID}); err != nil {
		t.Fatal(err)
	}
	if _, ok := storedFingerprint(t, db, "key-a"); ok {
		t.Fatal("内容の参照が無くなっても指紋が残る")
	}
}

// 上限まで失敗した指紋の仕事は問題として記録し、次の走査が積み直す。指紋ができれば
// 積み直さない。
func TestFailedFingerprintIsRequeuedByNextScan(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()
	probeDone(t, db, videoID)
	if _, err := db.sql.Exec(`update videos set seek_thumbnail_state = 'done' where id = ?`, videoID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.Scans().StartScan(ctx); err != nil {
		t.Fatal(err)
	}
	job := claimAtLastAttempt(t, db, domain.JobFingerprint, videoID)
	if err := db.Ingest().FailClaimedJob(ctx, job, errors.New("no sprite")); err != nil {
		t.Fatal(err)
	}
	if got := jobCounts(t, db, videoID); got["fingerprint:failed"] != 1 {
		t.Fatalf("jobs = %v, want one failed fingerprint", got)
	}
	var issues int
	if err := db.sql.QueryRow(`select count(*) from scan_issues where video_id = ? and kind = 'fingerprint_failed'`,
		videoID).Scan(&issues); err != nil {
		t.Fatal(err)
	}
	if issues != 1 {
		t.Fatalf("fingerprint_failed = %d, want 1", issues)
	}

	missing := func() bool {
		t.Helper()
		indexed, err := db.ScanIndex().IndexedVideosByPath(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return indexed[fixturePath("/media/a.mp4")].FingerprintMissing
	}
	if !missing() {
		t.Fatal("指紋の無い動画が FingerprintMissing にならない")
	}
	if err := db.Ingest().EnsureJob(ctx, domain.JobFingerprint, videoID); err != nil {
		t.Fatal(err)
	}
	if got := jobCounts(t, db, videoID); !equalCounts(got, map[string]int{"fingerprint:queued": 1}) {
		t.Fatalf("jobs = %v, want one queued fingerprint", got)
	}
	if err := db.Ingest().EnsureJob(ctx, domain.JobFingerprint, videoID); err != nil {
		t.Fatal(err)
	}
	if got := jobCounts(t, db, videoID); !equalCounts(got, map[string]int{"fingerprint:queued": 1}) {
		t.Fatalf("2度目の jobs = %v", got)
	}

	job, err := db.Ingest().ClaimJob(ctx, domain.JobFingerprint)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Ingest().ApplyFingerprintForJob(ctx, job, sampleFingerprint()); err != nil {
		t.Fatal(err)
	}
	if err := db.Ingest().CompleteClaimedJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	if err := db.sql.QueryRow(`select count(*) from scan_issues where video_id = ? and kind = 'fingerprint_failed'`,
		videoID).Scan(&issues); err != nil {
		t.Fatal(err)
	}
	if issues != 0 {
		t.Fatalf("成功のあとも fingerprint_failed = %d", issues)
	}
	if missing() {
		t.Fatal("指紋のある動画が FingerprintMissing になる")
	}
	if err := db.Ingest().EnsureJob(ctx, domain.JobFingerprint, videoID); err != nil {
		t.Fatal(err)
	}
	if got := jobCounts(t, db, videoID); !equalCounts(got, map[string]int{"fingerprint:done": 1}) {
		t.Fatalf("指紋のある動画の jobs = %v", got)
	}

	// 旧い版の指紋は無いものとして作り直す。
	if _, err := db.sql.Exec(`update video_fingerprints set version = ?`, domain.FingerprintVersion-1); err != nil {
		t.Fatal(err)
	}
	if !missing() {
		t.Fatal("旧い版の指紋が FingerprintMissing にならない")
	}
	// 完了した行が残っていても、版を上げたら積み直す。
	if err := db.Ingest().EnsureJob(ctx, domain.JobFingerprint, videoID); err != nil {
		t.Fatal(err)
	}
	if got := jobCounts(t, db, videoID); !equalCounts(got, map[string]int{"fingerprint:queued": 1}) {
		t.Fatalf("版を上げたあとの jobs = %v, want one queued fingerprint", got)
	}
}

// シーク用サムネイルを作り直すときは、待っている指紋の仕事を捨てる。作り直しの完了で
// 積み直す。作り直しが上限まで失敗しても、取り出せない仕事が残りに残らない。
func TestRequeueMissingSeekThumbnailsDropsWaitingFingerprint(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()
	probeDone(t, db, videoID)
	seekDone(t, db, videoID)
	if requeued, err := db.Ingest().RequeueMissingSeekThumbnails(ctx, videoID, "key-a"); err != nil || !requeued {
		t.Fatalf("RequeueMissingSeekThumbnails = %v, %v", requeued, err)
	}
	if got := jobCounts(t, db, videoID); got["fingerprint:queued"] != 0 {
		t.Fatalf("jobs = %v, want no waiting fingerprint", got)
	}
	job, err := db.Ingest().ClaimJob(ctx, domain.JobSeekThumbnail)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Ingest().SetSeekThumbnailStateForJob(ctx, job, domain.SeekThumbnailDone, domain.SubstitutionUnknown); err != nil {
		t.Fatal(err)
	}
	if got := jobCounts(t, db, videoID); got["fingerprint:queued"] != 1 {
		t.Fatalf("作り直しの完了のあとの jobs = %v", got)
	}
}

// 00025 は完成したスプライトの登録された動画に指紋の仕事を積み、既存の仕事を残す。Down は
// 指紋の仕事と表を消し、ほかの仕事を戻す。
func TestFingerprintMigrationBackfillsAndRollsBack(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	fsy, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, db.sql, fsy)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := provider.UpTo(ctx, 24); err != nil {
		t.Fatal(err)
	}
	ids := map[string]int64{}
	for _, item := range []struct {
		key, seek string
		located   bool
	}{
		{key: "done", seek: "done", located: true},
		{key: "pending", seek: "pending", located: true},
		{key: "failed", seek: "failed", located: true},
		{key: "unlocated", seek: "done", located: false},
	} {
		res, err := db.sql.Exec(`insert into videos(content_key, probe_state, thumbnail_state, preview_state,
			seek_thumbnail_state) values (?, 'done', 'done', 'done', ?)`, item.key, item.seek)
		if err != nil {
			t.Fatal(err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			t.Fatal(err)
		}
		ids[item.key] = id
		if item.located {
			if _, err := db.sql.Exec(`insert into video_locations
				(video_id, path, title, size_bytes, mtime, created_at, updated_at)
				values (?, ?, ?, 1, 1, 1, 1)`, id, fixturePath("/media/")+item.key+".mp4", item.key); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := db.sql.Exec(`insert into jobs(kind, video_id, state, attempts, last_error, created_at, updated_at)
		values ('seek_thumbnail', ?, 'failed', 3, 'boom', 1, 1)`, ids["failed"]); err != nil {
		t.Fatal(err)
	}

	if _, err := provider.UpTo(ctx, 25); err != nil {
		t.Fatal(err)
	}
	if got := jobCounts(t, db, ids["done"]); !equalCounts(got, map[string]int{"fingerprint:queued": 1}) {
		t.Errorf("done の jobs = %v", got)
	}
	for _, key := range []string{"pending", "unlocated"} {
		if got := jobCounts(t, db, ids[key]); len(got) != 0 {
			t.Errorf("%s の jobs = %v, want none", key, got)
		}
	}
	var lastError string
	if err := db.sql.QueryRow(`select last_error from jobs where video_id = ? and kind = 'seek_thumbnail'`,
		ids["failed"]).Scan(&lastError); err != nil || lastError != "boom" {
		t.Errorf("既存の仕事 = %q, %v", lastError, err)
	}
	if _, err := db.sql.Exec(`insert into video_fingerprints (content_key, version, interval_ms, hashes, updated_at)
		values ('done', 1, 5000, x'', 1)`); err != nil {
		t.Fatal(err)
	}

	if _, err := provider.DownTo(ctx, 24); err != nil {
		t.Fatal(err)
	}
	if got := jobCounts(t, db, ids["done"]); len(got) != 0 {
		t.Errorf("Down のあとの done の jobs = %v", got)
	}
	if got := jobCounts(t, db, ids["failed"]); !equalCounts(got, map[string]int{"seek_thumbnail:failed": 1}) {
		t.Errorf("Down のあとの failed の jobs = %v", got)
	}
	var tables int
	if err := db.sql.QueryRow(`select count(*) from sqlite_master where name = 'video_fingerprints'`).Scan(&tables); err != nil {
		t.Fatal(err)
	}
	if tables != 0 {
		t.Error("Down のあとも video_fingerprints が残る")
	}
	if _, err := db.sql.Exec(`insert into jobs(kind, video_id, state, attempts, created_at, updated_at)
		values ('fingerprint', ?, 'queued', 0, 1, 1)`, ids["done"]); err == nil {
		t.Error("Down のあとも fingerprint の仕事を積める")
	}
}
