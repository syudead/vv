package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// ApplyProbe writes domain decisions and related probe rows atomically; unknown values remain NULL rather than zero.
func (s *IngestStore) ApplyProbe(
	ctx context.Context, id int64, probe domain.Probe, play domain.Playability,
) error {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot apply the probe result (id=%d): %w", id, err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now()
	res, err := tx.ExecContext(ctx, `
		update videos
		   set duration_ms = ?, width = ?, height = ?, display_aspect_ratio = ?,
		       video_codec = ?, audio_codec = ?,
		       playable = ?, unplayable_reason = ?,
		       probe_state = 'done', probe_error = null, probe_error_code = null, indexed_at = ?
		 where id = ?`,
		nullableInt64(probe.DurationMs), nullableInt(probe.Width), nullableInt(probe.Height), nullableFloat64(probe.DisplayAspectRatio),
		nullableString(probe.VideoCodec), nullableString(probe.AudioCodec),
		boolToInt(play.Playable), nullableString(string(play.Reason)),
		now.Unix(), id,
	)
	if err != nil {
		return fmt.Errorf("cannot apply the probe result (id=%d): %w", id, err)
	}
	count, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("cannot apply the probe result (id=%d): %w", id, err)
	}
	var succeeded succession
	if count == 1 {
		if err := upsertProbeRows(ctx, tx, id, probe, now); err != nil {
			return err
		}
		var contentKey string
		if err := tx.QueryRowContext(ctx, `select content_key from videos where id = ?`, id).Scan(&contentKey); err != nil {
			return fmt.Errorf("cannot apply the probe result (id=%d): %w", id, err)
		}
		if succeeded, err = applySuccession(ctx, tx, contentKey, probe.DurationMs); err != nil {
			return err
		}
	}
	if err := s.db.commitMatchChangeIf(ctx, tx, count == 1, successionEvents(succeeded)...); err != nil {
		return fmt.Errorf("cannot apply the probe result (id=%d): %w", id, err)
	}
	return nil
}

// Write related probe rows only after updating the matching video, so replaced files cannot retain stale results.
func upsertProbeRows(ctx context.Context, tx *sql.Tx, videoID int64, probe domain.Probe, now time.Time) error {
	if probe.Transcode != nil {
		if err := upsertTranscodeProbe(ctx, tx, videoID, probe.Source, *probe.Transcode, now); err != nil {
			return err
		}
	}
	if probe.Keyframes != nil && probe.Keyframes.Version == domain.KeyframeProbeVersion {
		if err := upsertKeyframeSpacing(ctx, tx, videoID, *probe.Keyframes, true); err != nil {
			return err
		}
	}
	if probe.Media != nil {
		return upsertMediaInfo(ctx, tx, videoID, *probe.Media, now)
	}
	return nil
}

// ApplyProbeForJob writes only while the claimed identity is current; enqueue preview work atomically so import never appears briefly complete.
func (s *IngestStore) ApplyProbeForJob(
	ctx context.Context, job domain.Job, probe domain.Probe, play domain.Playability,
) (bool, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now()
	res, err := tx.ExecContext(ctx, `
		update videos set duration_ms = ?, width = ?, height = ?, display_aspect_ratio = ?, video_codec = ?, audio_codec = ?,
		playable = ?, unplayable_reason = ?, probe_state = 'done', probe_error = null, probe_error_code = null, indexed_at = ?
		where id = ? and content_key = ? and exists (
			select 1 from video_locations where video_id = videos.id and id = ? and version = ? and path = ?)
		and location_generation = ?`,
		nullableInt64(probe.DurationMs), nullableInt(probe.Width), nullableInt(probe.Height), nullableFloat64(probe.DisplayAspectRatio),
		nullableString(probe.VideoCodec), nullableString(probe.AudioCodec), boolToInt(play.Playable),
		nullableString(string(play.Reason)), now.Unix(), job.VideoID, job.ContentKey,
		job.LocationID, job.LocationVersion, job.LocationPath, job.LocationGeneration)
	if err != nil {
		return false, err
	}
	count, err := res.RowsAffected()
	if err != nil || count != 1 {
		return false, err
	}
	if err := upsertProbeRows(ctx, tx, job.VideoID, probe, now); err != nil {
		return false, err
	}
	if err := clearFailedIssue(ctx, tx, domain.JobProbe, job.VideoID); err != nil {
		return false, err
	}
	var c changes
	if err := queueJob(ctx, tx, &c, domain.JobPreview, job.VideoID, now.Unix(), ruleRun(), job.Force); err != nil {
		return false, err
	}
	// If probing finishes after the scan, resolve same-path succession here.
	succeeded, err := applySuccession(ctx, tx, job.ContentKey, probe.DurationMs)
	if err != nil {
		return false, err
	}
	c.jobsQueued(domain.JobPreview)
	// Probe results can change smart-folder matches even without succession; the earlier scan event cannot cover this update.
	c.matchesChanged()
	if err := s.db.commit(ctx, tx, &c); err != nil {
		return false, err
	}
	s.db.publishEvents(successionEvents(succeeded)...)
	return true, nil
}

