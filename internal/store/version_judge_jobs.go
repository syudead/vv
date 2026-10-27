package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// A candidate's judging work is its version_judge job, keyed by the pair; the candidate row keeps only the judgement.
// The job goes with the row through the video_version_candidates_job_delete trigger. See
// docs/design-docs/duplicate-judging.md.

const versionJudgeJob = `'` + string(domain.JobVersionJudge) + `'`

// judgeRetryAtColumn is the run-after time of the queued job of the candidate c after a failed attempt, which the
// candidate list and the activity log show as the pair's retry time.
const judgeRetryAtColumn = `(select j.run_after from jobs j where j.kind = ` + versionJudgeJob + `
	and j.subject_key = c.key_a and j.subject_key_b = c.key_b and j.state = 'queued' and j.attempts > 0)`

// queueJudgeJobs queues a job in the import class for every waiting candidate c that condition selects and that has no
// unfinished job.
func queueJudgeJobs(ctx context.Context, tx *sql.Tx, c *changes, condition string, args ...any) error {
	at := jobNow().Unix()
	res, err := tx.ExecContext(ctx, `insert or ignore into jobs (kind, subject_key, subject_key_b, state, attempts,
		priority, run_after, created_at, updated_at, activity_id)
		select `+versionJudgeJob+`, c.key_a, c.key_b, 'queued', 0, ?, ?, ?, ?, c.activity_id
		from video_version_candidates c where c.judgement is null and (`+condition+`)
		order by c.created_at, c.key_a, c.key_b`,
		append([]any{int(domain.JobPriorityImport), at, at, at}, args...)...)
	if err != nil {
		return fmt.Errorf("cannot queue the version judgements: %w", err)
	}
	return markJudgeJobsQueued(c, res)
}

func markJudgeJobsQueued(c *changes, res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("cannot queue the version judgements: %w", err)
	}
	if n > 0 {
		c.jobsQueued(domain.JobVersionJudge)
	}
	return nil
}

