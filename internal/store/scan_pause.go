package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/syudead/vv/internal/domain"
)

// PauseScan closes the running scan id as paused for a pause of background work. The row keeps its scope, counters,
// video set, issues and forced marks, and finished_at stays null.
func (s *ScanStore) PauseScan(ctx context.Context, id int64) error {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot pause the scan (id=%d): %w", id, err)
	}
	defer func() { _ = tx.Rollback() }()
	var state string
	err = tx.QueryRowContext(ctx, `select state from scans where id = ?`, id).Scan(&state)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("cannot pause the scan (id=%d): %w", id, err)
	}
	if err := domain.CheckScanPause(domain.ScanState(state)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `update scans set state = 'paused' where id = ?`, id); err != nil {
		return fmt.Errorf("cannot pause the scan (id=%d): %w", id, err)
	}
	var c changes
	c.remainingChanged()
	if err := s.db.commit(ctx, tx, &c); err != nil {
		return fmt.Errorf("cannot pause the scan (id=%d): %w", id, err)
	}
	return nil
}

// ReopenScan sets the paused scan id back to running for the resume of background work, and deletes the scan-sourced
// issues its scope covers so that the resumed pass reports again only the files that still fail. It returns
// ErrScanRunning while another scan runs.
func (s *ScanStore) ReopenScan(ctx context.Context, id int64) (domain.Scan, error) {
	s.db.folderMu.Lock()
	defer s.db.folderMu.Unlock()
	scan, err := s.scanByID(ctx, id)
	if err != nil {
		return domain.Scan{}, err
	}
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.Scan{}, fmt.Errorf("cannot reopen the scan (id=%d): %w", id, err)
	}
	defer func() { _ = tx.Rollback() }()
	var state string
	var latest, running bool
	if err := tx.QueryRowContext(ctx, `select state, id = (select max(id) from scans),
			exists (select 1 from scans where state = 'running') from scans where id = ?`, id).
		Scan(&state, &latest, &running); err != nil {
		return domain.Scan{}, fmt.Errorf("cannot reopen the scan (id=%d): %w", id, err)
	}
	if err := domain.CheckScanReopen(domain.ScanState(state), latest, running); err != nil {
		return domain.Scan{}, err
	}
	if err := dropReopenedIssues(ctx, tx, scan); err != nil {
		return domain.Scan{}, err
	}
	if _, err := tx.ExecContext(ctx, `update scans set state = 'running' where id = ?`, id); err != nil {
		return domain.Scan{}, fmt.Errorf("cannot reopen the scan (id=%d): %w", id, err)
	}
	var c changes
	c.remainingChanged()
	if err := s.db.commit(ctx, tx, &c); err != nil {
		return domain.Scan{}, fmt.Errorf("cannot reopen the scan (id=%d): %w", id, err)
	}
	return s.scanByID(ctx, id)
}

func dropReopenedIssues(ctx context.Context, q queryExecer, scan domain.Scan) error {
	drop, err := reopenedIssues(ctx, q, scan)
	if err != nil {
		return err
	}
	for _, id := range drop {
		if _, err := q.ExecContext(ctx, `delete from scan_issues where id = ?`, id); err != nil {
			return fmt.Errorf("cannot delete the paused scan's issue (id=%d): %w", id, err)
		}
	}
	if len(drop) == 0 {
		return nil
	}
	return bumpIssuesRevision(ctx, q)
}

func reopenedIssues(ctx context.Context, q queryExecer, scan domain.Scan) ([]int64, error) {
	rows, err := q.QueryContext(ctx,
		`select id, coalesce(video_id, 0), path, kind from scan_issues where scan_id = ?`, scan.ID)
	if err != nil {
		return nil, fmt.Errorf("cannot read the paused scan's issues (id=%d): %w", scan.ID, err)
	}
	defer func() { _ = rows.Close() }()
	var drop []int64
	for rows.Next() {
		var id, videoID int64
		var path, kind string
		if err := rows.Scan(&id, &videoID, &path, &kind); err != nil {
			return nil, fmt.Errorf("cannot read the paused scan's issues (id=%d): %w", scan.ID, err)
		}
		if scan.ReopenDropsIssue(videoID, path, domain.ScanIssueKind(kind)) {
			drop = append(drop, id)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the paused scan's issues (id=%d): %w", scan.ID, err)
	}
	return drop, nil
}
