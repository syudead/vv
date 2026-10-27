package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"strconv"
	"strings"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// importJobKinds lists domain.JobKinds as SQL values.
var importJobKinds = func() string {
	values := make([]string, len(domain.JobKinds))
	for i, kind := range domain.JobKinds {
		values[i] = `'` + string(kind) + `'`
	}
	return strings.Join(values, ", ")
}()

// Tests replace these to place claims and failures in time, fix the spread of a retry delay, and override a kind's
// retry policy.
var (
	jobNow         = time.Now
	jobJitter      = rand.Float64
	jobRetryPolicy = domain.JobRetryPolicyFor
)

// EnqueueJob replaces terminal failures but preserves unfinished work through a partial unique index, joining one import activity.
func (s *IngestStore) EnqueueJob(ctx context.Context, kind domain.JobKind, videoID int64, decider domain.ActivityDecider) error {
	return s.EnqueueJobs(ctx, videoID, decider, kind)
}

// EnqueueJobs queues all required stages atomically so a worker cannot finish and close the activity between stages.
func (s *IngestStore) EnqueueJobs(
	ctx context.Context, videoID int64, decider domain.ActivityDecider, kinds ...domain.JobKind,
) error {
	return s.queueJobs(ctx, videoID, decider, false, kinds, nil)
}

// ForceJobs queues forced jobs in one transaction without changing the stage states: over a queued row it sets its
// force, over an unforced running row it sets force_requested, and over a forced running row it does nothing.
func (s *IngestStore) ForceJobs(
	ctx context.Context, videoID int64, decider domain.ActivityDecider, kinds ...domain.JobKind,
) error {
	return s.queueJobs(ctx, videoID, decider, true, kinds, nil)
}

// ForceScanJobs queues forced jobs as ForceJobs does and marks the video forced in the force rescan scanID in the
// same transaction, so a resumed rescan neither forces a queued video again nor skips one whose jobs were lost.
func (s *IngestStore) ForceScanJobs(
	ctx context.Context, scanID, videoID int64, decider domain.ActivityDecider, kinds ...domain.JobKind,
) error {
	return s.queueJobs(ctx, videoID, decider, true, kinds, func(tx *sql.Tx) error {
		return markForced(ctx, tx, scanID, videoID)
	})
}

// first, when not nil, runs at the start of the transaction that queues the jobs.
func (s *IngestStore) queueJobs(
	ctx context.Context, videoID int64, decider domain.ActivityDecider, force bool, kinds []domain.JobKind,
	first func(*sql.Tx) error,
) error {
	if len(kinds) == 0 {
		return nil
	}
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot start queueing the jobs (%v, video=%d): %w", kinds, videoID, err)
	}
	defer func() { _ = tx.Rollback() }()
	if first != nil {
		if err := first(tx); err != nil {
			return err
		}
	}
	run, err := scanRun(ctx, tx, decider)
	if err != nil {
		return err
	}
	var c changes
	for _, kind := range kinds {
		if err := queueJob(ctx, tx, &c, kind, videoID, time.Now().Unix(), run, force); err != nil {
			return err
		}
	}
	c.jobsQueued(kinds...)
	if err := s.db.commit(ctx, tx, &c); err != nil {
		return fmt.Errorf("cannot commit queueing the jobs (%v, video=%d): %w", kinds, videoID, err)
	}
	return nil
}

var jobStateColumns = map[domain.JobKind]string{
	domain.JobProbe:         "probe_state",
	domain.JobThumbnail:     "thumbnail_state",
	domain.JobSeekThumbnail: "seek_thumbnail_state",
	domain.JobPreview:       "preview_state",
}

// EnsureJob repairs missing pending work and stale terminal rows; missing current-version fingerprints are retried after sprites complete.
func (s *IngestStore) EnsureJob(ctx context.Context, kind domain.JobKind, videoID int64, decider domain.ActivityDecider) error {
	return s.EnsureJobs(ctx, videoID, decider, kind)
}

