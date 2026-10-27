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

// contentQueue describes per-content-key work whose items carry one activity entry each, such as auto-tagging and
// face analysis; a running item never moves when content is queued again. On the table path, rows of table outlive
// their work. On the jobs path, the work is a jobs row of kind job, deleted when it ends, and results holds the
// outcome.
type contentQueue struct {
	table string
	kind  domain.ActivityKind
	// what names the work in error messages.
	what string
	// upsert queues ?1 (the content key) at ?2 (Unix ms) for the waiting entry ?3; table path only.
	upsert string
	// claimOrder orders queued rows, the next one first; table path only.
	claimOrder string
	// failure is the column that holds the reason of a failed item.
	failure string
	// job is the job kind of the jobs path; empty selects the table path.
	job domain.JobKind
	// results holds the done or failed outcome per content key on the jobs path.
	results string
}

var autoTagQueue = contentQueue{
	kind: domain.ActivityAutoTag, what: "auto-tagging", failure: `error`,
	job: domain.JobAutoTag, results: "auto_tag_results",
}

var faceQueue = contentQueue{
	table: "face_queue", kind: domain.ActivityFaceAnalysis, what: "face analysis",
	upsert: `insert into face_queue (content_key, state, updated_at, activity_id) values (?1, 'queued', ?2, ?3)
		on conflict (content_key) do update set state = 'queued', reason = null, updated_at = excluded.updated_at,
			activity_id = excluded.activity_id`,
	claimOrder: `updated_at, content_key`, failure: `reason`,
}

// contentJobQueue returns the content queue whose work runs as jobs of kind.
func contentJobQueue(kind domain.JobKind) (contentQueue, bool) {
	if kind == autoTagQueue.job {
		return autoTagQueue, true
	}
	return contentQueue{}, false
}

func (q contentQueue) entry(key, label string, reason domain.ActivityReason, decider domain.ActivityDecider) domain.ActivityEntry {
	return domain.ActivityEntry{
		Kind:    q.kind,
		Subject: domain.ActivitySubject{Kind: domain.ActivitySubjectVideo, Key: key, Label: label},
		Status:  domain.ActivityRunning,
		Reason:  reason,
		Decider: decider,
	}
}

// stateOf is the SQL of the state of the content key keyExpr, empty when nothing is queued or recorded for it. On the
// jobs path an unfinished job's state comes before the recorded result.
func (q contentQueue) stateOf(keyExpr string) string {
	if q.job == "" {
		return `coalesce((select q.state from ` + q.table + ` q where q.content_key = ` + keyExpr + `), '')`
	}
	return `coalesce((select j.state from jobs j where j.kind = '` + string(q.job) + `' and j.subject_key = ` + keyExpr +
		` and j.state in ('queued', 'running')), (select r.state from ` + q.results + ` r where r.content_key = ` +
		keyExpr + `), '')`
}

// enqueue writes the queued item of key for the waiting entry activityID. The jobs path inserts a job in class
// priority and drops the previous result, as the table path overwrites its row; callers queue only a key without
// unfinished work.
func (q contentQueue) enqueue(
	ctx context.Context, tx *sql.Tx, c *changes, key string, activityID int64, priority domain.JobPriority, now time.Time,
) error {
	if q.job == "" {
		if _, err := tx.ExecContext(ctx, q.upsert, key, now.UnixMilli(), activityID); err != nil {
			return fmt.Errorf("cannot queue videos for %s: %w", q.what, err)
		}
		return nil
	}
	if _, err := tx.ExecContext(ctx, `delete from `+q.results+` where content_key = ?`, key); err != nil {
		return fmt.Errorf("cannot queue videos for %s: %w", q.what, err)
	}
	at := now.Unix()
	if _, err := tx.ExecContext(ctx, `insert into jobs (kind, subject_key, state, attempts, priority, run_after,
		created_at, updated_at, activity_id) values (?, ?, 'queued', 0, ?, ?, ?, ?, ?)`,
		string(q.job), key, int(priority), at, at, at, activityID); err != nil {
		return fmt.Errorf("cannot queue videos for %s: %w", q.what, err)
	}
	c.jobsQueued(q.job)
	return nil
}

