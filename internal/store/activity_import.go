package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// Each import activity spans all five stages; jobs reference it and update its state in their own transaction.

func pendingJobCondition(alias string) string {
	return alias + `.state in ('queued', 'running')`
}

// Prefer the overridden title, then the first location by path.
func videoActivitySubject(ctx context.Context, q rowQueryer, videoID int64) (domain.ActivitySubject, error) {
	subject := domain.ActivitySubject{Kind: domain.ActivitySubjectVideo}
	err := q.QueryRowContext(ctx, `select v.content_key, coalesce(
			(select ov.display_name from video_overrides ov where ov.content_key = v.content_key and v.content_key <> ''),
			(select l.title from video_locations l where l.video_id = v.id order by l.path limit 1), '')
		from videos v where v.id = ?`, videoID).Scan(&subject.Key, &subject.Label)
	if err != nil {
		return domain.ActivitySubject{}, fmt.Errorf("cannot read the activity subject (video=%d): %w", videoID, err)
	}
	return subject, nil
}

// Attach pending jobs to one running import activity; create a new entry instead of reviving a finished one.
func joinImportActivity(
	ctx context.Context, q activityWriter, c *changes, videoID int64, decider domain.ActivityDecider, now time.Time,
) error {
	var id int64
	err := q.QueryRowContext(ctx, `select j.activity_id from jobs j join activity_entries e on e.id = j.activity_id
		where j.video_id = ? and `+pendingJobCondition("j")+` and e.status = ? order by j.id limit 1`,
		videoID, string(domain.ActivityRunning)).Scan(&id)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		var pending bool
		if err := q.QueryRowContext(ctx, `select exists (select 1 from jobs j where j.video_id = ? and `+
			pendingJobCondition("j")+`)`, videoID).Scan(&pending); err != nil {
			return fmt.Errorf("cannot read the pending jobs (video=%d): %w", videoID, err)
		}
		if !pending {
			return nil
		}
		subject, err := videoActivitySubject(ctx, q, videoID)
		if err != nil {
			return err
		}
		id, err = recordActivity(ctx, q, c, domain.ActivityEntry{
			Kind: domain.ActivityImport, Subject: subject, Status: domain.ActivityRunning,
			Reason: domain.ActivityWaiting, Decider: decider,
		}, now)
		if err != nil {
			return err
		}
	case err != nil:
		return fmt.Errorf("cannot read the running import activity (video=%d): %w", videoID, err)
	}
	if _, err := q.ExecContext(ctx, `update jobs set activity_id = ? where video_id = ? and `+
		pendingJobCondition("jobs")+` and activity_id is not ?`, id, videoID, id); err != nil {
		return fmt.Errorf("cannot attach the jobs to the import activity (video=%d): %w", videoID, err)
	}
	return settleImportActivity(ctx, q, c, id, now)
}

// Jobs queued before activity tracking have no entry to settle.
func settleJobActivity(ctx context.Context, q activityWriter, c *changes, jobID int64, now time.Time) error {
	var id sql.NullInt64
	err := q.QueryRowContext(ctx, `select activity_id from jobs where id = ?`, jobID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !id.Valid) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot read the job's activity (job=%d): %w", jobID, err)
	}
	return settleImportActivity(ctx, q, c, id.Int64, now)
}

// Only running entries are settled; the first failed job by ID supplies the failure detail.
func settleImportActivity(ctx context.Context, q activityWriter, c *changes, id int64, now time.Time) error {
	var current domain.ActivityState
	var status, reason string
	err := q.QueryRowContext(ctx, `select status, coalesce(reason, '') from activity_entries where id = ?`, id).
		Scan(&status, &reason)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot read the import activity (id=%d): %w", id, err)
	}
	current.Status, current.Reason = domain.ActivityStatus(status), domain.ActivityReason(reason)
	if current.Status != domain.ActivityRunning {
		return nil
	}
	jobs, failure, err := importActivityJobs(ctx, q, id)
	if err != nil {
		return err
	}
	states := make([]domain.JobState, 0, len(jobs))
	for _, job := range jobs {
		states = append(states, job.State)
	}
	next := domain.ImportActivityState(states)
	if next == current {
		return nil
	}
	if err := setActivityState(ctx, q, c, id, next, now); err != nil {
		return err
	}
	if next.Reason != domain.ActivityFailed {
		return nil
	}
	if _, err := q.ExecContext(ctx, `update activity_entries set stage = ?, detail = ? where id = ?`,
		string(failure.kind), nullString(failure.message), id); err != nil {
		return fmt.Errorf("cannot record the failed import stage (id=%d): %w", id, err)
	}
	return nil
}

const goneImportCondition = `e.kind = 'import' and e.status = 'running' and not exists (
	select 1 from jobs j where j.activity_id = e.id and j.state in ('queued', 'running'))`

// Close a running import as gone when its last pending job loses all registered locations.
func closeGoneImportActivity(ctx context.Context, q activityWriter, c *changes, id int64, now time.Time) (bool, error) {
	return closeGoneImports(ctx, q, c, `e.id = ?`, now, id)
}