// EnsureJobs repairs all required stages in one transaction and one import activity.
func (s *IngestStore) EnsureJobs(
	ctx context.Context, videoID int64, decider domain.ActivityDecider, kinds ...domain.JobKind,
) error {
	if len(kinds) == 0 {
		return nil
	}
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot start restoring missing jobs (%v, video=%d): %w", kinds, videoID, err)
	}
	defer func() { _ = tx.Rollback() }()
	var queued []domain.JobKind
	for _, kind := range kinds {
		inserted, err := ensureJob(ctx, tx, kind, videoID)
		if err != nil {
			return err
		}
		if inserted {
			queued = append(queued, kind)
		}
	}
	var c changes
	if len(queued) > 0 {
		run, err := scanRun(ctx, tx, decider)
		if err != nil {
			return err
		}
		if err := joinImportActivity(ctx, tx, &c, videoID, run, time.Now()); err != nil {
			return err
		}
		if err := addScanVideos(ctx, tx, videoID); err != nil {
			return err
		}
		c.jobsQueued(queued...)
	}
	if err := s.db.commit(ctx, tx, &c); err != nil {
		return fmt.Errorf("cannot commit restoring missing jobs (%v, video=%d): %w", kinds, videoID, err)
	}
	return nil
}

func ensureJob(ctx context.Context, tx *sql.Tx, kind domain.JobKind, videoID int64) (bool, error) {
	var pending string
	discarded := `state = 'failed'`
	if kind == domain.JobFingerprint {
		discarded = `state in ('done', 'failed')`
		pending = `exists (select 1 from videos v where v.id = ? and ` + fingerprintMissingCondition("v") + `)`
	} else {
		column, ok := jobStateColumns[kind]
		if !ok {
			return false, fmt.Errorf("unknown job kind: %s", kind)
		}
		pending = `exists (select 1 from videos where id = ? and ` + column + ` = 'pending')`
	}
	if _, err := tx.ExecContext(ctx, `delete from jobs where kind = ? and video_id = ? and `+discarded+` and `+pending,
		string(kind), videoID, videoID); err != nil {
		return false, fmt.Errorf("cannot discard finished job rows (%s, video=%d): %w", kind, videoID, err)
	}
	now := time.Now().Unix()
	res, err := tx.ExecContext(ctx, `
		insert into jobs (kind, video_id, state, attempts, run_after, created_at, updated_at)
		select ?, ?, 'queued', 0, ?, ?, ?
		where not exists (select 1 from jobs where kind = ? and video_id = ?) and `+pending,
		string(kind), videoID, now, now, now, string(kind), videoID, videoID)
	if err != nil {
		return false, fmt.Errorf("cannot restore missing jobs (%s, video=%d): %w", kind, videoID, err)
	}
	inserted, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("cannot count restored jobs (%s, video=%d): %w", kind, videoID, err)
	}
	return inserted > 0, nil
}

// ClaimJob claims the kind's next job wherever its location is, for callers outside the scheduler.
func (s *IngestStore) ClaimJob(ctx context.Context, kind domain.JobKind) (domain.Job, error) {
	return s.ClaimJobOn(ctx, domain.AnyJobClaim(kind))
}