// contentRequest is who queues content: the decider of the new entries, the entries they continue, and the class of
// the jobs on the jobs path.
type contentRequest struct {
	decider   domain.ActivityDecider
	continued []int64
	priority  domain.JobPriority
}

// queueContent queues the content keys selectKeys yields whose current state (empty for no row) is in states, with
// a waiting entry each, and publishes ActivityChanged after commit.
func (db *DB) queueContent(
	ctx context.Context, q contentQueue, selectKeys string, args []any, states string, decider domain.ActivityDecider,
	continued []int64,
) (int, error) {
	return db.queueContentFor(ctx, q, selectKeys, args, states,
		contentRequest{decider: decider, continued: continued, priority: domain.JobPriorityImport})
}

// queueContentFor is queueContent for the request req.
func (db *DB) queueContentFor(
	ctx context.Context, q contentQueue, selectKeys string, args []any, states string, req contentRequest,
) (int, error) {
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	c := &changes{}
	n, err := queueContentRequest(ctx, tx, c, q, selectKeys, args, states, req)
	if err != nil {
		return 0, err
	}
	if err := db.commit(ctx, tx, c); err != nil {
		return 0, fmt.Errorf("cannot queue videos for %s: %w", q.what, err)
	}
	return n, nil
}

// queueContentTx is queueContent inside the caller's transaction.
func queueContentTx(
	ctx context.Context, tx *sql.Tx, c *changes, q contentQueue, selectKeys string, args []any, states string,
	decider domain.ActivityDecider, continued []int64,
) (int, error) {
	return queueContentRequest(ctx, tx, c, q, selectKeys, args, states,
		contentRequest{decider: decider, continued: continued, priority: domain.JobPriorityImport})
}

func queueContentRequest(
	ctx context.Context, tx *sql.Tx, c *changes, q contentQueue, selectKeys string, args []any, states string,
	req contentRequest,
) (int, error) {
	subjects, err := queueableSubjects(ctx, tx, `select k.content_key, `+contentLabelExpr("k.content_key")+`
		from (select distinct content_key from (`+selectKeys+`)) k
		where `+q.stateOf("k.content_key")+` in (`+states+`)
		order by k.content_key`, args)
	if err != nil {
		return 0, err
	}
	now := time.Now()
	for _, subject := range subjects {
		if _, err := queueContentSubject(ctx, tx, c, q, subject, req, now); err != nil {
			return 0, err
		}
	}
	if q.job != "" && req.priority == domain.JobPriorityOwner {
		if err := raiseQueuedContent(ctx, tx, c, q, selectKeys, args, now); err != nil {
			return 0, err
		}
	}
	return len(subjects), nil
}

