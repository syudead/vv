package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// StartScan は走査を始める。すでに実行中のものがあれば、新しく始めずに
// それを返す（started = false）。
//
// 409 にしないのは、利用者の意図が「今の状態を進めたい」であり、進行中なら
// それを返すのが素直だからである。
func (s *ScanStore) StartScan(ctx context.Context) (scan domain.Scan, started bool, err error) {
	s.db.folderMu.Lock()
	defer s.db.folderMu.Unlock()
	var folderCount int
	if err := s.db.sql.QueryRowContext(ctx, `select count(*) from media_folders`).Scan(&folderCount); err != nil {
		return domain.Scan{}, false, err
	}
	if folderCount == 0 {
		return domain.Scan{}, false, domain.ErrNoMediaFolders
	}
	if running, err := s.scanBy(ctx,
		`select `+scanColumns+` from scans where state = 'running' limit 1`,
	); err == nil {
		return running, false, nil
	} else if !errors.Is(err, domain.ErrNotFound) {
		return domain.Scan{}, false, err
	}

	res, err := s.db.sql.ExecContext(ctx,
		`insert into scans (state, started_at, total, completed, failed) values ('running', ?, 0, 0, 0)`,
		time.Now().Unix())
	if err != nil {
		return domain.Scan{}, false, fmt.Errorf("走査を始められません: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return domain.Scan{}, false, fmt.Errorf("走査を始められません: %w", err)
	}

	scan, err = s.scanByID(ctx, id)
	if err != nil {
		return domain.Scan{}, false, err
	}
	return scan, true, nil
}

// UpdateScanProgress は進捗を更新する。走査中も一覧・再生は通常どおり応答する
// ので、ここでは行を1つ書き換えるだけにする。
func (s *ScanStore) UpdateScanProgress(ctx context.Context, id int64, progress domain.ScanProgress) error {
	_, err := s.db.sql.ExecContext(ctx,
		`update scans set total = ?, completed = ?, failed = ? where id = ?`,
		progress.Total, progress.Completed, progress.Failed, id)
	if err != nil {
		return fmt.Errorf("走査の進捗を記録できません (id=%d): %w", id, err)
	}
	return nil
}

// FinishScan は走査を終える。reason は走査そのものが失敗した理由で、
// 個別のファイルの失敗はここではなく failed の数に入る。
func (s *ScanStore) FinishScan(ctx context.Context, id int64, state domain.ScanState, reason string) error {
	_, err := s.db.sql.ExecContext(ctx,
		`update scans set state = ?, finished_at = ?, error = ? where id = ?`,
		string(state), time.Now().Unix(), nullableString(reason), id)
	if err != nil {
		return fmt.Errorf("走査の終了を記録できません (id=%d): %w", id, err)
	}
	return nil
}

// CurrentScan は直近の走査を返す。実行中のものがあればそれを、無ければ最後に
// 終わったものを返す。一度も走査していなければ ErrNotFound を返す。
func (s *ScanStore) CurrentScan(ctx context.Context) (domain.Scan, error) {
	return s.scanBy(ctx, `
		select `+scanColumns+` from scans
		 order by (state = 'running') desc, id desc
		 limit 1`)
}

// FailInterruptedScans は running のまま残っている走査を failed で閉じ、その数を
// 返す。起動時に1度だけ呼ぶ。
//
// 閉じないと「実行中は1件だけ」の制約が働いたまま、二度と走査を始められなく
// なる。前回の走査が最後まで走ったかどうかは分からないので、成功とはみなさない。
func (s *ScanStore) FailInterruptedScans(ctx context.Context) (int64, error) {
	res, err := s.db.sql.ExecContext(ctx, `
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

func (s *ScanStore) scanByID(ctx context.Context, id int64) (domain.Scan, error) {
	return s.scanBy(ctx, `select `+scanColumns+` from scans where id = ?`, id)
}

func (s *ScanStore) scanBy(ctx context.Context, query string, args ...any) (domain.Scan, error) {
	var (
		scan                  domain.Scan
		state                 string
		startedAt, finishedAt sql.NullInt64
		reason                sql.NullString
	)

	//nolint:gosec // scanColumns は定数で、利用者の入力は混ざらない。
	err := s.db.sql.QueryRowContext(ctx, query, args...).Scan(
		&scan.ID, &state, &startedAt, &finishedAt,
		&scan.Total, &scan.Completed, &scan.Failed, &reason,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Scan{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Scan{}, fmt.Errorf("走査の記録を読み出せません: %w", err)
	}

	scan.State = domain.ScanState(state)
	if startedAt.Valid {
		scan.StartedAt = time.Unix(startedAt.Int64, 0)
	}
	if finishedAt.Valid {
		scan.FinishedAt = time.Unix(finishedAt.Int64, 0)
	}
	scan.Error = reason.String
	return scan, nil
}
