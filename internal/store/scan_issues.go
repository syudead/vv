package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// Advance issue revision for every issue change, including changes that preserve the count.
func bumpIssuesRevision(ctx context.Context, q queryExecer) error {
	if _, err := q.ExecContext(ctx, `update scans set issues_revision = issues_revision + 1
		where id = (select max(id) from scans)`); err != nil {
		return fmt.Errorf("cannot advance the import issue revision: %w", err)
	}
	return nil
}

// Location changes alter issue labels and folders; mark issuesChanged when an affected video has issues.
func videosHaveIssues(ctx context.Context, q queryExecer, videoIDs ...int64) (bool, error) {
	if len(videoIDs) == 0 {
		return false, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(videoIDs)), ",")
	args := make([]any, 0, len(videoIDs))
	for _, id := range videoIDs {
		args = append(args, id)
	}
	rows, err := q.QueryContext(ctx, `select 1 from scan_issues
		where scan_id = (select max(id) from scans) and video_id in (`+placeholders+`) limit 1`, args...)
	if err != nil {
		return false, fmt.Errorf("cannot read the import issues: %w", err)
	}
	defer func() { _ = rows.Close() }()
	found := rows.Next()
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("cannot read the import issues: %w", err)
	}
	return found, nil
}

// Deduplicate issues by video or unregistered path and kind; no scan means no record, and zero video ID means unregistered.
func recordScanIssue(
	ctx context.Context, q queryExecer, videoID int64, path string, kind domain.ScanIssueKind, now int64,
) error {
	var video any
	if videoID != 0 {
		video = videoID
	}
	// A missing video is recorded as an unregistered file.
	res, err := q.ExecContext(ctx, `insert into scan_issues (scan_id, video_id, path, kind, created_at)
		select s.id, (select id from videos where id = ?), ?, ?, ? from (select max(id) as id from scans) s
		where s.id is not null
		on conflict do nothing`, video, path, string(kind), now)
	if err != nil {
		return fmt.Errorf("cannot record the import issue (%s, %s): %w", kind, path, err)
	}
	inserted, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("cannot record the import issue (%s, %s): %w", kind, path, err)
	}
	if inserted == 0 {
		return nil
	}
	return bumpIssuesRevision(ctx, q)
}

// Clear stage issues in the successful-result transaction; advance revision only if rows were removed.
func clearScanIssues(ctx context.Context, q queryExecer, videoID int64, kinds ...domain.ScanIssueKind) error {
	_, err := removeScanIssues(ctx, q, videoID, kinds...)
	return err
}

// Report whether anything changed so writes outside job completion can emit a scan event.
func removeScanIssues(ctx context.Context, q queryExecer, videoID int64, kinds ...domain.ScanIssueKind) (bool, error) {
	if len(kinds) == 0 {
		return false, nil
	}
	args := []any{videoID}
	for _, kind := range kinds {
		args = append(args, string(kind))
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(kinds)), ",")
	res, err := q.ExecContext(ctx, `delete from scan_issues
		where scan_id = (select max(id) from scans) and video_id = ? and kind in (`+placeholders+`)`, args...)
	if err != nil {
		return false, fmt.Errorf("cannot clear the import issues (video=%d): %w", videoID, err)
	}
	deleted, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("cannot clear the import issues (video=%d): %w", videoID, err)
	}
	if deleted == 0 {
		return false, nil
	}
	return true, bumpIssuesRevision(ctx, q)
}

func clearFailedIssue(ctx context.Context, q queryExecer, kind domain.JobKind, videoID int64) error {
	issue, ok := domain.FailedIssueKind(kind)
	if !ok {
		return nil
	}
	return clearScanIssues(ctx, q, videoID, issue)
}

// Successful substitutions add issues, full regeneration clears them, and unknown provenance leaves them unchanged.
func applySubstitution(
	ctx context.Context, q queryExecer, kind domain.JobKind, job domain.Job, substitution domain.Substitution,
) error {
	issue, ok := domain.SubstitutedIssueKind(kind)
	if !ok {
		return nil
	}
	switch substitution {
	case domain.SubstitutionUsed:
		return recordScanIssue(ctx, q, job.VideoID, job.LocationPath, issue, time.Now().Unix())
	case domain.SubstitutionNone:
		return clearScanIssues(ctx, q, job.VideoID, issue)
	case domain.SubstitutionUnknown:
	}
	return nil
}

func (s *ScanStore) RecordScanIssue(ctx context.Context, issue domain.ScanFileIssue) error {
	if !issue.Kind.Valid() || !issue.Kind.FromScan() {
		return fmt.Errorf("not a scan issue kind: %q", issue.Kind)
	}
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot record the import issue: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := recordScanIssue(ctx, tx, issue.VideoID, issue.Path, issue.Kind, time.Now().Unix()); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("cannot record the import issue: %w", err)
	}
	return nil
}

// Use the smallest registered path for each video; no registered location yields an empty path.
func (s *ScanStore) ScanIssueRecords(ctx context.Context, scanID int64) ([]domain.ScanIssueRecord, error) {
	rows, err := s.db.sql.QueryContext(ctx, `
		select i.video_id,
		       case when i.video_id is null then i.path else (
		           select l.path from video_locations l
		            where l.video_id = i.video_id and `+registeredLocationCondition("l")+`
		            order by l.path limit 1) end,
		       group_concat(i.kind, ','),
		       exists (select 1 from scan_videos sv where sv.scan_id = i.scan_id and sv.video_id = i.video_id)
		  from scan_issues i
		 where i.scan_id = ?
		 group by i.scan_id, coalesce(i.video_id, i.path)`, scanID)
	if err != nil {
		return nil, fmt.Errorf("cannot read the import issues: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []domain.ScanIssueRecord{}
	for rows.Next() {
		var (
			videoID sql.NullInt64
			path    sql.NullString
			kinds   string
			inSet   bool
		)
		if err := rows.Scan(&videoID, &path, &kinds, &inSet); err != nil {
			return nil, fmt.Errorf("cannot read the import issues: %w", err)
		}
		record := domain.ScanIssueRecord{VideoID: videoID.Int64, Path: path.String, InImport: inSet}
		for kind := range strings.SplitSeq(kinds, ",") {
			record.Kinds = append(record.Kinds, domain.ScanIssueKind(kind))
		}
		out = append(out, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the import issues: %w", err)
	}
	return out, nil
}

// ScanIssues excludes locations outside all registered folders.
func (s *ScanStore) ScanIssues(ctx context.Context, scanID int64) ([]domain.ScanIssue, error) {
	records, err := s.ScanIssueRecords(ctx, scanID)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return []domain.ScanIssue{}, nil
	}
	roots, err := listMediaFolders(ctx, s.db.sql)
	if err != nil {
		return nil, err
	}
	return domain.ScanIssues(roots, records), nil
}
