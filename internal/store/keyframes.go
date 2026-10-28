package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/syudead/vv/internal/domain"
)

func (s *LibraryStore) KeyframeSpacing(ctx context.Context, videoID int64) (domain.KeyframeProbe, error) {
	var p domain.KeyframeProbe
	err := s.db.sql.QueryRowContext(ctx, `select version, size_bytes, mtime_ns, spacing
		from video_keyframe_spacing where video_id = ?`, videoID).
		Scan(&p.Version, &p.Source.SizeBytes, &p.Source.ModTimeNs, &p.Spacing)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.KeyframeProbe{}, nil
	}
	return p, err
}

func upsertKeyframeSpacing(ctx context.Context, db queryExecer, videoID int64, p domain.KeyframeProbe, replace bool) error {
	_, err := db.ExecContext(ctx, `insert into video_keyframe_spacing (video_id, version, size_bytes, mtime_ns, spacing)
		values (?, ?, ?, ?, ?) on conflict (video_id) do update set version = excluded.version,
		size_bytes = excluded.size_bytes, mtime_ns = excluded.mtime_ns, spacing = excluded.spacing
		where ? or video_keyframe_spacing.version != excluded.version
			or video_keyframe_spacing.size_bytes != excluded.size_bytes
			or video_keyframe_spacing.mtime_ns != excluded.mtime_ns`,
		videoID, p.Version, p.Source.SizeBytes, p.Source.ModTimeNs, p.Spacing, replace)
	return err
}

// QueueKeyframeJobs queues, in class priority, a keyframes job for each H.264 video whose stored spacing is missing,
// of another sample version, or for another size or modification time. Rows, unknown results included, survive
// restarts, so only a changed file or sample version is read again.
func (s *IngestStore) QueueKeyframeJobs(ctx context.Context, priority domain.JobPriority) (int, error) {
	return s.queueBackfillJobs(ctx, domain.JobKeyframes, priority, `v.video_codec = 'h264'
		and not exists (select 1 from video_keyframe_spacing k where k.video_id = v.id and k.version = ?
			and k.size_bytes = l.size_bytes and k.mtime_ns = l.mtime_ns)`, domain.KeyframeProbeVersion)
}

// SaveKeyframeProbe rejects a source replaced while the background sample ran.
// A concurrent import's result wins; request-time probe saves use another table.
func (s *IngestStore) SaveKeyframeProbe(ctx context.Context, job domain.Job, p domain.KeyframeProbe) error {
	if p.Version != domain.KeyframeProbeVersion {
		return nil
	}
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	current, err := jobIdentityCurrent(ctx, tx, job)
	if err != nil || !current {
		return err
	}
	// If the file was unavailable, record unknown against the indexed stamp so
	// one missing file cannot keep the background worker busy on every pass.
	if p.Source == (domain.FileStamp{}) {
		if p.Source, err = indexedStamp(ctx, tx, job.LocationID); err != nil {
			return err
		}
		p.Spacing = domain.KeyframesUnknown
	}
	if matches, err := stampMatchesLocation(ctx, tx, job.LocationID, p.Source); err != nil || !matches {
		return err
	}
	if err := upsertKeyframeSpacing(ctx, tx, job.VideoID, p, false); err != nil {
		return err
	}
	return tx.Commit()
}

func indexedStamp(ctx context.Context, tx *sql.Tx, locationID int64) (domain.FileStamp, error) {
	var stamp domain.FileStamp
	err := tx.QueryRowContext(ctx, `select size_bytes, coalesce(mtime_ns, mtime * 1000000000) from video_locations where id = ?`,
		locationID).Scan(&stamp.SizeBytes, &stamp.ModTimeNs)
	return stamp, err
}

// stampMatchesLocation reports whether a background read saw the indexed file. Legacy locations have only second
// precision, so the first read teaches them its exact stamp; later reads must match it at nanosecond precision.
func stampMatchesLocation(ctx context.Context, tx *sql.Tx, locationID int64, source domain.FileStamp) (bool, error) {
	if _, err := tx.ExecContext(ctx, `update video_locations set mtime_ns = ?
		where id = ? and mtime_ns is null and size_bytes = ? and mtime = ?`,
		source.ModTimeNs, locationID, source.SizeBytes, source.ModTimeNs/1e9); err != nil {
		return false, err
	}
	var matches bool
	err := tx.QueryRowContext(ctx, `select exists (select 1 from video_locations
		where id = ? and size_bytes = ? and mtime_ns = ?)`, locationID, source.SizeBytes, source.ModTimeNs).Scan(&matches)
	return matches, err
}
