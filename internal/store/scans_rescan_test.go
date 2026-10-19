package store

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// addOtherMediaFolder registers a second media folder beside /media and returns its id.
func addOtherMediaFolder(t *testing.T, db *DB) int64 {
	t.Helper()
	res, err := db.sql.Exec(`insert into media_folders(path, version, created_at, updated_at) values (?, 1, 1, 1)`,
		fixturePath("/other"))
	if err != nil {
		t.Fatal(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func mediaFolderID(t *testing.T, db *DB, path string) int64 {
	t.Helper()
	var id int64
	if err := db.sql.QueryRow(`select id from media_folders where path = ?`, path).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func recordIssue(t *testing.T, db *DB, videoID int64, path string) {
	t.Helper()
	if err := db.Scans().RecordScanIssue(context.Background(), domain.ScanFileIssue{
		VideoID: videoID, Path: fixturePath(path), Kind: domain.IssueUnreadable,
	}); err != nil {
		t.Fatal(err)
	}
}

// currentIssuePaths lists the issue paths of the current scan, without the registered-folder filter.
func currentIssuePaths(t *testing.T, db *DB) []string {
	t.Helper()
	ctx := context.Background()
	scan, err := db.Scans().CurrentScan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	records, err := db.Scans().ScanIssueRecords(ctx, scan.ID)
	if err != nil {
		t.Fatal(err)
	}
	paths := make([]string, 0, len(records))
	for _, record := range records {
		paths = append(paths, record.Path)
	}
	slices.Sort(paths)
	return paths
}

func startTestRescan(t *testing.T, db *DB, scope domain.RescanScope) domain.Scan {
	t.Helper()
	scan, err := db.Scans().StartRescan(context.Background(), scope)
	if err != nil {
		t.Fatalf("StartRescan(%+v) = %v", scope, err)
	}
	return scan
}

// A rescan rediscovers the issues of what it reads, so it clears those and keeps the rest, stage issues included.
func TestStartRescanCarriesIssuesOutsideItsScope(t *testing.T) {
	media := fixturePath("/media")
	cases := []struct {
		name  string
		scope func(db *DB, a int64) domain.RescanScope
		want  []string
	}{
		{
			name: "library",
			scope: func(*DB, int64) domain.RescanScope {
				return domain.RescanScope{Kind: domain.RescanLibrary}
			},
			want: []string{},
		},
		{
			name: "folder",
			scope: func(db *DB, _ int64) domain.RescanScope {
				return domain.RescanScope{Kind: domain.RescanFolder, FolderID: mediaFolderID(t, db, media)}
			},
			want: []string{fixturePath("/other/y.mp4")},
		},
		{
			name: "video",
			scope: func(_ *DB, a int64) domain.RescanScope {
				return domain.RescanScope{Kind: domain.RescanVideo, VideoID: a}
			},
			want: []string{fixturePath("/media/b.mp4"), fixturePath("/media/x.mp4"), fixturePath("/other/y.mp4")},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := migratedDB(t)
			addOtherMediaFolder(t, db)
			_, ids := startImport(t, db, "a", "b")
			failJobToLimit(t, db, domain.JobProbe) // a stage issue on a
			recordIssue(t, db, ids[1], "/media/b.mp4")
			recordIssue(t, db, 0, "/media/x.mp4")
			recordIssue(t, db, 0, "/other/y.mp4")
			if got := currentIssuePaths(t, db); len(got) != 4 {
				t.Fatalf("準備: 問題 = %v", got)
			}

			startTestRescan(t, db, tc.scope(db, ids[0]))
			if got := currentIssuePaths(t, db); !slices.Equal(got, tc.want) {
				t.Fatalf("残った問題 = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestStartRescanRecordsTheScope(t *testing.T) {
	db := migratedDB(t)
	media := fixturePath("/media")
	folderID := mediaFolderID(t, db, media)

	scan := startTestRescan(t, db, domain.RescanScope{Kind: domain.RescanFolder, FolderID: folderID, VideoID: 9})
	want := domain.RescanScope{Kind: domain.RescanFolder, FolderID: folderID, Path: media}
	if scan.Origin != domain.ScanOriginManual || scan.Rescan == nil || *scan.Rescan != want {
		t.Fatalf("走査 = origin %q rescan %+v, want manual %+v", scan.Origin, scan.Rescan, want)
	}
	finishScan(t, db, scan.ID)

	ordinary, _, err := db.Scans().StartScan(context.Background())
	if err != nil || ordinary.Rescan != nil {
		t.Fatalf("通常の走査の rescan = %+v, err %v, want nil", ordinary.Rescan, err)
	}
}

func TestStartRescanRefusesARunningScan(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	running, _, err := db.Scans().StartScan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Scans().StartRescan(ctx, domain.RescanScope{Kind: domain.RescanLibrary}); !errors.Is(err, domain.ErrScanRunning) {
		t.Fatalf("StartRescan = %v, want ErrScanRunning", err)
	}
	if current, err := db.Scans().CurrentScan(ctx); err != nil || current.ID != running.ID || current.Rescan != nil {
		t.Fatalf("CurrentScan = %+v, err %v, want 走っている #%d のまま", current, err, running.ID)
	}
}

func TestStartRescanRejectsAMissingTarget(t *testing.T) {
	db := migratedDB(t)
	for _, scope := range []domain.RescanScope{
		{Kind: domain.RescanFolder, FolderID: 999},
		{Kind: domain.RescanVideo, VideoID: 999},
	} {
		if _, err := db.Scans().StartRescan(context.Background(), scope); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("StartRescan(%+v) = %v, want ErrNotFound", scope, err)
		}
	}
	if _, err := db.Scans().CurrentScan(context.Background()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("CurrentScan = %v, want 走査なし", err)
	}
}

// A restart continues the interrupted rescan: same scope, the same forced videos, and its carried issues.
func TestResumeScanContinuesAnInterruptedFolderRescan(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	addOtherMediaFolder(t, db)
	_, ids := startImport(t, db, "a", "b")
	recordIssue(t, db, 0, "/other/y.mp4")
	interrupted := startTestRescan(t, db, domain.RescanScope{
		Kind: domain.RescanFolder, FolderID: mediaFolderID(t, db, fixturePath("/media")),
	})
	if err := db.Scans().MarkForced(ctx, interrupted.ID, ids[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Scans().FailInterruptedScans(ctx); err != nil {
		t.Fatal(err)
	}

	resumed, started, err := db.Scans().ResumeScan(ctx, interrupted.ID)
	if err != nil || !started || resumed.ID == interrupted.ID {
		t.Fatalf("ResumeScan = #%d started=%v err=%v, want 新しい走査", resumed.ID, started, err)
	}
	if resumed.Rescan == nil || *resumed.Rescan != *interrupted.Rescan || resumed.Origin != domain.ScanOriginManual {
		t.Fatalf("再開した走査 = origin %q rescan %+v, want manual %+v", resumed.Origin, resumed.Rescan, interrupted.Rescan)
	}
	forced, err := db.Scans().ForcedVideos(ctx, resumed.ID)
	if err != nil || !slices.Equal(forced, []int64{ids[0]}) {
		t.Fatalf("ForcedVideos = %v, err %v, want [%d]", forced, err, ids[0])
	}
	if got, want := currentIssuePaths(t, db), []string{fixturePath("/other/y.mp4")}; !slices.Equal(got, want) {
		t.Fatalf("再開後の問題 = %v, want %v", got, want)
	}
}

// Only a rescan that walks the whole library may resolve succession; a folder or video rescan cannot tell
// whether the old content still exists elsewhere.
func TestFinishedRescanResolvesSuccessionOnlyForTheLibrary(t *testing.T) {
	cases := []struct {
		name      string
		scope     func(db *DB, a int64) domain.RescanScope
		carriesTo bool
	}{
		{
			name:      "library",
			scope:     func(*DB, int64) domain.RescanScope { return domain.RescanScope{Kind: domain.RescanLibrary} },
			carriesTo: true,
		},
		{
			name: "folder",
			scope: func(db *DB, _ int64) domain.RescanScope {
				return domain.RescanScope{Kind: domain.RescanFolder, FolderID: mediaFolderID(t, db, fixturePath("/media"))}
			},
		},
		{
			name: "video",
			scope: func(_ *DB, a int64) domain.RescanScope {
				return domain.RescanScope{Kind: domain.RescanVideo, VideoID: a}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, a := taggedVideoFixture(t)
			rescan := startTestRescan(t, db, tc.scope(db, a))
			b := upsertOne(t, db, listingFile(fixturePath("/media/a.mp4"), "a", "key-b", 2))
			probeDuration(t, db, b, 100_000)

			finishTestScan(t, db, rescan.ID, domain.ScanDone)
			tags := manualTagNames(t, db, ownerVideo(t, db, b).UserKey)
			if tc.carriesTo {
				if !slices.Equal(tags, []string{"好き"}) {
					t.Fatalf("全体の再スキャンが引き継がなかった: %v", tags)
				}
				return
			}
			if len(tags) != 0 || successionCount(t, db) != 1 {
				t.Fatalf("部分の再スキャンが後継を判定した: タグ %v, 記録 %d", tags, successionCount(t, db))
			}
		})
	}
}
