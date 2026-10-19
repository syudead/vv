package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// UpsertVideo matches content before path so moves and copies retain identity; changed content resets probe results to pending.
func (s *ScanIndexStore) UpsertVideo(ctx context.Context, file domain.VideoFile) (domain.UpsertResult, error) {
	now := time.Now().Unix()
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.UpsertResult{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var locationID, oldVideoID, oldVersion, oldSize, oldMtime int64
	var oldKey string
	err = tx.QueryRowContext(ctx, `
		select l.id, l.video_id, l.version, l.size_bytes, coalesce(l.mtime_ns, l.mtime * 1000000000), v.content_key
		from video_locations l join videos v on v.id = l.video_id where l.path = ?`, file.Path).
		Scan(&locationID, &oldVideoID, &oldVersion, &oldSize, &oldMtime, &oldKey)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return domain.UpsertResult{}, err
	}
	locationExists := err == nil
	if locationExists && oldKey == file.ContentKey && oldSize == file.SizeBytes && oldMtime == file.MTime.UnixNano() {
		return domain.UpsertResult{ID: oldVideoID, Outcome: domain.OutcomeUnchanged}, tx.Commit()
	}

	var videoID int64
	var probeState, thumbnailState, seekThumbnailState, previewState string
	err = tx.QueryRowContext(ctx, `select id, probe_state, thumbnail_state, seek_thumbnail_state, preview_state
		from videos where content_key = ?`, file.ContentKey).
		Scan(&videoID, &probeState, &thumbnailState, &seekThumbnailState, &previewState)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return domain.UpsertResult{}, err
	}
	newVideo := errors.Is(err, sql.ErrNoRows)
	addedAt := file.AddedAt
	if addedAt.IsZero() {
		addedAt = time.Now()
	}
	if newVideo {
		probeState = string(domain.ProbeStatePending)
		thumbnailState = string(domain.ThumbnailStatePending)
		seekThumbnailState = string(domain.SeekThumbnailPending)
		previewState = string(domain.PreviewStatePending)
		res, err := tx.ExecContext(ctx, `
		insert into videos
			(added_at, indexed_at, content_key, container, playable, probe_state, thumbnail_state)
		values (?, ?, ?, ?, 0, 'pending', 'pending')`,
			addedAt.Unix(), now, file.ContentKey, nullableString(file.Container))
		if err != nil {
			return domain.UpsertResult{}, fmt.Errorf("cannot import the video (%s): %w", file.Path, err)
		}
		videoID, err = res.LastInsertId()
		if err != nil {
			return domain.UpsertResult{}, err
		}
	}

	if locationExists {
		_, err = tx.ExecContext(ctx, `update video_locations set video_id = ?, version = version + 1, title = ?, size_bytes = ?, mtime = ?, mtime_ns = ?, file_created_at = ?, updated_at = ? where id = ?`,
			videoID, file.Title, file.SizeBytes, file.MTime.Unix(), file.MTime.UnixNano(), nullableUnix(file.FileCreatedAt), now, locationID)
	} else {
		_, err = tx.ExecContext(ctx, `insert into video_locations(video_id, path, version, title, size_bytes, mtime, mtime_ns, file_created_at, created_at, updated_at) values (?, ?, 1, ?, ?, ?, ?, ?, ?, ?)`,
			videoID, file.Path, file.Title, file.SizeBytes, file.MTime.Unix(), file.MTime.UnixNano(), nullableUnix(file.FileCreatedAt), now, now)
	}
	if err != nil {
		return domain.UpsertResult{}, fmt.Errorf("cannot save the video location (%s): %w", file.Path, err)
	}
	if err := refreshSearchKeysByPath(ctx, tx, file.Path); err != nil {
		return domain.UpsertResult{}, err
	}
	if !locationExists {
		if _, err := tx.ExecContext(ctx, `update videos set location_generation = location_generation + 1 where id = ?`, videoID); err != nil {
			return domain.UpsertResult{}, err
		}
	} else if oldVideoID != videoID {
		if _, err := tx.ExecContext(ctx, `update videos set location_generation = location_generation + 1 where id in (?, ?)`, oldVideoID, videoID); err != nil {
			return domain.UpsertResult{}, err
		}
	}
	if err := syncRepresentativeContainer(ctx, tx, videoID); err != nil {
		return domain.UpsertResult{}, err
	}
	// Cancel pending succession when the old content reappears elsewhere.
	if newVideo {
		if err := cancelSuccessionsFrom(ctx, tx, file.ContentKey); err != nil {
			return domain.UpsertResult{}, err
		}
	}
	var released []domain.DeletedVideo
	if locationExists && oldVideoID != videoID {
		if err := syncRepresentativeContainer(ctx, tx, oldVideoID); err != nil {
			return domain.UpsertResult{}, err
		}
		// Capture the old duration before deleting its video row.
		var oldDurationMs sql.NullInt64
		if err := tx.QueryRowContext(ctx, `select duration_ms from videos where id = ?`, oldVideoID).Scan(&oldDurationMs); err != nil {
			return domain.UpsertResult{}, err
		}
		released, err = collectDeletedVideos(tx.QueryContext(ctx, `delete from videos where id = ? and not exists (select 1 from video_locations where video_id = ?)
			returning id, content_key`, oldVideoID, oldVideoID))
		if err != nil {
			return domain.UpsertResult{}, err
		}
		if err := releaseContentIndex(ctx, tx, released); err != nil {
			return domain.UpsertResult{}, err
		}
		// Record succession only when replacing same-path content removes the previous video.
		if newVideo && oldKey != file.ContentKey && len(released) == 1 {
			if err := recordSuccession(ctx, tx, file.ContentKey, oldKey, oldDurationMs, now); err != nil {
				return domain.UpsertResult{}, err
			}
		}
	}
	var c changes
	c.videosDeleted(released)
	if !locationExists || oldVideoID != videoID {
		changed, err := videosHaveIssues(ctx, tx, videoID, oldVideoID)
		if err != nil {
			return domain.UpsertResult{}, err
		}
		if changed {
			c.issuesChanged()
		}
	}
	c.remainingChanged()
	outcome := domain.OutcomeMoved
	if locationExists {
		outcome = domain.OutcomeUpdated
	} else if newVideo {
		outcome = domain.OutcomeAdded
	}
	result := domain.UpsertResult{
		ID: videoID, Outcome: outcome,
		NeedsProbe:         probeState != string(domain.ProbeStateDone),
		NeedsThumbnail:     thumbnailState != string(domain.ThumbnailStateDone),
		NeedsSeekThumbnail: seekThumbnailState != string(domain.SeekThumbnailDone),
		NeedsPreview:       probeState == string(domain.ProbeStateDone) && previewState != string(domain.PreviewStateDone),
	}
	if err := recordLocationImport(ctx, tx, &c, result, file.Decider); err != nil {
		return domain.UpsertResult{}, err
	}
	if err := s.db.commit(ctx, tx, &c); err != nil {
		return domain.UpsertResult{}, err
	}
	return result, nil
}

