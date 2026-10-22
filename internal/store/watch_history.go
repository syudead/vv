package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// SaveProgress writes history in its transaction; history reads and deletes emit no domain events.

// ListWatchHistory filters before keyset paging; attach a video only when audience-visible, including nonrepresentative bundle members.
func (p *PlaybackStore) ListWatchHistory(
	ctx context.Context, audience domain.Audience, query domain.WatchHistoryQuery, cursor string, limit int,
) (domain.WatchHistoryPage, error) {
	limit = max(1, limit)
	from, conditions, args := watchHistoryFilter(audience, query)
	if !query.Before.IsZero() {
		conditions = append(conditions, `h.played_at < ?`)
		args = append(args, query.Before.UnixMilli())
	}
	if cursor != "" {
		after, err := domain.DecodeWatchHistoryCursor(cursor)
		if err != nil {
			return domain.WatchHistoryPage{}, err
		}
		conditions = append(conditions, `(h.played_at < ? or (h.played_at = ? and h.id < ?))`)
		args = append(args, after.PlayedAtMs, after.PlayedAtMs, after.ID)
	}
	statement := `select h.id, h.content_key, h.title, h.played_at` + from + whereClause(conditions) +
		` order by h.played_at desc, h.id desc limit ?`
	args = append(args, limit+1)

	type row struct {
		entry      domain.WatchHistoryEntry
		contentKey string
		playedAtMs int64
	}
	rows, err := p.sql.QueryContext(ctx, statement, args...)
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

// ListWatchHistoryDays ignores Before and groups matching timestamps in Go using the requested timezone, which SQLite does not know.
func (p *PlaybackStore) ListWatchHistoryDays(
	ctx context.Context, audience domain.Audience, query domain.WatchHistoryQuery, loc *time.Location,
) ([]string, error) {
	from, conditions, args := watchHistoryFilter(audience, query)
	rows, err := p.sql.QueryContext(ctx, `select h.played_at`+from+whereClause(conditions)+
		` order by h.played_at desc, h.id desc`, args...)
	if err != nil {
		return nil, fmt.Errorf("cannot read the watch history days: %w", err)
	}
	defer func() { _ = rows.Close() }()
	days := []string{}
	for rows.Next() {
		var playedAtMs int64
		if err := rows.Scan(&playedAtMs); err != nil {
			return nil, fmt.Errorf("cannot read the watch history days: %w", err)
		}
		day := time.UnixMilli(playedAtMs).In(loc).Format(time.DateOnly)
		if len(days) == 0 || days[len(days)-1] != day {
			days = append(days, day)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the watch history days: %w", err)
	}
	return days, nil
}

// Keep watch-state filtering equivalent to domain.ClassifyWatch; WatchHistoryAll applies no condition.
func watchHistoryStateCondition(filter domain.WatchHistoryFilter) string {
	switch filter {
	case domain.WatchHistoryInProgress:
		return `(p.content_key is not null and p.completed = 0 and p.position_ms > 0)`
	case domain.WatchHistoryWatched:
		return `p.completed = 1`
	default:
		return ""
	}
}

// Join videos only when filtering; shared bundle progress determines member watch-state classification.
func watchHistoryFilter(audience domain.Audience, query domain.WatchHistoryQuery) (string, []string, []any) {
	from := ` from watch_history h`
	watch := watchHistoryStateCondition(query.Filter)
	if watch == "" && query.Search.Empty() {
		return from, nil, nil
	}
	from += ` left join videos hv on hv.content_key = h.content_key and hv.content_key <> '' and ` +
		visibleVideoCondition("hv", audience) +
		` left join playback_progress p on p.content_key = ` + userKeyExpr("hv") + ` and hv.id is not null`

	var conditions []string
	var args []any
	if watch != "" {
		// Entries without a current video appear only in the unfiltered view.
		conditions = append(conditions, `hv.id is not null and `+watch)
	}
	if !query.Search.Empty() {
		// The entire expression must match one location, as in library search.
		locationExpr, locationArgs := titleSearchCondition(query.Search, func(text string) (string, []any) {
			return `(instr(` + searchKeyTitleLine("sl") + `, ?) > 0 or instr(` +
				searchKeyDisplayNameLine("sl") + `, ?) > 0)`, []any{text, text}
		})
		snapshotExpr, snapshotArgs := titleSearchCondition(query.Search, func(text string) (string, []any) {
			return `instr(coalesce(h.title_key, ''), ?) > 0`, []any{text}
		})
		conditions = append(conditions, `((hv.id is not null and exists (select 1 from video_locations sl where sl.video_id = hv.id and `+
			visibleLocationCondition("sl", audience)+` and `+locationExpr+`)) or (hv.id is null and `+snapshotExpr+`))`)
		args = append(args, locationArgs...)
		args = append(args, snapshotArgs...)
	}
	return from, conditions, args
}

func titleSearchCondition(expr domain.SearchExpr, match func(text string) (string, []any)) (string, []any) {
	var args []any
	clauses := make([]string, 0, len(expr.Clauses))
	for _, clause := range expr.Clauses {
		terms := make([]string, 0, len(clause.Terms))
		for _, term := range clause.Terms {
			// Normalize term newlines to match stored title keys.
			condition, termArgs := match(strings.ReplaceAll(term.Text, "\n", " "))
			if term.Negated {
				condition = `not ` + condition
			}
			terms = append(terms, condition)
			args = append(args, termArgs...)
		}
		clauses = append(clauses, `(`+strings.Join(terms, " or ")+`)`)
	}
	return `(` + strings.Join(clauses, " and ") + `)`, args
}

// The first search-key line is the folded title; component newlines are already normalized to spaces.
func searchKeyTitleLine(alias string) string {
	return `substr(` + alias + `.search_key, 1, instr(` + alias + `.search_key, char(10)) - 1)`
}

// Read only the optional third display-name line, excluding the relative path.
func searchKeyDisplayNameLine(alias string) string {
	rest := `substr(` + alias + `.search_key, instr(` + alias + `.search_key, char(10)) + 1)`
	return `(case when instr(` + alias + `.search_key, char(10)) > 0 and instr(` + rest + `, char(10)) > 0 ` +
		`then substr(` + rest + `, instr(` + rest + `, char(10)) + 1) else '' end)`
}

func whereClause(conditions []string) string {
	if len(conditions) == 0 {
		return ""
	}
	return ` where ` + strings.Join(conditions, " and ")
}

// RefreshWatchHistoryTitleKeys fills only NULL keys in resumable startup batches before requests are served.
func (p *PlaybackStore) RefreshWatchHistoryTitleKeys(ctx context.Context) (int, error) {
	total := 0
	for {
		count, err := p.refreshWatchHistoryTitleKeyBatch(ctx)
		if err != nil {
			return total, err
		}
		total += count
		if count < searchKeyBatchSize {
			return total, nil
		}
	}
}

func (p *PlaybackStore) refreshWatchHistoryTitleKeyBatch(ctx context.Context) (int, error) {
	tx, err := p.sql.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("cannot fill the watch history title keys: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	batch, err := watchHistoryRowsWithoutTitleKey(ctx, tx)
	if err != nil {
		return 0, err
	}
	for _, item := range batch {
		if _, err := tx.ExecContext(ctx, `update watch_history set title_key = ? where id = ?`,
			searchKeyPart(item.title), item.id); err != nil {
			return 0, fmt.Errorf("cannot fill the watch history title key (id=%d): %w", item.id, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("cannot fill the watch history title keys: %w", err)
	}
	return len(batch), nil
}

type watchHistoryTitle struct {
	id    int64
	title string
}

func watchHistoryRowsWithoutTitleKey(ctx context.Context, tx *sql.Tx) ([]watchHistoryTitle, error) {
	rows, err := tx.QueryContext(ctx, `select id, title from watch_history where title_key is null order by id limit ?`,
		searchKeyBatchSize)
	if err != nil {
		return nil, fmt.Errorf("cannot fill the watch history title keys: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var batch []watchHistoryTitle
	for rows.Next() {
		var item watchHistoryTitle
		if err := rows.Scan(&item.id, &item.title); err != nil {
			return nil, fmt.Errorf("cannot fill the watch history title keys: %w", err)
		}
		batch = append(batch, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot fill the watch history title keys: %w", err)
	}
	return batch, nil
}

// Batch keys through JSON to avoid bind limits; nonempty content keys identify at most one video.
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
	_ = rows.Close()
	if err := attachVersionsIn(ctx, p.sql, audience, out); err != nil {
		return nil, err
	}
	return out, nil
}

// DeleteWatchHistoryEntry changes no other tables and reports whether the entry existed.
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

// ClearWatchHistory changes no other tables and succeeds when already empty.
func (p *PlaybackStore) ClearWatchHistory(ctx context.Context) error {
	if _, err := p.sql.ExecContext(ctx, `delete from watch_history`); err != nil {
		return fmt.Errorf("cannot clear the watch history: %w", err)
	}
	return nil
}
