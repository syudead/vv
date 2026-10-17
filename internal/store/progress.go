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
//
// play があって ContentKey が空でないときは、同じ取引で視聴履歴を 1 件書く。playback_id が一意なので、同じ視聴の 2 回目以降の
// 保存は何も足さず、時刻と題名は最初の保存のものに留まる。時刻はサーバーの現在時刻
// （Unix ミリ秒）で、クライアントの時計は使わない。
//
// 保存で domain.WatchState が変わるときだけ、確定後に domain.SmartFoldersChanged を発行する。再生中は数秒ごとに保存されるので、視聴状態の
// 変わらない保存は知らせない。
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

	// 視聴状態が変わったときだけ、スマートフォルダの一致も変わりうる。
	if err := p.db.commitMatchChangeIf(ctx, tx, before != domain.ClassifyWatch(&progress)); err != nil {
		return domain.Progress{}, fmt.Errorf("cannot save the playback position (%s): %w", contentKey, err)
	}

	progress.UpdatedAt = updatedAt.Truncate(time.Second)
	return progress, nil
}

// storedWatchState は取引 tx の中で、content_key の今の記録の視聴状態を返す。記録が無ければ未視聴である。
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
		return domain.Progress{}, fmt.Errorf("cannot read the playback position (%s): %w", contentKey, err)
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
// なる。鍵は JSON の配列 1 つで渡す（tagsByContentKeys と同じ）。鍵ごとに束縛変数を
// 使うと、件数の上限が無いプレイリストの項目が SQLite の束縛変数の上限を超える。
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
