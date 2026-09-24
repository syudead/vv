package store

import (
	"context"
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
		_, err := db.ScanIndex().UpsertVideo(context.Background(), sampleFile("/media/wait.mp4", "wait", "wait-key", 1, 0))
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
