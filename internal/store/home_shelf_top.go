package store

// ホームの棚の最初のページを、並べ替えの鍵の索引で先頭から読む（specs/057-home/research.md R-12）。
// 追加日・更新日・作成日・最後の再生日の並び順では、グループの項目の値はメンバーの最大なので、
// hs_base を並び順の索引でなぞり、当たった動画ごとに「どの根にも入らないか、全部は当たっていない根に
// 入る動画の項目」か「全部が当たった根の、値が最大のメンバーとして出るグループの項目」かを決めれば、
// 項目は並び順のまま出てくる。なぞるのは条件だけの軽い問い合わせで、keyset でまとまりごとに進め、
// まとまりに初めて現れた根だけをまとめて判定し（wholeOf）、判定は読み出しのあいだ覚えておく。項目が
// limit + 1 件そろい、limit 件目と同じ値の行を読み切ったところで止める。棚の項目の数は limit + 1 を
// 上限に数える（HomeShelfItems.total）。
//
// 同じ値の項目の並びはライブラリと同じく項目の id（動画は id の 2 倍、グループは 2 倍に 1 を足す）で
// 決まるが、なぞる順はメンバーの動画の id なので、集めた項目を最後に並べ直す。

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/syudead/vv/internal/domain"
)

// homeShelfTopSort は索引で先頭から読める並び順 1 つで、column は hs_base の列、desc は向きである。
type homeShelfTopSort struct {
	column string
	desc   bool
}

// homeShelfTopSorts は索引で先頭から読む並び順である。値が無い項目を末尾に置くため、無いことのある
// 最後の再生日は降順だけ（降順では NULL が末尾になる）。合計（長さ・大きさ）、グループ自身の値（題名、
// お気に入りにした日時）、シャッフルはメンバー 1 本から決まらないので、項目を組み立てて並べる
// （homeShelfItemsSQL）。
var homeShelfTopSorts = map[domain.VideoSort]homeShelfTopSort{
	domain.SortAddedDesc: {column: `added_at`, desc: true}, domain.SortAddedAsc: {column: `added_at`},
	domain.SortModifiedDesc: {column: `mtime`, desc: true}, domain.SortModifiedAsc: {column: `mtime`},
	domain.SortCreatedDesc: {column: `created_at`, desc: true}, domain.SortCreatedAsc: {column: `created_at`},
	domain.SortPlayedDesc: {column: `played_at`, desc: true},
}

// homeShelfTopBatch は最初のまとまりの行数で、まとまりごとに homeShelfTopGrowth 倍にする。
const (
	homeShelfTopBatch    = 64
	homeShelfTopGrowth   = 4
	homeShelfTopMaxBatch = 4096
)

// homeShelfSortIndexSQL は、棚 specs が索引で先頭から読む並び順の列ごとに、hs_base の索引を作る文を返す。
func homeShelfSortIndexSQL(specs []listSpec) []homeShelfTableSQL {
	var stmts []homeShelfTableSQL
	seen := map[string]bool{}
	for _, spec := range specs {
		sort, ok := homeShelfTopSorts[spec.sort]
		if !ok || seen[sort.column] {
			continue
		}
		seen[sort.column] = true
		stmts = append(stmts, homeShelfTableSQL{sql: `create index hs_base_` + sort.column + ` on hs_base (` + sort.column + `, id)`})
	}
	return stmts
}

// direction は並び順の向きの SQL である。
func (s homeShelfTopSort) direction() string {
	if s.desc {
		return `desc`
	}
	return `asc`
}

// homeShelfScanRow は索引でなぞった行 1 つ（当たった動画と、入っている根の 1 つ）である。
type homeShelfScanRow struct {
	videoID   int64
	path      string
	value     any
	groupID   sql.NullInt64
	watch     string
	favorited bool
	playable  bool
}

// homeShelfGroup は根 1 つの判定である。
type homeShelfGroup struct {
	whole     bool
	maxMember int64
	watch     string
}

// homeShelfTopRow は読んだ項目 1 つである。
type homeShelfTopRow struct {
	key   itemRow
	value any
}

// homeShelfTopScan は 1 つの棚を先頭からなぞる状態である。
type homeShelfTopScan struct {
	ctx    context.Context
	tx     *sql.Tx
	spec   listSpec
	rule   homeShelfRule
	sort   homeShelfTopSort
	groups map[int64]homeShelfGroup
	// last はなぞった最後の行の鍵で、started が真なら次のまとまりはその続きから読む。
	last    homeShelfScanRow
	started bool
	// watches は項目の視聴状態の絞り込みの値（各段。すべてに等しい項目だけが出る）である。
	watches  []domain.WatchFilter
	playable bool
	favorite bool
}