// SaveTranscodeProbe uses the opened file's size and modification time; one upsert makes concurrent writes last-writer-wins.
func (s *IngestStore) SaveTranscodeProbe(
	ctx context.Context, videoID int64, source domain.FileStamp, probe domain.TranscodeProbe,
) error {
	return upsertTranscodeProbe(ctx, s.db.sql, videoID, source, probe, time.Now())
}

func upsertTranscodeProbe(
	ctx context.Context, db queryExecer, videoID int64, source domain.FileStamp, probe domain.TranscodeProbe, now time.Time,
) error {
	encoded, err := json.Marshal(probe)
	if err != nil {
		return fmt.Errorf("cannot encode the transcode probe as JSON (id=%d): %w", videoID, err)
	}
	if _, err := db.ExecContext(ctx, `
		insert into video_transcode_probes (video_id, version, size_bytes, mtime_ns, probe, updated_at)
		values (?, ?, ?, ?, ?, ?)
		on conflict (video_id) do update
		   set version = excluded.version, size_bytes = excluded.size_bytes, mtime_ns = excluded.mtime_ns,
		       probe = excluded.probe, updated_at = excluded.updated_at`,
		videoID, domain.TranscodeProbeVersion, source.SizeBytes, source.ModTimeNs, string(encoded), now.Unix(),
	); err != nil {
		return fmt.Errorf("cannot save the transcode probe (id=%d): %w", videoID, err)
	}
	return nil
}

// MarkProbeFailed keeps the video listed and stores its reason code, defaulting unclassified errors to internal.
func (s *IngestStore) MarkProbeFailed(ctx context.Context, id int64, cause error) error {
	_, err := s.db.sql.ExecContext(ctx, `
		update videos
		   set probe_state = 'failed', probe_error = ?, probe_error_code = ?, playable = 0, indexed_at = ?
		 where id = ?`,
		cause.Error(), string(domain.ProbeErrorCodeOf(cause)), time.Now().Unix(), id,
	)
	if err != nil {
		return fmt.Errorf("cannot record the probe failure (id=%d): %w", id, err)
	}
	return nil
}

func (s *IngestStore) SetThumbnailState(ctx context.Context, id int64, state domain.ThumbnailState) error {
	_, err := s.db.sql.ExecContext(ctx,
		`update videos set thumbnail_state = ?, indexed_at = ? where id = ?`,
		string(state), time.Now().Unix(), id)
	if err != nil {
		return fmt.Errorf("cannot record the thumbnail state (id=%d): %w", id, err)
	}
	return nil
}

// SetThumbnailStateForJob checks the claimed identity and atomically clears failures and updates substitution when complete.
func (s *IngestStore) SetThumbnailStateForJob(
	ctx context.Context, job domain.Job, state domain.ThumbnailState, substitution domain.Substitution,
) (bool, error) {
	return s.setStageStateForJob(ctx, job, domain.JobThumbnail, "thumbnail_state", string(state),
		state == domain.ThumbnailStateDone, substitution, nil)
}

