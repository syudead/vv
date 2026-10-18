package store

// Only ingest probes write media information; LibraryStore owns reads.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// The caller must validate video identity in the same transaction.
func upsertMediaInfo(ctx context.Context, db queryExecer, videoID int64, info domain.MediaInfo, now time.Time) error {
	encoded, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("cannot encode the media information as JSON (id=%d): %w", videoID, err)
	}
	if _, err := db.ExecContext(ctx, `
		insert into video_media_info (video_id, version, info, updated_at)
		values (?, ?, ?, ?)
		on conflict (video_id) do update
		   set version = excluded.version, info = excluded.info, updated_at = excluded.updated_at`,
		videoID, domain.MediaInfoVersion, string(encoded), now.Unix(),
	); err != nil {
		return fmt.Errorf("cannot save the media information (id=%d): %w", videoID, err)
	}
	return nil
}

// MediaInfo is detail-only and returns nil for missing, outdated or malformed rows.
func (s *LibraryStore) MediaInfo(ctx context.Context, videoID int64) (*domain.MediaInfo, error) {
	var version int
	var encoded string
	err := s.db.sql.QueryRowContext(ctx,
		`select version, info from video_media_info where video_id = ?`, videoID,
	).Scan(&version, &encoded)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cannot read the media information (id=%d): %w", videoID, err)
	}
	if version != domain.MediaInfoVersion {
		return nil, nil
	}
	var info domain.MediaInfo
	if json.Unmarshal([]byte(encoded), &info) != nil {
		return nil, nil
	}
	return &info, nil
}
