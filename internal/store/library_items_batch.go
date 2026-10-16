package store

// 絞り込みをいくつも並べて読むライブラリの項目（specs/057-home/research.md R-12）。ホームの棚の
// 読み出しは画面に入った棚の最初の項目を読む。棚ごとに libraryItemsCTE を読むと、絞り込みによらない所在（chosen）とグループの子孫の動画
// （gm）を絞り込みの数だけ求めることになる。ここでは 1 つの with 句にそれらを 1 度だけ置き、
// 絞り込みごとの項目の表（libraryItemsPart）を名前を変えて並べる。項目は libraryItemsCTE と同じ
// である。

import (
	"cmp"
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/syudead/vv/internal/domain"
)

// itemsBatchTables は、まとめて読む絞り込み 1 つの項目の表の名前である。
type itemsBatchTables struct {
	// items は libraryItemsCTE の items にあたる表である。
	items string
}

// itemsBatchBranch は絞り込み specs[index] の項目の表 tables から読む select 文とその引数を返す。
// どの絞り込みの文も同じ列を返し、union all でつなぐ。
type itemsBatchBranch func(index int, tables itemsBatchTables) (string, []any)

// readItemsBatch は、見る人 audience の絞り込み specs の項目を、グループの根（itemGroupsFavorite）
// ごとに、引数が shelvedQueryMaxArgs に収まるまとまりごとに 1 つの問い合わせで読む。まとまりの
// 問い合わせは、chosen と gm を 1 度だけ求め、絞り込みごとに branch の文を並べる。行は scan に渡す。
// どの絞り込みも見る人 audience の範囲で読むこと。
func readItemsBatch(ctx context.Context, tx *sql.Tx, audience domain.Audience, specs []listSpec, branch itemsBatchBranch, scan func(*sql.Rows) error) error {
	for _, favorite := range []bool{false, true} {
		var indexes []int
		for i, spec := range specs {
			if spec.itemGroupsFavorite() == favorite {
				indexes = append(indexes, i)
			}
		}
		for len(indexes) > 0 {
			read, err := readItemsChunk(ctx, tx, audience, specs, indexes, branch, scan)
			if err != nil {
				return err
			}
			indexes = indexes[read:]
		}
	}
	return nil
}

// readItemsChunk は indexes の先頭から、引数が shelvedQueryMaxArgs に収まるだけの絞り込み（少なくとも
// 1 つ）を 1 つの問い合わせで読み、読んだ数を返す。検索式を持つ絞り込みは、libraryItemsCTE と同じく
// 検索式で選ぶ所在の表を自分のために持つ。
func readItemsChunk(ctx context.Context, tx *sql.Tx, audience domain.Audience, specs []listSpec, indexes []int, branch itemsBatchBranch, scan func(*sql.Rows) error) (int, error) {
	scope := libraryScope(audience)
	cte, args := chosenLocationsCTEFor(scope, audience, nil)
	cte += `,
	` + itemGroupsCTE(specs[indexes[0]])
	var selects []string
	var selectArgs []any
	read := 0
	for _, index := range indexes {
		spec := specs[index]
		suffix := `_` + strconv.Itoa(index)
		chosen, chosenSQL := `chosen`, ``
		var chosenArgs []any
		if spec.scope != scope || spec.hasSearch() {
			chosen = `chosen` + suffix
			var selectSQL string
			selectSQL, chosenArgs = chosenLocationsSelect(spec.scope, spec.searchAudience(), spec.searchExprs())
			chosenSQL = `,
	` + chosen + ` as (` + selectSQL + `)`
		}
		part, partArgs := libraryItemsPart(spec, chosen, suffix)
		selectSQL, branchArgs := branch(index, itemsBatchTables{items: `items` + suffix})
		if read > 0 && len(args)+len(selectArgs)+len(chosenArgs)+len(partArgs)+len(branchArgs) > shelvedQueryMaxArgs {
			break
		}
		cte += chosenSQL + `,
	` + part
		args = append(append(args, chosenArgs...), partArgs...)
		selects = append(selects, selectSQL)
		selectArgs = append(selectArgs, branchArgs...)
		read++
	}
	rows, err := tx.QueryContext(ctx, cte+` `+strings.Join(selects, ` union all `), append(args, selectArgs...)...)
	if err != nil {
		return 0, fmt.Errorf("cannot read the library items: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		if err := scan(rows); err != nil {
			return 0, err
		}
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("cannot read the library items: %w", err)
	}
	return read, rows.Close()
}

// firstItemsPage は絞り込み 1 つの最初の項目の鍵と並べ替えの値と、項目の数である。
type firstItemsPage struct {
	rows  []firstItemsRow
	total int
}

// firstItemsRow は最初のページの項目 1 つの鍵と並べ替えの値と、並べ順の位置 rank である。
type firstItemsRow struct {
	rank  int
	key   itemRow
	value any
}

// listFirstItemPages は、絞り込み specs のそれぞれの最初の 1 ページ（カーソルは読まない）を specs の
// 順に返す。各ページは listLibraryPageTx の最初のページと同じ項目と数で、次のページのカーソルは
// 持たない。項目の表はまとめて読む（readItemsBatch）。
func listFirstItemPages(ctx context.Context, tx *sql.Tx, audience domain.Audience, specs []listSpec) ([]domain.LibraryPage, error) {
	orders := make([]listOrder, len(specs))
	for i := range specs {
		if !specs[i].sort.Valid() {
			specs[i].sort = domain.SortAddedDesc
		}
		orders[i] = itemOrder(specs[i].sort)
	}
	pages := make([]firstItemsPage, len(specs))
	err := readItemsBatch(ctx, tx, audience, specs, func(index int, tables itemsBatchTables) (string, []any) {
		spec, order := specs[index], orders[index]
		var args []any
		if order.seeded {
			args = append(args, spec.seed)
		}
		where, watchArgs := itemsWhere(spec)
		args = append(append(args, watchArgs...), normalizeLimit(spec.limit))
		// 行の順は rn が運ぶ（union all は枝の中の順を保つと決まっていない）。
		return `select * from (select ` + strconv.Itoa(index) + `, group_id, id, video_id, path, sort_value,
			count(*) over (), row_number() over (order by ` + order.orderBy() + `)
			from (select group_id, id, video_id, path, ` + order.value + ` as sort_value from ` + tables.items + where + `)
			order by ` + order.orderBy() + ` limit ?)`, args
	}, func(rows *sql.Rows) error {
		var index, total int
		var row firstItemsRow
		if err := rows.Scan(&index, &row.key.groupID, &row.key.id, &row.key.videoID, &row.key.path, &row.value,
			&total, &row.rank); err != nil {
			return fmt.Errorf("cannot read the list: %w", err)
		}
		pages[index].rows = append(pages[index].rows, row)
		pages[index].total = total
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.LibraryPage, len(specs))
	for i, page := range pages {
		slices.SortFunc(page.rows, func(a, b firstItemsRow) int { return cmp.Compare(a.rank, b.rank) })
		keys, values := make([]itemRow, len(page.rows)), make([]any, len(page.rows))
		for j, row := range page.rows {
			keys[j], values[j] = row.key, row.value
		}
		items, _, err := libraryPageItems(ctx, tx, specs[i], orders[i], keys, values, normalizeLimit(specs[i].limit))
		if err != nil {
			return nil, err
		}
		out[i] = domain.LibraryPage{Items: items, Total: page.total, Limit: normalizeLimit(specs[i].limit)}
	}
	return out, nil
}
