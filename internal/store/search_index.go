package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// The search index is a rebuildable index keyed by content, removed when its content loses its last reference.

// ApplySearchIndexForJob writes only if the claimed content, location and generation still match, so a job that read
// a replaced file writes nothing.
func (s *IngestStore) ApplySearchIndexForJob(ctx context.Context, job domain.Job, index domain.SearchIndex) (bool, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("cannot start saving the search index (id=%d): %w", job.VideoID, err)
	}
	defer func() { _ = tx.Rollback() }()
	current, err := jobIdentityCurrent(ctx, tx, job)
	if err != nil {
		return false, fmt.Errorf("cannot check the search index job location (id=%d): %w", job.VideoID, err)
	}
	if !current {
		return false, nil
	}
	if _, err := tx.ExecContext(ctx, `insert into video_search_index (content_key, version, entries, updated_at)
		values (?, ?, ?, ?)
		on conflict (content_key) do update
		   set version = excluded.version, entries = excluded.entries, updated_at = excluded.updated_at`,
		job.ContentKey, index.Version, index.Encode(), time.Now().Unix(),
	); err != nil {
		return false, fmt.Errorf("cannot save the search index (id=%d): %w", job.VideoID, err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("cannot commit the search index (id=%d): %w", job.VideoID, err)
	}
	return true, nil
}

// QueueFrameIndexJobs queues, in class priority, a frame_index job for each content with a file and without a search
// index of the current version, so contents imported before the index existed, or indexed by another version, are read.
func (s *IngestStore) QueueFrameIndexJobs(ctx context.Context, priority domain.JobPriority) (int, error) {
	return s.queueBackfillJobs(ctx, domain.JobFrameIndex, priority, `v.content_key <> ''
		and not exists (select 1 from video_search_index i where i.content_key = v.content_key and i.version = ?)`,
		domain.SearchIndexVersion)
}

// queueFrameIndexJob queues a backfill-class frame_index job for the video unless its content has a current-version
// index or another of its videos already has one unfinished, so one content is read once.
func queueFrameIndexJob(ctx context.Context, tx *sql.Tx, c *changes, videoID int64, contentKey string, now int64) error {
	res, err := tx.ExecContext(ctx, `insert into jobs (kind, video_id, state, attempts, priority, run_after,
			created_at, updated_at)
		select ?1, ?2, 'queued', 0, ?3, ?4, ?4, ?4
		where not exists (select 1 from video_search_index where content_key = ?5 and version = ?6)
		and not exists (select 1 from jobs j join videos v on v.id = j.video_id
			where j.kind = ?1 and j.state in ('queued', 'running') and v.content_key = ?5)
		`+unfinishedJobConflict+`do nothing`,
		string(domain.JobFrameIndex), videoID, int(domain.JobPriorityBackfill), now, contentKey, domain.SearchIndexVersion)
	if err != nil {
		return fmt.Errorf("cannot queue the search index job (video=%d): %w", videoID, err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return fmt.Errorf("cannot queue the search index job (video=%d): %w", videoID, err)
	} else if n > 0 {
		c.backfillQueued(domain.JobFrameIndex)
	}
	return nil
}
