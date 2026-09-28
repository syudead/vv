package store

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"testing"

	"github.com/pressly/goose/v3"

	"github.com/syudead/vv/internal/domain"
)

// importIssues は直近の取り込みの問題と、それを数えて決めた状態を返す。internal/app の
// Scans.withImport と同じ組み立てである。
func importIssues(t *testing.T, db *DB) (domain.Scan, []domain.ScanIssue, domain.ImportProgress) {
	t.Helper()
	ctx := context.Background()
	scan, err := db.Scans().CurrentScan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	issues, err := db.Scans().ScanIssues(ctx, scan.ID)
	if err != nil {
		t.Fatal(err)
	}
	counts := domain.CountScanIssues(issues)
	return scan, issues, scan.Tally(counts.Unregistered, counts.Failed).Progress(scan.SettledAt)
}

// failJobToLimit は kind の仕事をやり直しの上限まで失敗させる。
func failJobToLimit(t *testing.T, db *DB, kind domain.JobKind) {
	t.Helper()
	for range domain.MaxJobAttempts {
		failJobOnce(t, db, kind)
	}
}

// failJobOnce は kind の仕事を1回取り出して失敗で記録する。
func failJobOnce(t *testing.T, db *DB, kind domain.JobKind) {
	t.Helper()
	ctx := context.Background()
	job, err := db.Ingest().ClaimJob(ctx, kind)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ingest().FailClaimedJob(ctx, job, errors.New("unreadable")); err != nil {
		t.Fatal(err)
	}
}

// runAllJobs は取り出せる仕事をすべて成功で終える。
func runAllJobs(t *testing.T, db *DB) {
	t.Helper()
	for _, kind := range domain.JobKinds {
		for runOneJob(t, db, kind) {
		}
	}
}

// startImport は走査を始め、名前ごとに動画を登録して解析の仕事を積み、走査を閉じる。
func startImport(t *testing.T, db *DB, names ...string) (int64, []int64) {
	t.Helper()
	scan, _, err := db.Scans().StartScan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]int64, 0, len(names))
	for _, name := range names {
		ids = append(ids, upsertForImport(t, db, name))
	}
	if err := db.Scans().UpdateScanProgress(context.Background(), scan.ID,
		domain.ScanProgress{Total: len(names), Completed: len(names)}); err != nil {
		t.Fatal(err)
	}
	finishScan(t, db, scan.ID)
	return scan.ID, ids
}

// 解析が上限まで失敗した動画は probe_failed の問題になり、取り込みは partial になる。
func TestProbeFailureAtLimitBecomesIssue(t *testing.T) {
	db := migratedDB(t)
	_, ids := startImport(t, db, "a", "b")

	// b の解析は成功させ、a だけを上限まで失敗させる。
	ctx := context.Background()
	for {
		job, err := db.Ingest().ClaimJob(ctx, domain.JobProbe)
		if errors.Is(err, domain.ErrNoJob) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if job.VideoID == ids[0] {
			if err := db.Ingest().FailClaimedJob(ctx, job, errors.New("unreadable")); err != nil {
				t.Fatal(err)
			}
			continue
		}
		probe := domain.Probe{DurationMs: 1000, VideoCodec: "h264", AudioCodec: "aac"}
		if _, err := db.Ingest().ApplyProbeForJob(ctx, job, probe, domain.EvaluatePlayability("mp4", probe)); err != nil {
			t.Fatal(err)
		}
		if err := db.Ingest().CompleteClaimedJob(ctx, job); err != nil {
			t.Fatal(err)
		}
	}
	runAllJobs(t, db)

	scan, issues, progress := importIssues(t, db)
	if len(issues) != 1 || issues[0].VideoID != ids[0] ||
		!slices.Equal(issues[0].Kinds, []domain.ScanIssueKind{domain.IssueProbeFailed}) ||
		issues[0].Severity != domain.IssueFailed || issues[0].FileName != "a.mp4" {
		t.Fatalf("問題 = %+v, want a の probe_failed", issues)
	}
	if progress.Status != domain.ImportPartial || progress.Total != 2 || progress.Settled != 2 {
		t.Fatalf("状態 = %+v, want 2本のうち2本・partial", progress)
	}
	if scan.IssuesRevision == 0 {
		t.Fatal("問題を記録したのに issues_revision が増えていない")
	}
}