// Record location-only imports as completed only when no queued, running or missing stage work needs an import activity.
func recordLocationImport(
	ctx context.Context, tx activityWriter, c *changes, result domain.UpsertResult, decider domain.ActivityDecider,
) error {
	if result.Outcome != domain.OutcomeMoved || result.NeedsProbe || result.NeedsThumbnail ||
		result.NeedsSeekThumbnail || result.NeedsPreview {
		return nil
	}
	var work bool
	if err := tx.QueryRowContext(ctx, `select exists (select 1 from jobs j where j.video_id = ? and `+
		pendingJobCondition("j")+`) or exists (select 1 from videos v where v.id = ? and `+
		fingerprintMissingCondition("v")+`)`, result.ID, result.ID).Scan(&work); err != nil {
		return fmt.Errorf("cannot read the video's remaining work (video=%d): %w", result.ID, err)
	}
	if work {
		return nil
	}
	if decider == "" {
		decider = domain.ActivityByRule
	}
	subject, err := videoActivitySubject(ctx, tx, result.ID)
	if err != nil {
		return err
	}
	now := time.Now()
	entry := domain.ActivityEntry{
		Kind: domain.ActivityImport, Subject: subject, Status: domain.ActivityDone,
		Outcome: domain.ActivityCompleted, Decider: decider,
	}
	if entry.ID, err = recordActivity(ctx, tx, c, entry, now); err != nil {
		return err
	}
	return openImportPipeline(ctx, tx, c, entry, 0, now)
}