// Write only for the current claimed identity; success clears failures, records substitutions and queues the next stage atomically.
func (s *IngestStore) setStageStateForJob(
	ctx context.Context, job domain.Job, kind domain.JobKind, column, state string, succeeded bool,
	substitution domain.Substitution, next []domain.JobKind,
) (bool, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, `update videos set `+column+` = ?, indexed_at = ?`+buildIncrement(kind, job, succeeded)+`
		where id = ? and content_key = ? and exists (
			select 1 from video_locations where video_id = videos.id and id = ? and version = ? and path = ?)
		and location_generation = ?`,
		state, time.Now().Unix(), job.VideoID, job.ContentKey, job.LocationID,
		job.LocationVersion, job.LocationPath, job.LocationGeneration)
	if err != nil {
		return false, err
	}
	count, err := res.RowsAffected()
	if err != nil || count != 1 {
		return false, err
	}
	if succeeded {
		if err := clearFailedIssue(ctx, tx, kind, job.VideoID); err != nil {
			return false, err
		}
		if err := applySubstitution(ctx, tx, kind, job, substitution); err != nil {
			return false, err
		}
	}
	var c changes
	if succeeded && len(next) > 0 {
		for _, following := range next {
			if err := queueJob(ctx, tx, &c, following, job.VideoID, time.Now().Unix(), ruleRun(), job.Force); err != nil {
				return false, err
			}
		}
		c.jobsQueued(next...)
	}
	// A forced seek stage belongs to a rescan, which re-runs the import stages only (docs/design-docs/auto-tagging.md).
	if kind == domain.JobSeekThumbnail && !job.Force {
		if err := queueAfterSeekStage(ctx, tx, &c, job, time.Now()); err != nil {
			return false, err
		}
	}
	if err := s.db.commit(ctx, tx, &c); err != nil {
		return false, err
	}
	return true, nil
}

// buildColumnsByKind names the column that counts a kind's forced rebuilds; probe and fingerprint publish no artifact URL.
var buildColumnsByKind = map[domain.JobKind]string{
	domain.JobThumbnail:     "thumbnail_build",
	domain.JobSeekThumbnail: "seek_thumbnail_build",
	domain.JobPreview:       "preview_build",
}

// buildIncrement is the SET clause that counts a forced job's successful rebuild, so artifact URLs change with it.
func buildIncrement(kind domain.JobKind, job domain.Job, succeeded bool) string {
	column, ok := buildColumnsByKind[kind]
	if !ok || !succeeded || !job.Force {
		return ""
	}
	return `, ` + column + ` = ` + column + ` + 1`
}

// SetSeekThumbnailStateForJob checks the claimed identity and queues fingerprints on completion, including rebuilt
// sprites, and after-import auto-tagging when the stage finishes for good.
func (s *IngestStore) SetSeekThumbnailStateForJob(
	ctx context.Context, job domain.Job, state domain.SeekThumbnailState, substitution domain.Substitution,
) (bool, error) {
	applied, err := s.setStageStateForJob(ctx, job, domain.JobSeekThumbnail, "seek_thumbnail_state", string(state),
		state == domain.SeekThumbnailDone, substitution, []domain.JobKind{domain.JobFingerprint})
	if err != nil {
		return false, fmt.Errorf("cannot record the seek thumbnail state (id=%d): %w", job.VideoID, err)
	}
	return applied, nil
}

// SetPreviewStateForJob applies only to the content and location generation
// captured when the preview job was claimed.
func (s *IngestStore) SetPreviewStateForJob(ctx context.Context, job domain.Job, state domain.PreviewState) (bool, error) {
	return s.setStageStateForJob(ctx, job, domain.JobPreview, "preview_state", string(state),
		state == domain.PreviewStateDone, domain.SubstitutionUnknown, nil)
}

