package store

// ホームの棚の最初のページを、並べ替えの鍵の索引で先頭から読む（specs/057-home/research.md R-12）。
// 追加日・更新日・作成日・最後の再生日の並び順では、グループの項目の値はメンバーの最大なので、
// hs_base を並び順の索引でなぞり、当たった動画ごとに「どの根にも入らないか、全部は当たっていない根に
// 入る動画の項目」か「全部が当たった根の、値が最大のメンバーとして出るグループの項目」かを決めれば、
// 項目は並び順のまま出てくる。グループの判定（wholeOf）は、なぞった候補の根についてだけ行う。先頭の
// limit + 1 件で止め、棚の項目の数は limit + 1 を上限に数える（HomeShelfItems.total）。
//
// 同じ値の項目の並びはライブラリと同じく項目の id（動画は id の 2 倍、グループは 2 倍に 1 を足す）で
// 決まるが、なぞる順はメンバーの動画の id なので、limit 件目の値と同じ値の項目をすべて読み足してから
// 並べ直す（homeShelfTopPage）。

import (
	"cmp"
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"

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

// homeShelfTopQuery は先頭から読む問い合わせの組み立てである。
type homeShelfTopQuery struct {
	rule homeShelfRule
	sort homeShelfTopSort
	args []any
}

// wholeOf は根 g のメンバーがすべて棚の条件に当たる（再生可否を求める棚では再生できるメンバーがある）
// 条件句を返し、その引数を足す。
func (q *homeShelfTopQuery) wholeOf(g string) string {
	q.args = append(q.args, q.rule.args...)
	whole := `not exists (select 1 from ` + q.rule.names.gm + ` w join hs_base videos on videos.id = w.video_id
			where w.group_id = ` + g + ` and not (` + q.rule.condition + `))`
	if q.rule.groupFilter != "" {
		whole += ` and (select g.playable from ` + q.rule.names.groups + ` g where g.group_id = ` + g + `) = 1`
	}
	return `(` + whole + `)`
}

// videoFilter は動画の項目に掛ける再生可否・お気に入り・視聴状態の句（` and ` で始まる）を返し、引数を足す。
func (q *homeShelfTopQuery) videoFilter() string {
	filter := q.rule.videoFilter
	if q.rule.watch != "" {
		filter += ` and ` + strings.ReplaceAll(strings.TrimPrefix(q.rule.watch, ` where `), `watch_state`, `videos.watch_state`)
		q.args = append(q.args, q.rule.watchArgs...)
	}
	return filter
}

// cardFilter はグループの項目に掛ける視聴状態の句（` and ` で始まる）を返し、引数を足す。
func (q *homeShelfTopQuery) cardFilter() string {
	if q.rule.watch == "" {
		return ""
	}
	q.args = append(q.args, q.rule.watchArgs...)
	return ` and ` + strings.ReplaceAll(strings.TrimPrefix(q.rule.watch, ` where `), `watch_state`, `g.watch_state`)
}

// direction は並び順の向きの SQL である。
func (s homeShelfTopSort) direction() string {
	if s.desc {
		return `desc`
	}
	return `asc`
}

// sql は、値が boundary（nil なら制限なし。*boundary が nil なら値の無い行）の項目を並び順に limit 件
// （0 なら制限なし）まで読む文とその引数を返す。列は動画の id、所在のパス、根の id、並べ替えの値、
// グループの項目か（is_card）である。
func (q *homeShelfTopQuery) sql(boundary *any, limit int) (string, []any) {
	names, column := q.rule.names, `videos.`+q.sort.column
	q.args = nil
	isCard := `(x.group_id is not null and ` + q.wholeOf(`x.group_id`) + `)`
	text := `select videos.id, videos.path, x.group_id, ` + column + `, ` + isCard + `
		from hs_base videos left join ` + names.gm + ` x on x.video_id = videos.id
		left join ` + names.groups + ` g on g.group_id = x.group_id
		where ` + q.rule.condition
	q.args = append(q.args, q.rule.args...)
	text += ` and ((x.group_id is null` + q.videoFilter() + `)
			or (x.group_id is not null and not ` + q.wholeOf(`x.group_id`) + ` and x.group_id = (select min(y.group_id) from ` +
		names.gm + ` y where y.video_id = videos.id and not ` + q.wholeOf(`y.group_id`) + `)` + q.videoFilter() + `)
			or (` + isCard + ` and videos.id = (select b.id from ` + names.gm + ` y join hs_base b on b.id = y.video_id
				where y.group_id = x.group_id order by b.` + q.sort.column + ` desc, b.id desc limit 1)` + q.cardFilter() + `))`
	q.args = append(q.args, q.rule.args...)
	if boundary != nil {
		if *boundary == nil {
			text += ` and ` + column + ` is null`
		} else {
			text += ` and ` + column + ` = ?`
			q.args = append(q.args, *boundary)
		}
	}
	text += ` order by ` + column + ` ` + q.sort.direction() + `, videos.id ` + q.sort.direction()
	if limit > 0 {
		text += ` limit ?`
		q.args = append(q.args, limit)
	}
	return text, q.args
}

// homeShelfTopRow は先頭から読んだ項目 1 つである。
type homeShelfTopRow struct {
	key   itemRow
	value any
}

// read は boundary と limit の問い合わせを読み、項目にする。
func (q *homeShelfTopQuery) read(ctx context.Context, tx *sql.Tx, boundary *any, limit int) ([]homeShelfTopRow, error) {
	query, args := q.sql(boundary, limit)
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("cannot read the home shelf: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []homeShelfTopRow
	for rows.Next() {
		var videoID int64
		var path string
		var groupID sql.NullInt64
		var value any
		var isCard bool
		if err := rows.Scan(&videoID, &path, &groupID, &value, &isCard); err != nil {
			return nil, fmt.Errorf("cannot read the home shelf: %w", err)
		}
		row := homeShelfTopRow{value: value}
		if isCard {
			row.key = itemRow{groupID: groupID, id: groupID.Int64*2 + 1}
		} else {
			row.key = itemRow{id: videoID * 2, videoID: sql.NullInt64{Int64: videoID, Valid: true}, path: sql.NullString{String: path, Valid: true}}
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the home shelf: %w", err)
	}
	return out, rows.Close()
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

// homeShelfTopPage は棚 spec の最初の limit 件と、limit + 1 を上限にした項目の数を、索引で先頭から読む。
// limit 件目の値と同じ値の項目をすべて読み足し、ライブラリの並びに並べ直してから limit 件を切る。
func homeShelfTopPage(ctx context.Context, tx *sql.Tx, spec listSpec, rule homeShelfRule, limit int) (firstPageRows, error) {
	q := &homeShelfTopQuery{rule: rule, sort: homeShelfTopSorts[spec.sort]}
	head, err := q.read(ctx, tx, nil, limit+1)
	if err != nil {
		return firstPageRows{}, err
	}
	rows := head
	if len(head) > limit {
		boundary := head[limit-1].value
		ties, err := q.read(ctx, tx, &boundary, 0)
		if err != nil {
			return firstPageRows{}, err
		}
		rows = slices.DeleteFunc(head, func(row homeShelfTopRow) bool { return sameSortValue(row.value, boundary) })
		rows = append(rows, ties...)
	}
	slices.SortFunc(rows, q.sort.compare)
	page := firstPageRows{total: min(len(rows), limit+1)}
	for _, row := range rows[:min(len(rows), limit)] {
		page.keys = append(page.keys, row.key)
		page.values = append(page.values, row.value)
	}
	return page, nil
}

// sameSortValue は 2 つの並べ替えの値（int64 か nil）が同じかを返す。
func sameSortValue(a, b any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.(int64) == b.(int64)
}
