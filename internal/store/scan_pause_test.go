package store

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

const scanPausedVersion = 100

// The rebuild of scans keeps every row with its id, and the scan_videos and scan_issues rows that cascade from it;
// the rebuilt table accepts paused and still allows one running scan only.
func TestScanPausedMigrationKeepsScansWithTheirVideosAndIssues(t *testing.T) {
	db, provider := migrateTo(t, scanPausedVersion-1)
	ctx := context.Background()
	execAll(t, db,
		`insert into videos (id, content_key, probe_state, added_at, indexed_at) values (1, 'key-a', 'done', 1, 1)`,
		`insert into scans (id, state, origin, started_at, finished_at, total, completed, failed, error_code,
			rescan_scope, rescan_folder_id, rescan_path)
			values (3, 'failed', 'manual', 1, 2, 5, 4, 1, 'interrupted', 'folder', 7, '/media')`,
		`insert into scans (id, state, origin, started_at, total, completed, failed) values (4, 'running', 'watch', 3, 2, 1, 0)`,
		`insert into scan_videos (scan_id, video_id, forced) values (3, 1, 1), (4, 1, 0)`,
		`insert into scan_issues (id, scan_id, video_id, path, kind, created_at)
			values (10, 3, 1, '/media/a.mp4', 'probe_failed', 5), (11, 4, null, '/media/b.mp4', 'unreadable', 6)`)

	if _, err := provider.UpTo(ctx, scanPausedVersion); err != nil {
		t.Fatal(err)
	}

	wantScans := [][]any{
		{int64(3), "failed", "manual", int64(5), int64(4), int64(1), "interrupted", "folder", int64(7), "/media"},
		{int64(4), "running", "watch", int64(2), int64(1), int64(0), nil, nil, nil, nil},
	}
	if got := queryRows(t, db.sql, `select id, state, origin, total, completed, failed, error_code, rescan_scope,
		rescan_folder_id, rescan_path from scans order by id`, 10); !reflect.DeepEqual(got, wantScans) {
		t.Fatalf("scans = %v, want %v", got, wantScans)
	}
	wantVideos := [][]any{{int64(3), int64(1), int64(1)}, {int64(4), int64(1), int64(0)}}
	if got := queryRows(t, db.sql, `select scan_id, video_id, forced from scan_videos order by scan_id`,
		3); !reflect.DeepEqual(got, wantVideos) {
		t.Fatalf("scan_videos = %v, want %v", got, wantVideos)
	}
	wantIssues := [][]any{{int64(10), int64(3)}, {int64(11), int64(4)}}
	if got := queryRows(t, db.sql, `select id, scan_id from scan_issues order by id`, 2); !reflect.DeepEqual(got,
		wantIssues) {
		t.Fatalf("scan_issues = %v, want %v", got, wantIssues)
	}
	execAll(t, db, `insert into scans (id, state, origin, started_at) values (5, 'paused', 'manual', 4)`,
		`insert into held_watch_directories (path, recursive) values ('/media/new', 1)`)
	if _, err := db.sql.Exec(`insert into scans (state, started_at) values ('running', 5)`); err == nil {
		t.Fatal("a second running scan was accepted")
	}
}

// startPausedFolderRescan leaves a folder force rescan of /media paused with one forced video, a stage issue, a
// scan-sourced issue inside its scope and one carried from /other outside it. It returns the scan and the videos.
func startPausedFolderRescan(t *testing.T, db *DB) (domain.Scan, []int64) {
	t.Helper()
	ctx := context.Background()
	addOtherMediaFolder(t, db)
	_, ids := startImport(t, db, "a", "b")
	recordIssue(t, db, 0, "/other/y.mp4")
	scan := startTestRescan(t, db, domain.RescanScope{
		Kind: domain.RescanFolder, FolderID: mediaFolderID(t, db, fixturePath("/media")),
	})
	if err := db.Scans().MarkForced(ctx, scan.ID, ids[0]); err != nil {
		t.Fatal(err)
	}
	if err := db.ScanIndex().MarkLocationRead(ctx, scan.ID, fixturePath("/media/a.mp4")); err != nil {
		t.Fatal(err)
	}
	failJobToLimit(t, db, domain.JobProbe) // a stage issue on a
	recordIssue(t, db, ids[1], "/media/b.mp4")
	if err := db.Scans().UpdateScanProgress(ctx, scan.ID, domain.ScanProgress{Total: 3, Completed: 1, Failed: 1}); err != nil {
		t.Fatal(err)
	}
	if err := db.Scans().PauseScan(ctx, scan.ID); err != nil {
		t.Fatal(err)
	}
	return scan, ids
}