// requestJudgeJob is an owner request on one pair: its job is queued if missing, raised to the owner class and freed
// from its retry wait. fresh starts the attempts again.
func requestJudgeJob(ctx context.Context, tx *sql.Tx, c *changes, keys [2]string, fresh bool) error {
	if fresh {
		if _, err := tx.ExecContext(ctx, `delete from jobs where kind = `+versionJudgeJob+`
			and subject_key = ? and subject_key_b = ? and state = 'queued'`, keys[0], keys[1]); err != nil {
			return fmt.Errorf("cannot queue the version judgement: %w", err)
		}
	}
	at := jobNow().Unix()
	owner := int(domain.JobPriorityOwner)
	if _, err := tx.ExecContext(ctx, `insert or ignore into jobs (kind, subject_key, subject_key_b, state, attempts,
		priority, run_after, created_at, updated_at, activity_id)
		select `+versionJudgeJob+`, c.key_a, c.key_b, 'queued', 0, ?1, ?2, ?2, ?2, c.activity_id
		from video_version_candidates c where c.key_a = ?3 and c.key_b = ?4`, owner, at, keys[0], keys[1]); err != nil {
		return fmt.Errorf("cannot queue the version judgement: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `update jobs set priority = ?1, run_after = min(run_after, ?2), updated_at = ?2
		where kind = `+versionJudgeJob+` and subject_key = ?3 and subject_key_b = ?4 and state = 'queued'`,
		owner, at, keys[0], keys[1]); err != nil {
		return fmt.Errorf("cannot queue the version judgement: %w", err)
	}
	c.jobsQueued(domain.JobVersionJudge)
	return nil
}

// claimJudgeJob takes the next due job whose pair is judgeable now, marks the candidate running and its entry
// processing, and counts the attempt. A held, unbundled or unregistered pair keeps its job queued. An empty queue
// returns domain.ErrNoJob.
func (db *DB) claimJudgeJob(ctx context.Context) (domain.Job, error) {
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.Job{}, fmt.Errorf("cannot claim a version candidate: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	job := domain.Job{Kind: domain.JobVersionJudge, StartsRound: true, LastLocation: true}
	err = tx.QueryRowContext(ctx, `update jobs set state = 'running', attempts = attempts + 1, updated_at = ?1
		where id = (select j.id from jobs j where j.kind = `+versionJudgeJob+` and j.state = 'queued'
			and j.run_after <= ?1
			and exists (select 1 `+judgeablePairs+` and c.key_a = j.subject_key and c.key_b = j.subject_key_b)
			order by j.priority, j.run_after, j.id limit 1)
		returning id, subject_key, subject_key_b, attempts, priority`, jobNow().Unix()).
		Scan(&job.ID, &job.ContentKey, &job.ContentKeyB, &job.Attempts, &job.Priority)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Job{}, domain.ErrNoJob
	}
	if err != nil {
		return domain.Job{}, fmt.Errorf("cannot claim a version candidate: %w", err)
	}
	keys := [2]string{job.ContentKey, job.ContentKeyB}
	var activityID sql.NullInt64
	if err := tx.QueryRowContext(ctx, `update video_version_candidates
		   set judgement = 'running', judgement_error = null, probability = null
		 where key_a = ? and key_b = ? returning activity_id`, keys[0], keys[1]).Scan(&activityID); err != nil {
		return domain.Job{}, fmt.Errorf("cannot claim a version candidate: %w", err)
	}
	c := &changes{}
	now := time.Now()
	processing := domain.VersionJudgementActivityState(domain.VersionJudgementRunning)
	id, err := pairActivityFor(ctx, tx, c, keys, activityID.Int64, processing, now)
	if err != nil {
		return domain.Job{}, err
	}
	if err := updateActivity(ctx, tx, c, id, activityUpdate{state: processing}, now); err != nil {
		return domain.Job{}, err
	}
	if err := db.commit(ctx, tx, c); err != nil {
		return domain.Job{}, fmt.Errorf("cannot claim a version candidate: %w", err)
	}
	return job, nil
}

// runningJudgeJob is the claim of a pair whose result is being applied.
type runningJudgeJob struct {
	attempts int
	priority domain.JobPriority
}

func readRunningJudgeJob(ctx context.Context, tx *sql.Tx, keys [2]string) (runningJudgeJob, error) {
	// A claim made before jobs held the work counts as the first attempt.
	running := runningJudgeJob{attempts: 1, priority: domain.JobPriorityImport}
	err := tx.QueryRowContext(ctx, `select attempts, priority from jobs where kind = `+versionJudgeJob+`
		and subject_key = ? and subject_key_b = ? and state = 'running'`, keys[0], keys[1]).
		Scan(&running.attempts, &running.priority)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return running, fmt.Errorf("cannot read the version judgement job: %w", err)
	}
	return running, nil
}

// settleJudgeJob ends the running job after a result: a pair back to waiting is queued again, a failed one waits out
// the kind's retry delay with its attempts, and any other result, or a removed candidate, leaves no job.
func settleJudgeJob(ctx context.Context, tx *sql.Tx, c *changes, keys [2]string, running runningJudgeJob, reason string) error {
	if _, err := tx.ExecContext(ctx, `delete from jobs where kind = `+versionJudgeJob+`
		and subject_key = ? and subject_key_b = ? and state = 'running'`, keys[0], keys[1]); err != nil {
		return fmt.Errorf("cannot end the version judgement job: %w", err)
	}
	var judgement sql.NullString
	err := tx.QueryRowContext(ctx, `select judgement from video_version_candidates where key_a = ? and key_b = ?`,
		keys[0], keys[1]).Scan(&judgement)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot read the version candidate: %w", err)
	}
	if !judgement.Valid {
		return queueJudgeJobs(ctx, tx, c, `c.key_a = ? and c.key_b = ?`, keys[0], keys[1])
	}
	if domain.VersionJudgement(judgement.String) != domain.VersionJudgementFailed {
		return nil
	}
	outcome := jobRetryPolicy(domain.JobVersionJudge).AfterFailure(running.attempts, errors.New(reason), true, jobJitter())
	if outcome.State == domain.JobFailed {
		return nil
	}
	at := jobNow().Unix()
	res, err := tx.ExecContext(ctx, `insert or ignore into jobs (kind, subject_key, subject_key_b, state, attempts,
		priority, run_after, created_at, updated_at, last_error, activity_id)
		select `+versionJudgeJob+`, key_a, key_b, 'queued', ?, ?, ?, ?, ?, ?, activity_id
		from video_version_candidates where key_a = ? and key_b = ?`,
		running.attempts, int(running.priority), at+int64(math.Ceil(outcome.Delay.Seconds())), at, at, reason,
		keys[0], keys[1])
	if err != nil {
		return fmt.Errorf("cannot schedule the version judgement retry: %w", err)
	}
	return markJudgeJobsQueued(c, res)
}

// returnToWaiting puts the candidates in state from that condition selects back to waiting, with their entries and
// jobs, and returns how many it moved.
func returnToWaiting(
	ctx context.Context, tx *sql.Tx, c *changes, from domain.VersionJudgement, condition string, args ...any,
) (int, error) {
	rows, err := tx.QueryContext(ctx, `update video_version_candidates as c set judgement = null
		where c.judgement = ? and (`+condition+`) returning coalesce(activity_id, 0)`,
		append([]any{string(from)}, args...)...)
	if err != nil {
		return 0, fmt.Errorf("cannot return the version judgements to waiting: %w", err)
	}
	ids, count, err := scanRequeuedActivityIDs(rows)
	if err != nil {
		return 0, fmt.Errorf("cannot return the version judgements to waiting: %w", err)
	}
	if err := returnPairActivitiesToWaiting(ctx, tx, c, ids, domain.VersionJudgementActivityState(from), time.Now()); err != nil {
		return 0, err
	}
	return count, queueJudgeJobs(ctx, tx, c, condition, args...)
}

// failJudgeJob records a handler's error as a failed judgement, which retries after the kind's delay. A snooze, such
// as judging turned off mid-judgement, returns the pair to waiting without counting the attempt.
func (db *DB) failJudgeJob(ctx context.Context, job domain.Job, cause error) error {
	keys := [2]string{job.ContentKey, job.ContentKeyB}
	if !errors.Is(cause, domain.ErrJobSnoozed) {
		_, err := db.Versions().ApplyJudgement(ctx, keys,
			domain.VersionJudgeResult{Verdict: domain.VersionFailed, Error: cause.Error()})
		return err
	}
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot return the version judgement to waiting: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	at := jobNow().Unix()
	if _, err := tx.ExecContext(ctx, `update jobs set state = 'queued', attempts = max(attempts - 1, 0),
		run_after = ?, updated_at = ? where id = ? and state = 'running'`, at, at, job.ID); err != nil {
		return fmt.Errorf("cannot return the version judgement to waiting: %w", err)
	}
	c := &changes{}
	c.jobsQueued(domain.JobVersionJudge)
	if _, err := returnToWaiting(ctx, tx, c, domain.VersionJudgementRunning, `c.key_a = ? and c.key_b = ?`,
		keys[0], keys[1]); err != nil {
		return err
	}
	if err := db.commit(ctx, tx, c); err != nil {
		return fmt.Errorf("cannot return the version judgement to waiting: %w", err)
	}
	return nil
}
