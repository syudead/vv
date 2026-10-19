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
	return s.startScan(ctx, scanStart{origin: domain.ScanOriginManual})
}

// StartWatchScan carries issues outside the watched directories; manual scans clear previous scan issues.
func (s *ScanStore) StartWatchScan(ctx context.Context, dirs []domain.DirtyDirectory) (scan domain.Scan, started bool, err error) {
	return s.startScan(ctx, scanStart{origin: domain.ScanOriginWatch, dirs: dirs})
}

// ResumeScan carries stage failures and substitutions so terminal jobs remain visible; scan-discovered issues are rediscovered instead.
// A resumed force rescan keeps its scope and the marks of the videos it already forced.
func (s *ScanStore) ResumeScan(ctx context.Context, from int64) (scan domain.Scan, started bool, err error) {
	return s.startScan(ctx, scanStart{from: from, origin: domain.ScanOriginManual})
}

// StartRescan starts a manual scan carrying a force rescan scope, recording the folder's current path.
// It returns ErrScanRunning instead of reusing a running scan; superseding a watch scan is the caller's job.
func (s *ScanStore) StartRescan(ctx context.Context, scope domain.RescanScope) (domain.Scan, error) {
	scan, _, err := s.startScan(ctx, scanStart{origin: domain.ScanOriginManual, rescan: &scope, exclusive: true})
	return scan, err
}

type scanStart struct {
	from   int64
	origin domain.ScanOrigin
	dirs   []domain.DirtyDirectory
	rescan *domain.RescanScope
	// exclusive refuses a running scan instead of returning it.
	exclusive bool
}

