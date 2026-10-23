package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// Applied thumbnail images persist by content key across rescans; ingest republishes files from these rows.

// ApplyThumbnailImage runs under the generation lock after publication, checks current registered content, and atomically records the image, revision and completed state.
func (s *OverrideStore) ApplyThumbnailImage(
	ctx context.Context, videoID int64, contentKey string, image domain.AppliedThumbnailImage,
) (domain.Video, error) {
	if contentKey == "" || len(image.Image) == 0 || len(image.Source) == 0 {
		return domain.Video{}, errors.New("cannot apply an empty thumbnail image")
	}
	if !image.Origin.Valid() {
		return domain.Video{}, fmt.Errorf("cannot apply a thumbnail image of origin %q", image.Origin)
	}
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.Video{}, fmt.Errorf("cannot apply the thumbnail image: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	key, err := thumbnailKeyOf(ctx, tx, videoID)
	if err != nil {
		return domain.Video{}, err
	}
	if key != contentKey {
		return domain.Video{}, domain.ErrContentChanged
	}
	now := time.Now()
	var previous sql.NullInt64
	err = tx.QueryRowContext(ctx,
		`select revision from thumbnail_images where content_key = ?`, contentKey,
	).Scan(&previous)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return domain.Video{}, fmt.Errorf("cannot read the thumbnail image revision: %w", err)
	}
	revision := s.db.nextThumbnailRevision(now, previous.Int64)
	if _, err := tx.ExecContext(ctx,
		`insert into thumbnail_images (content_key, image, source, origin, revision, applied_at)
		 values (?, ?, ?, ?, ?, ?)
		 on conflict (content_key) do update set
		     image = excluded.image, source = excluded.source, origin = excluded.origin,
		     revision = excluded.revision, applied_at = excluded.applied_at`,
		contentKey, image.Image, image.Source, string(image.Origin), revision, now.UnixMilli(),
	); err != nil {
		return domain.Video{}, fmt.Errorf("cannot save the thumbnail image: %w", err)
	}
	return s.finishThumbnailImageChange(ctx, tx, videoID, contentKey, now)
}

// RevertThumbnailImage runs after publishing the replacement frame under the generation lock; even an absent applied image advances edit time after regeneration.
func (s *OverrideStore) RevertThumbnailImage(ctx context.Context, videoID int64) (domain.Video, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.Video{}, fmt.Errorf("cannot revert the thumbnail image: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	keys, err := registeredContentKeysForVideoIDs(ctx, tx, []int64{videoID})
	if err != nil {
		return domain.Video{}, err
	}
	if len(keys) == 0 {
		return domain.Video{}, domain.ErrNotFound
	}
	if _, err := deleteThumbnailImage(ctx, tx, keys[0]); err != nil {
		return domain.Video{}, err
	}
	return s.finishThumbnailImageChange(ctx, tx, videoID, keys[0], time.Now())
}

// RemoveVirtualThumbnailImage removes a virtual video's uploaded image and returns its thumbnail to the placeholder
// state; contentKey is the key read under the generation lock. A missing video is ErrNotFound, a file video
// ErrNotVirtual and another key ErrContentChanged.
func (s *OverrideStore) RemoveVirtualThumbnailImage(
	ctx context.Context, videoID int64, contentKey string,
) (domain.Video, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.Video{}, fmt.Errorf("cannot remove the thumbnail image: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	key, err := virtualKeyFor(ctx, tx, videoID, contentKey)
	if err != nil {
		return domain.Video{}, err
	}
	if _, err := deleteThumbnailImage(ctx, tx, key); err != nil {
		return domain.Video{}, err
	}
	now := time.Now()
	if _, err := tx.ExecContext(ctx, `update videos set thumbnail_state = ?, indexed_at = ? where content_key = ?`,
		string(domain.ThumbnailStatePending), now.Unix(), key); err != nil {
		return domain.Video{}, fmt.Errorf("cannot record the thumbnail state: %w", err)
	}
	return s.commitThumbnailImageChange(ctx, tx, videoID, key, now, false)
}

// VirtualThumbnailKeys lists the virtual videos that have an uploaded thumbnail image.
func (s *IngestStore) VirtualThumbnailKeys(ctx context.Context) ([]string, error) {
	keys, err := queryStrings(ctx, s.db.sql, `select ti.content_key from thumbnail_images ti
		join virtual_videos vv on vv.user_key = ti.content_key order by vv.id`)
	if err != nil {
		return nil, fmt.Errorf("cannot read the virtual video thumbnails: %w", err)
	}
	return keys, nil
}

// thumbnailKeyOf returns the content key of a registered file video or a virtual video, and "" for any other video.
func thumbnailKeyOf(ctx context.Context, tx *sql.Tx, videoID int64) (string, error) {
	var key string
	err := tx.QueryRowContext(ctx, `select v.content_key from videos v where v.id = ? and v.content_key <> '' and (`+
		registeredVideoCondition("v")+` or exists (select 1 from virtual_videos vv where vv.user_key = v.content_key))`,
		videoID).Scan(&key)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("cannot read the video's content key: %w", err)
	}
	return key, nil
}

func (s *OverrideStore) finishThumbnailImageChange(
	ctx context.Context, tx *sql.Tx, videoID int64, key string, now time.Time,
) (domain.Video, error) {
	issuesCleared, err := markThumbnailDoneForContent(ctx, tx, key, now)
	if err != nil {
		return domain.Video{}, err
	}
	return s.commitThumbnailImageChange(ctx, tx, videoID, key, now, issuesCleared)
}

func (s *OverrideStore) commitThumbnailImageChange(
	ctx context.Context, tx *sql.Tx, videoID int64, key string, now time.Time, issuesCleared bool,
) (domain.Video, error) {
	if err := touchEditedAt(ctx, tx, []string{key}, now); err != nil {
		return domain.Video{}, err
	}
	video, err := getVideo(ctx, tx, domain.AudienceOwner, videoID)
	if err != nil {
		return domain.Video{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Video{}, fmt.Errorf("cannot record the thumbnail image: %w", err)
	}
	events := []domain.Event{domain.VideoOverrideChanged{VideoID: videoID}}
	if issuesCleared {
		events = append(events, domain.ScanChanged{})
	}
	s.db.publishEvents(events...)
	return video, nil
}

// ThumbnailImage returns nil when no image is applied.
func (s *OverrideStore) ThumbnailImage(ctx context.Context, contentKey string) ([]byte, error) {
	var image []byte
	err := s.db.sql.QueryRowContext(ctx,
		`select image from thumbnail_images where content_key = ? and content_key <> ''`, contentKey,
	).Scan(&image)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cannot read the thumbnail image: %w", err)
	}
	return image, nil
}

// ThumbnailImageSource returns the source JPEG, capped at 2048 pixels on its long edge, or nil if absent.
func (s *OverrideStore) ThumbnailImageSource(ctx context.Context, contentKey string) ([]byte, error) {
	var source []byte
	err := s.db.sql.QueryRowContext(ctx,
		`select source from thumbnail_images where content_key = ? and content_key <> ''`, contentKey,
	).Scan(&source)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cannot read the thumbnail image source: %w", err)
	}
	return source, nil
}