// homeShelfTopPage は棚 spec の最初の limit 件と、limit + 1 を上限にした項目の数を、索引で先頭から読む。
func homeShelfTopPage(ctx context.Context, tx *sql.Tx, spec listSpec, rule homeShelfRule, limit int) (firstPageRows, error) {
	scan := &homeShelfTopScan{ctx: ctx, tx: tx, spec: spec, rule: rule, sort: homeShelfTopSorts[spec.sort],
		groups: map[int64]homeShelfGroup{}, playable: spec.anyLevel(func(level listSpec) bool { return level.playableOnly }),
		favorite: spec.itemGroupsFavorite()}
	for _, level := range spec.levels() {
		if level.watch != "" {
			scan.watches = append(scan.watches, level.watch)
		}
	}
	var items []homeShelfTopRow
	batch := homeShelfTopBatch
	for {
		rows, done, err := scan.next(batch)
		if err != nil {
			return firstPageRows{}, err
		}
		if err := scan.decide(rows); err != nil {
			return firstPageRows{}, err
		}
		items = append(items, scan.classify(rows)...)
		slices.SortFunc(items, scan.sort.compare)
		// limit + 1 件そろい、limit 件目と同じ値の行を読み切れば止める（同じ値の項目はその値の行から出る）。
		if done || (len(items) > limit && !scan.started) || (len(items) > limit && !sameSortValue(scan.last.value, items[limit-1].value)) {
			break
		}
		batch = min(batch*homeShelfTopGrowth, homeShelfTopMaxBatch)
	}
	page := firstPageRows{total: min(len(items), limit+1)}
	for _, item := range items[:min(len(items), limit)] {
		page.keys = append(page.keys, item.key)
		page.values = append(page.values, item.value)
	}
	return page, nil
}

