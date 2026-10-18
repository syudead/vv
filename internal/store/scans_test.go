package store

import (
	"context"
	"errors"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func scanDB(t *testing.T) *DB {
	t.Helper()
	return migratedDB(t)
}

func TestStartScanRejectsEmptyFolderSetWithoutCreatingScan(t *testing.T) {
	db := migratedDB(t)
	if _, err := db.sql.Exec(`delete from media_folders`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.Scans().StartScan(context.Background()); !errors.Is(err, domain.ErrNoMediaFolders) {
		t.Fatalf("error = %v, want domain.ErrNoMediaFolders", err)
	}
	var count int
	if err := db.sql.QueryRow(`select count(*) from scans`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("scan rows = %d, want 0", count)
	}
}

func TestStartScanReturnsRunningInsteadOfStartingAnother(t *testing.T) {
	db := scanDB(t)
	ctx := context.Background()

	first, started, err := db.Scans().StartScan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !started {
		t.Error("1件目が開始扱いになっていない")
	}
	if first.State != domain.ScanRunning {
		t.Errorf("State = %q, want running", first.State)
	}
	if first.StartedAt.IsZero() {
		t.Error("開始時刻が入っていない")
	}

	second, started, err := db.Scans().StartScan(ctx)
	if err != nil {
		t.Fatalf("実行中に呼んで失敗した（409 にはしない）: %v", err)
	}
	if started {
		t.Error("実行中なのに新しいスキャンが始まった")
	}
	if second.ID != first.ID {
		t.Errorf("別のスキャンが返った: %d, want %d", second.ID, first.ID)
	}
}

func TestUpdateScanProgress(t *testing.T) {
	db := scanDB(t)
	ctx := context.Background()

	scan, _, err := db.Scans().StartScan(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if err := db.Scans().UpdateScanProgress(ctx, scan.ID, domain.ScanProgress{Total: 10, Completed: 3, Failed: 1}); err != nil {
		t.Fatal(err)
	}

	got, err := db.Scans().CurrentScan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 10 || got.Completed != 3 || got.Failed != 1 {
		t.Errorf("進捗 = %+v, want total=10 completed=3 failed=1", got)
	}
}

func TestFinishScan(t *testing.T) {
	db := scanDB(t)
	ctx := context.Background()

	scan, _, err := db.Scans().StartScan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Scans().FinishScan(ctx, scan.ID, domain.ScanDone, nil, domain.ActivityByOwner); err != nil {
		t.Fatal(err)
	}

	got, err := db.Scans().CurrentScan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != domain.ScanDone {
		t.Errorf("State = %q, want done", got.State)
	}
	if got.FinishedAt.IsZero() {
		t.Error("終了時刻が入っていない")
	}

	next, started, err := db.Scans().StartScan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !started || next.ID == scan.ID {
		t.Error("終了後に次のスキャンを始められない")
	}
}

func TestFinishScanRecordsError(t *testing.T) {
	db := scanDB(t)
	ctx := context.Background()

	scan, _, err := db.Scans().StartScan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	folder := fixturePath("/media/unreadable")
	cause := domain.NewScanFailure(domain.ScanErrorMediaFolderUnreadable, folder,
		errors.New("could not read the media folder"))
	if err := db.Scans().FinishScan(ctx, scan.ID, domain.ScanFailed, cause, domain.ActivityByOwner); err != nil {
		t.Fatal(err)
	}

	got, err := db.Scans().CurrentScan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != domain.ScanFailed {
		t.Errorf("State = %q, want failed", got.State)
	}
	if got.Error != "could not read the media folder" {
		t.Errorf("Error = %q, want the reason", got.Error)
	}
	if got.ErrorCode != domain.ScanErrorMediaFolderUnreadable || got.ErrorPath != folder {
		t.Errorf("ErrorCode = %q (%q), want media_folder_unreadable (%q)", got.ErrorCode, got.ErrorPath, folder)
	}
}

func TestFinishScanRecordsInternalForUncodedError(t *testing.T) {
	db := scanDB(t)
	ctx := context.Background()

	scan, _, err := db.Scans().StartScan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Scans().FinishScan(ctx, scan.ID, domain.ScanFailed, errors.New("database is locked"), domain.ActivityByOwner); err != nil {
		t.Fatal(err)
	}
	got, err := db.Scans().CurrentScan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.ErrorCode != domain.ScanErrorInternal || got.ErrorPath != "" {
		t.Errorf("ErrorCode = %q (%q), want internal without a path", got.ErrorCode, got.ErrorPath)
	}

	next, _, err := db.Scans().StartScan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Scans().FinishScan(ctx, next.ID, domain.ScanDone, nil, domain.ActivityByOwner); err != nil {
		t.Fatal(err)
	}
	got, err = db.Scans().CurrentScan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.Error != "" || got.ErrorCode != "" || got.ErrorPath != "" {
		t.Errorf("done の走査に理由がある: %+v", got)
	}
}

func TestCurrentScanPrefersRunning(t *testing.T) {
	db := scanDB(t)
	ctx := context.Background()

	finished, _, err := db.Scans().StartScan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Scans().FinishScan(ctx, finished.ID, domain.ScanDone, nil, domain.ActivityByOwner); err != nil {
		t.Fatal(err)
	}

	running, _, err := db.Scans().StartScan(ctx)
	if err != nil {
		t.Fatal(err)
	}

	got, err := db.Scans().CurrentScan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != running.ID {
		t.Errorf("実行中ではなく %d が返った, want %d", got.ID, running.ID)
	}
}

func TestCurrentScanWhenNeverScanned(t *testing.T) {
	db := scanDB(t)

	if _, err := db.Scans().CurrentScan(context.Background()); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("err = %v, want domain.ErrNotFound", err)
	}
}

func TestFailInterruptedScans(t *testing.T) {
	db := scanDB(t)
	ctx := context.Background()

	interrupted, _, err := db.Scans().StartScan(ctx)
	if err != nil {
		t.Fatal(err)
	}

	closed, err := db.Scans().FailInterruptedScans(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if closed != 1 {
		t.Errorf("閉じた数 = %d, want 1", closed)
	}

	got, err := db.Scans().CurrentScan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != interrupted.ID || got.State != domain.ScanFailed {
		t.Errorf("中断したスキャンが failed になっていない: %+v", got)
	}
	if got.ErrorCode != domain.ScanErrorInterrupted || got.ErrorPath != "" || got.Error == "" {
		t.Errorf("中断したスキャンの理由 = %q (%q, %q), want interrupted", got.ErrorCode, got.ErrorPath, got.Error)
	}

	if _, started, err := db.Scans().StartScan(ctx); err != nil || !started {
		t.Errorf("閉じたあとにスキャンを始められない: started=%v err=%v", started, err)
	}
}