// raiseQueuedContent moves the queued jobs of the keys selectKeys yields to the owner class and ends their retry
// wait, so an owner request never waits behind a delay or a bulk queue.
func raiseQueuedContent(
	ctx context.Context, tx *sql.Tx, c *changes, q contentQueue, selectKeys string, args []any, now time.Time,
) error {
	owner := int(domain.JobPriorityOwner)
	res, err := tx.ExecContext(ctx, `update jobs set priority = ?, run_after = min(run_after, ?), updated_at = ?
		where kind = ? and state = 'queued' and (priority <> ? or run_after > ?)
		and subject_key in (select content_key from (`+selectKeys+`))`,
		append([]any{owner, now.Unix(), now.Unix(), string(q.job), owner, now.Unix()}, args...)...)
	if err != nil {
		return fmt.Errorf("cannot move the queued %s jobs first: %w", q.what, err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return fmt.Errorf("cannot move the queued %s jobs first: %w", q.what, err)
	} else if n > 0 {
		c.jobsQueued(q.job)
	}
	return nil
}

// queueContentSubject writes the queued item and its waiting entry; an entry that continues others joins their
// pipelines.
func queueContentSubject(
	ctx context.Context, tx *sql.Tx, c *changes, q contentQueue, subject domain.ActivitySubject, req contentRequest,
	now time.Time,
) (int64, error) {
	entry := q.entry(subject.Key, subject.Label, domain.ActivityWaiting, req.decider)
	var (
		id  int64
		err error
	)
	if len(req.continued) > 0 {
		id, err = recordContinuedActivity(ctx, tx, c, entry, req.continued, now)
	} else {
		id, err = recordActivity(ctx, tx, c, entry, now)
	}
	if err != nil {
		return 0, err
	}
	return id, q.enqueue(ctx, tx, c, subject.Key, id, req.priority, now)
}

// queueAgainStates queues a video again unless it is running or already queued.
const queueAgainStates = `'', 'done', 'failed'`

// afterSeek selects which videos a follow-up process of the seek stage queues and the decider of their entries.
type afterSeek struct {
	// switches are the settings that must all be on.
	switches []string
	// states are the queue states queued again; an empty state represents a missing row.
	states  string
	decider domain.ActivityDecider
}

// queueAfterImport is a follow-up process of the seek stage: the transaction that finishes the stage for good queues
// the video while every setting in rule.switches is on and its queue state is in rule.states, and the entry joins the
// pipelines of the job's import entry. Otherwise it writes nothing, so no pipeline waits for the process.
func queueAfterImport(
	ctx context.Context, tx *sql.Tx, c *changes, q contentQueue, rule afterSeek, job domain.Job, now time.Time,
) error {
	for _, key := range rule.switches {
		if on, err := readBoolSetting(ctx, tx, key); err != nil || !on {
			return err
		}
	}
	subjects, err := queueableSubjects(ctx, tx, `select v.content_key, `+contentLabelExpr("v.content_key")+`
		from videos v where v.id = ? and v.content_key = ? and v.content_key <> '' and v.location_generation = ?
		and v.seek_thumbnail_state in ('done', 'failed') and `+registeredVideoCondition("v")+`
		and `+q.stateOf("v.content_key")+` in (`+rule.states+`)`,
		[]any{job.VideoID, job.ContentKey, job.LocationGeneration})
	if err != nil {
		return err
	}
	for _, subject := range subjects {
		// The work a successful owner-class job queues takes the owner class too.
		id, err := queueContentSubject(ctx, tx, c, q, subject,
			contentRequest{decider: rule.decider, priority: job.Priority}, now)
		if err != nil {
			return err
		}
		if err := joinJobPipelines(ctx, tx, job.ID, id, now); err != nil {
			return err
		}
	}
	return nil
}

// queueAfterSeekStage queues every follow-up process of a seek stage that finished for good, succeeded or failed. An
// import queues a video never queued while automatic judging is on; a forced job, from a force rescan the owner
// started, queues the video again unless it is queued or running, as "Tag all videos again" does
// (docs/design-docs/force-rescan.md).
func queueAfterSeekStage(ctx context.Context, tx *sql.Tx, c *changes, job domain.Job, now time.Time) error {
	tagging := afterSeek{switches: []string{autoTagFeatureKey, autoTagAfterImportKey}, states: `''`,
		decider: domain.ActivityByRule}
	faces := afterSeek{switches: []string{faceEnabledKey}, states: `''`, decider: domain.ActivityByRule}
	if job.Force {
		tagging = afterSeek{switches: []string{autoTagFeatureKey}, states: queueAgainStates,
			decider: domain.ActivityByOwner}
		faces.states, faces.decider = queueAgainStates, domain.ActivityByOwner
	}
	if err := queueAfterImport(ctx, tx, c, autoTagQueue, tagging, job, now); err != nil {
		return err
	}
	return queueAfterImport(ctx, tx, c, faceQueue, faces, job, now)
}

func queueableSubjects(ctx context.Context, tx *sql.Tx, query string, args []any) ([]domain.ActivitySubject, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("cannot read the videos to queue: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var subjects []domain.ActivitySubject
	for rows.Next() {
		subject := domain.ActivitySubject{Kind: domain.ActivitySubjectVideo}
		if err := rows.Scan(&subject.Key, &subject.Label); err != nil {
			return nil, fmt.Errorf("cannot read the videos to queue: %w", err)
		}
		subjects = append(subjects, subject)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the videos to queue: %w", err)
	}
	return subjects, nil
}

// claimContent takes the next queued row of the table path or returns domain.ErrNoJob, creating a processing activity
// if necessary; read, when set, runs in the claim's transaction.
func (db *DB) claimContent(ctx context.Context, q contentQueue, read func(tx *sql.Tx) error) (string, error) {
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now()
	var key string
	var activityID int64
	err = tx.QueryRowContext(ctx, `
		update `+q.table+` set state = 'running', updated_at = ?
		where content_key = (select content_key from `+q.table+` where state = 'queued'
			order by `+q.claimOrder+` limit 1)
		returning content_key, coalesce(activity_id, 0)`, now.UnixMilli()).Scan(&key, &activityID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", domain.ErrNoJob
	}
	if err != nil {
		return "", fmt.Errorf("cannot claim a %s job: %w", q.what, err)
	}
	if read != nil {
		if err := read(tx); err != nil {
			return "", err
		}
	}
	c := &changes{}
	linked, err := claimQueueActivity(ctx, tx, c, q, key, activityID, now)
	if err != nil {
		return "", err
	}
	if linked != 0 {
		if _, err := tx.ExecContext(ctx, `update `+q.table+` set activity_id = ? where content_key = ?`,
			linked, key); err != nil {
			return "", fmt.Errorf("cannot link the %s job to its activity: %w", q.what, err)
		}
	}
	if err := db.commit(ctx, tx, c); err != nil {
		return "", fmt.Errorf("cannot claim a %s job: %w", q.what, err)
	}
	return key, nil
}

// claimContentJob takes the next due job of the jobs path in class, run-after and age order; each claim is one
// attempt. A job whose content key has no registered video left is dropped and its entry completed, and the next one
// is taken. An empty queue returns domain.ErrNoJob.
func (db *DB) claimContentJob(ctx context.Context, q contentQueue) (domain.Job, error) {
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.Job{}, fmt.Errorf("cannot claim a %s job: %w", q.what, err)
	}
	defer func() { _ = tx.Rollback() }()
	c := &changes{}
	for {
		job, activityID, err := takeContentJob(ctx, tx, q)
		if errors.Is(err, domain.ErrNoJob) {
			if err := db.commit(ctx, tx, c); err != nil {
				return domain.Job{}, fmt.Errorf("cannot claim a %s job: %w", q.what, err)
			}
			return domain.Job{}, domain.ErrNoJob
		}
		if err != nil {
			return domain.Job{}, err
		}
		now := time.Now()
		_, err = videoIDForContent(ctx, tx, job.ContentKey)
		if errors.Is(err, domain.ErrNotFound) {
			if err := dropContentJob(ctx, tx, c, q, job.ContentKey, activityID, now); err != nil {
				return domain.Job{}, err
			}
			continue
		}
		if err != nil {
			return domain.Job{}, err
		}
		linked, err := claimQueueActivity(ctx, tx, c, q, job.ContentKey, activityID, now)
		if err != nil {
			return domain.Job{}, err
		}
		if linked != 0 {
			if _, err := tx.ExecContext(ctx, `update jobs set activity_id = ? where id = ?`, linked, job.ID); err != nil {
				return domain.Job{}, fmt.Errorf("cannot link the %s job to its activity: %w", q.what, err)
			}
		}
		if err := db.commit(ctx, tx, c); err != nil {
			return domain.Job{}, fmt.Errorf("cannot claim a %s job: %w", q.what, err)
		}
		return job, nil
	}
}

// takeContentJob marks the next due job of the jobs path running and counts the attempt.
func takeContentJob(ctx context.Context, tx *sql.Tx, q contentQueue) (domain.Job, int64, error) {
	job := domain.Job{Kind: q.job, StartsRound: true, LastLocation: true}
	var activityID int64
	err := tx.QueryRowContext(ctx, `update jobs set state = 'running', attempts = attempts + 1, updated_at = ?1
		where id = (select id from jobs where kind = ?2 and state = 'queued' and run_after <= ?1
			order by priority, run_after, id limit 1)
		returning id, subject_key, attempts, priority, coalesce(activity_id, 0)`,
		jobNow().Unix(), string(q.job)).Scan(&job.ID, &job.ContentKey, &job.Attempts, &job.Priority, &activityID)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Job{}, 0, domain.ErrNoJob
	}
	if err != nil {
		return domain.Job{}, 0, fmt.Errorf("cannot claim a %s job: %w", q.what, err)
	}
	return job, activityID, nil
}