// SetPreviewStateForContent accepts a completed asset after a representative
// location changed, while still refusing a stale content identity.
func (s *IngestStore) SetPreviewStateForContent(ctx context.Context, job domain.Job, state domain.PreviewState) (bool, error) {
	res, err := s.db.sql.ExecContext(ctx, `update videos set preview_state = ?, indexed_at = ?
		where id = ? and content_key = ?`, string(state), time.Now().Unix(), job.VideoID, job.ContentKey)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// CompletePreviewForContent atomically marks the content-keyed asset and its
// claimed job complete. Location-only changes do not invalidate the asset.
func (s *IngestStore) CompletePreviewForContent(ctx context.Context, job domain.Job) (bool, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().Unix()
	res, err := tx.ExecContext(ctx, `update videos set preview_state = 'done', indexed_at = ?`+
		buildIncrement(domain.JobPreview, job, true)+`
		where id = ? and content_key = ?`, now, job.VideoID, job.ContentKey)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil || n == 0 {
		return false, err
	}
	res, err = tx.ExecContext(ctx, `update jobs set state = 'done', last_error = null, updated_at = ?
		where id = ? and kind = 'preview' and video_id = ? and state = 'running'`, now, job.ID, job.VideoID)
	if err != nil {
		return false, err
	}
	n, err = res.RowsAffected()
	if err != nil {
		return false, err
	}
	if n != 1 {
		return false, fmt.Errorf("preview job is not running (id=%d)", job.ID)
	}
	if err := clearFailedIssue(ctx, tx, domain.JobPreview, job.VideoID); err != nil {
		return false, err
	}
	var c changes
	if err := queueForceRequest(ctx, tx, &c, job.ID); err != nil {
		return false, err
	}
	if err := settleJobActivity(ctx, tx, &c, job.ID, time.Now()); err != nil {
		return false, err
	}
	c.remainingChanged()
	if err := s.db.commit(ctx, tx, &c); err != nil {
		return false, err
	}
	return true, nil
}

func (s *IngestStore) ContentKeyCurrent(ctx context.Context, videoID int64, key string) (bool, error) {
	var current int
	err := s.db.sql.QueryRowContext(ctx, `select exists(select 1 from videos where id = ? and content_key = ?)`, videoID, key).Scan(&current)
	return current != 0, err
}

// PreviewSourceCurrent accepts location-only churn while rejecting a claimed
// source path that was reassigned to different content during generation.
func (s *IngestStore) PreviewSourceCurrent(ctx context.Context, job domain.Job) (bool, error) {
	var current int
	err := s.db.sql.QueryRowContext(ctx, `select exists (
		select 1 from videos where id = ? and content_key = ?
	) and not exists (
		select 1 from video_locations l join videos v on v.id = l.video_id
		where l.path = ? and v.content_key <> ?
	)`, job.VideoID, job.ContentKey, job.LocationPath, job.ContentKey).Scan(&current)
	return current != 0, err
}

// ThumbnailSourceCurrent rejects locations removed or reassigned since generation was requested.
func (s *IngestStore) ThumbnailSourceCurrent(
	ctx context.Context, videoID int64, key, locationPath string,
) (bool, error) {
	var current int
	err := s.db.sql.QueryRowContext(ctx, `select exists (
		select 1 from videos where id = ? and content_key = ?
	) and exists (
		select 1 from video_locations l join videos v on v.id = l.video_id
		where l.path = ? and v.content_key = ?
	)`, videoID, key, locationPath, key).Scan(&current)
	return current != 0, err
}

func (s *IngestStore) SetPreviewState(ctx context.Context, id int64, state domain.PreviewState) error {
	_, err := s.db.sql.ExecContext(ctx, `update videos set preview_state = ?, indexed_at = ? where id = ?`, string(state), time.Now().Unix(), id)
	return err
}

// RequeueMissingPreview queues at most once for unchanged completed content; the caller checks the filesystem.
func (s *IngestStore) RequeueMissingPreview(ctx context.Context, id int64, contentKey string) (bool, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("cannot start regenerating the preview (id=%d): %w", id, err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().Unix()
	res, err := tx.ExecContext(ctx, `update videos set preview_state = 'pending', indexed_at = ?
		where id = ? and content_key = ? and preview_state = 'done'`, now, id, contentKey)
	if err != nil {
		return false, fmt.Errorf("cannot reset the preview state (id=%d): %w", id, err)
	}
	reset, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("cannot check the preview state update count (id=%d): %w", id, err)
	}
	if reset == 0 {
		return false, nil
	}
	var c changes
	if err := requeueJob(ctx, tx, &c, domain.JobPreview, id, now, ruleRun()); err != nil {
		return false, err
	}
	c.jobsQueued(domain.JobPreview)
	if err := s.db.commit(ctx, tx, &c); err != nil {
		return false, fmt.Errorf("cannot commit regenerating the preview (id=%d): %w", id, err)
	}
	return true, nil
}

// RequeueMissingSeekThumbnails queues at most once for unchanged completed content; the caller checks the filesystem.
func (s *IngestStore) RequeueMissingSeekThumbnails(ctx context.Context, id int64, contentKey string) (bool, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("cannot start regenerating seek thumbnails (id=%d): %w", id, err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().Unix()
	res, err := tx.ExecContext(ctx, `update videos set seek_thumbnail_state = 'pending', indexed_at = ?
		where id = ? and content_key = ? and seek_thumbnail_state = 'done'`, now, id, contentKey)
	if err != nil {
		return false, fmt.Errorf("cannot reset the seek thumbnail state (id=%d): %w", id, err)
	}
	reset, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("cannot check the seek thumbnail state update count (id=%d): %w", id, err)
	}
	if reset == 0 {
		return false, nil
	}
	var c changes
	if err := requeueJob(ctx, tx, &c, domain.JobSeekThumbnail, id, now, ruleRun()); err != nil {
		return false, err
	}
	// Drop blocked fingerprint work so failed sprite rebuilds do not count forever; completion queues it again.
	if _, err := tx.ExecContext(ctx, `delete from jobs where kind = ? and video_id = ? and state = 'queued'`,
		string(domain.JobFingerprint), id); err != nil {
		return false, fmt.Errorf("cannot drop the waiting fingerprint job (id=%d): %w", id, err)
	}
	c.jobsQueued(domain.JobSeekThumbnail)
	if err := s.db.commit(ctx, tx, &c); err != nil {
		return false, fmt.Errorf("cannot commit regenerating seek thumbnails (id=%d): %w", id, err)
	}
	return true, nil
}

// Replace finished jobs and join the running import activity; callers mark jobsQueued so commit recomputes completion.
func requeueJob(
	ctx context.Context, tx *sql.Tx, c *changes, kind domain.JobKind, id, now int64, run importRun,
) error {
	return queueJob(ctx, tx, c, kind, id, now, run, false)
}

// queueJob replaces finished rows of the kind; force follows the existing-row rules of insertQueuedJob.
func queueJob(
	ctx context.Context, tx *sql.Tx, c *changes, kind domain.JobKind, id, now int64, run importRun, force bool,
) error {
	if _, err := tx.ExecContext(ctx, `delete from jobs where kind = ? and video_id = ? and state in ('done', 'failed')`,
		string(kind), id); err != nil {
		return fmt.Errorf("cannot clean up old jobs (%s, video=%d): %w", kind, id, err)
	}
	return insertQueuedJob(ctx, tx, c, kind, id, now, run, force)
}

// A forced insert over a queued row sets its force, and over an unforced running row sets force_requested, because
// the unique index allows one queued or running row per kind and video.
const (
	keepPendingJob  = `do nothing`
	forcePendingJob = `do update set
		force = case when state = 'queued' then 1 else force end,
		force_requested = case when state = 'running' and force = 0 then 1 else force_requested end`
)

func insertQueuedJob(
	ctx context.Context, tx *sql.Tx, c *changes, kind domain.JobKind, id, now int64, run importRun, force bool,
) error {
	onPending := keepPendingJob
	if force {
		onPending = forcePendingJob
	}
	if _, err := tx.ExecContext(ctx, `insert into jobs (kind, video_id, state, attempts, force, created_at, updated_at)
		values (?, ?, 'queued', 0, ?, ?, ?)
		on conflict (kind, video_id) where state in ('queued', 'running') `+onPending,
		string(kind), id, boolToInt(force), now, now); err != nil {
		return fmt.Errorf("cannot queue the job (%s, video=%d): %w", kind, id, err)
	}
	if err := joinImportActivity(ctx, tx, c, id, run, time.Now()); err != nil {
		return err
	}
	return addScanVideos(ctx, tx, id)
}

// queueForceRequest runs after a row's state change in the same transaction: a row back in queued takes the request
// as its force, and a finished row queues a forced row of its kind beside it, so the row's activity still settles.
func queueForceRequest(ctx context.Context, tx *sql.Tx, c *changes, jobID int64) error {
	var kind, state string
	var videoID int64
	err := tx.QueryRowContext(ctx, `select kind, video_id, state from jobs
		where id = ? and force_requested = 1 and state <> 'running'`, jobID).
		Scan(&kind, &videoID, &state)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot read the job's force request (id=%d): %w", jobID, err)
	}
	if _, err := tx.ExecContext(ctx, `update jobs set force_requested = 0,
		force = case when state = 'queued' then 1 else force end where id = ?`, jobID); err != nil {
		return fmt.Errorf("cannot take the job's force request (id=%d): %w", jobID, err)
	}
	if state == string(domain.JobQueued) {
		return nil
	}
	if err := insertQueuedJob(ctx, tx, c, domain.JobKind(kind), videoID, time.Now().Unix(), ruleRun(), true); err != nil {
		return err
	}
	c.jobsQueued(domain.JobKind(kind))
	return nil
}

