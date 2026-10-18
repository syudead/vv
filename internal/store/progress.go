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

// SaveProgress persists by content key without a video foreign key and records history once per playback.
// Notify watch-state changes, but invalidate the continue-watching cache on every save.
func (p *PlaybackStore) SaveProgress(
	ctx context.Context, contentKey string, progress domain.Progress, play *domain.Play,
) (domain.Progress, error) {
	updatedAt := time.Now()

	tx, err := p.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.Progress{}, fmt.Errorf("cannot save the playback position (%s): %w", contentKey, err)
	}
	defer func() { _ = tx.Rollback() }()

	before, err := storedWatchState(ctx, tx, contentKey)
	if err != nil {
		return domain.Progress{}, fmt.Errorf("cannot save the playback position (%s): %w", contentKey, err)
	}

	_, err = tx.ExecContext(ctx, `
		insert into playback_progress (content_key, position_ms, duration_ms, completed, updated_at)
		values (?, ?, ?, ?, ?)
		on conflict (content_key) do update set
			position_ms = excluded.position_ms,
			duration_ms = excluded.duration_ms,
			completed   = excluded.completed,
			updated_at  = excluded.updated_at`,
		contentKey, progress.PositionMs, nullableInt64(progress.DurationMs),
		boolToInt(progress.Completed), updatedAt.Unix(),
	)
	if err != nil {
		return domain.Progress{}, fmt.Errorf("cannot save the playback position (%s): %w", contentKey, err)
	}

	if play != nil && play.ContentKey != "" {
		if _, err := tx.ExecContext(ctx, `
			insert or ignore into watch_history (content_key, playback_id, title, title_key, played_at)
			values (?, ?, ?, ?, ?)`,
			play.ContentKey, play.PlaybackID, play.Title, searchKeyPart(play.Title), updatedAt.UnixMilli(),
		); err != nil {
			return domain.Progress{}, fmt.Errorf("cannot record the watch history (%s): %w", play.ContentKey, err)
		}
	}

	if err := p.db.commitMatchChangeIf(ctx, tx, before != domain.ClassifyWatch(&progress)); err != nil {
		return domain.Progress{}, fmt.Errorf("cannot save the playback position (%s): %w", contentKey, err)
	}
	progress.UpdatedAt = updatedAt.Truncate(time.Second)
	return progress, nil
}

func storedWatchState(ctx context.Context, tx *sql.Tx, contentKey string) (domain.WatchState, error) {
	var stored domain.Progress
	var completed int
	err := tx.QueryRowContext(ctx, `select position_ms, completed from playback_progress where content_key = ?`, contentKey).
		Scan(&stored.PositionMs, &completed)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ClassifyWatch(nil), nil
	}
	if err != nil {
		return "", err
	}
	stored.Completed = completed != 0
	return domain.ClassifyWatch(&stored), nil
}

func (p *PlaybackStore) Progress(ctx context.Context, contentKey string) (domain.Progress, error) {
	var (
		progress   domain.Progress
		durationMs sql.NullInt64
		completed  int
		updatedAt  int64
	)

	err := p.sql.QueryRowContext(ctx, `
		select position_ms, duration_ms, completed, updated_at
		  from playback_progress where content_key = ?`, contentKey,
	).Scan(&progress.PositionMs, &durationMs, &completed, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Progress{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Progress{}, fmt.Errorf("cannot read the playback position (%s): %w", contentKey, err)
	}

	progress.DurationMs = durationMs.Int64
	progress.Completed = completed != 0
	progress.UpdatedAt = time.Unix(updatedAt, 0)
	return progress, nil
}

// Batch keys as JSON to avoid N+1 reads and SQLite's bind-variable limit for unbounded playlists.
func (p *PlaybackStore) ProgressByContentKeys(
	ctx context.Context, contentKeys []string,
) (map[string]domain.Progress, error) {
	if len(contentKeys) == 0 {
		return map[string]domain.Progress{}, nil
	}
	encoded, err := json.Marshal(contentKeys)
	if err != nil {
		return nil, fmt.Errorf("cannot build content_key values: %w", err)
	}

	rows, err := p.sql.QueryContext(ctx, `
		select content_key, position_ms, duration_ms, completed, updated_at
		  from playback_progress where content_key in (select value from json_each(?))`, string(encoded))
	if err != nil {
		return nil, fmt.Errorf("cannot read playback positions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make(map[string]domain.Progress, len(contentKeys))
	for rows.Next() {
		var (
			key        string
			progress   domain.Progress
			durationMs sql.NullInt64
			completed  int
			updatedAt  int64
		)
		if err := rows.Scan(&key, &progress.PositionMs, &durationMs, &completed, &updatedAt); err != nil {
			return nil, fmt.Errorf("cannot read playback positions: %w", err)
		}
		progress.DurationMs = durationMs.Int64
		progress.Completed = completed != 0
		progress.UpdatedAt = time.Unix(updatedAt, 0)
		out[key] = progress
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read playback positions: %w", err)
	}
	return out, nil
}