// next は続きの行を limit 件まで読む。最後の動画の行は、まとまりの切れ目で根が欠けるかもしれないので、
// 行が limit 件あって 2 本以上の動画を含むときは次のまとまりへ送る。done は行を読み切ったかである。
func (s *homeShelfTopScan) next(limit int) ([]homeShelfScanRow, bool, error) {
	names, column := s.rule.names, `videos.`+s.sort.column
	query := `select videos.id, videos.path, ` + column + `, x.group_id, videos.watch_state, videos.favorited_at is not null,
		` + playableCondition + `
		from hs_base videos left join ` + names.gm + ` x on x.video_id = videos.id
		where ` + s.rule.condition
	args := append([]any{}, s.rule.args...)
	if s.started {
		keyset, keysetArgs := s.sort.after(column, s.last)
		query += ` and ` + keyset
		args = append(args, keysetArgs...)
	}
	query += ` order by ` + column + ` ` + s.sort.direction() + `, videos.id ` + s.sort.direction() + ` limit ?`
	rows, err := s.tx.QueryContext(s.ctx, query, append(args, limit)...)
	if err != nil {
		return nil, false, fmt.Errorf("cannot read the home shelf: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []homeShelfScanRow
	for rows.Next() {
		var row homeShelfScanRow
		if err := rows.Scan(&row.videoID, &row.path, &row.value, &row.groupID, &row.watch, &row.favorited, &row.playable); err != nil {
			return nil, false, fmt.Errorf("cannot read the home shelf: %w", err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("cannot read the home shelf: %w", err)
	}
	done := len(out) < limit
	if !done {
		lastVideo := out[len(out)-1].videoID
		cut := len(out)
		for cut > 0 && out[cut-1].videoID == lastVideo {
			cut--
		}
		if cut > 0 {
			out = out[:cut]
		}
	}
	if len(out) > 0 {
		s.last, s.started = out[len(out)-1], true
	}
	return out, done, nil
}

// after は行 last より後ろの行の条件句と引数を返す。降順では値の無い行が末尾に来る。
func (s homeShelfTopSort) after(column string, last homeShelfScanRow) (string, []any) {
	if last.value == nil {
		return `(` + column + ` is null and videos.id < ?)`, []any{last.videoID}
	}
	if s.desc {
		return `(` + column + ` < ? or (` + column + ` = ? and videos.id < ?) or ` + column + ` is null)`,
			[]any{last.value, last.value, last.videoID}
	}
	return `(` + column + ` > ? or (` + column + ` = ? and videos.id > ?))`, []any{last.value, last.value, last.videoID}
}

// decide は rows に初めて現れた根をまとめて判定する。whole はメンバーのすべてが棚の条件に当たる（再生
// 可否を求める棚では再生できるメンバーがある）か、maxMember は並べ替えの値が最大（同じ値なら id が
// 最大）のメンバーである。
func (s *homeShelfTopScan) decide(rows []homeShelfScanRow) error {
	var fresh []int64
	for _, row := range rows {
		if row.groupID.Valid {
			if _, known := s.groups[row.groupID.Int64]; !known && !slices.Contains(fresh, row.groupID.Int64) {
				fresh = append(fresh, row.groupID.Int64)
			}
		}
	}
	if len(fresh) == 0 {
		return nil
	}
	encoded, err := json.Marshal(fresh)
	if err != nil {
		return fmt.Errorf("cannot build group ids: %w", err)
	}
	names := s.rule.names
	whole := `not exists (select 1 from ` + names.gm + ` w join hs_base videos on videos.id = w.video_id
			where w.group_id = g.group_id and not (` + s.rule.condition + `))`
	if s.rule.groupFilter != "" {
		whole += ` and g.playable = 1`
	}
	result, err := s.tx.QueryContext(s.ctx, `select g.group_id, g.watch_state, `+whole+`,
			(select b.id from `+names.gm+` y join hs_base b on b.id = y.video_id where y.group_id = g.group_id
				order by b.`+s.sort.column+` desc, b.id desc limit 1)
		from `+names.groups+` g where g.group_id in (select value from json_each(?))`,
		append(append([]any{}, s.rule.args...), string(encoded))...)
	if err != nil {
		return fmt.Errorf("cannot decide the home shelf groups: %w", err)
	}
	defer func() { _ = result.Close() }()
	for result.Next() {
		var id int64
		var group homeShelfGroup
		if err := result.Scan(&id, &group.watch, &group.whole, &group.maxMember); err != nil {
			return fmt.Errorf("cannot decide the home shelf groups: %w", err)
		}
		s.groups[id] = group
	}
	return result.Err()
}

// classify は判定の済んだ行 rows を項目にする。動画は、どの根にも入らないか、全部は当たっていない根の
// どれかに入れば動画の項目になり、全部が当たった根は、その maxMember の行でグループの項目になる。
func (s *homeShelfTopScan) classify(rows []homeShelfScanRow) []homeShelfTopRow {
	var items []homeShelfTopRow
	for start := 0; start < len(rows); {
		end := start
		for end < len(rows) && rows[end].videoID == rows[start].videoID {
			end++
		}
		video := rows[start]
		videoItem := false
		for _, row := range rows[start:end] {
			if !row.groupID.Valid {
				videoItem = true
				continue
			}
			group := s.groups[row.groupID.Int64]
			if !group.whole {
				videoItem = true
			} else if group.maxMember == video.videoID && s.cardShown(group) {
				items = append(items, homeShelfTopRow{key: itemRow{groupID: row.groupID, id: row.groupID.Int64*2 + 1}, value: video.value})
			}
		}
		if videoItem && s.videoShown(video) {
			items = append(items, homeShelfTopRow{key: itemRow{id: video.videoID * 2,
				videoID: sql.NullInt64{Int64: video.videoID, Valid: true}, path: sql.NullString{String: video.path, Valid: true}}, value: video.value})
		}
		start = end
	}
	return items
}

// videoShown は動画の項目に再生可否・お気に入り・視聴状態の絞り込みを掛ける。
func (s *homeShelfTopScan) videoShown(row homeShelfScanRow) bool {
	if s.playable && !row.playable || s.favorite && !row.favorited {
		return false
	}
	return s.watchShown(row.watch)
}

// cardShown はグループの項目に視聴状態の絞り込みを掛ける（再生可否は whole の判定に入っている）。
func (s *homeShelfTopScan) cardShown(group homeShelfGroup) bool {
	return s.watchShown(group.watch)
}

// watchShown は視聴状態 state が各段の絞り込みのすべてに等しいかを返す（itemWatchConditions と同じ）。
func (s *homeShelfTopScan) watchShown(state string) bool {
	for _, watch := range s.watches {
		if string(watch) != state {
			return false
		}
	}
	return true
}

// compare は項目をライブラリの並び（値の無い項目は末尾、値、項目の id）で比べる。
func (s homeShelfTopSort) compare(a, b homeShelfTopRow) int {
	if (a.value == nil) != (b.value == nil) {
		if a.value == nil {
			return 1
		}
		return -1
	}
	c := 0
	if a.value != nil {
		c = cmp.Compare(a.value.(int64), b.value.(int64))
	}
	c = cmp.Or(c, cmp.Compare(a.key.id, b.key.id))
	if s.desc {
		return -c
	}
	return c
}

// sameSortValue は 2 つの並べ替えの値（int64 か nil）が同じかを返す。
func sameSortValue(a, b any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.(int64) == b.(int64)
}
