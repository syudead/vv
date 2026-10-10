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

// 視聴履歴の一覧と削除（specs/043-watch-history/data-model.md「Store operations」）。書き込みは
// 再生位置と同じ取引で行うので SaveProgress（progress.go）にあり、ここは読みと削除だけを持つ。
// どの操作もドメインイベントを出さない（research.md R-6）。

// ListWatchHistory は query を満たす視聴履歴を (played_at desc, id desc) の順に、cursor の次から
// limit 件返す。cursor が空なら先頭から。読めない cursor は domain.ErrInvalidCursor。条件は
// ページを切る前に同じ SQL の文で掛けるので、カーソルは絞った並びの中を指す（research.md R-8）。
//
// 各件の Video は、同じ content_key の動画に audience が開ける所在があるときだけ入る
// （visibleVideoCondition。集まりの代表の規則ではなく動画ごとの条件なので、代表でないメンバーも
// その動画を持つ。research.md R-5）。audience は境界が分類した見る人で、ハンドラが渡す。
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
	// 1 件多く読んで、続きがあるかを知る。
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

// ListWatchHistoryDays は query の絞り込みと検索を満たす件のある日を、loc の暦の YYYY-MM-DD で
// 新しい順に返す（research.md R-11）。query.Before は見ない。SQLite は地域を知らないので、合う件の
// played_at だけを読んで Go で日に分ける。日は時刻の順に並ぶので、隣と同じ日を捨てれば重複は残らない。
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

// watchHistoryFilter は視聴履歴（別名 h）の from 句と、query の絞り込みと検索の条件句と引数を返す。
// 絞り込みも検索も無ければ動画を結ばない。
//
// 動画（別名 hv）は同じ content_key の、audience が開ける所在を持つ動画で、一覧の各件の Video と
// 同じ条件である。再生位置（別名 p）は一覧と同じく利用者データの鍵（userKeyExpr）で結ぶので、
// 集まりのメンバーの件は集まりの共有の位置で分類される（research.md R-9）。
func watchHistoryFilter(audience domain.Audience, query domain.WatchHistoryQuery) (string, []string, []any) {
	from := ` from watch_history h`
	watch := watchCondition(query.Filter.WatchFilter())
	if watch == "" && query.Search.Empty() {
		return from, nil, nil
	}
	from += ` left join videos hv on hv.content_key = h.content_key and hv.content_key <> '' and ` +
		visibleVideoCondition("hv", audience) +
		` left join playback_progress p on p.content_key = ` + userKeyExpr("hv") + ` and hv.id is not null`

	var conditions []string
	var args []any
	if watch != "" {
		// 動画の無い件は「すべて」にだけ出る（Edge Case）。
		conditions = append(conditions, `hv.id is not null and `+watch)
	}
	if !query.Search.Empty() {
		// 式全体を所在 1 行に対して評価する。語ごとに別の所在で満たした動画は当たらない
		// （ライブラリの chosenLocationsCTE と同じ、要件 9）。
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

// titleSearchCondition は検索式を、語ごとに match が返す条件句でつないだ 1 つの条件句にする。
// 節は and、節の中の語は or、除外の語は not でつなぐ（searchExprCondition と同じ）。
func titleSearchCondition(expr domain.SearchExpr, match func(text string) (string, []any)) (string, []any) {
	var args []any
	clauses := make([]string, 0, len(expr.Clauses))
	for _, clause := range expr.Clauses {
		terms := make([]string, 0, len(clause.Terms))
		for _, term := range clause.Terms {
			// search_key と title_key の題名の中の改行は空白にしてあるので、語の側もそろえる
			// （searchExprCondition と同じ）。
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

// searchKeyTitleLine は所在（別名 alias）の search_key の 1 行目（題名の照合形）を返す式である。
// search_key は「題名\n相対パス[\n表示名]」の形で、各部分の中の改行は空白にしてある
// （locationSearchKey）。鍵が空なら空文字になる。
func searchKeyTitleLine(alias string) string {
	return `substr(` + alias + `.search_key, 1, instr(` + alias + `.search_key, char(10)) - 1)`
}

// searchKeyDisplayNameLine は所在（別名 alias）の search_key の 3 行目（表示名の照合形）を返す式
// である。表示名の無い所在では空文字になる。相対パスの行は含めない（research.md R-10）。
func searchKeyDisplayNameLine(alias string) string {
	rest := `substr(` + alias + `.search_key, instr(` + alias + `.search_key, char(10)) + 1)`
	return `(case when instr(` + alias + `.search_key, char(10)) > 0 and instr(` + rest + `, char(10)) > 0 ` +
		`then substr(` + rest + `, instr(` + rest + `, char(10)) + 1) else '' end)`
}

// whereClause は条件句を and で結んだ where 句にする。条件が無ければ空。
func whereClause(conditions []string) string {
	if len(conditions) == 0 {
		return ""
	}
	return ` where ` + strings.Join(conditions, " and ")
}

// RefreshWatchHistoryTitleKeys は title_key が null の行（移行の前に書かれた行）に題名の照合形を
// 書き、書いた行数を返す（data-model.md「Migration」）。起動時に、移行の後で受け付けより前に
// 呼ぶ。null でない行は変えない。searchKeyBatchSize 行ずつのトランザクションで書くので、途中で
// 止まっても次の起動で続きから埋まる。
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

// watchHistoryTitle は title_key を埋める行の id と題名である。
type watchHistoryTitle struct {
	id    int64
	title string
}

// watchHistoryRowsWithoutTitleKey は title_key が null の行を id の順に searchKeyBatchSize 行まで読む。
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