// ClaimJobOn takes the claim's queued job of the lowest class, then the earliest run-after time, then the oldest,
// among those whose run-after time has come and whose next location is under the claim's roots. The next location is
// the first available one in path order after the previous attempt's, so a job stored only under unavailable roots
// stays queued. A content job has no location, so only its class applies. BEGIN IMMEDIATE prevents duplicate claims;
// an empty queue returns ErrNoJob.
func (s *IngestStore) ClaimJobOn(ctx context.Context, claim domain.JobClaim) (domain.Job, error) {
	if q, ok := contentJobQueue(claim.Kind); ok {
		return s.db.claimContentJob(ctx, q, claim.MaxPriority)
	}
	conn, err := s.db.sql.Conn(ctx)
	if err != nil {
		return domain.Job{}, fmt.Errorf("cannot claim a job: %w", err)
	}
	// Publish after returning the connection, so a subscriber that reads the store never waits for it.
	var published *changes
	defer func() {
		if published != nil {
			s.db.publish(published)
		}
	}()
	defer func() { _ = conn.Close() }()

	if _, err := conn.ExecContext(ctx, `begin immediate`); err != nil {
		return domain.Job{}, fmt.Errorf("cannot claim a job: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(context.WithoutCancel(ctx), `rollback`)
		}
	}()

	row, err := selectClaimableJob(ctx, conn, claim)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Job{}, domain.ErrNoJob
	}
	if err != nil {
		return domain.Job{}, fmt.Errorf("cannot claim a job: %w", err)
	}
	job := row.job
	err = locateClaimedJob(ctx, conn, &job, row, claim.Unavailable)
	if errors.Is(err, sql.ErrNoRows) {
		dropped, dropErr := s.dropUnplacedJob(ctx, conn, job.ID)
		if dropErr != nil {
			return domain.Job{}, dropErr
		}
		published = dropped
		committed = true
		return domain.Job{}, domain.ErrNoJob
	}
	if err != nil {
		return domain.Job{}, fmt.Errorf("cannot choose a location for the job: %w", err)
	}

	if _, err := conn.ExecContext(ctx, `
		update jobs set state = 'running', attempts = ?, location_id = ?, location_version = ?,
		location_path = ?, updated_at = ? where id = ?`,
		job.Attempts, job.LocationID, job.LocationVersion, job.LocationPath, time.Now().Unix(), job.ID,
	); err != nil {
		return domain.Job{}, fmt.Errorf("cannot take ownership of the job (id=%d): %w", job.ID, err)
	}
	if err := refreshScanSettled(ctx, conn, time.Now().Unix()); err != nil {
		return domain.Job{}, err
	}
	// Interrupted or legacy jobs start a new rule-owned processing activity.
	var c changes
	if err := joinImportActivity(ctx, conn, &c, job.VideoID, ruleRun(), time.Now()); err != nil {
		return domain.Job{}, err
	}

	if _, err := conn.ExecContext(ctx, `commit`); err != nil {
		return domain.Job{}, fmt.Errorf("cannot take ownership of the job (id=%d): %w", job.ID, err)
	}
	committed = true
	published = &c

	return job, nil
}

// claimableJob is the job a claim selected, before its location is taken.
type claimableJob struct {
	job domain.Job
	// previous is the location of the previous attempt, if any.
	previous sql.NullString
	// next is the location this claim takes.
	next string
}

func selectClaimableJob(ctx context.Context, conn *sql.Conn, claim domain.JobClaim) (claimableJob, error) {
	available := availableLocationCondition("l", claim.Unavailable)
	next := `coalesce(
		(select l.path from video_locations l where l.video_id = j.video_id and ` + available + `
			and l.path > j.location_path order by l.path limit 1),
		(select l.path from video_locations l where l.video_id = j.video_id and ` + available + ` order by l.path limit 1))`
	var onRoots string
	if claim.Roots != nil {
		onRoots = ` and exists (select 1 from media_folders mf where mf.id in (` + idList(claim.Roots) + `) and ` +
			inMediaFolderCondition("c.next_path") + `)`
	}
	// Keep SQL eligibility equivalent to domain.ClaimConditionFor.
	query := `select c.id, c.kind, c.video_id, c.attempts, c.force, c.priority, c.location_path, c.next_path from (
		select j.id, j.kind, j.video_id, j.attempts, j.force, j.priority, j.run_after, j.location_path, ` + next + ` as next_path
		from jobs j where j.state = 'queued' and j.kind = ? and j.run_after <= ? and j.priority <= ?` +
		claimConditionSQL(domain.ClaimConditionFor(claim.Kind), "j", claim.Unavailable) + `) c
		where c.next_path is not null` + onRoots + `
		order by c.priority, c.run_after, c.id limit 1`
	var row claimableJob
	var kind string
	err := conn.QueryRowContext(ctx, query, string(claim.Kind), jobNow().Unix(), int(claim.MaxPriority)).Scan(
		&row.job.ID, &kind, &row.job.VideoID, &row.job.Attempts, &row.job.Force, &row.job.Priority,
		&row.previous, &row.next)
	row.job.Kind = domain.JobKind(kind)
	return row, err
}

