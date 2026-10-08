package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// StartScan は走査を始める。すでに実行中のものがあれば、新しく始めずに
// それを返す（started = false）。
//
// 409 にしないのは、利用者の意図が「今の状態を進めたい」であり、進行中なら
// それを返すのが素直だからである。
//
// 新しい走査の行を入れる取引で、前の走査の対象の動画の集合を入れ替える。残りの
// 仕事がある動画は新しい走査の対象へ持ち越す
// （specs/024-import-progress/research.md R-3）。
func (s *ScanStore) StartScan(ctx context.Context) (scan domain.Scan, started bool, err error) {
	return s.startScan(ctx, 0, domain.ScanOriginManual, nil)
}

// StartWatchScan はフォルダの監視の走査を始める。StartScan と同じだが、origin は watch で、
// 前の走査の問題のうち dirs の読む範囲の外のものは、消さずに新しい走査へ付け替える
// （範囲の中は走査が読み直して見つけ直す。specs/042-folder-watch-import/data-model.md の Rules）。
// 手動の走査は今までどおり、問題をすべて消す。
func (s *ScanStore) StartWatchScan(ctx context.Context, dirs []domain.DirtyDirectory) (scan domain.Scan, started bool, err error) {
	return s.startScan(ctx, 0, domain.ScanOriginWatch, dirs)
}

// ResumeScan は中断で終わった走査 from の続きとして走査を始める。StartScan と
// 同じだが、from の対象の動画の集合と、仕事の段階の問題（失敗と代用）を新しい
// 走査へ持ち越す。持ち越さないと、上限まで失敗した仕事は変わらないファイルでは
// 積み直されない（scanner の ensurePendingJobs）ので、その失敗が一覧から消え、
// 取り込みが partial でなく done になる。始め直しを「続きから」と同じ結果にする
// ための持ち越しである（specs/037-windows-app/research.md R-9）。走査が自分で
// 見つける種類（domain.ScanIssueKind.FromScan）は、新しい走査が見つけ直すので
// 持ち越さない。
func (s *ScanStore) ResumeScan(ctx context.Context, from int64) (scan domain.Scan, started bool, err error) {
	return s.startScan(ctx, from, domain.ScanOriginManual, nil)
}