// RetryProbe requires a failed probe and atomically resets unfinished stages; probe completion queues preview work.
func (s *IngestStore) RetryProbe(ctx context.Context, id int64) error {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot start re-reading (id=%d): %w", id, err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().Unix()
	res, err := tx.ExecContext(ctx, `update videos set probe_state = 'pending', probe_error = null, probe_error_code = null, indexed_at = ?
		where id = ? and probe_state = 'failed'`, now, id)
	if err != nil {
		return fmt.Errorf("cannot reset the probe state (id=%d): %w", id, err)
	}
	reset, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("cannot check the probe state update count (id=%d): %w", id, err)
	}
	if reset == 0 {
		var exists int
		if err := tx.QueryRowContext(ctx, `select exists(select 1 from videos where id = ?)`, id).Scan(&exists); err != nil {
			return fmt.Errorf("cannot check whether the video exists (id=%d): %w", id, err)
		}
		if exists == 0 {
			return domain.ErrNotFound
		}
		return domain.ErrProbeNotFailed
	}

	following, err := resetFailedStages(ctx, tx, id, now, false)
	if err != nil {
		return err
	}
	kinds := append([]domain.JobKind{domain.JobProbe}, following...)
	var c changes
	for _, kind := range kinds {
		if err := requeueJob(ctx, tx, &c, kind, id, now, importRun{decider: domain.ActivityByOwner}); err != nil {
			return err
		}
	}
	c.jobsQueued(kinds...)
	if err := s.db.commit(ctx, tx, &c); err != nil {
		return fmt.Errorf("cannot commit re-reading (id=%d): %w", id, err)
	}
	return nil
}

