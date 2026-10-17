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

// NextKeyframeProbe pages through registered locations without walking the disk.
// Rows (including unknown results) survive restarts. A changed file or sample
// version becomes eligible again. Normal import work has priority for each video.
func (s *IngestStore) NextKeyframeProbe(ctx context.Context, afterID int64) (domain.Job, error) {
	var job domain.Job
	err := s.db.sql.QueryRowContext(ctx, `select v.id, v.content_key, v.location_generation, l.id, l.version, l.path
		from videos v join video_locations l on l.video_id = v.id
		where v.id > ? and v.probe_state = 'done' and v.video_codec = 'h264'
		and `+registeredLocationCondition("l")+`
		and l.path = (select min(first.path) from video_locations first where first.video_id = v.id
			and `+registeredLocationCondition("first")+`)
		and not exists (select 1 from jobs j where j.video_id = v.id and j.kind = 'probe'
			and j.state in ('queued', 'running'))
		and not exists (select 1 from video_keyframe_spacing k where k.video_id = v.id and k.version = ?
			and k.size_bytes = l.size_bytes and k.mtime_ns / 1000000000 = l.mtime)
		order by v.id, l.path limit 1`, afterID, domain.KeyframeProbeVersion).
		Scan(&job.VideoID, &job.ContentKey, &job.LocationGeneration, &job.LocationID, &job.LocationVersion, &job.LocationPath)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Job{}, domain.ErrNoJob
	}
	return job, err
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
		if err := tx.QueryRowContext(ctx, `select size_bytes, mtime * 1000000000 from video_locations where id = ?`,
			job.LocationID).Scan(&p.Source.SizeBytes, &p.Source.ModTimeNs); err != nil {
			return err
		}
		p.Spacing = domain.KeyframesUnknown
	}
	var matches bool
	if err := tx.QueryRowContext(ctx, `select exists (select 1 from video_locations
		where id = ? and size_bytes = ? and mtime = ?)`,
		job.LocationID, p.Source.SizeBytes, p.Source.ModTimeNs/1e9).Scan(&matches); err != nil {
		return err
	}
	if !matches {
		return nil
	}
	if err := upsertKeyframeSpacing(ctx, tx, job.VideoID, p, false); err != nil {
		return err
	}
	return tx.Commit()
}