// locateClaimedJob fills job's location from the selected row, counts the round the claim starts, and records
// whether an available location remains after it in this round.
func locateClaimedJob(ctx context.Context, conn *sql.Conn, job *domain.Job, row claimableJob, unavailable []int64) error {
	if err := conn.QueryRowContext(ctx, `select l.id, l.version, l.path, v.content_key, v.location_generation,
		coalesce((select mf.id from media_folders mf where `+inMediaFolderCondition("l.path")+`
			order by length(mf.path) desc limit 1), 0)
		from video_locations l join videos v on v.id = l.video_id where l.video_id = ? and l.path = ?`,
		job.VideoID, row.next).Scan(&job.LocationID, &job.LocationVersion, &job.LocationPath, &job.ContentKey,
		&job.LocationGeneration, &job.RootID); err != nil {
		return err
	}
	// Wrapping to a path at or before the previous attempt's starts a new round.
	job.StartsRound = !row.previous.Valid || row.next <= row.previous.String
	job.Attempts = domain.ClaimAttempts(job.Attempts, job.StartsRound)
	var hasLaterLocation int
	if err := conn.QueryRowContext(ctx, `select exists (
		select 1 from video_locations l where l.video_id = ? and l.path > ? and `+
		availableLocationCondition("l", unavailable)+`)`,
		job.VideoID, job.LocationPath).Scan(&hasLaterLocation); err != nil {
		return fmt.Errorf("cannot check the job location's final state: %w", err)
	}
	job.LastLocation = hasLaterLocation == 0
	return nil
}

// availableLocationCondition matches a registered location that is not under an unavailable media folder.
func availableLocationCondition(alias string, unavailable []int64) string {
	var excluded string
	if len(unavailable) > 0 {
		excluded = ` and mf.id not in (` + idList(unavailable) + `)`
	}
	return `exists (select 1 from media_folders mf where ` + inMediaFolderCondition(alias+`.path`) + excluded + `)`
}

// idList writes ids for an SQL in list; they are integers, so no quoting is needed.
func idList(ids []int64) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.FormatInt(id, 10)
	}
	return strings.Join(parts, ", ")
}

// Drop an unplaced job and settle its activity inside the claim's BEGIN IMMEDIATE transaction; the caller publishes the
// returned changes.
func (s *IngestStore) dropUnplacedJob(ctx context.Context, conn *sql.Conn, id int64) (*changes, error) {
	var activityID sql.NullInt64
	if err := conn.QueryRowContext(ctx, `select activity_id from jobs where id = ?`, id).Scan(&activityID); err != nil {
		return nil, err
	}
	if _, err := conn.ExecContext(ctx, `delete from jobs where id = ?`, id); err != nil {
		return nil, err
	}
	if err := refreshScanSettled(ctx, conn, time.Now().Unix()); err != nil {
		return nil, err
	}
	// The last pending job closes as gone rather than completed when registered locations disappear.
	var c changes
	if activityID.Valid {
		closed, err := closeGoneImportActivity(ctx, conn, &c, activityID.Int64, time.Now())
		if err != nil {
			return nil, err
		}
		if !closed {
			if err := settleImportActivity(ctx, conn, &c, activityID.Int64, time.Now()); err != nil {
				return nil, err
			}
		}
	}
	if _, err := conn.ExecContext(ctx, `commit`); err != nil {
		return nil, err
	}
	return &c, nil
}

