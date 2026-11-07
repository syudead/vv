package store

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWriteTransactionsWaitForTheCurrentWriter(t *testing.T) {
	db := migratedDB(t)

	tx, err := db.sql.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("書き込みトランザクションを開始できない: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	done := make(chan error, 1)
	go func() {
		_, err := db.ScanIndex().UpsertVideo(context.Background(), sampleFile(fixturePath("/media/wait.mp4"), "wait", "wait-key", 1, 0))
		done <- err
	}()

	select {
	case err := <-done:
		t.Fatalf("先行writerの終了前に後続トランザクションが完了した: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("先行トランザクションを完了できない: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("writer解放後の取り込みに失敗した: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("writer解放後も取り込みが再開しない")
	}
}

func TestListReadTransactionDoesNotReserveTheWriter(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	tx, err := db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatalf("読み取りトランザクションを開始できない: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	var count int
	if err := tx.QueryRowContext(ctx, `select count(*) from videos`).Scan(&count); err != nil {
		t.Fatalf("読み取りスナップショットを開始できない: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := db.ScanIndex().UpsertVideo(ctx, sampleFile(fixturePath("/media/read-does-not-block.mp4"), "read", "read-key", 1, 0))
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("読み取り中の取り込みに失敗した: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("読み取りトランザクションが取り込みの書き込みを妨げた")
	}
}

func TestOpenKeepsURIDelimitersInTheDataDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a?b#c%41")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := OpenMigratedForTest(dir)
	if err != nil {
		t.Fatalf("cannot open the database under %q: %v", dir, err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if got := db.Path(); got != filepath.Join(dir, DatabaseFileName) {
		t.Fatalf("Path() = %q", got)
	}
	// Both pools must reach the migrated file, not a sibling named by the part before '?'.
	for name, handle := range map[string]*sql.DB{"write": db.sql, "read": db.read} {
		var count int
		if err := handle.QueryRow(`select count(*) from media_folders`).Scan(&count); err != nil {
			t.Errorf("%s pool cannot read the migrated schema: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "a")); !os.IsNotExist(err) {
		t.Errorf("a database was created at the truncated path: %v", err)
	}
}

func TestReadPoolRejectsWrites(t *testing.T) {
	db := migratedDB(t)

	if _, err := db.read.Exec(`delete from media_folders`); err == nil {
		t.Fatal("the read pool accepted a write")
	}
	var count int
	if err := db.read.QueryRow(`select count(*) from media_folders`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("media folders after the rejected write = %d, %v; want 1", count, err)
	}
}
