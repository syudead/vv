package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// Content-keyed overrides survive rescans and location changes; display-name edits rebuild all location search and sort keys atomically.

// Column order matches scanVideo; unidentified content has no override and scanVideo resolves the effective title.
const overrideColumns = `(select ov.display_name from video_overrides ov
		where ov.content_key = videos.content_key and videos.content_key <> '') as display_name,
	(select ov.thumbnail_position_ms from video_overrides ov
		where ov.content_key = videos.content_key and videos.content_key <> '') as thumbnail_position_ms,
	(select ov.thumbnail_revision from video_overrides ov
		where ov.content_key = videos.content_key and videos.content_key <> '') as thumbnail_revision,
	(select ti.revision from thumbnail_images ti
		where ti.content_key = videos.content_key and videos.content_key <> '') as thumbnail_image_revision,
	(select ti.origin from thumbnail_images ti
		where ti.content_key = videos.content_key and videos.content_key <> '') as thumbnail_image_origin`

// SetDisplayName normalizes or clears the name and requires registered, identified content; notify overrides and affected smart folders after commit.
func (s *OverrideStore) SetDisplayName(ctx context.Context, videoID int64, name string) (domain.Video, error) {
	normalized, _, err := domain.NormalizeDisplayName(name)
	if err != nil {
		return domain.Video{}, err
	}

	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.Video{}, fmt.Errorf("cannot update the display name: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	keys, err := registeredContentKeysForVideoIDs(ctx, tx, []int64{videoID})
	if err != nil {
		return domain.Video{}, err
	}
	if len(keys) == 0 {
		return domain.Video{}, domain.ErrNotFound
	}
	now := time.Now()
	before, err := displayNamesOf(ctx, tx, keys)
	if err != nil {
		return domain.Video{}, err
	}
	if err := writeDisplayName(ctx, tx, keys[0], normalized, now); err != nil {
		return domain.Video{}, err
	}
	if before[keys[0]] != normalized {
		if err := touchEditedAt(ctx, tx, keys, now); err != nil {
			return domain.Video{}, err
		}
	}
	if err := refreshSearchKeysForContentKeys(ctx, tx, keys); err != nil {
		return domain.Video{}, err
	}
	video, err := getVideo(ctx, tx, domain.AudienceOwner, videoID)
	if err != nil {
		return domain.Video{}, err
	}
	if err := s.db.commitMatchChangeIf(ctx, tx, before[keys[0]] != normalized,
		domain.VideoOverrideChanged{VideoID: videoID}); err != nil {
		return domain.Video{}, fmt.Errorf("cannot update the display name: %w", err)
	}
	return video, nil
}

// SetDisplayNames is atomic and preserves input order; repeated references use the last name and indexed errors identify invalid entries.
func (s *OverrideStore) SetDisplayNames(ctx context.Context, changes []domain.DisplayNameChange) ([]domain.Video, error) {
	names := make([]string, 0, len(changes))
	refs := make([]domain.VideoRef, 0, len(changes))
	for i, change := range changes {
		name, _, err := domain.NormalizeDisplayName(change.DisplayName)
		if err != nil {
			return nil, &domain.DisplayNameAtError{Index: i, Err: err}
		}
		names = append(names, name)
		refs = append(refs, change.Video)
	}

	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("cannot update the display names: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	targets, err := resolveTaggableVideos(ctx, tx, refs)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	keys := uniqueContentKeys(targets)
	before, err := displayNamesOf(ctx, tx, keys)
	if err != nil {
		return nil, err
	}
	final := make(map[string]string, len(keys))
	for i, target := range targets {
		if err := writeDisplayName(ctx, tx, target.contentKey, names[i], now); err != nil {
			return nil, err
		}
		final[target.contentKey] = names[i]
	}
	// Compare initial and final names so A-to-B-to-A within one batch does not advance edit time.
	edited := make([]string, 0, len(keys))
	for _, key := range keys {
		if before[key] != final[key] {
			edited = append(edited, key)
		}
	}
	if err := touchEditedAt(ctx, tx, edited, now); err != nil {
		return nil, err
	}
	if err := refreshSearchKeysForContentKeys(ctx, tx, keys); err != nil {
		return nil, err
	}

	ids := make([]int64, 0, len(targets))
	seen := make(map[int64]bool, len(targets))
	for _, target := range targets {
		if !seen[target.id] {
			seen[target.id] = true
			ids = append(ids, target.id)
		}
	}
	out, err := readTargetVideos(ctx, tx, targets)
	if err != nil {
		return nil, err
	}
	events := make([]domain.Event, 0, len(ids))
	for _, id := range ids {
		events = append(events, domain.VideoOverrideChanged{VideoID: id})
	}
	if err := s.db.commitMatchChangeIf(ctx, tx, len(edited) > 0, events...); err != nil {
		return nil, fmt.Errorf("cannot update the display names: %w", err)
	}
	return out, nil
}

// Represent an unset display name as empty, matching the write representation.
func displayNamesOf(ctx context.Context, tx *sql.Tx, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		var name sql.NullString
		err := tx.QueryRowContext(ctx,
			`select display_name from video_overrides where content_key = ?`, key,
		).Scan(&name)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("cannot read the display name: %w", err)
		}
		out[key] = name.String
	}
	return out, nil
}