// Keep each SQL predicate equivalent to domain.JobClaimCondition.Allows. A thumbnail parked under unavailable folders
// is not claimable, so it does not hold back seek sprites on other disks.
func claimConditionSQL(c domain.JobClaimCondition, alias string, unavailable []int64) string {
	var cond string
	if c.RegisteredLocation {
		cond += ` and exists (select 1 from video_locations l where l.video_id = ` + alias + `.video_id and ` +
			registeredLocationCondition("l") + `)`
	}
	if c.ProbeFinished {
		cond += ` and exists (select 1 from videos v where v.id = ` + alias + `.video_id and v.probe_state <> '` +
			string(domain.ProbeStatePending) + `')`
	}
	if c.SeekThumbnailFinished {
		cond += ` and exists (select 1 from videos v where v.id = ` + alias + `.video_id and v.seek_thumbnail_state = '` +
			string(domain.SeekThumbnailDone) + `')`
	}
	if c.NoClaimableThumbnail {
		// Include thumbnail work waiting on probes, matching remainingJobCondition.
		cond += ` and not exists (select 1 from jobs t where t.kind = '` + string(domain.JobThumbnail) +
			`' and t.state in ('queued', 'running') and exists (
				select 1 from video_locations tl where tl.video_id = t.video_id and ` +
			availableLocationCondition("tl", unavailable) + `))`
	}
	return cond
}

// CompleteClaimedJob requeues when the claimed location or content changed. A content job's handler has already
// ended it with its result, as AutoTagStore.Finish does, so there is nothing left to record.
func (s *IngestStore) CompleteClaimedJob(ctx context.Context, job domain.Job) error {
	if _, ok := contentJobQueue(job.Kind); ok {
		return nil
	}
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot start recording job completion (id=%d): %w", job.ID, err)
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `
		update jobs set
		state = case when exists (
			select 1 from video_locations l join videos v on v.id = l.video_id
			where v.id = ? and v.content_key = ? and l.id = ? and l.version = ? and l.path = ?
			and v.location_generation = ?
		) then 'done' else 'queued' end,
		last_error = null,
		location_id = case when exists (select 1 from video_locations where id = ? and version = ? and path = ?) then location_id else null end,
		updated_at = ? where id = ? and state = 'running'`,
		job.VideoID, job.ContentKey, job.LocationID, job.LocationVersion, job.LocationPath, job.LocationGeneration,
		job.LocationID, job.LocationVersion, job.LocationPath, time.Now().Unix(), job.ID)
	if err != nil {
		return fmt.Errorf("cannot record job completion (id=%d): %w", job.ID, err)
	}
	if err := s.commitJobChange(ctx, tx, job.ID); err != nil {
		return fmt.Errorf("cannot commit job completion (id=%d): %w", job.ID, err)
	}
	return nil
}

// Settle import activity and recompute remaining work in the job-state transaction.
func (s *IngestStore) commitJobChange(ctx context.Context, tx *sql.Tx, jobID int64) error {
	return s.commitJobChanges(ctx, tx, &changes{}, jobID)
}

// commitJobChanges hands on a force request and settles the job's activity after the transaction's other changes in c.
func (s *IngestStore) commitJobChanges(ctx context.Context, tx *sql.Tx, c *changes, jobID int64) error {
	if err := queueForceRequest(ctx, tx, c, jobID); err != nil {
		return err
	}
	if err := settleJobActivity(ctx, tx, c, jobID, time.Now()); err != nil {
		return err
	}
	c.remainingChanged()
	return s.db.commit(ctx, tx, c)
}

// CompleteJob skips claim-identity validation for callers without a claim, such as test setup.
func (s *IngestStore) CompleteJob(ctx context.Context, id int64) error {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `update jobs set state = 'done', last_error = null, updated_at = ? where id = ?`,
		time.Now().Unix(), id); err != nil {
		return err
	}
	return s.commitJobChange(ctx, tx, id)
}