// CurrentThumbnailSelection reads image and position in one statement, falling back to the thumbnail when no source image is stored.
func (s *OverrideStore) CurrentThumbnailSelection(
	ctx context.Context, contentKey string,
) (domain.CurrentThumbnailSelection, error) {
	if contentKey == "" {
		return domain.CurrentThumbnailSelection{}, nil
	}
	var image []byte
	var position sql.NullInt64
	err := s.db.sql.QueryRowContext(ctx,
		`select (select coalesce(source, image) from thumbnail_images where content_key = ?1),
		        (select thumbnail_position_ms from video_overrides where content_key = ?1)`,
		contentKey,
	).Scan(&image, &position)
	if err != nil {
		return domain.CurrentThumbnailSelection{}, fmt.Errorf("cannot read the current thumbnail: %w", err)
	}
	selection := domain.CurrentThumbnailSelection{Image: image}
	if position.Valid {
		selection.PositionMs = &position.Int64
	}
	return selection, nil
}

func deleteThumbnailImage(ctx context.Context, tx *sql.Tx, key string) (bool, error) {
	result, err := tx.ExecContext(ctx, `delete from thumbnail_images where content_key = ?`, key)
	if err != nil {
		return false, fmt.Errorf("cannot remove the thumbnail image: %w", err)
	}
	removed, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("cannot remove the thumbnail image: %w", err)
	}
	return removed > 0, nil
}

// Expose thumbnail operations through the ingest port used by image generation.

func (s *IngestStore) ApplyThumbnailImage(
	ctx context.Context, videoID int64, contentKey string, image domain.AppliedThumbnailImage,
) (domain.Video, error) {
	return (&OverrideStore{db: s.db}).ApplyThumbnailImage(ctx, videoID, contentKey, image)
}

func (s *IngestStore) RevertThumbnailImage(ctx context.Context, videoID int64) (domain.Video, error) {
	return (&OverrideStore{db: s.db}).RevertThumbnailImage(ctx, videoID)
}

func (s *IngestStore) RemoveVirtualThumbnailImage(
	ctx context.Context, videoID int64, contentKey string,
) (domain.Video, error) {
	return (&OverrideStore{db: s.db}).RemoveVirtualThumbnailImage(ctx, videoID, contentKey)
}

func (s *IngestStore) ThumbnailImage(ctx context.Context, contentKey string) ([]byte, error) {
	return (&OverrideStore{db: s.db}).ThumbnailImage(ctx, contentKey)
}

func (s *IngestStore) ThumbnailImageSource(ctx context.Context, contentKey string) ([]byte, error) {
	return (&OverrideStore{db: s.db}).ThumbnailImageSource(ctx, contentKey)
}
