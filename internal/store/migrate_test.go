package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// FR-005: 初回起動でスキーマが手作業なしに適用される。
func TestMigrateAppliesSchemaOnEmptyDirectory(t *testing.T) {
	dataDir := t.TempDir()

	db, err := Open(dataDir)
	if err != nil {
		t.Fatalf("空のディレクトリから開けない: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	result, err := Migrate(context.Background(), db)
	if err != nil {
		t.Fatalf("マイグレーションに失敗した: %v", err)
	}
	if result.Applied == 0 {
		t.Error("適用された数が 0 件")
	}
	if result.Version < 1 {
		t.Errorf("適用後の版 = %d, want >= 1", result.Version)
	}

	for _, name := range []string{"videos", "videos_fts", versionTableName} {
		var count int
		err := db.SQL().QueryRow(
			`select count(*) from sqlite_master where name = ?`, name,
		).Scan(&count)
		if err != nil {
			t.Fatal(err)
		}
		if count == 0 {
			t.Errorf("%s が作成されていない", name)
		}
	}
}

// 2 度目の呼び出しで何も適用されないこと（冪等）。
func TestMigrateIsIdempotent(t *testing.T) {
	dataDir := t.TempDir()

	db, err := Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	first, err := Migrate(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Migrate(context.Background(), db)
	if err != nil {
		t.Fatalf("2 度目の適用で失敗した: %v", err)
	}

	if second.Applied != 0 {
		t.Errorf("2 度目に %d 件適用された, want 0", second.Applied)
	}
	if second.Version != first.Version {
		t.Errorf("版が変わった: %d -> %d", first.Version, second.Version)
	}
}

// SC-006: データベースファイルを削除しても、手作業なしに次の起動で復旧する。
func TestMigrateRecoversAfterDatabaseFileIsDeleted(t *testing.T) {
	dataDir := t.TempDir()

	db, err := Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	// WAL の付随ファイルも含めて消す。利用者が丸ごと削除した状況を再現する。
	for _, suffix := range []string{"", "-wal", "-shm"} {
		path := DatabasePath(dataDir) + suffix
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(DatabasePath(dataDir)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("削除できていない: %v", err)
	}

	reopened, err := Open(dataDir)
	if err != nil {
		t.Fatalf("削除後に開けない: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })

	result, err := Migrate(context.Background(), reopened)
	if err != nil {
		t.Fatalf("削除後の復旧に失敗した: %v", err)
	}
	if result.Applied == 0 {
		t.Error("復旧時に何も適用されていない")
	}
}

// ダウングレードによる破壊を防ぐ。将来の版を持つデータベースは書き換えずに中止する。
func TestMigrateAbortsOnFutureSchemaWithoutWriting(t *testing.T) {
	dataDir := t.TempDir()

	db, err := Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}

	// アプリケーションが知らない将来の版が適用済みである状況を作る。
	const futureVersion = 9999
	_, err = db.SQL().Exec(
		`insert into `+versionTableName+` (version_id, is_applied, tstamp) values (?, 1, current_timestamp)`,
		futureVersion,
	)
	if err != nil {
		t.Fatal(err)
	}
	// 将来の版が触るはずのない印を置き、書き換えられないことを確かめる。
	if _, err := db.SQL().Exec(
		`insert into videos(path, title, size_bytes, mtime) values ('/media/sentinel.mp4', 'sentinel', 1, 1)`,
	); err != nil {
		t.Fatal(err)
	}

	_, err = Migrate(context.Background(), db)
	if err == nil {
		t.Fatal("将来の版を持つデータベースで成功してしまった")
	}
	if !errors.Is(err, ErrFutureSchema) {
		t.Errorf("err = %v, want ErrFutureSchema", err)
	}
	if !strings.Contains(err.Error(), "9999") {
		t.Errorf("出力にデータベース側の版が含まれていない:\n%s", err)
	}

	var version int64
	if err := db.SQL().QueryRow(
		`select max(version_id) from ` + versionTableName,
	).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != futureVersion {
		t.Errorf("記録された版が書き換えられた: %d, want %d", version, futureVersion)
	}

	var rows int
	if err := db.SQL().QueryRow(`select count(*) from videos`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Errorf("既存のデータが失われた: %d 行, want 1", rows)
	}
}

// データベースのパスは DataDir/mdm.db に固定する（設定項目にしない）。
func TestDatabasePathIsFixedUnderDataDir(t *testing.T) {
	got := DatabasePath("/data")
	want := filepath.Join("/data", DatabaseFileName)
	if got != want {
		t.Errorf("DatabasePath = %q, want %q", got, want)
	}
	if DatabaseFileName != "mdm.db" {
		t.Errorf("DatabaseFileName = %q, want mdm.db", DatabaseFileName)
	}
}
