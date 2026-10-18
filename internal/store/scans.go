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

// StartScan reuses a running scan and atomically carries unfinished work into each new scan.
func (s *ScanStore) StartScan(ctx context.Context) (scan domain.Scan, started bool, err error) {
	return s.startScan(ctx, 0, domain.ScanOriginManual, nil)
}

// StartWatchScan carries issues outside the watched directories; manual scans clear previous scan issues.
func (s *ScanStore) StartWatchScan(ctx context.Context, dirs []domain.DirtyDirectory) (scan domain.Scan, started bool, err error) {
	return s.startScan(ctx, 0, domain.ScanOriginWatch, dirs)
}

// ResumeScan carries stage failures and substitutions so terminal jobs remain visible; scan-discovered issues are rediscovered instead.
func (s *ScanStore) ResumeScan(ctx context.Context, from int64) (scan domain.Scan, started bool, err error) {
	return s.startScan(ctx, from, domain.ScanOriginManual, nil)
}

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

func (s *ScanStore) UpdateScanProgress(ctx context.Context, id int64, progress domain.ScanProgress) error {
	_, err := s.db.sql.ExecContext(ctx,
		`update scans set total = ?, completed = ?, failed = ? where id = ?`,
		progress.Total, progress.Completed, progress.Failed, id)
	if err != nil {
		return fmt.Errorf("cannot record scan progress (id=%d): %w", id, err)
	}
	return nil
}

// FinishScan records scan-level failure separately from per-file failures, preserving structured reason and path when available.
func (s *ScanStore) FinishScan(
	ctx context.Context, id int64, state domain.ScanState, cause error, decider domain.ActivityDecider,
) error {
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
	// Only successful manual scans may resolve succession: failed or watched scans may not have visited paths containing old content.
	var origin string
	if err := tx.QueryRowContext(ctx, `select origin from scans where id = ?`, id).Scan(&origin); err != nil {
		return fmt.Errorf("cannot record the end of the scan (id=%d): %w", id, err)
	}
	var succeeded succession
	if state == domain.ScanDone && domain.ScanOrigin(origin) == domain.ScanOriginManual {
		if succeeded, err = applyReadySuccessions(ctx, tx); err != nil {
			return fmt.Errorf("cannot record the end of the scan (id=%d): %w", id, err)
		}
	}
	var c changes
	if err := recordScanActivity(ctx, tx, &c, id, domain.ScanOrigin(origin), state, cause, decider, time.Now()); err != nil {
		return fmt.Errorf("cannot record the end of the scan (id=%d): %w", id, err)
	}
	c.remainingChanged()
	// Notify smart folders once when the scan closes, rather than after every changed file.
	c.matchesChanged()
	if err := s.db.commit(ctx, tx, &c); err != nil {
		return fmt.Errorf("cannot record the end of the scan (id=%d): %w", id, err)
	}
	s.db.publishEvents(successionEvents(succeeded)...)
	return nil
}

// CurrentScan returns the running or latest finished scan, or ErrNotFound before any scan.
func (s *ScanStore) CurrentScan(ctx context.Context) (domain.Scan, error) {
	return s.scanBy(ctx, `
		select `+scanColumns+` from scans
		 order by (state = 'running') desc, id desc
		 limit 1`)
}

// FailInterruptedScans releases the single-running-scan constraint at startup without assuming interrupted work succeeded.
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

// Read target and completed counts from current job state rather than cached totals.
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

// Carry issues outside watched scope and all stage issues; unchanged files will not rerun terminal jobs to rediscover failures.
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