// dropContentJob ends the running job of content that is gone, recording no result, and completes its entry.
func dropContentJob(
	ctx context.Context, tx *sql.Tx, c *changes, q contentQueue, key string, activityID int64, now time.Time,
) error {
	if err := endContentJob(ctx, tx, q, key, "", "", now); err != nil {
		return err
	}
	completed := domain.ActivityState{Status: domain.ActivityDone, Outcome: domain.ActivityCompleted}
	return closeQueueActivity(ctx, tx, c, activityID, completed, "", "", now)
}

// claimQueueActivity creates a new processing activity if the previous entry is no longer running, preserving its
// decider and pipelines when available, and returns the new entry for the caller to link; zero keeps the entry.
func claimQueueActivity(
	ctx context.Context, tx *sql.Tx, c *changes, q contentQueue, key string, activityID int64, now time.Time,
) (int64, error) {
	decider := domain.ActivityByRule
	var continued []int64
	if activityID != 0 {
		var status, stored string
		err := tx.QueryRowContext(ctx, `select status, decider from activity_entries where id = ?`, activityID).
			Scan(&status, &stored)
		switch {
		case err == nil && domain.ActivityStatus(status) == domain.ActivityRunning:
			return 0, setActivityState(ctx, tx, c, activityID,
				domain.ActivityState{Status: domain.ActivityRunning, Reason: domain.ActivityProcessing}, now)
		case err == nil:
			decider = domain.ActivityDecider(stored)
			continued = []int64{activityID}
		case !errors.Is(err, sql.ErrNoRows):
			return 0, fmt.Errorf("cannot read the %s activity (id=%d): %w", q.what, activityID, err)
		}
	}
	var label string
	if err := tx.QueryRowContext(ctx, `select `+contentLabelExpr("?1"), key).Scan(&label); err != nil {
		return 0, fmt.Errorf("cannot read the label of the %s subject: %w", q.what, err)
	}
	return recordContinuedActivity(ctx, tx, c, q.entry(key, label, domain.ActivityProcessing, decider), continued, now)
}