// Deleted jobs cascade with their video, so close their activities by content key during commit.
func closeDeletedVideoImports(ctx context.Context, q activityWriter, c *changes, deleted []domain.DeletedVideo) error {
	if len(deleted) == 0 {
		return nil
	}
	keys := make([]string, 0, len(deleted))
	for _, video := range deleted {
		keys = append(keys, video.ContentKey)
	}
	encoded, err := json.Marshal(keys)
	if err != nil {
		return fmt.Errorf("cannot build the deleted content keys: %w", err)
	}
	_, err = closeGoneImports(ctx, q, c, `e.subject_key in (select value from json_each(?))`, time.Now(),
		string(encoded))
	return err
}

func closeGoneImports(
	ctx context.Context, q activityWriter, c *changes, where string, now time.Time, args ...any,
) (bool, error) {
	res, err := q.ExecContext(ctx, `update activity_entries as e set status = ?, reason = null, outcome = ?,
		updated_at = ? where `+where+` and `+goneImportCondition,
		append([]any{string(domain.ActivityDone), string(domain.ActivityGone), now.UnixMilli()}, args...)...)
	if err != nil {
		return false, fmt.Errorf("cannot close the gone import activity: %w", err)
	}
	closed, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("cannot close the gone import activity: %w", err)
	}
	if closed > 0 {
		c.activityChanged()
	}
	return closed > 0, nil
}

type importFailure struct {
	kind    domain.JobKind
	message string
}

func importActivityJobs(ctx context.Context, q queryExecer, id int64) ([]domain.ImportJobState, importFailure, error) {
	rows, err := q.QueryContext(ctx, `select kind, state, coalesce(last_error, '') from jobs where activity_id = ?
		order by id`, id)
	if err != nil {
		return nil, importFailure{}, fmt.Errorf("cannot read the import activity's jobs (id=%d): %w", id, err)
	}
	defer func() { _ = rows.Close() }()
	var jobs []domain.ImportJobState
	var failure importFailure
	for rows.Next() {
		var kind, state, message string
		if err := rows.Scan(&kind, &state, &message); err != nil {
			return nil, importFailure{}, fmt.Errorf("cannot read the import activity's jobs (id=%d): %w", id, err)
		}
		job := domain.ImportJobState{Kind: domain.JobKind(kind), State: domain.JobState(state)}
		if job.State == domain.JobFailed && failure.kind == "" {
			failure = importFailure{kind: job.Kind, message: message}
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, importFailure{}, fmt.Errorf("cannot read the import activity's jobs (id=%d): %w", id, err)
	}
	return jobs, failure, nil
}

func attachImportProgress(ctx context.Context, q rowsQueryer, entries []domain.ActivityEntry) error {
	index, encoded := activityEntryIDs(entries, func(entry domain.ActivityEntry) bool {
		return entry.Kind == domain.ActivityImport && entry.Status == domain.ActivityRunning &&
			entry.Reason == domain.ActivityProcessing
	})
	if len(index) == 0 {
		return nil
	}
	// Count completed stages across all jobs: retries retain earlier completed jobs on the previous activity.
	rows, err := q.QueryContext(ctx, `select a.activity_id, j.kind, j.state from jobs j join (
			select distinct activity_id, video_id from jobs where activity_id in (select value from json_each(?))
		) a on a.video_id = j.video_id order by j.id`, encoded)
	if err != nil {
		return fmt.Errorf("cannot read the import progress: %w", err)
	}
	defer func() { _ = rows.Close() }()
	jobs := map[int64][]domain.ImportJobState{}
	for rows.Next() {
		var id int64
		var kind, state string
		if err := rows.Scan(&id, &kind, &state); err != nil {
			return fmt.Errorf("cannot read the import progress: %w", err)
		}
		jobs[id] = append(jobs[id], domain.ImportJobState{Kind: domain.JobKind(kind), State: domain.JobState(state)})
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("cannot read the import progress: %w", err)
	}
	for id, i := range index {
		entries[i].Progress = domain.ImportActivityProgress(jobs[id])
	}
	return nil
}

// Record failed scans except startup interruptions; a successful scan supersedes open scan failures.
func recordScanActivity(
	ctx context.Context, tx activityWriter, c *changes, id int64, origin domain.ScanOrigin, state domain.ScanState,
	cause error, decider domain.ActivityDecider, now time.Time,
) error {
	if state == domain.ScanDone {
		return closeScanActivity(ctx, tx, c, now)
	}
	var detail string
	if cause != nil {
		if code, _ := domain.ScanFailureOf(cause); code == domain.ScanErrorInterrupted {
			return nil
		}
		detail = cause.Error()
	}
	_, err := recordActivity(ctx, tx, c, domain.ActivityEntry{
		Kind: domain.ActivityScan,
		Subject: domain.ActivitySubject{
			Kind: domain.ActivitySubjectScan, Key: strconv.FormatInt(id, 10), Label: string(origin),
		},
		Status: domain.ActivityAttention, Reason: domain.ActivityFailed, Detail: detail, Decider: decider,
	}, now)
	return err
}

func closeScanActivity(ctx context.Context, tx activityWriter, c *changes, now time.Time) error {
	res, err := tx.ExecContext(ctx, `update activity_entries set status = ?, reason = null, outcome = ?, updated_at = ?
		where kind = ? and status <> ?`, string(domain.ActivityDone), string(domain.ActivitySuperseded),
		now.UnixMilli(), string(domain.ActivityScan), string(domain.ActivityDone))
	if err != nil {
		return fmt.Errorf("cannot close the open scan activity: %w", err)
	}
	closed, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("cannot close the open scan activity: %w", err)
	}
	if closed > 0 {
		c.activityChanged()
	}
	return nil
}