// DeleteVideos ignores an empty selection rather than deleting all rows; job rows cascade.
func (s *ScanIndexStore) DeleteVideos(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}

	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot delete videos: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	released, err := collectDeletedVideos(tx.QueryContext(ctx,
		`delete from videos where id in (`+placeholders+`) returning id, content_key`, args...))
	if err != nil {
		return fmt.Errorf("cannot delete videos: %w", err)
	}
	if err := releaseContentIndex(ctx, tx, released); err != nil {
		return fmt.Errorf("cannot delete videos: %w", err)
	}
	var c changes
	c.videosDeleted(released)
	if err := s.db.commit(ctx, tx, &c); err != nil {
		return fmt.Errorf("cannot delete videos: %w", err)
	}
	return nil
}

// DeleteVideoLocations removes filesystem facts and then only orphaned logical videos.
func (s *ScanIndexStore) DeleteVideoLocations(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	affected := map[int64]struct{}{}
	for _, id := range ids {
		var videoID int64
		if err := tx.QueryRowContext(ctx, `select video_id from video_locations where id = ?`, id).Scan(&videoID); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		} else if err == nil {
			affected[videoID] = struct{}{}
		}
		if _, err := tx.ExecContext(ctx, `delete from video_locations where id = ?`, id); err != nil {
			return err
		}
	}
	for videoID := range affected {
		if _, err := tx.ExecContext(ctx, `update videos set location_generation = location_generation + 1 where id = ?`, videoID); err != nil {
			return err
		}
		if err := syncRepresentativeContainer(ctx, tx, videoID); err != nil {
			return err
		}
	}
	affectedIDs := make([]int64, 0, len(affected))
	for videoID := range affected {
		affectedIDs = append(affectedIDs, videoID)
	}
	issuesMoved, err := videosHaveIssues(ctx, tx, affectedIDs...)
	if err != nil {
		return err
	}
	released, err := deleteOrphanVideos(ctx, tx)
	if err != nil {
		return err
	}
	var c changes
	c.videosDeleted(released)
	if issuesMoved {
		c.issuesChanged()
	}
	c.remainingChanged()
	return s.db.commit(ctx, tx, &c)
}

// ForcedVideos lists, in id order, the videos the force rescan has already forced, so a resumed walk skips them.
func (s *ScanIndexStore) ForcedVideos(ctx context.Context, scanID int64) ([]int64, error) {
	return forcedVideos(ctx, s.db.sql, scanID)
}

// UpdateLocationCreatedAt fixes location facts without edit timestamps, versions or events; zero restores the modification-time fallback.
func (s *ScanIndexStore) UpdateLocationCreatedAt(ctx context.Context, locationID int64, createdAt time.Time) error {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `update video_locations set file_created_at = ? where id = ?`,
		nullableUnix(createdAt), locationID); err != nil {
		return fmt.Errorf("cannot update the file creation time (location=%d): %w", locationID, err)
	}
	return s.db.commitEvents(ctx, tx)
}

func (s *ScanIndexStore) IndexedVideosByPath(ctx context.Context) (map[string]domain.IndexedVideo, error) {
	rows, err := s.db.sql.QueryContext(ctx, `select v.id, l.id, l.version, l.path, v.content_key, l.size_bytes, coalesce(l.mtime_ns, l.mtime * 1000000000), l.file_created_at, v.probe_state, v.thumbnail_state, v.seek_thumbnail_state, v.preview_state,
		`+fingerprintMissingCondition("v")+`
		from video_locations l join videos v on v.id = l.video_id`)
	if err != nil {
		return nil, fmt.Errorf("cannot read the index: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := map[string]domain.IndexedVideo{}
	for rows.Next() {
		var path string
		var video domain.IndexedVideo
		var mtime int64
		var fileCreatedAt sql.NullInt64
		if err := rows.Scan(&video.ID, &video.LocationID, &video.LocationVersion, &path, &video.ContentKey, &video.SizeBytes, &mtime, &fileCreatedAt, &video.ProbeState, &video.ThumbnailState, &video.SeekThumbnailState, &video.PreviewState, &video.FingerprintMissing); err != nil {
			return nil, fmt.Errorf("cannot read the index: %w", err)
		}
		video.MTime = time.Unix(0, mtime)
		video.FileCreatedAt = unixOrZero(fileCreatedAt)
		out[path] = video
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the index: %w", err)
	}
	return out, nil
}