// closeQueueActivity ignores legacy zero activity IDs; completed entries retain a decision mark for undo.
func closeQueueActivity(
	ctx context.Context, tx *sql.Tx, c *changes, id int64, state domain.ActivityState, decider domain.ActivityDecider,
	detail string, now time.Time,
) error {
	if id == 0 {
		return nil
	}
	_, err := tx.ExecContext(ctx, `update activity_entries set status = ?, reason = ?, outcome = ?,
		decider = coalesce(nullif(?, ''), decider), detail = coalesce(nullif(?, ''), detail), updated_at = ?
		where id = ?`, string(state.Status), nullString(string(state.Reason)), nullString(string(state.Outcome)),
		string(decider), detail, now.UnixMilli(), id)
	if err != nil {
		return fmt.Errorf("cannot update the activity (id=%d): %w", id, err)
	}
	if state.Status == domain.ActivityDone {
		if err := markDecided(ctx, tx, id); err != nil {
			return err
		}
	}
	c.activityChanged()
	return rollUpPipelines(ctx, tx, []int64{id}, now.UnixMilli())
}

// runningContent reads a running item's activity; found is false for an item that is missing or no longer running.
func runningContent(ctx context.Context, tx *sql.Tx, q contentQueue, key string) (activityID int64, found bool, err error) {
	var state string
	query, args := `select state, coalesce(activity_id, 0) from `+q.table+` where content_key = ?`, []any{key}
	if q.job != "" {
		query = `select state, coalesce(activity_id, 0) from jobs where kind = ? and subject_key = ? and state = 'running'`
		args = []any{string(q.job), key}
	}
	err = tx.QueryRowContext(ctx, query, args...).Scan(&state, &activityID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && state != "running") {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("cannot read the %s job: %w", q.what, err)
	}
	return activityID, true, nil
}