// 上限の手前で失敗して後で成功した動画は、問題にならない（受け入れ条件 9）。
func TestFailureBeforeLimitThenSuccessIsNotIssue(t *testing.T) {
	db := migratedDB(t)
	startImport(t, db, "a")
	failJobOnce(t, db, domain.JobProbe)
	failJobOnce(t, db, domain.JobProbe)
	runAllJobs(t, db)

	scan, issues, progress := importIssues(t, db)
	if len(issues) != 0 || scan.IssuesRevision != 0 {
		t.Fatalf("問題 = %+v (revision %d), want なし", issues, scan.IssuesRevision)
	}
	if progress.Status != domain.ImportDone {
		t.Fatalf("状態 = %+v, want done", progress)
	}
}

// 解析のやり直しで成功すると、その問題が消えて取り込みは done になる。
func TestRetryProbeSuccessClearsIssue(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	_, ids := startImport(t, db, "a")
	failJobToLimit(t, db, domain.JobProbe)
	runAllJobs(t, db)
	before, issues, _ := importIssues(t, db)
	if len(issues) != 1 {
		t.Fatalf("準備: 問題 = %+v", issues)
	}

	if err := db.Ingest().RetryProbe(ctx, ids[0]); err != nil {
		t.Fatal(err)
	}
	runAllJobs(t, db)

	after, issues, progress := importIssues(t, db)
	if len(issues) != 0 {
		t.Fatalf("やり直しで成功したのに問題が残った: %+v", issues)
	}
	if progress.Status != domain.ImportDone {
		t.Fatalf("状態 = %+v, want done", progress)
	}
	if after.IssuesRevision <= before.IssuesRevision {
		t.Fatalf("issues_revision = %d → %d, want 増える", before.IssuesRevision, after.IssuesRevision)
	}
}

// 新しい走査を始めると、前の走査の問題は消える。
func TestStartScanClearsPreviousIssues(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	startImport(t, db, "a")
	failJobToLimit(t, db, domain.JobProbe)
	if err := db.Scans().RecordScanIssue(ctx, domain.ScanFileIssue{
		Path: fixturePath("/media/broken.mp4"), Kind: domain.IssueUnreadable,
	}); err != nil {
		t.Fatal(err)
	}
	if _, issues, _ := importIssues(t, db); len(issues) != 2 {
		t.Fatalf("準備: 問題 = %+v", issues)
	}

	if _, _, err := db.Scans().StartScan(ctx); err != nil {
		t.Fatal(err)
	}
	_, issues, _ := importIssues(t, db)
	if len(issues) != 0 {
		t.Fatalf("新しい走査に前の問題が残った: %+v", issues)
	}
	var rows int
	if err := db.sql.QueryRow(`select count(*) from scan_issues`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Fatalf("scan_issues に %d 行残った", rows)
	}
}

// 1本の動画に2つの種類が起きると、1件にまとまる。別の種類が加わっても件数は変わらず、
// issues_revision は増える。
func TestIssuesOfOneVideoAreGroupedAndRevisionAdvances(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	_, ids := startImport(t, db, "a")
	if err := db.Ingest().EnqueueJob(ctx, domain.JobThumbnail, ids[0]); err != nil {
		t.Fatal(err)
	}
	failJobToLimit(t, db, domain.JobProbe)
	first, issues, _ := importIssues(t, db)
	if len(issues) != 1 {
		t.Fatalf("解析の失敗のあと = %+v", issues)
	}
	counts := domain.CountScanIssues(issues)

	failJobToLimit(t, db, domain.JobThumbnail)
	second, issues, progress := importIssues(t, db)
	if len(issues) != 1 || issues[0].VideoID != ids[0] ||
		!slices.Equal(issues[0].Kinds, []domain.ScanIssueKind{domain.IssueProbeFailed, domain.IssueThumbnailFailed}) {
		t.Fatalf("問題 = %+v, want 1件に2つの種類", issues)
	}
	if got := domain.CountScanIssues(issues); got != counts {
		t.Fatalf("本数 = %+v, want 変わらない %+v", got, counts)
	}
	if second.IssuesRevision <= first.IssuesRevision {
		t.Fatalf("issues_revision = %d → %d, want 増える", first.IssuesRevision, second.IssuesRevision)
	}
	if progress.Status != domain.ImportPartial {
		t.Fatalf("状態 = %+v, want partial", progress)
	}
}