// FailJob skips claim-identity validation for callers without a claim, such as test setup.
func (s *IngestStore) FailJob(ctx context.Context, id int64, reason string) error {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var attempts int
	var kind string
	err = tx.QueryRowContext(ctx, `select attempts, kind from jobs where id = ?`, id).Scan(&attempts, &kind)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	// Setup wants the state only, so the requeued row keeps its run-after time.
	state := jobRetryPolicy(domain.JobKind(kind)).AfterFailure(attempts, errors.New(reason), true, 0).State
	if _, err := tx.ExecContext(ctx, `update jobs set state = ?, last_error = ?, updated_at = ? where id = ?`,
		string(state), reason, time.Now().Unix(), id); err != nil {
		return err
	}
	return s.commitJobChange(ctx, tx, id)
}

// FailClaimedJob applies the kind's retry policy: it requeues at once inside a round, waits out the delay after a round,
// and records a terminal video failure in the same transaction. A snooze requeues without counting the claim.
func (s *IngestStore) FailClaimedJob(ctx context.Context, job domain.Job, cause error) error {
	if q, ok := contentJobQueue(job.Kind); ok {
		return s.db.failContentJob(ctx, q, job, cause)
	}
	reason := cause.Error()
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot start recording the job failure (id=%d): %w", job.ID, err)
	}
	defer func() { _ = tx.Rollback() }()

	// The row's force, not the claimed one: an owner's thumbnail edit since the claim clears it to keep that done state.
	var attempts int
	var forced bool
	err = tx.QueryRowContext(ctx, `select attempts, force from jobs where id = ? and state = 'running'`, job.ID).
		Scan(&attempts, &forced)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("cannot record a failure on the running job (id=%d, affected=0)", job.ID)
	}
	if err != nil {
		return fmt.Errorf("cannot check the job attempt count (id=%d): %w", job.ID, err)
	}
	current, err := jobIdentityCurrent(ctx, tx, job)
	if err != nil {
		return fmt.Errorf("cannot check the job location (id=%d): %w", job.ID, err)
	}
	outcome := jobRetryPolicy(job.Kind).AfterFailure(attempts, cause, job.LastLocation && current, jobJitter())
	state := outcome.State
	// An uncounted claim that started a round undoes the round, so the next claim starts and counts a new one.
	undoRound := outcome.Uncounted && job.StartsRound
	if undoRound {
		attempts--
	}

	now := jobNow().Unix()
	res, err := tx.ExecContext(ctx, `
		update jobs
		set state = ?,
		attempts = ?,
		last_error = ?,
		run_after = ?,
		location_id = case when exists (select 1 from video_locations where id = ? and version = ? and path = ?) then location_id else null end,
		location_path = case when ? then null else location_path end,
		updated_at = ? where id = ? and state = 'running'`,
		string(state), attempts, reason, now+int64(math.Ceil(outcome.Delay.Seconds())),
		job.LocationID, job.LocationVersion, job.LocationPath, undoRound,
		now, job.ID)
	if err != nil {
		return fmt.Errorf("cannot record the job failure (id=%d): %w", job.ID, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("cannot check the job update count (id=%d): %w", job.ID, err)
	}
	if affected != 1 {
		return fmt.Errorf("cannot record a failure on the running job (id=%d, affected=%d)", job.ID, affected)
	}

	var c changes
	if state == domain.JobFailed {
		if err := recordTerminalFailure(ctx, tx, job, cause, now, forced); err != nil {
			return err
		}
		if job.Kind == domain.JobSeekThumbnail {
			// Follow-up processes also run on a failed seek stage, with the images that remain.
			if err := queueAfterSeekStage(ctx, tx, &c, job, time.Now()); err != nil {
				return err
			}
		}
	}
	if err := s.commitJobChanges(ctx, tx, &c, job.ID); err != nil {
		return fmt.Errorf("cannot commit the job failure (id=%d): %w", job.ID, err)
	}
	return nil
}

