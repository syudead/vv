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

// ListTags は条件に合うタグを並び順に 1 ページ返す。本数 0 のタグも返す
// （specs/036-tag-admin-scale/data-model.md §2）。本数は video_tags を
// videos.content_key と結び、いまライブラリにある動画だけを数える
// （taggedVideosSQL の集計）。query.Limit が 0 なら条件と並び順だけを掛けて
// 全件を返し、カーソルは無視する（外部連携 API と候補の全件）。
//
// タグ・件数・シノニムは ListVideos と同じく読み取り用の接続（s.read、deferred）の
// 1 つの読み取りの取引で読むので、Total とページの行は同じ時点のものになり、書き込みの
// 枠を取らず走査やタグの書き込みを待たせない。本数の集計（taggedVideosSQL）は Total とページの行を 1 つの問い合わせで読んで
// 1 回だけ掛ける（research.md R-1）。シノニムはページのタグの分だけを 1 回で読む
// （N+1 にしない）。
func (s *TagStore) ListTags(ctx context.Context, query domain.TagListQuery) (domain.TagPage, error) {
	sort := query.Sort
	if sort == "" {
		sort = domain.TagSortName
	}
	order, ok := adminListOrders[sort]
	if !ok {
		return domain.TagPage{}, fmt.Errorf("unknown tag sort order: %q", sort)
	}
	limit := min(max(query.Limit, 0), domain.MaxTagPageLimit)

	var cursorClause string
	var cursorArgs []any
	if limit > 0 && query.Cursor != "" {
		var err error
		cursorClause, cursorArgs, err = order.cursorCondition(tagCursorKind, sort, query.Cursor)
		if err != nil {
			return domain.TagPage{}, err
		}
	}

	tx, err := s.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.TagPage{}, fmt.Errorf("cannot start reading tags: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	listed, listedArgs := listedTagsCTE(query)
	page := domain.TagPage{Items: []domain.Tag{}}
	if err := tx.QueryRowContext(ctx, `select count(*) from tags`).Scan(&page.TotalAll); err != nil {
		return domain.TagPage{}, fmt.Errorf("cannot count tags: %w", err)
	}

	statement, args := order.pageStatement(listed, listedArgs,
		`id, name, tentative, created_at, sort_key, video_count, kind_id, kind_name`, cursorClause, cursorArgs, limit)

	items, keys, total, err := readListedTags(ctx, tx, statement, args)
	if err != nil {
		return domain.TagPage{}, err
	}
	page.Total = total
	if len(items) == 0 && cursorClause != "" {
		// カーソルより後ろに行が無いとき（読むあいだに後ろのタグが消えた）は、ページの
		// 行から数を取れないので、条件に合う数だけを数える。
		if err := tx.QueryRowContext(ctx, listed+` select count(*) from listed`, listedArgs...).Scan(&page.Total); err != nil {
			return domain.TagPage{}, fmt.Errorf("cannot count tags: %w", err)
		}
	}
	if limit > 0 && len(items) > limit {
		items = items[:limit]
		last := items[limit-1]
		page.NextCursor = order.encodeCursor(tagCursorKind, sort,
			adminListRow{id: last.ID, videoCount: last.VideoCount, createdAt: last.CreatedAt, sortKey: keys[limit-1]})
	}
	if err := addSynonymsToPage(ctx, tx, items); err != nil {
		return domain.TagPage{}, err
	}
	if limit > 0 && query.Cursor == "" {
		if page.Exact, err = exactListedTag(ctx, tx, query); err != nil {
			return domain.TagPage{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return domain.TagPage{}, fmt.Errorf("cannot finish reading tags: %w", err)
	}
	page.Items = items
	return page, nil
}

// exactListedTag は検索語（前後の空白を落とした生の綴り）と元の名前かシノニムの綴りが
// 完全に一致し、絞り込みにも合うタグを返す。無ければ nil。ページの行は名前の自然順の
// 部分一致なので、完全に一致するタグが 1 ページ目に入るとは限らない（統合の窓の
// 統合先、specs/036-tag-admin-scale/contracts/screen-api.md §5）。
func exactListedTag(ctx context.Context, tx *sql.Tx, query domain.TagListQuery) (*domain.Tag, error) {
	name := strings.TrimSpace(query.Search)
	if name == "" {
		return nil, nil
	}
	lookup, found, err := lookupTagName(ctx, tx, name)
	if err != nil || !found {
		return nil, err
	}
	if query.TentativeOnly && !lookup.tentative {
		return nil, nil
	}
	tag, err := tagByID(ctx, tx, lookup.tagID)
	if err != nil {
		return nil, err
	}
	if query.UnusedOnly && tag.VideoCount != 0 || !query.Kind.Matches(tag.Kind) {
		return nil, nil
	}
	return &tag, nil
}

// listedTagsCTE は条件に合うタグを listed（id・name・tentative・created_at・
// sort_key・video_count・kind_id・kind_name）として返す CTE と、その引数を返す。条件はすべて AND。
func listedTagsCTE(query domain.TagListQuery) (string, []any) {
	var conditions []string
	var args []any
	if search := strings.TrimSpace(domain.FoldForMatch(query.Search)); search != "" {
		// 元の名前かシノニムの照合形に部分一致する（014 の data-model.md §7 と同じ照合形）。
		conditions = append(conditions,
			`exists (select 1 from tag_names sn where sn.tag_id = t.id and instr(sn.search_key, ?) > 0)`)
		args = append(args, search)
	}
	if query.TentativeOnly {
		conditions = append(conditions, `t.tentative = 1`)
	}
	if query.UnusedOnly {
		conditions = append(conditions, `coalesce(c.video_count, 0) = 0`)
	}
	switch {
	case query.Kind.None:
		conditions = append(conditions, `t.kind_id is null`)
	case query.Kind.ID != 0:
		conditions = append(conditions, `t.kind_id = ?`)
		args = append(args, query.Kind.ID)
	}
	where := ``
	if len(conditions) > 0 {
		where = ` where ` + strings.Join(conditions, ` and `)
	}
	return `with tag_counts as (
			select tag_id, count(*) as video_count from (` + taggedVideosSQL("") + `) group by tag_id
		), listed as (
			select t.id as id, tn.name as name, t.tentative as tentative, t.created_at as created_at,
			       tn.sort_key as sort_key, coalesce(c.video_count, 0) as video_count,
			       tk.id as kind_id, tk.name as kind_name
			  from tags t
			  join tag_names tn on tn.tag_id = t.id and tn.canonical = 1
			  left join tag_kinds tk on tk.id = t.kind_id
			  left join tag_counts c on c.tag_id = t.id` + where + `
		)`, args
}

// readListedTags は listed の行を読み、行ごとの名前の鍵（カーソルに包む）と、行が
// 持つ条件に合う数を返す。行が無ければ数は 0。
func readListedTags(ctx context.Context, tx *sql.Tx, statement string, args []any) ([]domain.Tag, []string, int, error) {
	rows, err := tx.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("cannot read tags: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := []domain.Tag{}
	var keys []string
	total := 0
	for rows.Next() {
		var tag domain.Tag
		var createdAt int64
		var sortKey string
		var kind scannedTagKind
		if err := rows.Scan(&tag.ID, &tag.Name, &tag.Tentative, &createdAt, &sortKey, &tag.VideoCount,
			&kind.id, &kind.name, &total); err != nil {
			return nil, nil, 0, fmt.Errorf("cannot read tags: %w", err)
		}
		tag.Kind = kind.ref()
		tag.CreatedAt = time.Unix(createdAt, 0)
		tag.Synonyms = []string{}
		items = append(items, tag)
		keys = append(keys, sortKey)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, 0, fmt.Errorf("cannot read tags: %w", err)
	}
	return items, keys, total, nil
}

// addSynonymsToPage は items の各タグにシノニムを名前の自然順で足す。ページの
// タグの id を json_each に 1 つの引数で渡し、1 回で読む。
func addSynonymsToPage(ctx context.Context, tx *sql.Tx, items []domain.Tag) error {
	if len(items) == 0 {
		return nil
	}
	index := make(map[int64]int, len(items))
	ids := make([]int64, len(items))
	for i, tag := range items {
		index[tag.ID] = i
		ids[i] = tag.ID
	}
	encoded, err := json.Marshal(ids)
	if err != nil {
		return fmt.Errorf("cannot encode tag ids: %w", err)
	}
	rows, err := tx.QueryContext(ctx, `
		select tag_id, name from tag_names
		 where canonical = 0 and tag_id in (select value from json_each(?))`, string(encoded))
	if err != nil {
		return fmt.Errorf("cannot read synonyms: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var tagID int64
		var name string
		if err := rows.Scan(&tagID, &name); err != nil {
			return fmt.Errorf("cannot read synonyms: %w", err)
		}
		if i, ok := index[tagID]; ok {
			items[i].Synonyms = append(items[i].Synonyms, name)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("cannot read synonyms: %w", err)
	}
	for i := range items {
		domain.SortTagNames(items[i].Synonyms)
	}
	return nil
}

// tagCursorKind はタグの一覧のカーソルの先頭に置き、動画の一覧や人物・団体の一覧のカーソルと
// 取り違えないようにする。
const tagCursorKind = "tags"
