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
// face analysis; a running item never moves when content is queued again. The work is a jobs row of kind job, deleted
// when it ends, and results holds the outcome.
type contentQueue struct {
	kind domain.ActivityKind
	// what names the work in error messages.
	what string
	// failure is the column that holds the reason of a failed item.
	failure string
	job     domain.JobKind
	// results holds the done or failed outcome per content key.
	results string
	// located marks work that reads the source: its claim takes a location, as an import job's does.
	located bool
}

var autoTagQueue = contentQueue{
	kind: domain.ActivityAutoTag, what: "auto-tagging", failure: `error`,
	job: domain.JobAutoTag, results: "auto_tag_results",
}

var faceQueue = contentQueue{
	kind: domain.ActivityFaceAnalysis, what: "face analysis", failure: `reason`,
	job: domain.JobFace, results: "face_results", located: true,
}

// contentJobQueue returns the content queue whose work runs as jobs of kind.
func contentJobQueue(kind domain.JobKind) (contentQueue, bool) {
	for _, q := range []contentQueue{autoTagQueue, faceQueue} {
		if q.job == kind {
			return q, true
		}
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

// stateOf is the SQL of the state of the content key keyExpr, empty when nothing is queued or recorded for it; an
// unfinished job's state comes before the recorded result.
func (q contentQueue) stateOf(keyExpr string) string {
	return `coalesce((select j.state from jobs j where j.kind = '` + string(q.job) + `' and j.subject_key = ` + keyExpr +
		` and j.state in ('queued', 'running')), (select r.state from ` + q.results + ` r where r.content_key = ` +
		keyExpr + `), '')`
}

// enqueue inserts the job of key in class priority for the waiting entry activityID and drops the previous result;
// callers queue only a key without unfinished work.
func (q contentQueue) enqueue(
	ctx context.Context, tx *sql.Tx, c *changes, key string, activityID int64, priority domain.JobPriority, now time.Time,
) error {
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
// the jobs.
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
	if req.priority == domain.JobPriorityOwner {
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

// claimContentJob takes the claim's next due job in class, run-after and age order; each claim is one attempt. A
// located job takes the first available location, in path order, of any video with its content key. A job whose
// content key has no registered video left is dropped and its entry completed, and the next one is taken. An empty
// queue returns domain.ErrNoJob.
func (db *DB) claimContentJob(ctx context.Context, q contentQueue, claim domain.JobClaim) (domain.Job, error) {
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.Job{}, fmt.Errorf("cannot claim a %s job: %w", q.what, err)
	}
	defer func() { _ = tx.Rollback() }()
	c := &changes{}
	for {
		job, activityID, gone, err := takeContentJob(ctx, tx, q, claim)
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
		if gone {
			if err := dropContentJob(ctx, tx, c, q, job.ID, activityID, now); err != nil {
				return domain.Job{}, err
			}
			continue
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

// takeContentJob marks the claim's next due job running and counts the attempt; gone reports a job whose content key
// has no registered video left.
func takeContentJob(
	ctx context.Context, tx *sql.Tx, q contentQueue, claim domain.JobClaim,
) (job domain.Job, activityID int64, gone bool, err error) {
	if q.located {
		return takeLocatedContentJob(ctx, tx, q, claim)
	}
	job, activityID, err = takeJob(ctx, tx, q.job, claim.MaxPriority)
	if err != nil {
		return domain.Job{}, 0, false, err
	}
	_, err = videoIDForContent(ctx, tx, job.ContentKey)
	if errors.Is(err, domain.ErrNotFound) {
		return job, activityID, true, nil
	}
	return job, activityID, false, err
}

// takeLocatedContentJob takes the next due job whose content has an available location under the claim's roots, or
// whose content has no registered video left, and fills the location; a job stored only under unavailable or other
// roots stays queued. Like an import job, it takes the content's locations in path order after the previous
// attempt's, and only the claim that wraps to the first one counts an attempt.
func takeLocatedContentJob(
	ctx context.Context, tx *sql.Tx, q contentQueue, claim domain.JobClaim,
) (domain.Job, int64, bool, error) {
	var onRoots string
	if claim.Roots != nil {
		onRoots = ` and exists (select 1 from media_folders mf where mf.id in (` + idList(claim.Roots) + `) and ` +
			inMediaFolderCondition("c.next_path") + `)`
	}
	locations := contentLocations("j.subject_key", claim.Unavailable)
	job := domain.Job{Kind: q.job, StartsRound: true, LastLocation: true}
	var activityID int64
	var registered bool
	var previous, next sql.NullString
	now := jobNow().Unix()
	err := tx.QueryRowContext(ctx, `select c.id, c.subject_key, c.attempts, c.priority, c.activity_id, c.location_path,
			c.next_path, c.registered from (
		select j.id, j.subject_key, j.attempts, j.priority, j.run_after, coalesce(j.activity_id, 0) as activity_id,
			j.location_path,
			coalesce((`+locations+` and l.path > j.location_path order by l.path limit 1),
				(`+locations+` order by l.path limit 1)) as next_path,
			exists (select 1 from videos v where v.content_key = j.subject_key and `+registeredVideoCondition("v")+`)
				as registered
		from jobs j where j.kind = ? and j.state = 'queued' and j.run_after <= ? and j.priority <= ?) c
		where not c.registered or (c.next_path is not null`+onRoots+`)
		order by c.priority, c.run_after, c.id limit 1`, string(q.job), now, int(claim.MaxPriority)).
		Scan(&job.ID, &job.ContentKey, &job.Attempts, &job.Priority, &activityID, &previous, &next, &registered)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Job{}, 0, false, domain.ErrNoJob
	}
	if err != nil {
		return domain.Job{}, 0, false, fmt.Errorf("cannot claim a %s job: %w", q.what, err)
	}
	if !registered {
		return job, activityID, true, nil
	}
	job.StartsRound = !previous.Valid || next.String <= previous.String
	job.Attempts = domain.ClaimAttempts(job.Attempts, job.StartsRound)
	var later bool
	if err := tx.QueryRowContext(ctx, `select exists (`+contentLocations("?", claim.Unavailable)+` and l.path > ?)`,
		job.ContentKey, next.String).Scan(&later); err != nil {
		return domain.Job{}, 0, false, fmt.Errorf("cannot check the %s job's later locations: %w", q.what, err)
	}
	job.LastLocation = !later
	if err := tx.QueryRowContext(ctx, `select l.id, l.version, l.path,
		coalesce((select mf.id from media_folders mf where `+inMediaFolderCondition("l.path")+`
			order by length(mf.path) desc limit 1), 0)
		from video_locations l where l.path = ?`, next.String).
		Scan(&job.LocationID, &job.LocationVersion, &job.LocationPath, &job.RootID); err != nil {
		return domain.Job{}, 0, false, fmt.Errorf("cannot choose a location for the %s job: %w", q.what, err)
	}
	if _, err := tx.ExecContext(ctx, `update jobs set state = 'running', attempts = ?, location_id = ?,
		location_version = ?, location_path = ?, updated_at = ? where id = ?`,
		job.Attempts, job.LocationID, job.LocationVersion, job.LocationPath, now, job.ID); err != nil {
		return domain.Job{}, 0, false, fmt.Errorf("cannot claim a %s job: %w", q.what, err)
	}
	return job, activityID, false, nil
}

// contentLocations selects the path of every available location of the content key the SQL expression key names.
func contentLocations(key string, unavailable []int64) string {
	return `select l.path from videos v join video_locations l on l.video_id = v.id
		where v.content_key = ` + key + ` and ` + availableLocationCondition("l", unavailable)
}

// dropContentJob deletes the job of content that is gone, recording no result, and completes its entry.
func dropContentJob(
	ctx context.Context, tx *sql.Tx, c *changes, q contentQueue, id, activityID int64, now time.Time,
) error {
	if _, err := tx.ExecContext(ctx, `delete from jobs where id = ?`, id); err != nil {
		return fmt.Errorf("cannot end the %s job: %w", q.what, err)
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

// closeQueueActivity ignores legacy zero activity IDs; completed entries retain a decision mark for undo. An empty
// detail keeps the stored one, except that a done entry drops the message of an earlier failed attempt.
func closeQueueActivity(
	ctx context.Context, tx *sql.Tx, c *changes, id int64, state domain.ActivityState, decider domain.ActivityDecider,
	detail string, now time.Time,
) error {
	if id == 0 {
		return nil
	}
	_, err := tx.ExecContext(ctx, `update activity_entries set status = ?1, reason = ?2, outcome = ?3,
		decider = coalesce(nullif(?4, ''), decider),
		detail = case when ?1 = 'done' then nullif(?5, '') else coalesce(nullif(?5, ''), detail) end, updated_at = ?6
		where id = ?7`, string(state.Status), nullString(string(state.Reason)), nullString(string(state.Outcome)),
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

// runningContent reads a running job's activity; found is false for content without a running job.
func runningContent(ctx context.Context, tx *sql.Tx, q contentQueue, key string) (activityID int64, found bool, err error) {
	err = tx.QueryRowContext(ctx, `select coalesce(activity_id, 0) from jobs
		where kind = ? and subject_key = ? and state = 'running'`, string(q.job), key).Scan(&activityID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("cannot read the %s job: %w", q.what, err)
	}
	return activityID, true, nil
}

// endContentJob deletes the running job of key and records state ("done" or "failed") with reason as
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

// failContentJob applies the kind's retry policy to a claimed job: a retry requeues it after the
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
	// A face job whose content has a later location tries it at once; the other kinds read no location.
	outcome := jobRetryPolicy(job.Kind).AfterFailure(attempts, cause, !q.located || job.LastLocation, jobJitter())
	now := time.Now()
	c := &changes{}
	// The entry needs attention after the last attempt and otherwise waits for the retry; the detail says why.
	state := domain.ActivityState{Status: domain.ActivityAttention, Reason: domain.ActivityFailed}
	if outcome.State == domain.JobFailed {
		err = endContentJob(ctx, tx, q, job.ContentKey, string(domain.JobFailed), reason, now)
	} else {
		state = domain.ActivityState{Status: domain.ActivityRunning, Reason: domain.ActivityWaiting}
		err = requeueClaimedJob(ctx, tx, c, job, attempts, outcome, reason)
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

// requeueClaimedJob queues the claimed job again after the outcome's delay. A snooze of a claim that started a round
// undoes the round, refunding its attempt, so the next claim starts a new one from the first location.
func requeueClaimedJob(
	ctx context.Context, tx *sql.Tx, c *changes, job domain.Job, attempts int, outcome domain.JobFailure, reason string,
) error {
	undoRound := outcome.Uncounted && job.StartsRound
	if undoRound {
		attempts--
	}
	at := jobNow().Unix()
	// A requeued job reads its subject again, which serves a request to run it again that came while it ran.
	if _, err := tx.ExecContext(ctx, `update jobs set state = 'queued', attempts = ?, last_error = ?, run_after = ?,
		location_path = case when ? then null else location_path end, force_requested = 0, updated_at = ?
		where id = ?`, attempts, reason, at+int64(math.Ceil(outcome.Delay.Seconds())), undoRound, at, job.ID); err != nil {
		return fmt.Errorf("cannot requeue the %s job (id=%d): %w", job.Kind, job.ID, err)
	}
	c.jobsQueued(job.Kind)
	return nil
}

// contentCounts counts q's work by state, with the reason of the latest failure: unfinished work from jobs and
// outcomes from the results.
func (db *DB) contentCounts(ctx context.Context, q contentQueue) (domain.QueueCounts, error) {
	var counts domain.QueueCounts
	err := db.sql.QueryRowContext(ctx, `
		select
			(select count(*) from jobs where kind = ?1 and state = 'queued'),
			(select count(*) from jobs where kind = ?1 and state = 'running'),
			(select count(*) from `+q.results+` where state = 'done'),
			(select count(*) from `+q.results+` where state = 'failed'),
			coalesce((select `+q.failure+` from `+q.results+` where state = 'failed'
				order by updated_at desc, content_key limit 1), '')`, string(q.job)).
		Scan(&counts.Queued, &counts.Running, &counts.Done, &counts.Failed, &counts.LastError)
	if err != nil {
		return domain.QueueCounts{}, fmt.Errorf("cannot count %s jobs: %w", q.what, err)
	}
	return counts, nil
}