// endContentJob deletes the running job of key on the jobs path and records state ("done" or "failed") with reason as
// its result; an empty state records nothing, for content that is gone.
func endContentJob(ctx context.Context, tx *sql.Tx, q contentQueue, key, state, reason string, now time.Time) error {
	if _, err := tx.ExecContext(ctx, `delete from jobs where kind = ? and subject_key = ? and state = 'running'`,
		string(q.job), key); err != nil {
		return fmt.Errorf("cannot end the %s job: %w", q.what, err)
	}
	if state == "" {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `insert into `+q.results+` (content_key, state, `+q.failure+`, updated_at)
		values (?, ?, ?, ?) on conflict (content_key) do update set state = excluded.state,
			`+q.failure+` = excluded.`+q.failure+`, updated_at = excluded.updated_at`,
		key, state, reason, now.UnixMilli()); err != nil {
		return fmt.Errorf("cannot record the %s result: %w", q.what, err)
	}
	return nil
}

// failContent ignores rows no longer running and records the failure with their activity atomically.
func (db *DB) failContent(ctx context.Context, q contentQueue, key, reason string) error {
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now()
	var activityID int64
	err = tx.QueryRowContext(ctx, `update `+q.table+` set state = 'failed', `+q.failure+` = ?, updated_at = ?
		where content_key = ? and state = 'running' returning coalesce(activity_id, 0)`,
		reason, now.UnixMilli(), key).Scan(&activityID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot record the %s failure: %w", q.what, err)
	}
	c := &changes{}
	if err := closeQueueActivity(ctx, tx, c, activityID,
		domain.ActivityState{Status: domain.ActivityAttention, Reason: domain.ActivityFailed}, "", reason, now); err != nil {
		return err
	}
	if err := db.commit(ctx, tx, c); err != nil {
		return fmt.Errorf("cannot record the %s failure: %w", q.what, err)
	}
	return nil
}

