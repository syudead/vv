package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// subjectJobKind reports a kind whose job row is the whole of its queue state: it reads no source location, opens no
// activity entry and records no result, so a finished job is deleted.
func subjectJobKind(kind domain.JobKind) bool {
	switch kind {
	case domain.JobFaceImage, domain.JobFaceCast, domain.JobQueueFaceImages:
		return true
	default:
		return false
	}
}

// claimSubjectJob takes the claim's next due job in class, run-after and age order; each claim is one attempt.
func (db *DB) claimSubjectJob(ctx context.Context, claim domain.JobClaim) (domain.Job, error) {
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.Job{}, fmt.Errorf("cannot claim a %s job: %w", claim.Kind, err)
	}
	defer func() { _ = tx.Rollback() }()
	job, _, err := takeJob(ctx, tx, claim.Kind, claim.MaxPriority, string(claim.Kind))
	if err != nil {
		return domain.Job{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Job{}, fmt.Errorf("cannot claim a %s job: %w", claim.Kind, err)
	}
	return job, nil
}

// completeSubjectJob deletes the finished job.
func (db *DB) completeSubjectJob(ctx context.Context, job domain.Job) error {
	if _, err := db.sql.ExecContext(ctx, `delete from jobs where id = ? and state = 'running'`, job.ID); err != nil {
		return fmt.Errorf("cannot finish the %s job (id=%d): %w", job.Kind, job.ID, err)
	}
	return nil
}

// failSubjectJob applies the kind's retry policy: a retry requeues the job after the delay, and a final failure keeps
// the failed row with its error. A job no longer running is ignored.
func (db *DB) failSubjectJob(ctx context.Context, job domain.Job, cause error) error {
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var attempts int
	err = tx.QueryRowContext(ctx, `select attempts from jobs where id = ? and state = 'running'`, job.ID).Scan(&attempts)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot read the %s job (id=%d): %w", job.Kind, job.ID, err)
	}
	outcome := jobRetryPolicy(job.Kind).AfterFailure(attempts, cause, true, jobJitter())
	c := &changes{}
	if outcome.State == domain.JobFailed {
		_, err = tx.ExecContext(ctx, `update jobs set state = 'failed', last_error = ?, updated_at = ? where id = ?`,
			cause.Error(), jobNow().Unix(), job.ID)
	} else {
		err = requeueClaimedJob(ctx, tx, c, job, attempts, outcome, cause.Error())
	}
	if err != nil {
		return fmt.Errorf("cannot record the %s failure (id=%d): %w", job.Kind, job.ID, err)
	}
	if err := db.commit(ctx, tx, c); err != nil {
		return fmt.Errorf("cannot record the %s failure (id=%d): %w", job.Kind, job.ID, err)
	}
	return nil
}

// subjectJob is one job a write queues in its own transaction.
type subjectJob struct {
	kind domain.JobKind
	// videoID or subject names the job's subject, by the subject check of jobs; neither is set for a singleton kind.
	videoID  int64
	subject  string
	priority domain.JobPriority
}

// queueSubjectJob inserts the job unless its subject has unfinished work; an owner request moves a queued one to the
// owner class and ends its retry wait. It reports whether a job was inserted or moved, for the caller to publish
// JobsQueued after commit.
func queueSubjectJob(ctx context.Context, tx *sql.Tx, job subjectJob) (bool, error) {
	at := time.Now().Unix()
	same := `kind = ?1 and coalesce(video_id, 0) = ?2 and coalesce(subject_key, '') = ?3`
	res, err := tx.ExecContext(ctx, `insert into jobs (kind, video_id, subject_key, state, attempts, priority, run_after,
		created_at, updated_at)
		select ?1, nullif(?2, 0), nullif(?3, ''), 'queued', 0, ?4, ?5, ?5, ?5
		where not exists (select 1 from jobs where `+same+` and state in ('queued', 'running'))`,
		string(job.kind), job.videoID, job.subject, int(job.priority), at)
	if err != nil {
		return false, fmt.Errorf("cannot queue a %s job: %w", job.kind, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("cannot queue a %s job: %w", job.kind, err)
	}
	if n == 0 && job.priority == domain.JobPriorityOwner {
		res, err = tx.ExecContext(ctx, `update jobs set priority = ?4, run_after = min(run_after, ?5), updated_at = ?5
			where `+same+` and state = 'queued' and (priority <> ?4 or run_after > ?5)`,
			string(job.kind), job.videoID, job.subject, int(job.priority), at)
		if err != nil {
			return false, fmt.Errorf("cannot move the queued %s job first: %w", job.kind, err)
		}
		if n, err = res.RowsAffected(); err != nil {
			return false, fmt.Errorf("cannot move the queued %s job first: %w", job.kind, err)
		}
	}
	return n > 0, nil
}

// jobsQueuedEvents is the wakeup of kind's handlers, published after the commit that queued its jobs.
func jobsQueuedEvents(queued bool, kind domain.JobKind) []domain.Event {
	if !queued {
		return nil
	}
	return []domain.Event{domain.JobsQueued{Kinds: []domain.JobKind{kind}}}
}

// queueFaceImageJob queues the embedding of the person's face as the owner's request, and returns the events to
// publish after commit.
func queueFaceImageJob(ctx context.Context, tx *sql.Tx, personID int64) ([]domain.Event, error) {
	queued, err := queueSubjectJob(ctx, tx, subjectJob{
		kind: domain.JobFaceImage, subject: strconv.FormatInt(personID, 10), priority: domain.JobPriorityOwner,
	})
	return jobsQueuedEvents(queued, domain.JobFaceImage), err
}
