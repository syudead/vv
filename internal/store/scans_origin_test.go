package store

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// 監視の走査（specs/042-folder-watch-import/data-model.md の Rules）。

func startTestWatchScan(t *testing.T, db *DB, dirs ...domain.DirtyDirectory) domain.Scan {
	t.Helper()
	scan, started, err := db.Scans().StartWatchScan(context.Background(), dirs)
	if err != nil || !started {
		t.Fatalf("StartWatchScan = started %v, err %v", started, err)
	}
	return scan
}

func issuePaths(issues []domain.ScanIssue) []string {
	paths := make([]string, 0, len(issues))
	for _, issue := range issues {
		paths = append(paths, issue.Path)
	}
	slices.Sort(paths)
	return paths
}

// 走査の起点は行に残り、既存の行と手動の走査は manual である。
func TestScanOriginIsRecorded(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	manual, _, err := db.Scans().StartScan(ctx)
	if err != nil || manual.Origin != domain.ScanOriginManual {
		t.Fatalf("手動の走査 = %+v, err %v, want origin manual", manual, err)
	}
	finishScan(t, db, manual.ID)

	watch := startTestWatchScan(t, db, domain.DirtyDirectory{Path: fixturePath("/media")})
	if watch.Origin != domain.ScanOriginWatch {
		t.Fatalf("監視の走査の origin = %q, want watch", watch.Origin)
	}
	current, err := db.Scans().CurrentScan(ctx)
	if err != nil || current.ID != watch.ID || current.Origin != domain.ScanOriginWatch {
		t.Fatalf("CurrentScan = %+v, err %v", current, err)
	}

	// 走査は同時に1つだけで、監視の走査が走っていれば手動の走査は新しく始まらない。
	again, started, err := db.Scans().StartScan(ctx)
	if err != nil || started || again.ID != watch.ID {
		t.Fatalf("StartScan = #%d started %v err %v, want 走っている監視の走査", again.ID, started, err)
	}
}

// 監視の走査は、前の走査の問題のうち読み直さない範囲のものを持ち越し、読み直す範囲の、走査が
// 見つける種類の問題は消す。仕事の段階の問題は、走査が見つけ直せないので範囲の中でも持ち越す。
func TestWatchScanCarriesIssuesOutsideItsScope(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	_, ids := startImport(t, db, "a")
	failJobToLimit(t, db, domain.JobProbe)
	for _, path := range []string{"/media/in/x.mp4", "/media/out/y.mp4"} {
		if err := db.Scans().RecordScanIssue(ctx, domain.ScanFileIssue{
			Path: fixturePath(path), Kind: domain.IssueUnreadable,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, issues, _ := importIssues(t, db); len(issues) != 3 {
		t.Fatalf("準備: 問題 = %+v", issues)
	}
	watch := startTestWatchScan(t, db,
		domain.DirtyDirectory{Path: fixturePath("/media/in"), Recursive: false},
		domain.DirtyDirectory{Path: fixturePath("/media"), Recursive: false})
	got, issues, _ := importIssues(t, db)
	if got.ID != watch.ID {
		t.Fatalf("CurrentScan = #%d, want #%d", got.ID, watch.ID)
	}
	want := []string{fixturePath("/media/a.mp4"), fixturePath("/media/out/y.mp4")}
	if paths := issuePaths(issues); !slices.Equal(paths, want) {
		t.Fatalf("持ち越した問題 = %v, want %v", paths, want)
	}
	for _, issue := range issues {
		if issue.Path == fixturePath("/media/a.mp4") && issue.VideoID != ids[0] {
			t.Errorf("段階の問題の動画 = %d, want %d", issue.VideoID, ids[0])
		}
	}
	var stale int
	if err := db.sql.QueryRow(`select count(*) from scan_issues where scan_id <> ?`, watch.ID).Scan(&stale); err != nil {
		t.Fatal(err)
	}
	if stale != 0 {
		t.Fatalf("前の走査の問題が %d 行残った", stale)
	}

	// 手動の走査は、これまでどおりすべて消す。
	finishScan(t, db, watch.ID)
	if _, _, err := db.Scans().StartScan(ctx); err != nil {
		t.Fatal(err)
	}
	if _, issues, _ := importIssues(t, db); len(issues) != 0 {
		t.Fatalf("手動の走査に問題が残った: %+v", issues)
	}
}

// done で閉じた監視の走査は、保留中の後継の記録を判定してよい状態にしない。次に done で閉じる
// 手動の走査が判定する（research.md R-9）。
func TestDoneWatchScanLeavesSuccessionsUnready(t *testing.T) {
	db, _ := taggedVideoFixture(t)
	watch := startTestWatchScan(t, db, domain.DirtyDirectory{Path: fixturePath("/media")})
	b := upsertOne(t, db, listingFile(fixturePath("/media/a.mp4"), "a", "key-b", 2))
	probeDuration(t, db, b, 100_000)

	finishTestScan(t, db, watch.ID, domain.ScanDone)
	var ready int
	if err := db.sql.QueryRow(`select count(*) from video_successions where ready = 1`).Scan(&ready); err != nil {
		t.Fatal(err)
	}
	if ready != 0 || successionCount(t, db) != 1 {
		t.Fatalf("監視の走査が後継を判定した: ready %d, 記録 %d", ready, successionCount(t, db))
	}
	if tags := manualTagNames(t, db, ownerVideo(t, db, b).UserKey); len(tags) != 0 {
		t.Fatalf("監視の走査が引き継いだ: %v", tags)
	}

	manual := startTestScan(t, db)
	finishTestScan(t, db, manual, domain.ScanDone)
	if tags := manualTagNames(t, db, ownerVideo(t, db, b).UserKey); !slices.Equal(tags, []string{"好き"}) {
		t.Fatalf("手動の走査が引き継がなかった: %v", tags)
	}
}

// 起動時に、中断した監視の走査は origin を保ったまま failed（interrupted）で閉じる。
// 再開するかは、アプリ層が origin を見て決める。
func TestInterruptedWatchScanKeepsItsOrigin(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	startTestWatchScan(t, db, domain.DirtyDirectory{Path: fixturePath("/media")})

	if closed, err := db.Scans().FailInterruptedScans(ctx); err != nil || closed != 1 {
		t.Fatalf("FailInterruptedScans = %d, %v", closed, err)
	}
	got, err := db.Scans().CurrentScan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != domain.ScanFailed || got.ErrorCode != domain.ScanErrorInterrupted || got.Origin != domain.ScanOriginWatch {
		t.Fatalf("閉じた走査 = %+v", got)
	}
}

// フォルダが無ければ、監視の走査も始まらない。
func TestStartWatchScanRejectsEmptyFolderSet(t *testing.T) {
	db := migratedDB(t)
	if _, err := db.sql.Exec(`delete from media_folders`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.Scans().StartWatchScan(context.Background(), nil); !errors.Is(err, domain.ErrNoMediaFolders) {
		t.Fatalf("error = %v, want domain.ErrNoMediaFolders", err)
	}
}