// Queue previews only after probing completes; otherwise ApplyProbeForJob queues them with its result.
func resetFailedStages(ctx context.Context, tx *sql.Tx, id, now int64, probeDone bool) ([]domain.JobKind, error) {
	stages := []struct {
		kind      domain.JobKind
		column    string
		condition string
		queue     bool
	}{
		{kind: domain.JobThumbnail, column: "thumbnail_state", condition: "<> 'done'", queue: true},
		{kind: domain.JobSeekThumbnail, column: "seek_thumbnail_state", condition: "= 'failed'", queue: true},
		{kind: domain.JobPreview, column: "preview_state", condition: "= 'failed'", queue: probeDone},
	}
	var kinds []domain.JobKind
	for _, stage := range stages {
		res, err := tx.ExecContext(ctx, `update videos set `+stage.column+` = 'pending', indexed_at = ?
			where id = ? and `+stage.column+` `+stage.condition, now, id)
		if err != nil {
			return nil, fmt.Errorf("cannot reset the %s state (id=%d): %w", stage.kind, id, err)
		}
		reset, err := res.RowsAffected()
		if err != nil {
			return nil, fmt.Errorf("cannot check the %s state update count (id=%d): %w", stage.kind, id, err)
		}
		if reset > 0 && stage.queue {
			kinds = append(kinds, stage.kind)
		}
	}
	return kinds, nil
}

