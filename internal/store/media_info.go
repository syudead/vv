package store

// Ingest probes and the background re-read write media information; LibraryStore owns reads.

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
	return readMediaInfo(ctx, s.db.sql, videoID)
}

func readMediaInfo(ctx context.Context, q rowQueryer, videoID int64) (*domain.MediaInfo, error) {
	var version int
	var encoded string
	err := q.QueryRowContext(ctx,
		`select version, info from video_media_info where video_id = ?`, videoID,
	).Scan(&version, &encoded)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cannot read the media information (id=%d): %w", videoID, err)
	}
	return decodeMediaInfo(version, encoded), nil
}

// MediaInfos reads MediaInfo for several videos in one query; videos without current information are absent.
func (s *LibraryStore) MediaInfos(ctx context.Context, videoIDs []int64) (map[int64]*domain.MediaInfo, error) {
	ids, _ := json.Marshal(videoIDs) // Marshaling integers cannot fail.
	rows, err := s.db.sql.QueryContext(ctx, `select video_id, version, info from video_media_info
		where video_id in (select value from json_each(?))`, string(ids))
	if err != nil {
		return nil, fmt.Errorf("cannot read the media information: %w", err)
	}
	defer func() { _ = rows.Close() }()
	infos := map[int64]*domain.MediaInfo{}
	for rows.Next() {
		var id int64
		var version int
		var encoded string
		if err := rows.Scan(&id, &version, &encoded); err != nil {
			return nil, fmt.Errorf("cannot read the media information: %w", err)
		}
		if info := decodeMediaInfo(version, encoded); info != nil {
			infos[id] = info
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the media information: %w", err)
	}
	return infos, nil
}

// An older version reads as what it holds, since a bump only adds fields; the background re-read replaces it.
func decodeMediaInfo(version int, encoded string) *domain.MediaInfo {
	if version < 1 || version > domain.MediaInfoVersion {
		return nil
	}
	var info domain.MediaInfo
	if json.Unmarshal([]byte(encoded), &info) != nil {
		return nil
	}
	return &info
}

// QueueMediaInfoJobs queues, in class priority, a media_info job for each video without media information of this
// version, skipping one whose re-read failed for the same file at this version.
func (s *IngestStore) QueueMediaInfoJobs(ctx context.Context, priority domain.JobPriority) (int, error) {
	return s.queueBackfillJobs(ctx, domain.JobMediaInfo, priority, `not exists (select 1 from video_media_info m
			where m.video_id = v.id and m.version = ?)
		and not exists (select 1 from video_media_info_failures f where f.video_id = v.id and f.version = ?
			and f.size_bytes = l.size_bytes and f.mtime_ns = coalesce(l.mtime_ns, l.mtime * 1000000000))`,
		domain.MediaInfoVersion, domain.MediaInfoVersion)
}

// SaveMediaInfoProbe writes a re-read of the indexed file; a failed read keeps the stored information and marks the
// file so it is not read again until it changes.
func (s *IngestStore) SaveMediaInfoProbe(ctx context.Context, job domain.Job, p domain.MediaInfoProbe) error {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	current, err := jobIdentityCurrent(ctx, tx, job)
	if err != nil || !current {
		return err
	}
	if p.Source == (domain.FileStamp{}) {
		if p.Source, err = indexedStamp(ctx, tx, job.LocationID); err != nil {
			return err
		}
		p.Info = nil
	}
	if matches, err := stampMatchesLocation(ctx, tx, job.LocationID, p.Source); err != nil || !matches {
		return err
	}
	if p.Info == nil {
		_, err = tx.ExecContext(ctx, `insert into video_media_info_failures (video_id, version, size_bytes, mtime_ns)
			values (?, ?, ?, ?) on conflict (video_id) do update set version = excluded.version,
			size_bytes = excluded.size_bytes, mtime_ns = excluded.mtime_ns`,
			job.VideoID, domain.MediaInfoVersion, p.Source.SizeBytes, p.Source.ModTimeNs)
	} else {
		err = upsertMediaInfo(ctx, tx, job.VideoID, *p.Info, time.Now())
	}
	if err != nil {
		return fmt.Errorf("cannot save the re-read media information (id=%d): %w", job.VideoID, err)
	}
	return tx.Commit()
}
