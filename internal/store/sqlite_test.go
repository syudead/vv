package store

import (
	"context"
	"testing"
	"time"
)

// 読み取りから始めたトランザクションの途中で別の接続が書き込んでも、
// 書き込みへ進んだ時点で SQLITE_BUSY にならない。走査の取り込み（UpsertVideo）
// とジョブの処理が同時に書き込んだとき、取り込みが database is locked で
// 落ちていた再発を防ぐ。
func TestTransactionDoesNotFailWhenAnotherConnectionWritesMeanwhile(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	tx, err := db.SQL().BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("トランザクションを始められない: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	var count int
	if err := tx.QueryRowContext(ctx, `select count(*) from videos`).Scan(&count); err != nil {
		t.Fatalf("読み取れない: %v", err)
	}

	// 別の接続からの書き込み。トランザクションが書き込みロックを持っていれば
	// ここは commit まで待たされる。
	otherDone := make(chan error, 1)
	go func() {
		_, err := db.SQL().ExecContext(ctx, `insert into videos
			(added_at, updated_at, content_key, playable, probe_state, thumbnail_state)
			values (1, 1, 'other', 0, 'pending', 'pending')`)
		otherDone <- err
	}()
	select {
	case err := <-otherDone:
		t.Fatalf("別の接続の書き込みが待たされずに終わった (err=%v)", err)
	case <-time.After(200 * time.Millisecond):
	}

	if _, err := tx.ExecContext(ctx, `insert into videos
		(added_at, updated_at, content_key, playable, probe_state, thumbnail_state)
		values (1, 1, 'mine', 0, 'pending', 'pending')`); err != nil {
		t.Fatalf("書き込めない: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("確定できない: %v", err)
	}
	if err := <-otherDone; err != nil {
		t.Fatalf("別の接続が書き込めない: %v", err)
	}
}