// RetryImport rechecks eligibility atomically and opens an owner activity for retried or interrupted work; concurrent repeated retries fail.
func (s *IngestStore) RetryImport(ctx context.Context, entryID int64, contentKey string) error {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot start retrying the import (%s): %w", contentKey, err)
	}
	defer func() { _ = tx.Rollback() }()
	if offered, err := activityRetryOffered(ctx, tx, entryID); err != nil {
		return err
	} else if !offered {
		return domain.ErrActivityNotRetryable
	}
	var id int64
	var probeState, seekState string
	err = tx.QueryRowContext(ctx, `select id, probe_state, seek_thumbnail_state from videos
		where content_key = ? and content_key <> '' order by id limit 1`, contentKey).Scan(&id, &probeState, &seekState)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrActivityNotRetryable
	}
	if err != nil {
		return fmt.Errorf("cannot read the video to retry (%s): %w", contentKey, err)
	}
	kinds, err := retryImportStages(ctx, tx, id, domain.ProbeState(probeState), domain.SeekThumbnailState(seekState))
	if err != nil {
		return err
	}
	var c changes
	now := time.Now()
	run := importRun{decider: domain.ActivityByOwner, retried: entryID}
	for _, kind := range kinds {
		if err := requeueJob(ctx, tx, &c, kind, id, now.Unix(), run); err != nil {
			return err
		}
	}
	if err := joinImportActivity(ctx, tx, &c, id, run, now); err != nil {
		return err
	}
	var pending bool
	if err := tx.QueryRowContext(ctx, `select exists (select 1 from jobs j where j.video_id = ? and `+
		pendingJobCondition("j")+`)`, id).Scan(&pending); err != nil {
		return fmt.Errorf("cannot read the pending jobs (video=%d): %w", id, err)
	}
	if !pending {
		return domain.ErrActivityNotRetryable
	}
	c.jobsQueued(kinds...)
	c.remainingChanged()
	if err := s.db.commit(ctx, tx, &c); err != nil {
		return fmt.Errorf("cannot commit retrying the import (video=%d): %w", id, err)
	}
	return nil
}

func retryImportStages(
	ctx context.Context, tx *sql.Tx, id int64, probe domain.ProbeState, seek domain.SeekThumbnailState,
) ([]domain.JobKind, error) {
	now := time.Now().Unix()
	var kinds []domain.JobKind
	if probe == domain.ProbeStateFailed {
		if _, err := tx.ExecContext(ctx, `update videos set probe_state = 'pending', probe_error = null,
			probe_error_code = null, indexed_at = ? where id = ?`, now, id); err != nil {
			return nil, fmt.Errorf("cannot reset the probe state (id=%d): %w", id, err)
		}
		kinds = append(kinds, domain.JobProbe)
	}
	following, err := resetFailedStages(ctx, tx, id, now, probe == domain.ProbeStateDone)
	if err != nil {
		return nil, err
	}
	kinds = append(kinds, following...)
	if seek != domain.SeekThumbnailDone {
		return kinds, nil
	}
	var fingerprintFailed bool
	if err := tx.QueryRowContext(ctx, `select exists (select 1 from jobs where kind = ? and video_id = ?
		and state = 'failed')`, string(domain.JobFingerprint), id).Scan(&fingerprintFailed); err != nil {
		return nil, fmt.Errorf("cannot read the fingerprint job (id=%d): %w", id, err)
	}
	if fingerprintFailed {
		kinds = append(kinds, domain.JobFingerprint)
	}
	return kinds, nil
}
