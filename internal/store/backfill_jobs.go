package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"

	"github.com/syudead/vv/internal/domain"
)

// isBackfillJob reports a kind that only refreshes stored results. It joins no import activity, scan settling or
// HasUnfinishedJobs, and its row is deleted when it ends, since the result lives in the domain tables.
func isBackfillJob(kind domain.JobKind) bool {
	return slices.Contains(domain.BackfillJobKinds, kind)
}

// isSubjectlessJob reports a kind whose job names no video or content key.
func isSubjectlessJob(kind domain.JobKind) bool {
	return slices.Contains(domain.StartupJobKinds, kind)
}

// EnqueueStartupJobs queues each of domain.StartupJobKinds in the backfill class, unless one of that kind is still
// unfinished from an earlier start.
func (s *IngestStore) EnqueueStartupJobs(ctx context.Context) error {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot queue the startup jobs: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	c := &changes{}
	now := jobNow().Unix()
	for _, kind := range domain.StartupJobKinds {
		res, err := tx.ExecContext(ctx, `insert into jobs (kind, state, attempts, priority, run_after, created_at, updated_at)
			select ?1, 'queued', 0, ?2, ?3, ?3, ?3
			where not exists (select 1 from jobs where kind = ?1 and state in ('queued', 'running'))`,
			string(kind), int(domain.JobPriorityBackfill), now)
		if err != nil {
			return fmt.Errorf("cannot queue the %s job: %w", kind, err)
		}
		if n, err := res.RowsAffected(); err != nil {
			return fmt.Errorf("cannot queue the %s job: %w", kind, err)
		} else if n > 0 {
			c.backfillQueued(kind)
		}
	}
	if err := s.db.commit(ctx, tx, c); err != nil {
		return fmt.Errorf("cannot queue the startup jobs: %w", err)
	}
	return nil
}

// queueBackfillJobs inserts a job of kind in class priority for each probed video whose first registered location,
// aliased l, meets condition, and returns how many it queued. A video with a probe pending is left to the import, and
// one whose job of kind is unfinished keeps that job.
func (s *IngestStore) queueBackfillJobs(
	ctx context.Context, kind domain.JobKind, priority domain.JobPriority, condition string, args ...any,
) (int, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("cannot queue %s jobs: %w", kind, err)
	}
	defer func() { _ = tx.Rollback() }()
	now := jobNow().Unix()
	res, err := tx.ExecContext(ctx, `insert into jobs (kind, video_id, state, attempts, priority, run_after,
			created_at, updated_at)
		select ?, v.id, 'queued', 0, ?, ?, ?, ?
		from videos v join video_locations l on l.video_id = v.id
		where v.probe_state = 'done' and `+registeredLocationCondition("l")+`
		and l.path = (select min(first.path) from video_locations first where first.video_id = v.id
			and `+registeredLocationCondition("first")+`)
		and not exists (select 1 from jobs j where j.video_id = v.id and j.state in ('queued', 'running')
			and j.kind in (?, ?))
		and `+condition+`
		order by v.id`,
		append([]any{string(kind), int(priority), now, now, now, string(domain.JobProbe), string(kind)}, args...)...)
	if err != nil {
		return 0, fmt.Errorf("cannot queue %s jobs: %w", kind, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("cannot queue %s jobs: %w", kind, err)
	}
	c := &changes{}
	if n > 0 {
		c.backfillQueued(kind)
	}
	if err := s.db.commit(ctx, tx, c); err != nil {
		return 0, fmt.Errorf("cannot queue %s jobs: %w", kind, err)
	}
	return int(n), nil
}

// takeJob marks the next due job of kind, in maxPriority or a more urgent class, running and counts the attempt. It
// suits kinds whose claim is one attempt with no location, and returns the job's subject key and activity entry.
func takeJob(
	ctx context.Context, q rowQueryer, kind domain.JobKind, maxPriority domain.JobPriority,
) (domain.Job, int64, error) {
	job := domain.Job{Kind: kind, StartsRound: true, LastLocation: true}
	var activityID int64
	err := q.QueryRowContext(ctx, `update jobs set state = 'running', attempts = attempts + 1, updated_at = ?1
		where id = (select id from jobs where kind = ?2 and state = 'queued' and run_after <= ?1 and priority <= ?3
			order by priority, run_after, id limit 1)
		returning id, coalesce(subject_key, ''), attempts, priority, coalesce(activity_id, 0)`,
		jobNow().Unix(), string(kind), int(maxPriority)).
		Scan(&job.ID, &job.ContentKey, &job.Attempts, &job.Priority, &activityID)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Job{}, 0, domain.ErrNoJob
	}
	if err != nil {
		return domain.Job{}, 0, fmt.Errorf("cannot claim a %s job: %w", kind, err)
	}
	return job, activityID, nil
}

// commitBackfillFailure commits a backfill job's failure, deleting the job when it failed for good: nothing records
// the failure, and the next start queues the work again.
func (s *IngestStore) commitBackfillFailure(ctx context.Context, tx *sql.Tx, job domain.Job, state domain.JobState) error {
	if state == domain.JobFailed {
		if _, err := tx.ExecContext(ctx, `delete from jobs where id = ?`, job.ID); err != nil {
			return fmt.Errorf("cannot end the failed job (id=%d): %w", job.ID, err)
		}
	}
	if err := s.db.commit(ctx, tx, &changes{}); err != nil {
		return fmt.Errorf("cannot commit the job failure (id=%d): %w", job.ID, err)
	}
	return nil
}

// completeBackfillJob deletes a backfill job that succeeded; its handler has already saved the result. A video's job
// whose claimed location or content changed during the read saved nothing, so it is queued again, as an import stage is.
func (s *IngestStore) completeBackfillJob(ctx context.Context, job domain.Job) error {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot start recording job completion (id=%d): %w", job.ID, err)
	}
	defer func() { _ = tx.Rollback() }()
	current := isSubjectlessJob(job.Kind)
	if !current {
		if current, err = jobIdentityCurrent(ctx, tx, job); err != nil {
			return fmt.Errorf("cannot check the job location (id=%d): %w", job.ID, err)
		}
	}
	c := &changes{}
	if current {
		_, err = tx.ExecContext(ctx, `delete from jobs where id = ? and state = 'running'`, job.ID)
	} else {
		_, err = tx.ExecContext(ctx, `update jobs set state = 'queued', updated_at = ? where id = ? and state = 'running'`,
			jobNow().Unix(), job.ID)
		c.backfillQueued(job.Kind)
	}
	if err != nil {
		return fmt.Errorf("cannot record job completion (id=%d): %w", job.ID, err)
	}
	if err := s.db.commit(ctx, tx, c); err != nil {
		return fmt.Errorf("cannot commit job completion (id=%d): %w", job.ID, err)
	}
	return nil
}
