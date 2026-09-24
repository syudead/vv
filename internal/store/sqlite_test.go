package store

import (
	"context"
	"fmt"
	"testing"
)

// BeginTx で始めたトランザクションは、読み取りから始めても開始時点で書き込み
// ロックを持つ。deferred のままだと、読み取りと書き込みの間に別の接続が
// 書き込めてしまい、その後の書き込みが busy_timeout を待たずに SQLITE_BUSY で
// 落ちる。走査の取り込み（UpsertVideo）とジョブの処理が同時に書き込んだとき、
// 取り込みが database is locked で落ちていた再発を防ぐ。
func TestTransactionHoldsWriteLockFromBegin(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	// 別の接続は待たずに結果を返すようにする。ロックが取られていれば即座に
	// SQLITE_BUSY になるので、待ち時間やゴルーチンの起動順に依存しない。
	other, err := db.SQL().Conn(ctx)
	if err != nil {
		t.Fatalf("別の接続を取れない: %v", err)
	}
	defer func() {
		_, _ = other.ExecContext(ctx, fmt.Sprintf("pragma busy_timeout = %d", busyTimeout))
		_ = other.Close()
	}()
	if _, err := other.ExecContext(ctx, `pragma busy_timeout = 0`); err != nil {
		t.Fatalf("待ち時間を変えられない: %v", err)
	}

	tx, err := db.SQL().BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("トランザクションを始められない: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	var count int
	if err := tx.QueryRowContext(ctx, `select count(*) from videos`).Scan(&count); err != nil {
		t.Fatalf("読み取れない: %v", err)
	}

	if _, err := other.ExecContext(ctx, `insert into videos
		(added_at, updated_at, content_key, playable, probe_state, thumbnail_state)
		values (1, 1, 'other', 0, 'pending', 'pending')`); err == nil {
		t.Fatal("トランザクションの途中で別の接続が書き込めた（書き込みロックを持っていない）")
	}

	if _, err := tx.ExecContext(ctx, `insert into videos
		(added_at, updated_at, content_key, playable, probe_state, thumbnail_state)
		values (1, 1, 'mine', 0, 'pending', 'pending')`); err != nil {
		t.Fatalf("書き込めない: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("確定できない: %v", err)
	}
}
