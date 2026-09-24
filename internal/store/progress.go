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

// SaveProgress は再生位置を記録し、記録後の状態を返す。
//
// 鍵は content_key（videos.id ではない）。動画の行が消えても記録が残り、
// 同じ内容を置き直せば再生位置が戻る。
// videos への外部キーを張らないのは、参照整合性より利用者データの保全を
// 優先するためである。ファイルを一時的に外しただけで再生位置が消えると、
// 利用者から見て復旧不能な損失になる。
//
// 競合は最後の書き込みが残る（upsert）。複数のタブ・端末での同時再生は
// この単純化で割り切る。
func (db *DB) SaveProgress(ctx context.Context, contentKey string, progress domain.Progress) (domain.Progress, error) {
	return db.Playback().SaveProgress(ctx, contentKey, progress)
}

func (db *DB) Progress(ctx context.Context, contentKey string) (domain.Progress, error) {
	return db.Playback().Progress(ctx, contentKey)
}

func (p *PlaybackStore) SaveProgress(
	ctx context.Context, contentKey string, progress domain.Progress,
) (domain.Progress, error) {
	updatedAt := time.Now()

	_, err := p.sql.ExecContext(ctx, `
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
		return domain.Progress{}, fmt.Errorf("再生位置を記録できません (%s): %w", contentKey, err)
	}

	progress.UpdatedAt = updatedAt.Truncate(time.Second)
	return progress, nil
}

// Progress は再生位置を1件返す。記録が無ければ ErrNotFound を返す。
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
		return domain.Progress{}, fmt.Errorf("再生位置を読み出せません (%s): %w", contentKey, err)
	}

	progress.DurationMs = durationMs.Int64
	progress.Completed = completed != 0
	progress.UpdatedAt = time.Unix(updatedAt, 0)
	return progress, nil
}

// ProgressByContentKeys は複数の再生位置をまとめて引く。記録が無い鍵は
// 結果に現れない。
//
// 一覧に載せるために要る。1件ずつ引くと、60 件の一覧で 60 回の問い合わせに
// なる。
func (db *DB) ProgressByContentKeys(
	ctx context.Context, contentKeys []string,
) (map[string]domain.Progress, error) {
	return db.Playback().ProgressByContentKeys(ctx, contentKeys)
}

func (p *PlaybackStore) ProgressByContentKeys(
	ctx context.Context, contentKeys []string,
) (map[string]domain.Progress, error) {
	if len(contentKeys) == 0 {
		return map[string]domain.Progress{}, nil
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(contentKeys)), ",")
	args := make([]any, 0, len(contentKeys))
	for _, key := range contentKeys {
		args = append(args, key)
	}

	//nolint:gosec // 組み立てるのはプレースホルダの数だけで、値は引数で渡す。
	rows, err := p.sql.QueryContext(ctx, `
		select content_key, position_ms, duration_ms, completed, updated_at
		  from playback_progress where content_key in (`+placeholders+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("再生位置を読み出せません: %w", err)
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
			return nil, fmt.Errorf("再生位置を読み出せません: %w", err)
		}
		progress.DurationMs = durationMs.Int64
		progress.Completed = completed != 0
		progress.UpdatedAt = time.Unix(updatedAt, 0)
		out[key] = progress
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("再生位置を読み出せません: %w", err)
	}
	return out, nil
}
