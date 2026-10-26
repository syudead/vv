package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/syudead/vv/internal/domain"
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
		insert into jobs (kind, video_id, state, attempts, created_at, updated_at)
		select ?, ?, 'queued', 0, ?, ?
		where not exists (select 1 from jobs where kind = ? and video_id = ?) and `+pending,
		string(kind), videoID, now, now, string(kind), videoID, videoID)
	if err != nil {
		return false, fmt.Errorf("cannot restore missing jobs (%s, video=%d): %w", kind, videoID, err)
	}
	inserted, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("cannot count restored jobs (%s, video=%d): %w", kind, videoID, err)
	}
	return inserted > 0, nil
}

// ClaimJob uses BEGIN IMMEDIATE to prevent duplicate claims across workers; an empty queue returns ErrNoJob.
func (s *IngestStore) ClaimJob(ctx context.Context, kind domain.JobKind) (domain.Job, error) {
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

	var job domain.Job
	var kindName string
	var previousPath sql.NullString
	// Keep SQL eligibility equivalent to domain.ClaimConditionFor.
	queuedJobSQL := `select j.id, j.kind, j.video_id, j.attempts, j.force, j.location_path from jobs j
		where j.state = 'queued' and j.kind = ?` + claimConditionSQL(domain.ClaimConditionFor(kind), "j") + `
		order by j.id limit 1`
	err = conn.QueryRowContext(ctx, queuedJobSQL, string(kind)).Scan(&job.ID, &kindName, &job.VideoID, &job.Attempts, &job.Force, &previousPath)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Job{}, domain.ErrNoJob
	}
	if err != nil {
		return domain.Job{}, fmt.Errorf("cannot claim a job: %w", err)
	}
	job.Kind = domain.JobKind(kindName)
	var locationID, locationVersion int64
	var locationPath, contentKey string
	// Resume after the previous location in path order; wrapping starts a new attempt cycle.
	selectLocation := `select l.id, l.version, l.path, v.content_key
		from video_locations l join videos v on v.id = l.video_id
		where l.video_id = ? and ` + registeredLocationCondition("l")
	startsRound := !previousPath.Valid
	if previousPath.Valid {
		err = conn.QueryRowContext(ctx, selectLocation+` and l.path > ? order by l.path limit 1`, job.VideoID, previousPath.String).
			Scan(&locationID, &locationVersion, &locationPath, &contentKey)
		startsRound = errors.Is(err, sql.ErrNoRows)
	}
	if startsRound {
		err = conn.QueryRowContext(ctx, selectLocation+` order by l.path limit 1`, job.VideoID).
			Scan(&locationID, &locationVersion, &locationPath, &contentKey)
	}
	job.Attempts = domain.ClaimAttempts(job.Attempts, startsRound)
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
	job.ContentKey = contentKey
	job.LocationID = locationID
	job.LocationVersion = locationVersion
	job.LocationPath = locationPath
	var hasLaterLocation int
	if err := conn.QueryRowContext(ctx, `select exists (
		select 1 from video_locations l where l.video_id = ? and l.path > ? and `+registeredLocationCondition("l")+`)`,
		job.VideoID, job.LocationPath).Scan(&hasLaterLocation); err != nil {
		return domain.Job{}, fmt.Errorf("cannot check the job location's final state: %w", err)
	}
	job.LastLocation = hasLaterLocation == 0
	if err := conn.QueryRowContext(ctx, `select location_generation from videos where id = ?`, job.VideoID).
		Scan(&job.LocationGeneration); err != nil {
		return domain.Job{}, fmt.Errorf("cannot check the job's location generation: %w", err)
	}

	if _, err := conn.ExecContext(ctx, `
		update jobs set state = 'running', attempts = ?, location_id = ?, location_version = ?,
		location_path = ?, updated_at = ? where id = ?`,
		job.Attempts, locationID, locationVersion, locationPath, time.Now().Unix(), job.ID,
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

// Keep each SQL predicate equivalent to domain.JobClaimCondition.Allows.
func claimConditionSQL(c domain.JobClaimCondition, alias string) string {
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
			registeredLocationCondition("tl") + `))`
	}
	return cond
}

// CompleteClaimedJob requeues when the claimed location or content changed.
func (s *IngestStore) CompleteClaimedJob(ctx context.Context, job domain.Job) error {
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
	err = tx.QueryRowContext(ctx, `select attempts from jobs where id = ?`, id).Scan(&attempts)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	state := domain.JobStateAfterFailure(attempts, true, true)
	if _, err := tx.ExecContext(ctx, `update jobs set state = ?, last_error = ?, updated_at = ? where id = ?`,
		string(state), reason, time.Now().Unix(), id); err != nil {
		return err
	}
	return s.commitJobChange(ctx, tx, id)
}

// FailClaimedJob applies the domain retry decision and records terminal video failure in the same transaction.
func (s *IngestStore) FailClaimedJob(ctx context.Context, job domain.Job, cause error) error {
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
	state := domain.JobStateAfterFailure(attempts, job.LastLocation, current)

	now := time.Now().Unix()
	res, err := tx.ExecContext(ctx, `
		update jobs
		set state = ?,
		last_error = ?,
		location_id = case when exists (select 1 from video_locations where id = ? and version = ? and path = ?) then location_id else null end,
		updated_at = ? where id = ? and state = 'running'`,
		string(state), reason,
		job.LocationID, job.LocationVersion, job.LocationPath,
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

func (s *IngestStore) HasUnfinishedJobs(ctx context.Context) (bool, error) {
	var found bool
	err := s.db.sql.QueryRowContext(ctx,
		`select exists (select 1 from jobs where state in ('queued', 'running'))`).Scan(&found)
	if err != nil {
		return false, fmt.Errorf("cannot look up unfinished jobs: %w", err)
	}
	return found, nil
}

// RequeueRunningJobs recovers interrupted work at startup and recomputes completion using runtime OS-specific
// eligibility; a row with a force request comes back forced.
func (s *IngestStore) RequeueRunningJobs(ctx context.Context) (int64, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("cannot requeue interrupted jobs: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx,
		`update jobs set state = 'queued', force = max(force, force_requested), force_requested = 0, updated_at = ?
		where state = 'running'`,
		time.Now().Unix())
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
		c.jobsQueued(domain.JobKinds...)
	}
	if err := s.db.commit(ctx, tx, &c); err != nil {
		return 0, fmt.Errorf("cannot requeue interrupted jobs: %w", err)
	}
	return affected, nil
}
