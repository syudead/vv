package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"testing"

	"github.com/pressly/goose/v3"

	"github.com/syudead/vv/internal/domain"
)

// currentImport は直近の走査と、そこから決まる本数を返す。ここでの検査は問題を
// 起こさないので、問題の数は 0 とする（問題を含む組み立ては importIssues）。
func currentImport(t *testing.T, db *DB) (domain.Scan, domain.ImportProgress) {
	t.Helper()
	scan, err := db.Scans().CurrentScan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return scan, scan.Tally(0, 0).Progress(scan.SettledAt)
}

// scanVideoRows は scan_videos の行を scan_id ごとに数える。
func scanVideoRows(t *testing.T, db *DB) map[int64]int {
	t.Helper()
	rows, err := db.sql.Query(`select scan_id, count(*) from scan_videos group by scan_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	out := map[int64]int{}
	for rows.Next() {
		var id int64
		var count int
		if err := rows.Scan(&id, &count); err != nil {
			t.Fatal(err)
		}
		out[id] = count
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// upsertForImport は動画を登録し、走査と同じく解析の仕事を積む。
func upsertForImport(t *testing.T, db *DB, name string) int64 {
	t.Helper()
	ctx := context.Background()
	video, err := db.ScanIndex().UpsertVideo(ctx, sampleFile(fixturePath("/media/"+name+".mp4"), name, "key-"+name, 1024, 0))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ingest().EnqueueJob(ctx, domain.JobProbe, video.ID); err != nil {
		t.Fatal(err)
	}
	return video.ID
}

// runOneJob は kind の仕事を1件取り出して成功で記録する。解析は結果を書き、同じ取引で
// プレビューの仕事が積まれる。取り出せる仕事が無ければ false を返す。
func runOneJob(t *testing.T, db *DB, kind domain.JobKind) bool {
	t.Helper()
	ctx := context.Background()
	job, err := db.Ingest().ClaimJob(ctx, kind)
	if errors.Is(err, domain.ErrNoJob) {
		return false
	}
	if err != nil {
		t.Fatal(err)
	}
	if kind == domain.JobProbe {
		probe := domain.Probe{DurationMs: 100_000, VideoCodec: "h264", AudioCodec: "aac"}
		if applied, err := db.Ingest().ApplyProbeForJob(ctx, job, probe, domain.EvaluatePlayability("mp4", probe)); err != nil || !applied {
			t.Fatalf("ApplyProbeForJob = %v, %v", applied, err)
		}
	}
	if err := db.Ingest().CompleteClaimedJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	return true
}

func finishScan(t *testing.T, db *DB, id int64) {
	t.Helper()
	if err := db.Scans().FinishScan(context.Background(), id, domain.ScanDone, nil); err != nil {
		t.Fatal(err)
	}
}

// 10本の新しいファイルを登録すると対象は10本になり、仕事を順に終えると済みの本数は
// 減ることなく10本まで増える。解析のあとにプレビューが積まれるあいだも減らない。
func TestImportTenNewFilesSettleWithoutGoingBack(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	scan, _, err := db.Scans().StartScan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for i := range 10 {
		id := upsertForImport(t, db, fmt.Sprintf("v%02d", i))
		if err := db.Ingest().EnqueueJob(ctx, domain.JobThumbnail, id); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Scans().UpdateScanProgress(ctx, scan.ID, domain.ScanProgress{Total: 10, Completed: 10}); err != nil {
		t.Fatal(err)
	}
	finishScan(t, db, scan.ID)

	closed, progress := currentImport(t, db)
	if progress.Total != 10 || progress.Settled != 0 || progress.Status != domain.ImportRunning {
		t.Fatalf("閉じた直後 = %+v, want 10本のうち0本・running", progress)
	}
	if !closed.SettledAt.IsZero() {
		t.Fatalf("残りがあるのに完了の時刻が入った: %v", closed.SettledAt)
	}

	last := 0
	for _, kind := range []domain.JobKind{domain.JobProbe, domain.JobThumbnail, domain.JobPreview} {
		for runOneJob(t, db, kind) {
			_, progress := currentImport(t, db)
			if progress.Total != 10 {
				t.Fatalf("分母が変わった: %+v", progress)
			}
			if progress.Settled < last {
				t.Fatalf("済みの本数が %d から %d へ減った", last, progress.Settled)
			}
			last = progress.Settled
		}
	}
	scan, progress = currentImport(t, db)
	if progress.Settled != 10 || progress.Status != domain.ImportDone {
		t.Fatalf("すべて終えたあと = %+v, want 10本のうち10本・done", progress)
	}
	if scan.SettledAt.IsZero() || scan.SettledAt.Before(scan.FinishedAt) {
		t.Fatalf("settled_at = %v, finished_at = %v, want 走査の終了以後", scan.SettledAt, scan.FinishedAt)
	}
}

// 解析の結果とプレビューの仕事は同じ取引で書かれるので、その間に済みに数えられない。
func TestImportProbeKeepsVideoUnsettledUntilPreview(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	scan, _, err := db.Scans().StartScan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	videoID := upsertForImport(t, db, "a")
	finishScan(t, db, scan.ID)

	job, err := db.Ingest().ClaimJob(ctx, domain.JobProbe)
	if err != nil {
		t.Fatal(err)
	}
	probe := domain.Probe{DurationMs: 100_000, VideoCodec: "h264", AudioCodec: "aac"}
	if _, err := db.Ingest().ApplyProbeForJob(ctx, job, probe, domain.EvaluatePlayability("mp4", probe)); err != nil {
		t.Fatal(err)
	}
	if got := countJobsFor(t, db, domain.JobPreview, videoID); got != 1 {
		t.Fatalf("解析の結果と同じ取引でプレビューが積まれない: %d", got)
	}
	if _, progress := currentImport(t, db); progress.Settled != 0 {
		t.Fatalf("解析の結果を書いた時点で済みに数えた: %+v", progress)
	}
	if err := db.Ingest().CompleteClaimedJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	if scan, progress := currentImport(t, db); progress.Settled != 0 || !scan.SettledAt.IsZero() {
		t.Fatalf("プレビューが残るのに済みに数えた: %+v (%v)", progress, scan.SettledAt)
	}
	if !runOneJob(t, db, domain.JobPreview) {
		t.Fatal("プレビューの仕事を取り出せない")
	}
	if _, progress := currentImport(t, db); progress.Settled != 1 || progress.Status != domain.ImportDone {
		t.Fatalf("プレビューのあと = %+v, want 1本のうち1本・done", progress)
	}
}

func countJobsFor(t *testing.T, db *DB, kind domain.JobKind, videoID int64) int {
	t.Helper()
	var count int
	if err := db.sql.QueryRow(`select count(*) from jobs where kind = ? and video_id = ? and state in ('queued', 'running')`,
		string(kind), videoID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

// 変化の無い動画に仕事が積み直されると、その動画が対象に数えられる。
func TestImportCountsUnchangedVideoWithRestoredJob(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	video, err := db.ScanIndex().UpsertVideo(ctx, sampleFile(fixturePath("/media/a.mp4"), "a", "key-a", 1024, 0))
	if err != nil {
		t.Fatal(err)
	}
	scan, _, err := db.Scans().StartScan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, progress := currentImport(t, db); progress.Counted {
		t.Fatalf("列挙中に本数を示した: %+v", progress)
	}
	if err := db.Ingest().EnsureJob(ctx, domain.JobProbe, video.ID); err != nil {
		t.Fatal(err)
	}
	finishScan(t, db, scan.ID)
	if _, progress := currentImport(t, db); progress.Total != 1 || progress.Settled != 0 {
		t.Fatalf("積み直した動画 = %+v, want 1本のうち0本", progress)
	}
}

// 前回の未完了の動画は新しい走査へ持ち越され、前の走査の集合は消える。
func TestImportCarriesUnfinishedVideosIntoNextScan(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	first, _, err := db.Scans().StartScan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	upsertForImport(t, db, "a")
	unfinished := upsertForImport(t, db, "b")
	finishScan(t, db, first.ID)
	// 1本だけ解析を終える（プレビューは残る）ので、残りの仕事を持つ動画は2本になる。
	// どちらも持ち越されることを、片方のプレビューまで終えて確かめる。
	if !runOneJob(t, db, domain.JobProbe) || !runOneJob(t, db, domain.JobPreview) {
		t.Fatal("仕事を進められない")
	}
	if _, progress := currentImport(t, db); progress.Total != 2 || progress.Settled != 1 {
		t.Fatalf("前の取り込み = %+v, want 2本のうち1本", progress)
	}

	second, _, err := db.Scans().StartScan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Scans().UpdateScanProgress(ctx, second.ID, domain.ScanProgress{}); err != nil {
		t.Fatal(err)
	}
	finishScan(t, db, second.ID)
	rows := scanVideoRows(t, db)
	if rows[first.ID] != 0 {
		t.Fatalf("前の走査の集合が残った: %v", rows)
	}
	if rows[second.ID] != 1 {
		t.Fatalf("新しい走査の集合 = %v, want 未完了の1本", rows)
	}
	var carried int64
	if err := db.sql.QueryRow(`select video_id from scan_videos where scan_id = ?`, second.ID).Scan(&carried); err != nil {
		t.Fatal(err)
	}
	if carried != unfinished {
		t.Fatalf("持ち越した動画 = %d, want %d", carried, unfinished)
	}
}

// settleImport は走査を1つ閉じ、すべての仕事を終えて完了の時刻を入れる。
func settleImport(t *testing.T, db *DB, names ...string) []int64 {
	t.Helper()
	ctx := context.Background()
	scan, _, err := db.Scans().StartScan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var ids []int64
	for _, name := range names {
		ids = append(ids, upsertForImport(t, db, name))
	}
	finishScan(t, db, scan.ID)
	for runOneJob(t, db, domain.JobProbe) {
	}
	for runOneJob(t, db, domain.JobPreview) {
	}
	if settled, _ := currentImport(t, db); settled.SettledAt.IsZero() {
		t.Fatal("準備: 完了の時刻が入らない")
	}
	return ids
}

// 見つからないプレビューの積み直しと解析のやり直しで、その動画は直近の走査の対象に
// 加わり、完了の時刻が消える。
func TestImportRequeueAndRetryReopenCurrentImport(t *testing.T) {
	t.Run("プレビューの積み直し", func(t *testing.T) {
		ctx := context.Background()
		db := migratedDB(t)
		ids := settleImport(t, db, "a")
		// 集合から外した状態から、積み直しで加わることを確かめる。
		if _, err := db.sql.Exec(`delete from scan_videos`); err != nil {
			t.Fatal(err)
		}
		if _, err := db.sql.Exec(`update videos set preview_state = 'done' where id = ?`, ids[0]); err != nil {
			t.Fatal(err)
		}
		if requeued, err := db.Ingest().RequeueMissingPreview(ctx, ids[0], "key-a"); err != nil || !requeued {
			t.Fatalf("RequeueMissingPreview = %v, %v", requeued, err)
		}
		scan, progress := currentImport(t, db)
		if progress.Total != 1 || progress.Settled != 0 || progress.Status != domain.ImportRunning {
			t.Fatalf("積み直したあと = %+v, want 1本のうち0本・running", progress)
		}
		if !scan.SettledAt.IsZero() {
			t.Fatalf("完了の時刻が消えない: %v", scan.SettledAt)
		}
	})
	t.Run("解析のやり直し", func(t *testing.T) {
		ctx := context.Background()
		db := migratedDB(t)
		ids := settleImport(t, db, "a")
		if _, err := db.sql.Exec(`delete from scan_videos`); err != nil {
			t.Fatal(err)
		}
		if _, err := db.sql.Exec(`update videos set probe_state = 'failed', playable = 0 where id = ?`, ids[0]); err != nil {
			t.Fatal(err)
		}
		if err := db.Ingest().RetryProbe(ctx, ids[0]); err != nil {
			t.Fatal(err)
		}
		scan, progress := currentImport(t, db)
		if progress.Total != 1 || progress.Settled != 0 || !scan.SettledAt.IsZero() {
			t.Fatalf("やり直したあと = %+v (%v), want 1本のうち0本・時刻なし", progress, scan.SettledAt)
		}
	})
}

// 閉じた走査の残りの仕事がメディアフォルダの削除で着手できなくなると、完了の時刻が入る。
func TestImportSettlesWhenMediaFolderRemovalLeavesNoClaimableJobs(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	extra, err := db.Settings().AddMediaFolder(ctx, fixturePath("/extra"))
	if err != nil {
		t.Fatal(err)
	}
	scan, _, err := db.Scans().StartScan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// 動画は登録外になる所在（/elsewhere）も持つので、フォルダを外しても行は残る。
	video, err := db.ScanIndex().UpsertVideo(ctx, sampleFile(fixturePath("/extra/a.mp4"), "a", "key-a", 1024, 0))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ScanIndex().UpsertVideo(ctx, sampleFile(fixturePath("/elsewhere/a.mp4"), "a", "key-a", 1024, 0)); err != nil {
		t.Fatal(err)
	}
	if err := db.Ingest().EnqueueJob(ctx, domain.JobProbe, video.ID); err != nil {
		t.Fatal(err)
	}
	finishScan(t, db, scan.ID)
	if closed, progress := currentImport(t, db); !closed.SettledAt.IsZero() || progress.Status != domain.ImportRunning {
		t.Fatalf("準備: 残りがあるのに済んだ: %+v (%v)", progress, closed.SettledAt)
	}

	if err := db.Settings().DeleteMediaFolder(ctx, extra.ID, extra.Version); err != nil {
		t.Fatal(err)
	}
	settled, progress := currentImport(t, db)
	if settled.SettledAt.IsZero() || progress.Status != domain.ImportDone {
		t.Fatalf("フォルダを外したあと = %+v (%v), want done・時刻つき", progress, settled.SettledAt)
	}
	if progress.Total != 1 || progress.Settled != 1 {
		t.Fatalf("本数 = %+v, want 1本のうち1本（行は残る）", progress)
	}
}

// 対象の動画の行が消えると、分母から除かれる。
func TestImportDropsDeletedVideosFromTotal(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	scan, _, err := db.Scans().StartScan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	gone := upsertForImport(t, db, "a")
	upsertForImport(t, db, "b")
	finishScan(t, db, scan.ID)
	if _, progress := currentImport(t, db); progress.Total != 2 {
		t.Fatalf("準備: 本数 = %+v, want 2", progress)
	}
	if err := db.ScanIndex().DeleteVideos(ctx, []int64{gone}); err != nil {
		t.Fatal(err)
	}
	if _, progress := currentImport(t, db); progress.Total != 1 || progress.Settled != 0 {
		t.Fatalf("消したあと = %+v, want 1本のうち0本", progress)
	}
}

// running の仕事を積み直して再起動しても、済みの本数は二重に数えない。
func TestImportDoesNotDoubleCountRequeuedRunningJobs(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	scan, _, err := db.Scans().StartScan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	upsertForImport(t, db, "a")
	finishScan(t, db, scan.ID)
	if _, err := db.Ingest().ClaimJob(ctx, domain.JobProbe); err != nil {
		t.Fatal(err)
	}
	if restored, err := db.Ingest().RequeueRunningJobs(ctx); err != nil || restored != 1 {
		t.Fatalf("RequeueRunningJobs = %d, %v", restored, err)
	}
	if _, progress := currentImport(t, db); progress.Total != 1 || progress.Settled != 0 {
		t.Fatalf("積み直したあと = %+v, want 1本のうち0本", progress)
	}
	for runOneJob(t, db, domain.JobProbe) {
	}
	for runOneJob(t, db, domain.JobPreview) {
	}
	if _, progress := currentImport(t, db); progress.Total != 1 || progress.Settled != 1 {
		t.Fatalf("終えたあと = %+v, want 1本のうち1本", progress)
	}
	if rows := scanVideoRows(t, db); rows[scan.ID] != 1 {
		t.Fatalf("集合の行 = %v, want 1", rows)
	}
}

// 未完了の仕事がある状態から移行すると、その動画が直近の走査の対象に入り、完了の時刻は
// null のままになる。前の走査の完了の時刻は、閉じた時刻になる（data-model.md §1）。
func TestScanImportMigrationCarriesUnfinishedJobs(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	fsys, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, db.sql, fsys)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := provider.UpTo(ctx, 16); err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`insert into media_folders(path, version, created_at, updated_at) values ('` + fixturePath("/media") + `', 1, 1, 1)`,
		`insert into videos(id, content_key, probe_state, added_at, updated_at) values (1, 'key-a', 'pending', 1, 1)`,
		`insert into videos(id, content_key, probe_state, added_at, updated_at) values (2, 'key-b', 'done', 1, 1)`,
		`insert into video_locations(video_id, path, title, size_bytes, mtime, created_at, updated_at)
			values (1, '` + fixturePath("/media/a.mp4") + `', 'a', 1, 1, 1, 1)`,
		`insert into video_locations(video_id, path, title, size_bytes, mtime, created_at, updated_at)
			values (2, '` + fixturePath("/media/b.mp4") + `', 'b', 1, 1, 1, 1)`,
		`insert into jobs(kind, video_id, state, attempts, created_at, updated_at) values ('probe', 1, 'queued', 0, 1, 1)`,
		`insert into jobs(kind, video_id, state, attempts, created_at, updated_at) values ('probe', 2, 'done', 1, 1, 1)`,
		`insert into scans(id, state, started_at, finished_at, total, completed, failed) values (1, 'done', 1, 5, 0, 0, 0)`,
		`insert into scans(id, state, started_at, finished_at, total, completed, failed) values (2, 'done', 6, 9, 1, 1, 0)`,
	} {
		if _, err := db.sql.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}

	if _, err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}

	if rows := scanVideoRows(t, db); len(rows) != 1 || rows[2] != 1 {
		t.Fatalf("scan_videos = %v, want 直近の走査に未完了の1本", rows)
	}
	var older, latest sql.NullInt64
	if err := db.sql.QueryRow(`select settled_at from scans where id = 1`).Scan(&older); err != nil {
		t.Fatal(err)
	}
	if err := db.sql.QueryRow(`select settled_at from scans where id = 2`).Scan(&latest); err != nil {
		t.Fatal(err)
	}
	if !older.Valid || older.Int64 != 5 {
		t.Fatalf("前の走査の settled_at = %v, want 5（閉じた時刻）", older)
	}
	if latest.Valid {
		t.Fatalf("直近の走査の settled_at = %v, want null", latest)
	}
	scan, err := db.Scans().CurrentScan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if progress := scan.Tally(0, 0).Progress(scan.SettledAt); progress.Status != domain.ImportRunning || progress.Total != 1 {
		t.Fatalf("移行直後 = %+v, want 1本のうち0本・running", progress)
	}

	downTo(t, db, 16)
	if _, ok := tableColumns(t, db, "scans")["settled_at"]; ok {
		t.Error("Down 後も scans に settled_at 列が残っている")
	}
}

// 移行が登録外の所在の動画を対象に入れて完了の時刻を null にしても、起動時の積み直しが
// 実行時の条件で決め直し、閉じた走査に完了の時刻が入る。戻す仕事が無くても決め直す。
func TestRequeueRunningJobsSettlesImportLeftOpenByMigration(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	for _, stmt := range []string{
		`insert into videos(id, content_key, probe_state, added_at, updated_at) values (1, 'key-a', 'pending', 1, 1)`,
		`insert into video_locations(video_id, path, title, size_bytes, mtime, created_at, updated_at)
			values (1, '` + fixturePath("/elsewhere/a.mp4") + `', 'a', 1, 1, 1, 1)`,
		`insert into jobs(kind, video_id, state, attempts, created_at, updated_at) values ('probe', 1, 'queued', 0, 1, 1)`,
		`insert into scans(id, state, started_at, finished_at, total, completed, failed) values (1, 'done', 1, 5, 1, 1, 0)`,
		`insert into scan_videos(scan_id, video_id) values (1, 1)`,
	} {
		if _, err := db.sql.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	if scan, _ := currentImport(t, db); !scan.SettledAt.IsZero() {
		t.Fatalf("準備: settled_at = %v, want null", scan.SettledAt)
	}

	if restored, err := db.Ingest().RequeueRunningJobs(ctx); err != nil || restored != 0 {
		t.Fatalf("RequeueRunningJobs = %d, %v", restored, err)
	}
	scan, progress := currentImport(t, db)
	if scan.SettledAt.IsZero() || progress.Status != domain.ImportDone {
		t.Fatalf("起動後 = %+v (%v), want done・時刻つき", progress, scan.SettledAt)
	}
}