// failContentJob applies the kind's retry policy to a claimed job of the jobs path: a retry requeues it after the
// delay with its entry waiting, and a final failure records the failed result with its entry needing attention. A job
// no longer running is ignored.
func (db *DB) failContentJob(ctx context.Context, q contentQueue, job domain.Job, cause error) error {
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var attempts int
	var activityID int64
	err = tx.QueryRowContext(ctx, `select attempts, coalesce(activity_id, 0) from jobs
		where id = ? and state = 'running'`, job.ID).Scan(&attempts, &activityID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot read the %s job (id=%d): %w", q.what, job.ID, err)
	}
	reason := cause.Error()
	outcome := jobRetryPolicy(job.Kind).AfterFailure(attempts, cause, true, jobJitter())
	now := time.Now()
	c := &changes{}
	// The entry needs attention after the last attempt and otherwise waits for the retry; the detail says why.
	state := domain.ActivityState{Status: domain.ActivityAttention, Reason: domain.ActivityFailed}
	if outcome.State == domain.JobFailed {
		err = endContentJob(ctx, tx, q, job.ContentKey, string(domain.JobFailed), reason, now)
	} else {
		state = domain.ActivityState{Status: domain.ActivityRunning, Reason: domain.ActivityWaiting}
		err = requeueContentJob(ctx, tx, c, q, job.ID, attempts, outcome, reason)
	}
	if err != nil {
		return err
	}
	if err := closeQueueActivity(ctx, tx, c, activityID, state, "", reason, now); err != nil {
		return err
	}
	if err := db.commit(ctx, tx, c); err != nil {
		return fmt.Errorf("cannot record the %s failure: %w", q.what, err)
	}
	return nil
}

// requeueContentJob queues the job again after the outcome's delay; a snooze refunds the claim's attempt.
func requeueContentJob(
	ctx context.Context, tx *sql.Tx, c *changes, q contentQueue, id int64, attempts int, outcome domain.JobFailure,
	reason string,
) error {
	if outcome.Uncounted {
		attempts--
	}
	at := jobNow().Unix()
	if _, err := tx.ExecContext(ctx, `update jobs set state = 'queued', attempts = ?, last_error = ?, run_after = ?,
		updated_at = ? where id = ?`, attempts, reason, at+int64(math.Ceil(outcome.Delay.Seconds())), at, id); err != nil {
		return fmt.Errorf("cannot requeue the %s job (id=%d): %w", q.what, id, err)
	}
	c.jobsQueued(q.job)
	return nil
}

// requeueRunning puts running rows of the table path back to queued, keeping their activity for the next claim.
func (db *DB) requeueRunning(ctx context.Context, q contentQueue) (int, error) {
	result, err := db.sql.ExecContext(ctx, `update `+q.table+` set state = 'queued', updated_at = ?
		where state = 'running'`, time.Now().UnixMilli())
	if err != nil {
		return 0, fmt.Errorf("cannot requeue interrupted %s jobs: %w", q.what, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("cannot requeue interrupted %s jobs: %w", q.what, err)
	}
	return int(affected), nil
}

// contentCounts counts q's items by state, with the reason of the latest failure. The jobs path reads unfinished
// work from jobs and outcomes from the results.
func (db *DB) contentCounts(ctx context.Context, q contentQueue) (domain.QueueCounts, error) {
	query, args := `
		select
			coalesce(sum(state = 'queued'), 0),
			coalesce(sum(state = 'running'), 0),
			coalesce(sum(state = 'done'), 0),
			coalesce(sum(state = 'failed'), 0),
			coalesce((select `+q.failure+` from `+q.table+` where state = 'failed'
				order by updated_at desc, content_key limit 1), '')
		from `+q.table, []any(nil)
	if q.job != "" {
		query = `
		select
			(select count(*) from jobs where kind = ?1 and state = 'queued'),
			(select count(*) from jobs where kind = ?1 and state = 'running'),
			(select count(*) from ` + q.results + ` where state = 'done'),
			(select count(*) from ` + q.results + ` where state = 'failed'),
			coalesce((select ` + q.failure + ` from ` + q.results + ` where state = 'failed'
				order by updated_at desc, content_key limit 1), '')`
		args = []any{string(q.job)}
	}
	var counts domain.QueueCounts
	err := db.sql.QueryRowContext(ctx, query, args...).
		Scan(&counts.Queued, &counts.Running, &counts.Done, &counts.Failed, &counts.LastError)
	if err != nil {
		return domain.QueueCounts{}, fmt.Errorf("cannot count %s jobs: %w", q.what, err)
	}
	return counts, nil
}
