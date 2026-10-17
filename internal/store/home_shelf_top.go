package store

// ホームの棚の最初のページを、基底の表の索引で並び順になぞって読む（specs/057-home/research.md R-12）。
// 追加日は videos の索引を、更新日と作成日は video_locations の索引を、最後の再生日は playback_progress の
// 索引をなぞる。グループの項目の値はメンバーの最大なので、全部が当たった根はその値が最大の子孫の行で
// グループの項目として出る。keyset でまとまりごとに進め、まとまりに初めて現れた根だけを判定し
// （homeShelfReader.decideRoots）、項目が limit + 1 件そろい limit 件目と同じ値の行を読み切ったところで
// 止める。同じ値の項目の並びは項目の id なので、集めた項目を最後に並べ直す。

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/syudead/vv/internal/domain"
)

// homeShelfTopSort は索引でなぞれる並び順 1 つで、column は値の列（group_root_values の *_member の鍵）、
// desc は向き、source はなぞる表である。
type homeShelfTopSort struct {
	column string
	desc   bool
	source string
}

// homeShelfTopSorts は索引でなぞる並び順である。値が無い項目を末尾に置くため、無いことのある最後の再生日は
// 降順だけで、再生の記録のある動画をなぞったあとに記録の無い動画を id の降順でなぞる。
var homeShelfTopSorts = map[domain.VideoSort]homeShelfTopSort{
	domain.SortAddedDesc: {column: `added_at`, desc: true, source: `videos`}, domain.SortAddedAsc: {column: `added_at`, source: `videos`},
	domain.SortModifiedDesc: {column: `mtime`, desc: true, source: `locations`}, domain.SortModifiedAsc: {column: `mtime`, source: `locations`},
	domain.SortCreatedDesc: {column: `created_at`, desc: true, source: `locations`}, domain.SortCreatedAsc: {column: `created_at`, source: `locations`},
	domain.SortPlayedDesc: {column: `played_at`, desc: true, source: `progress`},
}

// homeShelfTopBatch は最初のまとまりの行数で、まとまりごとに homeShelfTopGrowth 倍にする。
const (
	homeShelfTopBatch    = 64
	homeShelfTopGrowth   = 4
	homeShelfTopMaxBatch = 4096
)

// direction は並び順の向きの SQL である。
func (s homeShelfTopSort) direction() string {
	if s.desc {
		return `desc`
	}
	return `asc`
}