// 走査が登録できなかったファイルは、分母と済みの本数に入り、取り込みを partial にする。
// 同じパスと種類を2度記録しても1行である。
func TestUnregisteredFileIssueCountsTowardImport(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	scan, _, err := db.Scans().StartScan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	upsertForImport(t, db, "a")
	broken := domain.ScanFileIssue{Path: fixturePath("/media/sub/broken.mp4"), Kind: domain.IssueUnreadable}
	for range 2 {
		if err := db.Scans().RecordScanIssue(ctx, broken); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Scans().UpdateScanProgress(ctx, scan.ID, domain.ScanProgress{Total: 2, Completed: 1, Failed: 1}); err != nil {
		t.Fatal(err)
	}
	finishScan(t, db, scan.ID)
	runAllJobs(t, db)

	current, issues, progress := importIssues(t, db)
	if len(issues) != 1 || issues[0].VideoID != 0 || issues[0].FileName != "broken.mp4" ||
		issues[0].Folder.Path != "sub" || !issues[0].Unregistered {
		t.Fatalf("問題 = %+v, want 未登録の broken.mp4", issues)
	}
	if progress.Status != domain.ImportPartial || progress.Total != 2 || progress.Settled != 2 {
		t.Fatalf("状態 = %+v, want 2本のうち2本・partial", progress)
	}
	if current.IssuesRevision != 1 {
		t.Fatalf("issues_revision = %d, want 1（2度目は入らない）", current.IssuesRevision)
	}
}

// 所在がどの登録フォルダにも含まれない件は、一覧にも本数にも入れない。メディアフォルダを
// 外すと issues_revision が増え、画面が一覧を読み直せる。
func TestIssuesOutsideMediaFoldersAreHidden(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	startImport(t, db, "a")
	failJobToLimit(t, db, domain.JobProbe)
	if err := db.Scans().RecordScanIssue(ctx, domain.ScanFileIssue{
		Path: fixturePath("/elsewhere/x.mp4"), Kind: domain.IssueUnreadable,
	}); err != nil {
		t.Fatal(err)
	}
	before, issues, _ := importIssues(t, db)
	if len(issues) != 1 || issues[0].FileName != "a.mp4" {
		t.Fatalf("問題 = %+v, want 登録フォルダの中の a.mp4 だけ", issues)
	}

	folders, err := db.Settings().ListMediaFolders(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Settings().DeleteMediaFolder(ctx, folders[0].ID, folders[0].Version); err != nil {
		t.Fatal(err)
	}
	after, issues, progress := importIssues(t, db)
	if len(issues) != 0 || progress.Status == domain.ImportPartial {
		t.Fatalf("外したフォルダの問題 = %+v (%+v), want なし", issues, progress)
	}
	if after.IssuesRevision <= before.IssuesRevision {
		t.Fatalf("issues_revision = %d → %d, want 増える", before.IssuesRevision, after.IssuesRevision)
	}
}

// 問題のある動画の所在だけが変わっても（パスの小さい所在を足す・代表の所在を消す）、
// 一覧に出るファイル名が変わるので issues_revision が増える。
func TestIssueLocationChangeAdvancesRevision(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	_, ids := startImport(t, db, "b")
	failJobToLimit(t, db, domain.JobProbe)
	before, issues, _ := importIssues(t, db)
	if len(issues) != 1 || issues[0].FileName != "b.mp4" {
		t.Fatalf("問題 = %+v, want b.mp4", issues)
	}

	// 同じ内容の、パスの小さい所在を足すと代表が a.mp4 になる。
	added, err := db.ScanIndex().UpsertVideo(ctx, sampleFile(fixturePath("/media/a.mp4"), "a", "key-b", 1024, 0))
	if err != nil {
		t.Fatal(err)
	}
	if added.ID != ids[0] {
		t.Fatalf("足した所在の動画 = %d, want %d", added.ID, ids[0])
	}
	moved, issues, _ := importIssues(t, db)
	if len(issues) != 1 || issues[0].FileName != "a.mp4" {
		t.Fatalf("問題 = %+v, want a.mp4", issues)
	}
	if moved.IssuesRevision <= before.IssuesRevision {
		t.Fatalf("所在を足したあとの issues_revision = %d → %d, want 増える", before.IssuesRevision, moved.IssuesRevision)
	}

	// 代表の所在を消すと、残った b.mp4 に戻る。
	indexed, err := db.ScanIndex().IndexedVideosByPath(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.ScanIndex().DeleteVideoLocations(ctx, []int64{indexed[fixturePath("/media/a.mp4")].LocationID}); err != nil {
		t.Fatal(err)
	}
	after, issues, _ := importIssues(t, db)
	if len(issues) != 1 || issues[0].FileName != "b.mp4" {
		t.Fatalf("問題 = %+v, want b.mp4", issues)
	}
	if after.IssuesRevision <= moved.IssuesRevision {
		t.Fatalf("所在を消したあとの issues_revision = %d → %d, want 増える", moved.IssuesRevision, after.IssuesRevision)
	}
}

// 数千件の問題を、カーソルで重ならずに最後まで辿れる。
func TestThousandsOfIssuesPageWithoutOverlap(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	scan, _, err := db.Scans().StartScan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	const total = 3000
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := range total {
		kind := domain.IssueUnreadable
		if i%3 == 0 {
			kind = domain.IssueThumbnailFirstFrame
		}
		// 同じファイル名を別のフォルダに置き、並びの決め手がフォルダまで及ぶようにする。
		path := fixturePath(fmt.Sprintf("/media/d%d/f%04d.mp4", i%7, i/7))
		if err := recordScanIssue(ctx, tx, 0, path, kind, 1); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	issues, err := db.Scans().ScanIssues(ctx, scan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != total {
		t.Fatalf("件数 = %d, want %d", len(issues), total)
	}
	seen := map[string]bool{}
	var walked []domain.ScanIssue
	cursor := ""
	for pages := 0; ; pages++ {
		if pages > total {
			t.Fatal("カーソルが進まない")
		}
		page, next, err := domain.PageScanIssues(issues, cursor, 200)
		if err != nil {
			t.Fatal(err)
		}
		for _, issue := range page {
			if seen[issue.Path] {
				t.Fatalf("%s を2度返した", issue.Path)
			}
			seen[issue.Path] = true
		}
		walked = append(walked, page...)
		if next == "" {
			break
		}
		cursor = next
	}
	if len(walked) != total {
		t.Fatalf("辿った件数 = %d, want %d", len(walked), total)
	}
	// 失敗が先、同じ重さの中はファイル名の順である。
	for i := 1; i < len(walked); i++ {
		a, b := walked[i-1], walked[i]
		if a.Severity == domain.IssueSubstituted && b.Severity == domain.IssueFailed {
			t.Fatalf("代用のあとに失敗が来た: %s, %s", a.Path, b.Path)
		}
		if a.Severity == b.Severity && a.FileName > b.FileName {
			t.Fatalf("ファイル名の順でない: %s, %s", a.FileName, b.FileName)
		}
	}
}

// 解析に失敗した動画がある状態から移行すると、その動画が probe_failed の問題になり、
// 直近の取り込みは partial になる（research.md R-11）。
func TestScanIssuesMigrationCarriesFailedVideos(t *testing.T) {
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
	if _, err := provider.UpTo(ctx, 18); err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`insert into media_folders(path, version, created_at, updated_at) values ('` + fixturePath("/media") + `', 1, 1, 1)`,
		`insert into videos(id, content_key, probe_state, added_at, updated_at) values (1, 'key-a', 'failed', 1, 1)`,
		`insert into videos(id, content_key, probe_state, added_at, updated_at) values (2, 'key-b', 'done', 1, 1)`,
		`insert into video_locations(video_id, path, title, size_bytes, mtime, created_at, updated_at)
			values (1, '` + fixturePath("/media/a.mp4") + `', 'a', 1, 1, 1, 1)`,
		`insert into video_locations(video_id, path, title, size_bytes, mtime, created_at, updated_at)
			values (2, '` + fixturePath("/media/b.mp4") + `', 'b', 1, 1, 1, 1)`,
		`insert into jobs(kind, video_id, state, attempts, created_at, updated_at) values ('probe', 1, 'failed', 3, 1, 1)`,
		`insert into scans(id, state, started_at, finished_at, total, completed, failed, settled_at)
			values (1, 'done', 1, 5, 2, 2, 0, 5)`,
	} {
		if _, err := db.sql.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}

	if _, err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}

	scan, issues, progress := importIssues(t, db)
	if len(issues) != 1 || issues[0].VideoID != 1 ||
		!slices.Equal(issues[0].Kinds, []domain.ScanIssueKind{domain.IssueProbeFailed}) {
		t.Fatalf("問題 = %+v, want 動画1の probe_failed", issues)
	}
	if progress.Status != domain.ImportPartial {
		t.Fatalf("移行直後 = %+v, want partial", progress)
	}
	if scan.IssuesRevision == 0 {
		t.Fatal("移行で問題を入れたのに issues_revision が 0 のまま")
	}

	downTo(t, db, 18)
	if _, ok := tableColumns(t, db, "scan_issues")["kind"]; ok {
		t.Error("Down 後も scan_issues が残っている")
	}
}
