package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// contentQueue describes a per-content-key queue whose rows carry one activity entry each, such as auto-tagging
// and face analysis; rows outlive their work, and a running row never moves when content is queued again.
type contentQueue struct {
	table string
	kind  domain.ActivityKind
	// what names the work in error messages.
	what string
	// upsert queues ?1 (the content key) at ?2 (Unix ms) for the waiting entry ?3.
	upsert string
	// claimOrder orders queued rows, the next one first.
	claimOrder string
	// failure is the column that holds the reason of a failed row.
	failure string
}

var autoTagQueue = contentQueue{
	table: "auto_tag_queue", kind: domain.ActivityAutoTag, what: "auto-tagging",
	upsert: `insert into auto_tag_queue (content_key, state, error, queued_at, updated_at, activity_id)
		values (?1, 'queued', '', ?2, ?2, ?3)
		on conflict (content_key) do update set state = 'queued', error = '', queued_at = excluded.queued_at,
			updated_at = excluded.updated_at, activity_id = excluded.activity_id`,
	claimOrder: `queued_at, content_key`, failure: `error`,
}

var faceQueue = contentQueue{
	table: "face_queue", kind: domain.ActivityFaceAnalysis, what: "face analysis",
	upsert: `insert into face_queue (content_key, state, updated_at, activity_id) values (?1, 'queued', ?2, ?3)
		on conflict (content_key) do update set state = 'queued', reason = null, updated_at = excluded.updated_at,
			activity_id = excluded.activity_id`,
	claimOrder: `updated_at, content_key`, failure: `reason`,
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

// queueContent queues the content keys selectKeys yields whose current state (empty for no row) is in states, with
// a waiting entry each, and publishes ActivityChanged after commit.
func (db *DB) queueContent(
	ctx context.Context, q contentQueue, selectKeys string, args []any, states string, decider domain.ActivityDecider,
	continued []int64,
) (int, error) {
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	c := &changes{}
	n, err := queueContentTx(ctx, tx, c, q, selectKeys, args, states, decider, continued)
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
	subjects, err := queueableSubjects(ctx, tx, `select k.content_key, `+contentLabelExpr("k.content_key")+`
		from (select distinct content_key from (`+selectKeys+`)) k
		where coalesce((select q.state from `+q.table+` q where q.content_key = k.content_key), '') in (`+states+`)
		order by k.content_key`, args)
	if err != nil {
		return 0, err
	}
	now := time.Now()
	for _, subject := range subjects {
		if _, err := queueContentSubject(ctx, tx, c, q, subject, decider, continued, now); err != nil {
			return 0, err
		}
	}
	return len(subjects), nil
}

// queueContentSubject writes the queue row and its waiting entry; an entry that continues others joins their pipelines.
func queueContentSubject(
	ctx context.Context, tx *sql.Tx, c *changes, q contentQueue, subject domain.ActivitySubject,
	decider domain.ActivityDecider, continued []int64, now time.Time,
) (int64, error) {
	entry := q.entry(subject.Key, subject.Label, domain.ActivityWaiting, decider)
	var (
		id  int64
		err error
	)
	if len(continued) > 0 {
		id, err = recordContinuedActivity(ctx, tx, c, entry, continued, now)
	} else {
		id, err = recordActivity(ctx, tx, c, entry, now)
	}
	if err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, q.upsert, subject.Key, now.UnixMilli(), id); err != nil {
		return 0, fmt.Errorf("cannot queue videos for %s: %w", q.what, err)
	}
	return id, nil
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
		and coalesce((select q.state from `+q.table+` q where q.content_key = v.content_key), '') in (`+rule.states+`)`,
		[]any{job.VideoID, job.ContentKey, job.LocationGeneration})
	if err != nil {
		return err
	}
	for _, subject := range subjects {
		id, err := queueContentSubject(ctx, tx, c, q, subject, rule.decider, nil, now)
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

// claimContent takes the next queued row or returns domain.ErrNoJob, creating a processing activity if necessary;
// read, when set, runs in the claim's transaction.
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
	if err := claimQueueActivity(ctx, tx, c, q, key, activityID, now); err != nil {
		return "", err
	}
	if err := db.commit(ctx, tx, c); err != nil {
		return "", fmt.Errorf("cannot claim a %s job: %w", q.what, err)
	}
	return key, nil
}

// claimQueueActivity creates a new processing activity if the previous entry is no longer running, preserving its
// decider and pipelines when available.
func claimQueueActivity(
	ctx context.Context, tx *sql.Tx, c *changes, q contentQueue, key string, activityID int64, now time.Time,
) error {
	decider := domain.ActivityByRule
	var continued []int64
	if activityID != 0 {
		var status, stored string
		err := tx.QueryRowContext(ctx, `select status, decider from activity_entries where id = ?`, activityID).
			Scan(&status, &stored)
		switch {
		case err == nil && domain.ActivityStatus(status) == domain.ActivityRunning:
			return setActivityState(ctx, tx, c, activityID,
				domain.ActivityState{Status: domain.ActivityRunning, Reason: domain.ActivityProcessing}, now)
		case err == nil:
			decider = domain.ActivityDecider(stored)
			continued = []int64{activityID}
		case !errors.Is(err, sql.ErrNoRows):
			return fmt.Errorf("cannot read the %s activity (id=%d): %w", q.what, activityID, err)
		}
	}
	var label string
	if err := tx.QueryRowContext(ctx, `select `+contentLabelExpr("?1"), key).Scan(&label); err != nil {
		return fmt.Errorf("cannot read the label of the %s subject: %w", q.what, err)
	}
	id, err := recordContinuedActivity(ctx, tx, c, q.entry(key, label, domain.ActivityProcessing, decider), continued, now)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `update `+q.table+` set activity_id = ? where content_key = ?`, id, key); err != nil {
		return fmt.Errorf("cannot link the %s job to its activity: %w", q.what, err)
	}
	return nil
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

// runningContent reads a running row's activity; found is false for a row that is missing or no longer running.
func runningContent(ctx context.Context, tx *sql.Tx, q contentQueue, key string) (activityID int64, found bool, err error) {
	var state string
	err = tx.QueryRowContext(ctx, `select state, coalesce(activity_id, 0) from `+q.table+` where content_key = ?`,
		key).Scan(&state, &activityID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && state != "running") {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("cannot read the %s job: %w", q.what, err)
	}
	return activityID, true, nil
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

// requeueRunning puts running rows back to queued, keeping their activity for the next claim.
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

// contentCounts counts q's rows by state, with the reason of the latest failure.
func (db *DB) contentCounts(ctx context.Context, q contentQueue) (domain.QueueCounts, error) {
	var counts domain.QueueCounts
	err := db.sql.QueryRowContext(ctx, `
		select
			coalesce(sum(state = 'queued'), 0),
			coalesce(sum(state = 'running'), 0),
			coalesce(sum(state = 'done'), 0),
			coalesce(sum(state = 'failed'), 0),
			coalesce((select `+q.failure+` from `+q.table+` where state = 'failed'
				order by updated_at desc, content_key limit 1), '')
		from `+q.table).Scan(&counts.Queued, &counts.Running, &counts.Done, &counts.Failed, &counts.LastError)
	if err != nil {
		return domain.QueueCounts{}, fmt.Errorf("cannot count %s jobs: %w", q.what, err)
	}
	return counts, nil
}