func (s *ScanStore) startScan(ctx context.Context, start scanStart) (scan domain.Scan, started bool, err error) {
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
		if start.exclusive {
			return domain.Scan{}, false, domain.ErrScanRunning
		}
		return running, false, nil
	} else if !errors.Is(err, domain.ErrNotFound) {
		return domain.Scan{}, false, err
	}
	if start.rescan, err = s.rescanToStart(ctx, start); err != nil {
		return domain.Scan{}, false, err
	}

	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.Scan{}, false, fmt.Errorf("cannot start the scan: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	kind, folderID, path, videoID := rescanColumns(start.rescan)
	res, err := tx.ExecContext(ctx,
		`insert into scans (state, origin, started_at, total, completed, failed,
			rescan_scope, rescan_folder_id, rescan_path, rescan_video_id)
		values ('running', ?, ?, 0, 0, 0, ?, ?, ?, ?)`,
		string(start.origin), time.Now().Unix(), kind, folderID, path, videoID)
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
	if start.from != 0 {
		if err := carryInterruptedImport(ctx, tx, start.from, id); err != nil {
			return domain.Scan{}, false, err
		}
	}
	if _, err := tx.ExecContext(ctx, `delete from scan_videos where scan_id <> ?`, id); err != nil {
		return domain.Scan{}, false, fmt.Errorf("cannot clear the previous import's videos: %w", err)
	}
	if err := carryPreviousIssues(ctx, tx, id, start); err != nil {
		return domain.Scan{}, false, err
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

// rescanToStart returns the scope the new scan records: the interrupted scan's on resume, or the
// requested one with the folder path read under folderMu, so it is the path the scanner walks.
func (s *ScanStore) rescanToStart(ctx context.Context, start scanStart) (*domain.RescanScope, error) {
	if start.from != 0 {
		from, err := s.scanByID(ctx, start.from)
		if err != nil {
			return nil, err
		}
		return from.Rescan, nil
	}
	if start.rescan == nil {
		return nil, nil
	}
	scope := domain.RescanScope{Kind: start.rescan.Kind}
	switch scope.Kind {
	case domain.RescanLibrary:
	case domain.RescanFolder:
		scope.FolderID = start.rescan.FolderID
		err := s.db.sql.QueryRowContext(ctx,
			`select path from media_folders where id = ?`, scope.FolderID).Scan(&scope.Path)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		if err != nil {
			return nil, fmt.Errorf("cannot read the media folder (id=%d): %w", scope.FolderID, err)
		}
	case domain.RescanVideo:
		scope.VideoID = start.rescan.VideoID
		var exists bool
		if err := s.db.sql.QueryRowContext(ctx,
			`select exists (select 1 from videos where id = ?)`, scope.VideoID).Scan(&exists); err != nil {
			return nil, fmt.Errorf("cannot read the video (id=%d): %w", scope.VideoID, err)
		}
		if !exists {
			return nil, domain.ErrNotFound
		}
	default:
		return nil, fmt.Errorf("%w: %q", domain.ErrInvalidRescanScope, scope.Kind)
	}
	return &scope, nil
}

func rescanColumns(scope *domain.RescanScope) (kind, folderID, path, videoID any) {
	if scope == nil {
		return nil, nil, nil, nil
	}
	switch scope.Kind {
	case domain.RescanFolder:
		folderID, path = scope.FolderID, scope.Path
	case domain.RescanVideo:
		videoID = scope.VideoID
	case domain.RescanLibrary:
	}
	return string(scope.Kind), folderID, path, videoID
}

// carryPreviousIssues keeps what the new scan will not rediscover and clears the rest: a watch scan
// keeps issues outside its directories, a force rescan those outside its scope.
func carryPreviousIssues(ctx context.Context, q queryExecer, id int64, start scanStart) error {
	if start.origin == domain.ScanOriginWatch {
		if err := carryWatchIssues(ctx, q, id, start.dirs); err != nil {
			return err
		}
	}
	if start.rescan != nil {
		if err := carryRescanIssues(ctx, q, id, *start.rescan); err != nil {
			return err
		}
	}
	cleared, err := q.ExecContext(ctx, `delete from scan_issues where scan_id <> ?`, id)
	if err != nil {
		return fmt.Errorf("cannot clear the previous import's issues: %w", err)
	}
	if n, err := cleared.RowsAffected(); err != nil {
		return fmt.Errorf("cannot clear the previous import's issues: %w", err)
	} else if n > 0 {
		return bumpIssuesRevision(ctx, q)
	}
	return nil
}

// MarkForced records that the force pass queued the video's jobs in the scan.
func (s *ScanStore) MarkForced(ctx context.Context, scanID, videoID int64) error {
	return markForced(ctx, s.db.sql, scanID, videoID)
}

// ForcedVideos lists, in id order, the videos the scan has already forced.
func (s *ScanStore) ForcedVideos(ctx context.Context, scanID int64) ([]int64, error) {
	return forcedVideos(ctx, s.db.sql, scanID)
}

func markForced(ctx context.Context, q queryExecer, scanID, videoID int64) error {
	if _, err := q.ExecContext(ctx, `insert into scan_videos (scan_id, video_id, forced)
		select s.id, v.id, 1 from scans s, videos v where s.id = ? and v.id = ?
		on conflict (scan_id, video_id) do update set forced = 1`, scanID, videoID); err != nil {
		return fmt.Errorf("cannot mark the video as forced (scan=%d, video=%d): %w", scanID, videoID, err)
	}
	return nil
}

func forcedVideos(ctx context.Context, q rowsQueryer, scanID int64) ([]int64, error) {
	rows, err := q.QueryContext(ctx,
		`select video_id from scan_videos where scan_id = ? and forced = 1 order by video_id`, scanID)
	if err != nil {
		return nil, fmt.Errorf("cannot read the forced videos (scan=%d): %w", scanID, err)
	}
	defer func() { _ = rows.Close() }()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("cannot read the forced videos (scan=%d): %w", scanID, err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the forced videos (scan=%d): %w", scanID, err)
	}
	return ids, nil
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
	// Only successful full manual scans may resolve succession: failed, watched, folder or video scans may not have
	// visited paths containing old content.
	var origin string
	var rescanScope sql.NullString
	if err := tx.QueryRowContext(ctx, `select origin, rescan_scope from scans where id = ?`, id).
		Scan(&origin, &rescanScope); err != nil {
		return fmt.Errorf("cannot record the end of the scan (id=%d): %w", id, err)
	}
	fullScan := domain.ScanOrigin(origin) == domain.ScanOriginManual &&
		(!rescanScope.Valid || domain.RescanScopeKind(rescanScope.String) == domain.RescanLibrary)
	var succeeded succession
	if state == domain.ScanDone && fullScan {
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
	rescan_scope, rescan_folder_id, rescan_path, rescan_video_id,
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
		rescan                nullableRescan
	)

	err := s.db.sql.QueryRowContext(ctx, query, args...).Scan(
		&scan.ID, &state, &origin, &startedAt, &finishedAt,
		&scan.Total, &scan.Completed, &scan.Failed, &reason, &code, &path, &settledAt, &scan.IssuesRevision,
		&rescan.kind, &rescan.folderID, &rescan.path, &rescan.videoID,
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
	scan.Rescan = rescan.scope()
	return scan, nil
}

type nullableRescan struct {
	kind, path        sql.NullString
	folderID, videoID sql.NullInt64
}

func (r nullableRescan) scope() *domain.RescanScope {
	if !r.kind.Valid {
		return nil
	}
	return &domain.RescanScope{
		Kind: domain.RescanScopeKind(r.kind.String), FolderID: r.folderID.Int64, Path: r.path.String, VideoID: r.videoID.Int64,
	}
}

func carryInterruptedImport(ctx context.Context, q queryExecer, from, to int64) error {
	if _, err := q.ExecContext(ctx, `insert into scan_videos (scan_id, video_id, forced)
		select ?, video_id, forced from scan_videos where scan_id = ?
		on conflict (scan_id, video_id) do update set forced = excluded.forced`, to, from); err != nil {
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
	keep, err := issuesToCarry(ctx, q, to, func(_ int64, path, kind string) bool {
		return !domain.ScanIssueKind(kind).FromScan() || !domain.DirtyDirectoriesContain(dirs, path)
	})
	if err != nil {
		return err
	}
	return moveIssues(ctx, q, to, keep)
}

// Carry every issue outside the rescan scope; a resume has already copied the stage issues inside it.
func carryRescanIssues(ctx context.Context, q queryExecer, to int64, scope domain.RescanScope) error {
	keep, err := issuesToCarry(ctx, q, to, func(videoID int64, path, _ string) bool {
		return scope.CarriesIssue(videoID, path)
	})
	if err != nil {
		return err
	}
	return moveIssues(ctx, q, to, keep)
}

func moveIssues(ctx context.Context, q queryExecer, to int64, ids []int64) error {
	for _, id := range ids {
		// A resumed scan may already hold the same issue; the duplicate is cleared with the rest.
		if _, err := q.ExecContext(ctx, `update or ignore scan_issues set scan_id = ? where id = ?`, to, id); err != nil {
			return fmt.Errorf("cannot carry the previous import's issues: %w", err)
		}
	}
	return nil
}

func issuesToCarry(
	ctx context.Context, q queryExecer, to int64, carries func(videoID int64, path, kind string) bool,
) ([]int64, error) {
	rows, err := q.QueryContext(ctx,
		`select id, coalesce(video_id, 0), path, kind from scan_issues where scan_id <> ?`, to)
	if err != nil {
		return nil, fmt.Errorf("cannot read the previous import's issues: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var keep []int64
	for rows.Next() {
		var id, videoID int64
		var path, kind string
		if err := rows.Scan(&id, &videoID, &path, &kind); err != nil {
			return nil, fmt.Errorf("cannot read the previous import's issues: %w", err)
		}
		if carries(videoID, path, kind) {
			keep = append(keep, id)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the previous import's issues: %w", err)
	}
	return keep, nil
}
