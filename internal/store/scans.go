package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// 走査の語彙は internal/domain が持つ。ここでは別名を置いて、store を使う側が
// domain を直接 import しなくても読めるようにする。
type (
	// ScanState は走査の状態である。
	ScanState = domain.ScanState
	// Scan は走査1回の記録である。
	Scan = domain.Scan
	// ScanProgress は進捗の値である。
	ScanProgress = domain.ScanProgress
)

const (
	// ScanRunning は走査中。同時に1件だけ存在できる。
	ScanRunning = domain.ScanRunning
	// ScanDone は最後まで走った。
	ScanDone = domain.ScanDone
	// ScanFailed は走査そのものが失敗した。
	ScanFailed = domain.ScanFailed
)

var ErrNoMediaFolders = domain.ErrNoMediaFolders

// StartScan は走査を始める。すでに実行中のものがあれば、新しく始めずに
// それを返す（started = false）。
//
// 409 にしないのは、利用者の意図が「今の状態を進めたい」であり、進行中なら
// それを返すのが素直だからである（R-108）。
func (db *DB) StartScan(ctx context.Context) (scan Scan, started bool, err error) {
	db.folderMu.Lock()
	defer db.folderMu.Unlock()
	var folderCount int
	if err := db.sql.QueryRowContext(ctx, `select count(*) from media_folders`).Scan(&folderCount); err != nil {
		return Scan{}, false, err
	}
	if folderCount == 0 {
		return Scan{}, false, ErrNoMediaFolders
	}
	if running, err := db.scanBy(ctx,
		`select `+scanColumns+` from scans where state = 'running' limit 1`,
	); err == nil {
		return running, false, nil
	} else if !errors.Is(err, ErrNotFound) {
		return Scan{}, false, err
	}

	res, err := db.sql.ExecContext(ctx,
		`insert into scans (state, started_at, total, completed, failed) values ('running', ?, 0, 0, 0)`,
		time.Now().Unix())
	if err != nil {
		return Scan{}, false, fmt.Errorf("走査を始められません: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Scan{}, false, fmt.Errorf("走査を始められません: %w", err)
	}

	scan, err = db.scanByID(ctx, id)
	if err != nil {
		return Scan{}, false, err
	}
	return scan, true, nil
}

// UpdateScanProgress は進捗を更新する。走査中も一覧・再生は通常どおり応答する
// ので（FR-007）、ここでは行を1つ書き換えるだけにする。
func (db *DB) UpdateScanProgress(ctx context.Context, id int64, progress ScanProgress) error {
	_, err := db.sql.ExecContext(ctx,
		`update scans set total = ?, completed = ?, failed = ? where id = ?`,
		progress.Total, progress.Completed, progress.Failed, id)
	if err != nil {
		return fmt.Errorf("走査の進捗を記録できません (id=%d): %w", id, err)
	}
	return nil
}

// FinishScan は走査を終える。reason は走査そのものが失敗した理由で、
// 個別のファイルの失敗はここではなく failed の数に入る。
func (db *DB) FinishScan(ctx context.Context, id int64, state ScanState, reason string) error {
	_, err := db.sql.ExecContext(ctx,
		`update scans set state = ?, finished_at = ?, error = ? where id = ?`,
		string(state), time.Now().Unix(), nullableString(reason), id)
	if err != nil {
		return fmt.Errorf("走査の終了を記録できません (id=%d): %w", id, err)
	}
	return nil
}

// CurrentScan は直近の走査を返す。実行中のものがあればそれを、無ければ最後に
// 終わったものを返す。一度も走査していなければ ErrNotFound を返す。
func (db *DB) CurrentScan(ctx context.Context) (Scan, error) {
	return db.scanBy(ctx, `
		select `+scanColumns+` from scans
		 order by (state = 'running') desc, id desc
		 limit 1`)
}

// FailInterruptedScans は running のまま残っている走査を failed で閉じ、その数を
// 返す。起動時に1度だけ呼ぶ。
//
// 閉じないと「実行中は1件だけ」の制約が働いたまま、二度と走査を始められなく
// なる。前回の走査が最後まで走ったかどうかは分からないので、成功とはみなさない。
func (db *DB) FailInterruptedScans(ctx context.Context) (int64, error) {
	res, err := db.sql.ExecContext(ctx, `
		update scans
		   set state = 'failed', finished_at = ?, error = ?
		 where state = 'running'`,
		time.Now().Unix(), "取り込みの途中でアプリケーションが停止しました")
	if err != nil {
		return 0, fmt.Errorf("中断した走査を閉じられません: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("中断した走査を閉じられません: %w", err)
	}
	return affected, nil
}

// scanColumns は Scan を組み立てるのに要る列である。並びは scanRow と対応させる。
const scanColumns = `id, state, started_at, finished_at, total, completed, failed, error`

func (db *DB) scanByID(ctx context.Context, id int64) (Scan, error) {
	return db.scanBy(ctx, `select `+scanColumns+` from scans where id = ?`, id)
}

func (db *DB) scanBy(ctx context.Context, query string, args ...any) (Scan, error) {
	var (
		scan                  Scan
		state                 string
		startedAt, finishedAt sql.NullInt64
		reason                sql.NullString
	)

	//nolint:gosec // scanColumns は定数で、利用者の入力は混ざらない。
	err := db.sql.QueryRowContext(ctx, query, args...).Scan(
		&scan.ID, &state, &startedAt, &finishedAt,
		&scan.Total, &scan.Completed, &scan.Failed, &reason,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Scan{}, ErrNotFound
	}
	if err != nil {
		return Scan{}, fmt.Errorf("走査の記録を読み出せません: %w", err)
	}

	scan.State = ScanState(state)
	if startedAt.Valid {
		scan.StartedAt = time.Unix(startedAt.Int64, 0)
	}
	if finishedAt.Valid {
		scan.FinishedAt = time.Unix(finishedAt.Int64, 0)
	}
	scan.Error = reason.String
	return scan, nil
}