// startScan は走査を始める。from が 0 でなければ、その走査の集合と仕事の段階の
// 問題を新しい走査へ持ち越す。origin が watch なら、dirs の外の問題も持ち越す。
func (s *ScanStore) startScan(
	ctx context.Context, from int64, origin domain.ScanOrigin, dirs []domain.DirtyDirectory,
) (scan domain.Scan, started bool, err error) {
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

	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.Scan{}, false, fmt.Errorf("cannot start the scan: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx,
		`insert into scans (state, origin, started_at, total, completed, failed) values ('running', ?, ?, 0, 0, 0)`,
		string(origin), time.Now().Unix())
	if err != nil {
		return domain.Scan{}, false, fmt.Errorf("cannot start the scan: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return domain.Scan{}, false, fmt.Errorf("cannot start the scan: %w", err)
	}
	if err := addScanVideosWithRemainingJobs(ctx, tx); err != nil {
		return domain.Scan{}, false, err
	}
	if from != 0 {
		if err := carryInterruptedImport(ctx, tx, from, id); err != nil {
			return domain.Scan{}, false, err
		}
	}
	if _, err := tx.ExecContext(ctx, `delete from scan_videos where scan_id <> ?`, id); err != nil {
		return domain.Scan{}, false, fmt.Errorf("cannot clear the previous import's videos: %w", err)
	}
	// 手動の走査は、前の走査の問題を新しい取り込みに持ち越さない（research.md R-3）。
	// 監視の走査は、自分が読み直さない範囲の問題を持ち越す。
	if origin == domain.ScanOriginWatch {
		if err := carryWatchIssues(ctx, tx, id, dirs); err != nil {
			return domain.Scan{}, false, err
		}
	}
	cleared, err := tx.ExecContext(ctx, `delete from scan_issues where scan_id <> ?`, id)
	if err != nil {
		return domain.Scan{}, false, fmt.Errorf("cannot clear the previous import's issues: %w", err)
	}
	if n, err := cleared.RowsAffected(); err != nil {
		return domain.Scan{}, false, fmt.Errorf("cannot clear the previous import's issues: %w", err)
	} else if n > 0 {
		if err := bumpIssuesRevision(ctx, tx); err != nil {
			return domain.Scan{}, false, err
		}
	}
	var c changes
	c.remainingChanged()
	if err := s.db.commit(ctx, tx, &c); err != nil {
		return domain.Scan{}, false, fmt.Errorf("cannot start the scan: %w", err)
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
		return fmt.Errorf("cannot record scan progress (id=%d): %w", id, err)
	}
	return nil
}

// FinishScan は走査を終える。cause は走査そのものが失敗した理由（成功なら nil）で、
// 個別のファイルの失敗はここではなく failed の数に入る。
//
// cause があれば、その文を error に、domain.ScanFailure で包まれた理由のコードと場所を
// error_code・error_path に書く。包まれていない失敗は internal である
// （specs/023-english-i18n/data-model.md §2）。
func (s *ScanStore) FinishScan(ctx context.Context, id int64, state domain.ScanState, cause error) error {
	var reason, code, path any
	if cause != nil {
		failureCode, failurePath := domain.ScanFailureOf(cause)
		reason = nullableString(cause.Error())
		code = nullableString(string(failureCode))
		path = nullableString(failurePath)
	}
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot record the end of the scan (id=%d): %w", id, err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx,
		`update scans set state = ?, finished_at = ?, error = ?, error_code = ?, error_path = ? where id = ?`,
		string(state), time.Now().Unix(), reason, code, path, id); err != nil {
		return fmt.Errorf("cannot record the end of the scan (id=%d): %w", id, err)
	}
	// 全パスを見終えた走査だけが、同じパスの中身の後継の記録を判定してよい状態にする
	// （specs/030-video-versions/data-model.md §5）。failed で閉じた走査は見ていないパスに
	// 前の中身が残りうるので触らず、次に done で閉じる走査に任せる。
	//
	// 監視の走査は読み直したディレクトリしか見ていないので、done でも判定しない。後継の
	// 判定は done で閉じた手動の走査だけが行う（specs/042-folder-watch-import/research.md R-9）。
	var succeeded []int64
	if state == domain.ScanDone {
		var origin string
		if err := tx.QueryRowContext(ctx, `select origin from scans where id = ?`, id).Scan(&origin); err != nil {
			return fmt.Errorf("cannot record the end of the scan (id=%d): %w", id, err)
		}
		if domain.ScanOrigin(origin) == domain.ScanOriginManual {
			if succeeded, err = applyReadySuccessions(ctx, tx); err != nil {
				return fmt.Errorf("cannot record the end of the scan (id=%d): %w", id, err)
			}
		}
	}
	// 走査が閉じると、対象に残りの仕事が無ければ取り込みは済む。
	var c changes
	c.remainingChanged()
	if err := s.db.commit(ctx, tx, &c); err != nil {
		return fmt.Errorf("cannot record the end of the scan (id=%d): %w", id, err)
	}
	s.db.publishEvents(successionEvents(succeeded)...)
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
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("cannot close interrupted scans: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, `
		update scans
		   set state = 'failed', finished_at = ?, error = ?, error_code = ?, error_path = null
		 where state = 'running'`,
		time.Now().Unix(), "The application stopped before the scan finished.", string(domain.ScanErrorInterrupted))
	if err != nil {
		return 0, fmt.Errorf("cannot close interrupted scans: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("cannot close interrupted scans: %w", err)
	}
	var c changes
	if affected > 0 {
		c.remainingChanged()
	}
	if err := s.db.commit(ctx, tx, &c); err != nil {
		return 0, fmt.Errorf("cannot close interrupted scans: %w", err)
	}
	return affected, nil
}

// scanColumns は Scan を組み立てるのに要る列である。並びは scanBy の Scan と対応させる。
// 対象の動画の本数と、そのうち残りの仕事が無い本数は、読み出しのたびに仕事の状態から
// 数える（specs/024-import-progress/research.md R-1）。
var scanColumns = `id, state, origin, started_at, finished_at, total, completed, failed, error, error_code, error_path, settled_at, issues_revision,
	(select count(*) from scan_videos sv where sv.scan_id = scans.id),
	(select count(*) from scan_videos sv where sv.scan_id = scans.id and not exists (
		select 1 from jobs j where j.video_id = sv.video_id and ` + remainingJobCondition("j") + `))`

func (s *ScanStore) scanByID(ctx context.Context, id int64) (domain.Scan, error) {
	return s.scanBy(ctx, `select `+scanColumns+` from scans where id = ?`, id)
}

func (s *ScanStore) scanBy(ctx context.Context, query string, args ...any) (domain.Scan, error) {
	var (
		scan                  domain.Scan
		state, origin         string
		startedAt, finishedAt sql.NullInt64
		settledAt             sql.NullInt64
		reason, code, path    sql.NullString
	)

	err := s.db.sql.QueryRowContext(ctx, query, args...).Scan(
		&scan.ID, &state, &origin, &startedAt, &finishedAt,
		&scan.Total, &scan.Completed, &scan.Failed, &reason, &code, &path, &settledAt, &scan.IssuesRevision,
		&scan.Videos, &scan.SettledVideos,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Scan{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Scan{}, fmt.Errorf("cannot read the scan record: %w", err)
	}

	scan.State = domain.ScanState(state)
	scan.Origin = domain.ScanOrigin(origin)
	if startedAt.Valid {
		scan.StartedAt = time.Unix(startedAt.Int64, 0)
	}
	if finishedAt.Valid {
		scan.FinishedAt = time.Unix(finishedAt.Int64, 0)
	}
	if settledAt.Valid {
		scan.SettledAt = time.Unix(settledAt.Int64, 0)
	}
	scan.Error = reason.String
	scan.ErrorCode = domain.ScanErrorCode(code.String)
	scan.ErrorPath = path.String
	return scan, nil
}

// carryInterruptedImport は中断した走査 from の対象の動画の集合と、仕事の段階の
// 問題を走査 to へ写す（ResumeScan）。
func carryInterruptedImport(ctx context.Context, q queryExecer, from, to int64) error {
	if _, err := q.ExecContext(ctx, `insert into scan_videos (scan_id, video_id)
		select ?, video_id from scan_videos where scan_id = ?
		on conflict (scan_id, video_id) do nothing`, to, from); err != nil {
		return fmt.Errorf("cannot carry the interrupted import's videos: %w", err)
	}
	fromScan := []any{}
	for _, kind := range domain.ScanIssueKinds() {
		if kind.FromScan() {
			fromScan = append(fromScan, string(kind))
		}
	}
	args := append([]any{to, from}, fromScan...)
	if _, err := q.ExecContext(ctx, `insert or ignore into scan_issues (scan_id, video_id, path, kind, created_at)
		select ?, video_id, path, kind, created_at from scan_issues
		where scan_id = ? and kind not in (`+strings.TrimSuffix(strings.Repeat("?,", len(fromScan)), ",")+`)`, args...); err != nil {
		return fmt.Errorf("cannot carry the interrupted import's issues: %w", err)
	}
	return nil
}

// carryWatchIssues は前の走査の問題のうち、監視の走査 to が読み直さないものを to へ付け替える。
// 付け替えないと、前の走査の失敗が一覧から消える。
//
// 付け替えるのは、パスが dirs の読む範囲の外にあるものと、仕事の段階の問題（走査が
// 見つけ直す種類ではないもの）である。段階の問題は、変わらないファイルでは仕事が積み
// 直されないので、読み直しても見つからず、消すと取り込みが済んだように見える
// （carryInterruptedImport と同じ理由）。範囲の中の、走査が見つける種類の問題は消し、
// 走査が見つけ直す。
func carryWatchIssues(ctx context.Context, q queryExecer, to int64, dirs []domain.DirtyDirectory) error {
	keep, err := issuesToCarry(ctx, q, to, dirs)
	if err != nil {
		return err
	}
	for _, id := range keep {
		if _, err := q.ExecContext(ctx, `update scan_issues set scan_id = ? where id = ?`, to, id); err != nil {
			return fmt.Errorf("cannot carry the previous import's issues: %w", err)
		}
	}
	return nil
}

// issuesToCarry は carryWatchIssues が走査 to へ付け替える問題の id を返す。
func issuesToCarry(ctx context.Context, q queryExecer, to int64, dirs []domain.DirtyDirectory) ([]int64, error) {
	rows, err := q.QueryContext(ctx, `select id, path, kind from scan_issues where scan_id <> ?`, to)
	if err != nil {
		return nil, fmt.Errorf("cannot read the previous import's issues: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var keep []int64
	for rows.Next() {
		var id int64
		var path, kind string
		if err := rows.Scan(&id, &path, &kind); err != nil {
			return nil, fmt.Errorf("cannot read the previous import's issues: %w", err)
		}
		if !domain.ScanIssueKind(kind).FromScan() || !domain.DirtyDirectoriesContain(dirs, path) {
			keep = append(keep, id)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the previous import's issues: %w", err)
	}
	return keep, nil
}