func writeDisplayName(ctx context.Context, tx *sql.Tx, key, name string, now time.Time) error {
	if _, err := tx.ExecContext(ctx,
		`insert into video_overrides (content_key, display_name, updated_at) values (?, ?, ?)
		 on conflict (content_key) do update set display_name = excluded.display_name, updated_at = excluded.updated_at`,
		key, nullableString(name), now.Unix(),
	); err != nil {
		return fmt.Errorf("cannot save the display name: %w", err)
	}
	if err := deleteEmptyOverride(ctx, tx, key); err != nil {
		return err
	}
	return nil
}

func deleteEmptyOverride(ctx context.Context, tx *sql.Tx, key string) error {
	if _, err := tx.ExecContext(ctx,
		`delete from video_overrides where content_key = ? and display_name is null and thumbnail_position_ms is null`,
		key,
	); err != nil {
		return fmt.Errorf("cannot save the video override: %w", err)
	}
	return nil
}

// SetThumbnailPosition runs under the generation lock after publishing the image, atomically updating revision, thumbnail state and related issues.
func (s *OverrideStore) SetThumbnailPosition(
	ctx context.Context, videoID int64, positionMs *int64,
) (domain.Video, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.Video{}, fmt.Errorf("cannot update the thumbnail position: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	keys, err := registeredContentKeysForVideoIDs(ctx, tx, []int64{videoID})
	if err != nil {
		return domain.Video{}, err
	}
	if len(keys) == 0 {
		return domain.Video{}, domain.ErrNotFound
	}
	now := time.Now()
	positionChanged, err := thumbnailPositionDiffers(ctx, tx, keys[0], positionMs)
	if err != nil {
		return domain.Video{}, err
	}
	if err := s.db.writeThumbnailPosition(ctx, tx, keys[0], positionMs, now); err != nil {
		return domain.Video{}, err
	}
	imageRemoved, err := deleteThumbnailImage(ctx, tx, keys[0])
	if err != nil {
		return domain.Video{}, err
	}
	// Regeneration at the same position does not advance edit time unless the position or applied image changes.
	if positionChanged || imageRemoved {
		if err := touchEditedAt(ctx, tx, keys, now); err != nil {
			return domain.Video{}, err
		}
	}
	issuesCleared, err := markThumbnailDoneForContent(ctx, tx, keys[0], now)
	if err != nil {
		return domain.Video{}, err
	}
	video, err := getVideo(ctx, tx, domain.AudienceOwner, videoID)
	if err != nil {
		return domain.Video{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Video{}, fmt.Errorf("cannot update the thumbnail position: %w", err)
	}
	events := []domain.Event{domain.VideoOverrideChanged{VideoID: videoID}}
	if issuesCleared {
		// Clearing issues changes import status without a processing event, so emit ScanChanged explicitly.
		events = append(events, domain.ScanChanged{})
	}
	s.db.publishEvents(events...)
	return video, nil
}

// Expose the same operation through the ingest port used by thumbnail generation.
func (s *IngestStore) SetThumbnailPosition(
	ctx context.Context, videoID int64, positionMs *int64,
) (domain.Video, error) {
	return (&OverrideStore{db: s.db}).SetThumbnailPosition(ctx, videoID, positionMs)
}

func thumbnailPositionDiffers(ctx context.Context, tx *sql.Tx, key string, positionMs *int64) (bool, error) {
	var current sql.NullInt64
	err := tx.QueryRowContext(ctx,
		`select thumbnail_position_ms from video_overrides where content_key = ?`, key,
	).Scan(&current)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("cannot read the thumbnail position: %w", err)
	}
	if positionMs == nil {
		return current.Valid, nil
	}
	return !current.Valid || current.Int64 != *positionMs, nil
}

// Allocate revisions above both the previous row and connection-wide last value so clear-and-reset in one millisecond cannot reuse a revision.
func (db *DB) writeThumbnailPosition(ctx context.Context, tx *sql.Tx, key string, positionMs *int64, now time.Time) error {
	if positionMs == nil {
		if _, err := tx.ExecContext(ctx,
			`update video_overrides set thumbnail_position_ms = null, thumbnail_revision = null, updated_at = ?
			 where content_key = ?`,
			now.Unix(), key,
		); err != nil {
			return fmt.Errorf("cannot clear the thumbnail position: %w", err)
		}
		return deleteEmptyOverride(ctx, tx, key)
	}

	var previous sql.NullInt64
	err := tx.QueryRowContext(ctx,
		`select thumbnail_revision from video_overrides where content_key = ?`, key,
	).Scan(&previous)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("cannot read the thumbnail revision: %w", err)
	}
	revision := db.nextThumbnailRevision(now, previous.Int64)
	if _, err := tx.ExecContext(ctx,
		`insert into video_overrides (content_key, thumbnail_position_ms, thumbnail_revision, updated_at)
		 values (?, ?, ?, ?)
		 on conflict (content_key) do update set
		     thumbnail_position_ms = excluded.thumbnail_position_ms,
		     thumbnail_revision = excluded.thumbnail_revision,
		     updated_at = excluded.updated_at`,
		key, *positionMs, revision, now.Unix(),
	); err != nil {
		return fmt.Errorf("cannot save the thumbnail position: %w", err)
	}
	return nil
}