// A paused scan keeps its counters and stays unfinished; reopening it keeps its video set, forced marks, read
// locations and stage issues, deletes the scan-sourced issues inside its scope and keeps those carried from outside it. A file that fails
// again after the resume has one issue.
func TestReopenScanKeepsThePausedScansWork(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	scan, ids := startPausedFolderRescan(t, db)
	paused, err := db.Scans().CurrentScan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if paused.ID != scan.ID || paused.State != domain.ScanPaused || !paused.FinishedAt.IsZero() ||
		paused.Completed != 1 {
		t.Fatalf("paused = #%d %q finished %v completed %d, want #%d paused, unfinished, 1 completed",
			paused.ID, paused.State, paused.FinishedAt, paused.Completed, scan.ID)
	}
	issuesBefore := currentIssuePaths(t, db)

	reopened, err := db.Scans().ReopenScan(ctx, scan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.ID != scan.ID || reopened.State != domain.ScanRunning || reopened.Completed != 1 ||
		reopened.Rescan == nil || *reopened.Rescan != *scan.Rescan || reopened.Videos != paused.Videos {
		t.Fatalf("reopened = %+v, want #%d running with the paused scan's scope, counters and videos", reopened, scan.ID)
	}
	if forced, err := db.Scans().ForcedVideos(ctx, scan.ID); err != nil || !slices.Equal(forced, ids[:1]) {
		t.Fatalf("ForcedVideos = %v, err %v, want %v", forced, err, ids[:1])
	}
	read, err := db.ScanIndex().ReadLocations(ctx, scan.ID)
	if want := []string{fixturePath("/media/a.mp4")}; err != nil || !slices.Equal(read, want) {
		t.Fatalf("ReadLocations = %v, err %v, want %v", read, err, want)
	}
	want := []string{fixturePath("/media/a.mp4"), fixturePath("/other/y.mp4")}
	if got := currentIssuePaths(t, db); !slices.Equal(got, want) {
		t.Fatalf("issues after reopening = %v (before %v), want %v", got, issuesBefore, want)
	}

	recordIssue(t, db, ids[1], "/media/b.mp4")
	want = []string{fixturePath("/media/a.mp4"), fixturePath("/media/b.mp4"), fixturePath("/other/y.mp4")}
	if got := currentIssuePaths(t, db); !slices.Equal(got, want) {
		t.Fatalf("issues after the file failed again = %v, want %v", got, want)
	}
}

// During a pause Scan library creates the scan as paused and a second request answers it; a force rescan is refused
// while it waits, and media folders can be added and deleted. Both answers hold after the last folder is deleted.
func TestStartScanWhileBackgroundWorkIsPaused(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	if err := db.Settings().SaveBackgroundPaused(ctx, true); err != nil {
		t.Fatal(err)
	}

	first, started, err := db.Scans().StartScan(ctx)
	if err != nil || !started || first.State != domain.ScanPaused {
		t.Fatalf("StartScan = %+v, started %v, err %v; want a new paused scan", first, started, err)
	}
	second, started, err := db.Scans().StartScan(ctx)
	if err != nil || started || second.ID != first.ID {
		t.Fatalf("second StartScan = #%d, started %v, err %v; want #%d", second.ID, started, err, first.ID)
	}
	if _, err := db.Scans().StartRescan(ctx, domain.RescanScope{Kind: domain.RescanLibrary}); !errors.Is(
		err, domain.ErrScanRunning) {
		t.Fatalf("StartRescan = %v, want ErrScanRunning", err)
	}

	folder, err := db.Settings().AddMediaFolder(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("AddMediaFolder while paused = %v", err)
	}
	if err := db.Settings().DeleteMediaFolder(ctx, folder.ID, folder.Version); err != nil {
		t.Fatalf("DeleteMediaFolder while paused = %v", err)
	}

	folders, err := db.Settings().ListMediaFolders(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range folders {
		if err := db.Settings().DeleteMediaFolder(ctx, f.ID, f.Version); err != nil {
			t.Fatalf("DeleteMediaFolder of the last folder while paused = %v", err)
		}
	}
	third, started, err := db.Scans().StartScan(ctx)
	if err != nil || started || third.ID != first.ID {
		t.Fatalf("StartScan without folders = #%d, started %v, err %v; want #%d", third.ID, started, err, first.ID)
	}
	if _, err := db.Scans().StartRescan(ctx, domain.RescanScope{Kind: domain.RescanLibrary}); !errors.Is(
		err, domain.ErrScanRunning) {
		t.Fatalf("StartRescan without folders = %v, want ErrScanRunning", err)
	}
}

// Reopening applies only to the latest scan while it is paused and no other scan runs.
func TestReopenScanRefusesAScanThatIsNotPaused(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	scan, _, err := db.Scans().StartScan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Scans().ReopenScan(ctx, scan.ID); !errors.Is(err, domain.ErrScanRunning) {
		t.Fatalf("ReopenScan of the running scan = %v, want ErrScanRunning", err)
	}
	finishScan(t, db, scan.ID)
	if _, err := db.Scans().ReopenScan(ctx, scan.ID); !errors.Is(err, domain.ErrScanNotPaused) {
		t.Fatalf("ReopenScan of a finished scan = %v, want ErrScanNotPaused", err)
	}
	if err := db.Scans().PauseScan(ctx, scan.ID); !errors.Is(err, domain.ErrScanNotPaused) {
		t.Fatalf("PauseScan of a finished scan = %v, want ErrScanNotPaused", err)
	}
}
