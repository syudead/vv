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

// 実行中のスキャンは同時に1件だけ。POST /api/scans が「実行中ならそれを返す」
// 振る舞いは、これが成り立つことを前提にしている。
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

// 進捗を更新できること。取り込みの規模と残りが利用者に見える。
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

// 終了すると done になり、終了時刻が入る。次のスキャンを始められる。
func TestFinishScan(t *testing.T) {
	db := scanDB(t)
	ctx := context.Background()

	scan, _, err := db.Scans().StartScan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Scans().FinishScan(ctx, scan.ID, domain.ScanDone, ""); err != nil {
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

// 走査そのものが失敗した場合（対象ディレクトリが読めない等）は理由を残す。
func TestFinishScanRecordsError(t *testing.T) {
	db := scanDB(t)
	ctx := context.Background()

	scan, _, err := db.Scans().StartScan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Scans().FinishScan(ctx, scan.ID, domain.ScanFailed, "メディアフォルダを読み取れません"); err != nil {
		t.Fatal(err)
	}

	got, err := db.Scans().CurrentScan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != domain.ScanFailed {
		t.Errorf("State = %q, want failed", got.State)
	}
	if got.Error == "" {
		t.Error("失敗の理由が記録されていない")
	}
}

// 直近のスキャンは「実行中があればそれ、無ければ最後に終わったもの」。
func TestCurrentScanPrefersRunning(t *testing.T) {
	db := scanDB(t)
	ctx := context.Background()

	finished, _, err := db.Scans().StartScan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Scans().FinishScan(ctx, finished.ID, domain.ScanDone, ""); err != nil {
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

// 一度もスキャンしていなければ「無い」と分かる誤りを返す（GET は 404 になる）。
func TestCurrentScanWhenNeverScanned(t *testing.T) {
	db := scanDB(t)

	if _, err := db.Scans().CurrentScan(context.Background()); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("err = %v, want domain.ErrNotFound", err)
	}
}

// プロセスが running のまま落ちた場合、次の起動で失敗として閉じる。
// 閉じないと「実行中は1件だけ」の制約が二度とスキャンを始めさせない。
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

	if _, started, err := db.Scans().StartScan(ctx); err != nil || !started {
		t.Errorf("閉じたあとにスキャンを始められない: started=%v err=%v", started, err)
	}
}