// Record terminal failure atomically with job completion so a failed video always has a finished stage job; require the
// claimed identity to match. replaceDone records a forced rebuild's failure over a done state.
func recordTerminalFailure(ctx context.Context, tx *sql.Tx, job domain.Job, cause error, now int64, replaceDone bool) error {
	const identity = `id = ? and content_key = ? and location_generation = ? and exists (
			select 1 from video_locations where id = ? and video_id = ? and version = ? and path = ?
		)`
	identityArgs := []any{job.VideoID, job.ContentKey, job.LocationGeneration,
		job.LocationID, job.VideoID, job.LocationVersion, job.LocationPath}

	// A forced job replaces a done or failed state too, so a failed rebuild is not shown as the old success.
	forced := boolToInt(replaceDone)

	var updated sql.Result
	switch job.Kind {
	case domain.JobProbe:
		// Only replace pending state so a later completion-recording failure cannot overwrite a saved probe result.
		res, err := tx.ExecContext(ctx, `update videos set probe_state = 'failed', probe_error = ?, probe_error_code = ?,
			playable = 0, indexed_at = ?
			where (probe_state = 'pending' or ? = 1) and `+identity,
			append([]any{cause.Error(), string(domain.ProbeErrorCodeOf(cause)), now, forced}, identityArgs...)...)
		if err != nil {
			return fmt.Errorf("cannot record the final probe failure (job=%d): %w", job.ID, err)
		}
		updated = res
	case domain.JobThumbnail:
		// A seek-preview failure must not replace a completed representative thumbnail's state.
		res, err := tx.ExecContext(ctx, `update videos set thumbnail_state = 'failed', indexed_at = ?
			where (thumbnail_state <> 'done' or ? = 1) and `+identity,
			append([]any{now, forced}, identityArgs...)...)
		if err != nil {
			return fmt.Errorf("cannot record the final thumbnail failure (job=%d): %w", job.ID, err)
		}
		updated = res
	case domain.JobSeekThumbnail:
		// The representative thumbnail is a separate job; change only seek-thumbnail state.
		res, err := tx.ExecContext(ctx, `update videos set seek_thumbnail_state = 'failed', indexed_at = ?
			where (seek_thumbnail_state <> 'done' or ? = 1) and `+identity,
			append([]any{now, forced}, identityArgs...)...)
		if err != nil {
			return fmt.Errorf("cannot record the final seek thumbnail failure (job=%d): %w", job.ID, err)
		}
		updated = res
	case domain.JobFingerprint:
		// Fingerprints have no video-state column; record an issue only for the current identity and let the next scan retry.
		var current int
		if err := tx.QueryRowContext(ctx, `select exists (select 1 from videos where `+identity+`)`,
			identityArgs...).Scan(&current); err != nil {
			return fmt.Errorf("cannot check the fingerprint job identity (job=%d): %w", job.ID, err)
		}
		if current != 1 {
			return nil
		}
		return recordFailedIssue(ctx, tx, job, now)
	case domain.JobPreview:
		res, err := tx.ExecContext(ctx, `update videos set preview_state = 'failed', indexed_at = ?
			where `+identity, append([]any{now}, identityArgs...)...)
		if err != nil {
			return fmt.Errorf("cannot record the final preview failure (job=%d): %w", job.ID, err)
		}
		affected, rowsErr := res.RowsAffected()
		if rowsErr != nil {
			return fmt.Errorf("cannot check the preview state update count (job=%d): %w", job.ID, rowsErr)
		}
		if affected != 1 {
			return fmt.Errorf("cannot record the final failure on the current preview (job=%d, affected=%d)", job.ID, affected)
		}
		return recordFailedIssue(ctx, tx, job, now)
	}
	if updated == nil {
		return nil
	}
	count, err := updated.RowsAffected()
	if err != nil {
		return fmt.Errorf("cannot check the final failure update count (job=%d): %w", job.ID, err)
	}
	if count != 1 {
		return nil
	}
	return recordFailedIssue(ctx, tx, job, now)
}

// Record import issues only after terminal failure, never for intermediate retries.
func recordFailedIssue(ctx context.Context, tx *sql.Tx, job domain.Job, now int64) error {
	issue, ok := domain.FailedIssueKind(job.Kind)
	if !ok {
		return nil
	}
	return recordScanIssue(ctx, tx, job.VideoID, job.LocationPath, issue, now)
}