// compare は項目をライブラリの並び（値の無い項目は末尾、値、項目の id）で比べる。
func (s homeShelfTopSort) compare(a, b homeShelfItem) int {
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

// homeShelfWalk は 1 つの表を並び順になぞる状態である。
type homeShelfWalk struct {
	// from は from 句（引数は fromArgs）、value は並べ替えの値の式、order は order by の列（値、動画の鍵）
	// である。
	from, value string
	fromArgs    []any
	order       [2]string
	// where は常に掛ける条件（見せる動画など）である。
	where string
	// last はなぞった最後の行の鍵で、started が真なら次のまとまりはその続きから読む。
	lastValue any
	lastKey   any
	started   bool
	// keyOf は行から keyset の鍵（order[1] の値）を取る。
	keyOf func(homeShelfRow) any
}

// walks は並び順 sort の棚 rule をなぞる表を順に返す。最後の再生日では、記録のある動画の表のあとに記録の
// 無い動画の表をなぞる。
func (r *homeShelfReader) walks(rule homeShelfRule, sort homeShelfTopSort) []*homeShelfWalk {
	shown := r.shownCondition()
	if rule.condition != "" {
		shown += ` and ` + rule.condition
	}
	videoKey := func(row homeShelfRow) any { return row.videoID }
	join, joinArgs := r.chosenLocationJoin(rule.spec)
	// 検索式の棚は当たりの集合（全文索引から読んだ id）から動画を引き、並べる。集合は小さいので索引を
	// なぞるより速い。視聴状態だけの棚は、視聴状態の合う根の並べ替えの値を持つメンバーと、グループに入らない
	// 視聴状態の合う動画の集合から引く（視聴状態の表は索引で引ける）。最後の再生日の表は集合と結ばず、
	// 下の表のまま読む。
	hits, hitArgs, ok := r.searchDriven(rule.spec)
	if !ok {
		if driven, err := r.watchDriven(rule); err == nil && driven {
			hits, ok = r.watchHits(rule.watches[0], sort.column), true
		}
	}
	if ok && sort.source != `progress` {
		value, order := `videos.added_at`, [2]string{`videos.added_at`, `videos.id`}
		from := `(` + hits + `) hits join videos on videos.id = hits.id` + join
		if sort.source == `locations` {
			value = `loc.mtime`
			if sort.column == `created_at` {
				value = fileCreatedAtExpr("loc")
			}
			order = [2]string{value, `videos.id`}
		}
		return []*homeShelfWalk{{from: from, fromArgs: append(append([]any{}, hitArgs...), joinArgs...), value: value, order: order,
			where: shown, keyOf: videoKey}}
	}
	switch sort.source {
	case `locations`:
		value := `loc.mtime`
		if sort.column == `created_at` {
			value = fileCreatedAtExpr("loc")
		}
		path, pathArgs := r.chosenPath(rule.spec)
		return []*homeShelfWalk{{
			from: `video_locations loc join videos on videos.id = loc.video_id`, value: value, order: [2]string{value, `loc.video_id`},
			fromArgs: pathArgs, where: visibleLocationCondition("loc", r.audience) + ` and loc.path = ` + path + ` and ` + shown,
			keyOf: videoKey}}
	case `progress`:
		userKey := userKeyExpr("videos")
		return []*homeShelfWalk{
			{from: `playback_progress p join videos on videos.id = ` + shownVideoForKeyExpr("p.content_key", r.audience) + join,
				fromArgs: joinArgs, value: `p.updated_at`, order: [2]string{`p.updated_at`, `p.content_key`}, where: shown,
				keyOf: func(row homeShelfRow) any { return row.userKey }},
			{from: `videos` + join, fromArgs: joinArgs, value: `null`, order: [2]string{`videos.id`, `videos.id`},
				where: shown + ` and not exists (select 1 from playback_progress p where p.content_key = ` + userKey + ` and videos.content_key <> '')`,
				keyOf: videoKey},
		}
	}
	return []*homeShelfWalk{{from: `videos` + join, fromArgs: joinArgs, value: `videos.added_at`,
		order: [2]string{`videos.added_at`, `videos.id`}, where: shown, keyOf: videoKey}}
}

// watchHits は視聴状態 watch の項目になりうる動画の id を選ぶ select 文である: 視聴状態の合ういちばん上の根の、
// 並べ替えの列 column の値を持つメンバー（group_root_values）と、グループに入らない視聴状態の合う動画
// （video_watch_states）。視聴状態は検査済みの列挙なので文に埋め込む。
func (r *homeShelfReader) watchHits(watch domain.WatchFilter, column string) string {
	state := `'` + string(watch) + `'`
	member := `v.` + strings.TrimSuffix(column, `_at`) + `_member`
	return `select ` + member + ` as id from group_root_values v
			where v.audience = ` + strconv.Itoa(audienceColumn(r.audience)) + ` and v.watch_state = ` + state + `
			and ` + member + ` is not null and not exists (select 1 from group_items pg where pg.child_id = v.root_id)
		union
		select videos.id from video_watch_states w join videos on videos.id = ` + shownVideoForKeyExpr("w.content_key", r.audience) + `
			where w.state = ` + state + ` and not exists (select 1 from group_closure c where c.content_key = w.content_key)`
}

// keyset は walk の最後の行より後ろの行の条件句と引数を返す。
func (w *homeShelfWalk) keyset(sort homeShelfTopSort) (string, []any) {
	op := `<`
	if !sort.desc {
		op = `>`
	}
	if w.order[0] == w.order[1] {
		return `(` + w.order[1] + ` ` + op + ` ?)`, []any{w.lastKey}
	}
	if w.lastValue == nil {
		return `(` + w.order[0] + ` is null and ` + w.order[1] + ` ` + op + ` ?)`, []any{w.lastKey}
	}
	nulls := ``
	if sort.desc {
		nulls = ` or ` + w.order[0] + ` is null`
	}
	return `(` + w.order[0] + ` ` + op + ` ? or (` + w.order[0] + ` = ? and ` + w.order[1] + ` ` + op + ` ?)` + nulls + `)`,
		[]any{w.lastValue, w.lastValue, w.lastKey}
}

// skipCarded は、判定の済んだ全部が当たった根 known のメンバーのうち、グループの項目として出る子孫
// （並び順の値が最大のもの）でない動画を SQL で飛ばす条件句と引数を返す。入っている根のどれかが未判定か
// 全部は当たっていなければ飛ばさない。
func (r *homeShelfReader) skipCarded(rule homeShelfRule, known map[int64]homeShelfRoot, sortColumn string) (string, []any) {
	var roots, members []string
	for root, decided := range known {
		if decided.whole {
			roots = append(roots, strconv.FormatInt(root, 10))
			members = append(members, strconv.FormatInt(decided.values.member[sortColumn], 10))
		}
	}
	if len(roots) == 0 {
		return "", nil
	}
	userKey := userKeyExpr("videos")
	closure := `group_closure c where c.content_key = ` + userKey + ` and not exists (select 1 from group_items pg where pg.child_id = c.root_id)`
	if rule.favorite {
		closure = `group_closure c join group_favorites gf on gf.group_id = c.root_id where c.content_key = ` + userKey
	}
	return ` and (videos.id in (select value from json_each(?)) or exists (select 1 from ` + closure + `
			and c.root_id not in (select value from json_each(?))) or not exists (select 1 from ` + closure + `))`,
		[]any{`[` + strings.Join(members, ",") + `]`, `[` + strings.Join(roots, ",") + `]`}
}

// next は walk の続きの行を limit 件以上読む。行が limit 件あるときは最後の動画の行を次のまとまりへ送り
// （根が欠けないように）、まとまりが 1 本の動画の行だけなら読み切るまで limit を増やす。done は読み切ったか。
func (r *homeShelfReader) next(rule homeShelfRule, sort homeShelfTopSort, w *homeShelfWalk, known map[int64]homeShelfRoot, limit int) ([]homeShelfRow, bool, error) {
	for {
		query := `select ` + r.rowColumns(rule, false) + `, ` + w.value + ` from ` + w.from + ` where ` + w.where
		args := append(append([]any{}, w.fromArgs...), rule.args...)
		if w.started {
			clause, keyArgs := w.keyset(sort)
			query += ` and ` + clause
			args = append(args, keyArgs...)
		}
		skip, skipArgs := r.skipCarded(rule, known, sort.column)
		query += skip
		args = append(args, skipArgs...)
		query += ` order by ` + w.order[0] + ` ` + sort.direction() + `, ` + w.order[1] + ` ` + sort.direction() + ` limit ?`
		rows, err := r.queryRows(query, append(args, limit))
		if err != nil {
			return nil, false, fmt.Errorf("cannot walk the home shelf: %w", err)
		}
		if len(rows) < limit {
			if len(rows) > 0 {
				w.lastValue, w.lastKey, w.started = rows[len(rows)-1].value, w.keyOf(rows[len(rows)-1]), true
			}
			return rows, true, nil
		}
		cut := len(rows)
		for cut > 0 && rows[cut-1].videoID == rows[len(rows)-1].videoID {
			cut--
		}
		if cut > 0 {
			rows = rows[:cut]
			w.lastValue, w.lastKey, w.started = rows[len(rows)-1].value, w.keyOf(rows[len(rows)-1]), true
			return rows, false, nil
		}
		limit *= homeShelfTopGrowth
	}
}

// topPage は棚 rule の最初の limit 件と、limit + 1 を上限にした項目の数を、索引でなぞって読む。
func (r *homeShelfReader) topPage(rule homeShelfRule, limit int) (firstPageRows, error) {
	sort := homeShelfTopSorts[rule.spec.sort]
	known := map[int64]homeShelfRoot{}
	var items []homeShelfItem
	for _, walk := range r.walks(rule, sort) {
		batch := homeShelfTopBatch
		for {
			rows, done, err := r.next(rule, sort, walk, known, batch)
			if err != nil {
				return firstPageRows{}, err
			}
			var roots []int64
			for _, row := range rows {
				roots = append(roots, row.roots...)
			}
			if err := r.decideRoots(rule, roots, known, true); err != nil {
				return firstPageRows{}, err
			}
			items = append(items, r.classify(rule, rows, known, sort.column)...)
			slices.SortFunc(items, sort.compare)
			// limit + 1 件そろい、limit 件目と同じ値の行を読み切れば止める（同じ値の項目はその値の行から出る）。
			if len(items) > limit && (done || !sameSortValue(walk.lastValue, items[limit-1].value)) {
				return pageOf(items, limit, known), nil
			}
			if done {
				break
			}
			batch = min(batch*homeShelfTopGrowth, homeShelfTopMaxBatch)
		}
	}
	return pageOf(items, limit, known), nil
}
