package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// 視聴履歴の一覧と削除（specs/043-watch-history/data-model.md「Store operations」）。書き込みは
// 再生位置と同じ取引で行うので SaveProgress（progress.go）にあり、ここは読みと削除だけを持つ。
// どの操作もドメインイベントを出さない（research.md R-6）。

// ListWatchHistory は視聴履歴を (played_at desc, id desc) の順に、cursor の次から limit 件返す。
// cursor が空なら先頭から。読めない cursor は domain.ErrInvalidCursor。
//
// 各件の Video は、同じ content_key の動画に audience が開ける所在があるときだけ入る
// （visibleVideoCondition。集まりの代表の規則ではなく動画ごとの条件なので、代表でないメンバーも
// その動画を持つ。research.md R-5）。audience は境界が分類した見る人で、ハンドラが渡す。
func (p *PlaybackStore) ListWatchHistory(
	ctx context.Context, audience domain.Audience, cursor string, limit int,
) (domain.WatchHistoryPage, error) {
	limit = max(1, limit)
	query := `select id, content_key, title, played_at from watch_history`
	args := []any{}
	if cursor != "" {
		after, err := domain.DecodeWatchHistoryCursor(cursor)
		if err != nil {
			return domain.WatchHistoryPage{}, err
		}
		query += ` where played_at < ? or (played_at = ? and id < ?)`
		args = append(args, after.PlayedAtMs, after.PlayedAtMs, after.ID)
	}
	// 1 件多く読んで、続きがあるかを知る。
	query += ` order by played_at desc, id desc limit ?`
	args = append(args, limit+1)

	type row struct {
		entry      domain.WatchHistoryEntry
		contentKey string
		playedAtMs int64
	}
	rows, err := p.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return domain.WatchHistoryPage{}, fmt.Errorf("cannot read the watch history: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var read []row
	for rows.Next() {
		var item row
		if err := rows.Scan(&item.entry.ID, &item.contentKey, &item.entry.Title, &item.playedAtMs); err != nil {
			return domain.WatchHistoryPage{}, fmt.Errorf("cannot read the watch history: %w", err)
		}
		item.entry.PlayedAt = time.UnixMilli(item.playedAtMs)
		read = append(read, item)
	}
	if err := rows.Err(); err != nil {
		return domain.WatchHistoryPage{}, fmt.Errorf("cannot read the watch history: %w", err)
	}
	_ = rows.Close()

	page := domain.WatchHistoryPage{Items: make([]domain.WatchHistoryEntry, 0, min(len(read), limit))}
	if len(read) > limit {
		read = read[:limit]
		last := read[len(read)-1]
		page.NextCursor = domain.EncodeWatchHistoryCursor(domain.WatchHistoryCursor{
			PlayedAtMs: last.playedAtMs, ID: last.entry.ID,
		})
	}

	keys := make([]string, 0, len(read))
	for _, item := range read {
		keys = append(keys, item.contentKey)
	}
	videos, err := p.visibleVideosByContentKey(ctx, audience, keys)
	if err != nil {
		return domain.WatchHistoryPage{}, err
	}
	for _, item := range read {
		if video, ok := videos[item.contentKey]; ok {
			item.entry.Video = &video
		}
		page.Items = append(page.Items, item.entry)
	}
	return page, nil
}

// visibleVideosByContentKey は content_key の集合から、audience が開ける所在を持つ動画を引く。
// videos.content_key は空でなければ一意なので、鍵 1 つに動画は高々 1 本である。鍵の集合は
// SQLite の引数の上限に掛からないよう、json_each に 1 つの引数で渡す。
func (p *PlaybackStore) visibleVideosByContentKey(
	ctx context.Context, audience domain.Audience, keys []string,
) (map[string]domain.Video, error) {
	out := map[string]domain.Video{}
	if len(keys) == 0 {
		return out, nil
	}
	encoded, err := json.Marshal(keys)
	if err != nil {
		return nil, fmt.Errorf("cannot build content keys: %w", err)
	}
	rows, err := p.sql.QueryContext(ctx, `select `+videoColumns(audience)+` from videos`+representativeJoin(audience)+`
		where videos.content_key in (select value from json_each(?)) and videos.content_key <> '' and `+
		visibleVideoCondition("videos", audience), string(encoded))
	if err != nil {
		return nil, fmt.Errorf("cannot read the watch history videos: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		video, err := scanVideo(rows)
		if err != nil {
			return nil, fmt.Errorf("cannot read the watch history videos: %w", err)
		}
		out[video.ContentKey] = video
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the watch history videos: %w", err)
	}
	return out, nil
}

// DeleteWatchHistoryEntry は視聴履歴を 1 件消し、その件があったかを返す。ほかの表の行は変えない
// （要件 9）。
func (p *PlaybackStore) DeleteWatchHistoryEntry(ctx context.Context, id int64) (bool, error) {
	result, err := p.sql.ExecContext(ctx, `delete from watch_history where id = ?`, id)
	if err != nil {
		return false, fmt.Errorf("cannot delete the watch history entry (id=%d): %w", id, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("cannot delete the watch history entry (id=%d): %w", id, err)
	}
	return affected > 0, nil
}

// ClearWatchHistory は視聴履歴をすべて消す。空でも誤りにしない。ほかの表の行は変えない（要件 9）。
func (p *PlaybackStore) ClearWatchHistory(ctx context.Context) error {
	if _, err := p.sql.ExecContext(ctx, `delete from watch_history`); err != nil {
		return fmt.Errorf("cannot clear the watch history: %w", err)
	}
	return nil
}