// JobIdentityCurrent checks the claimed content key, location and location generation.
func (s *IngestStore) JobIdentityCurrent(ctx context.Context, job domain.Job) (bool, error) {
	return jobIdentityCurrent(ctx, s.db.sql, job)
}

type rowQueryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func jobIdentityCurrent(ctx context.Context, q rowQueryer, job domain.Job) (bool, error) {
	var current int
	err := q.QueryRowContext(ctx, `select exists (
		select 1 from videos v join video_locations l on l.video_id = v.id
		where v.id = ? and v.content_key = ? and l.id = ? and l.version = ? and l.path = ?
		and v.location_generation = ?)`, job.VideoID, job.ContentKey, job.LocationID,
		job.LocationVersion, job.LocationPath, job.LocationGeneration).Scan(&current)
	return current == 1, err
}

// NextJobRunAfter returns the earliest run-after time of a queued job after since, or the zero time when none is.
// A time already past means the job came due after claims that began at since.
func (s *IngestStore) NextJobRunAfter(ctx context.Context, since time.Time) (time.Time, error) {
	var next sql.NullInt64
	if err := s.db.sql.QueryRowContext(ctx, `select min(run_after) from jobs where state = 'queued' and run_after > ?`,
		since.Unix()).Scan(&next); err != nil {
		return time.Time{}, fmt.Errorf("cannot look up the next job retry time: %w", err)
	}
	if !next.Valid {
		return time.Time{}, nil
	}
	return time.Unix(next.Int64, 0), nil
}

// DueJobKinds returns, for each kind with a queued job whose run-after time has come, the most urgent class among
// them, so the scheduler claims only kinds and classes that have work.
func (s *IngestStore) DueJobKinds(ctx context.Context) (map[domain.JobKind]domain.JobPriority, error) {
	rows, err := s.db.sql.QueryContext(ctx, `select kind, min(priority) from jobs
		where state = 'queued' and run_after <= ? group by kind`, jobNow().Unix())
	if err != nil {
		return nil, fmt.Errorf("cannot look up due jobs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	due := map[domain.JobKind]domain.JobPriority{}
	for rows.Next() {
		var kind string
		var priority domain.JobPriority
		if err := rows.Scan(&kind, &priority); err != nil {
			return nil, fmt.Errorf("cannot read due jobs: %w", err)
		}
		due[domain.JobKind(kind)] = priority
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read due jobs: %w", err)
	}
	return due, nil
}

// HasUnfinishedJobs reports unfinished import stages only; other kinds do not hold back an import.
func (s *IngestStore) HasUnfinishedJobs(ctx context.Context) (bool, error) {
	var found bool
	err := s.db.sql.QueryRowContext(ctx,
		`select exists (select 1 from jobs where state in ('queued', 'running') and kind in (`+importJobKinds+`))`).Scan(&found)
	if err != nil {
		return false, fmt.Errorf("cannot look up unfinished jobs: %w", err)
	}
	return found, nil
}

// RequeueRunningJobs recovers interrupted work at startup and recomputes completion using runtime OS-specific
// eligibility; a row with a force request comes back forced and in the owner class, as queueForceRequest would.
func (s *IngestStore) RequeueRunningJobs(ctx context.Context) (int64, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("cannot requeue interrupted jobs: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx,
		`update jobs set state = 'queued', force = max(force, force_requested),
		priority = case when force_requested = 1 then ? else priority end, force_requested = 0, updated_at = ?
		where state = 'running'`,
		int(domain.JobPriorityOwner), time.Now().Unix())
	if err != nil {
		return 0, fmt.Errorf("cannot requeue interrupted jobs: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("cannot requeue interrupted jobs: %w", err)
	}
	var c changes
	c.remainingChanged()
	if affected > 0 {
		c.jobsQueued(domain.ScheduledJobKinds...)
	}

	if err := s.db.commit(ctx, tx, &c); err != nil {
		return 0, fmt.Errorf("cannot requeue interrupted jobs: %w", err)
	}
	return affected, nil
}