// Use the maximum of current milliseconds and both previous revisions plus one; rolled-back allocations merely leave gaps.
func (db *DB) nextThumbnailRevision(now time.Time, previous int64) int64 {
	db.revisionMu.Lock()
	defer db.revisionMu.Unlock()
	revision := max(now.UnixMilli(), previous+1, db.lastThumbnailRevision+1)
	db.lastThumbnailRevision = revision
	return revision
}

// Mark all videos sharing the content done and clear thumbnail substitution/failure issues.
func markThumbnailDoneForContent(ctx context.Context, tx *sql.Tx, key string, now time.Time) (bool, error) {
	ids, err := videoIDsForContentKey(ctx, tx, key)
	if err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx,
		`update videos set thumbnail_state = ?, indexed_at = ? where content_key = ?`,
		string(domain.ThumbnailStateDone), now.Unix(), key,
	); err != nil {
		return false, fmt.Errorf("cannot record the thumbnail state: %w", err)
	}
	cleared := false
	for _, id := range ids {
		removed, err := removeScanIssues(ctx, tx, id, domain.IssueThumbnailFirstFrame, domain.IssueThumbnailFailed)
		if err != nil {
			return false, err
		}
		cleared = cleared || removed
	}
	return cleared, nil
}

func videoIDsForContentKey(ctx context.Context, tx *sql.Tx, key string) ([]int64, error) {
	rows, err := tx.QueryContext(ctx, `select id from videos where content_key = ?`, key)
	if err != nil {
		return nil, fmt.Errorf("cannot read the videos of the content: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("cannot read the videos of the content: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the videos of the content: %w", err)
	}
	return ids, nil
}
