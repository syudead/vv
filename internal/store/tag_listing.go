package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
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
	order, ok := tagListOrders[sort]
	if !ok {
		return domain.TagPage{}, fmt.Errorf("unknown tag sort order: %q", sort)
	}
	limit := min(max(query.Limit, 0), domain.MaxTagPageLimit)

	var cursorClause string
	var cursorArgs []any
	if limit > 0 && query.Cursor != "" {
		var err error
		cursorClause, cursorArgs, err = order.cursorCondition(sort, query.Cursor)
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

	// 条件に合う数（total）は窓関数でカーソルを掛ける前の listed 全体について数え、
	// ページの行と同じ問い合わせで返す。listed の本数の集計を 2 回走らせない。
	statement := listed + ` select id, name, tentative, created_at, sort_key, video_count, total from (
		select id, name, tentative, created_at, sort_key, video_count, count(*) over () as total from listed
	)`
	args := listedArgs
	if cursorClause != "" {
		statement += ` where ` + cursorClause
		args = append(args, cursorArgs...)
	}
	statement += ` order by ` + order.orderBy
	if limit > 0 {
		// 続きがあるかを知るために 1 件多く取る。
		statement += ` limit ?`
		args = append(args, limit+1)
	}

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
		page.NextCursor = order.encodeCursor(sort, items[limit-1], keys[limit-1])
	}
	if err := addSynonymsToPage(ctx, tx, items); err != nil {
		return domain.TagPage{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.TagPage{}, fmt.Errorf("cannot finish reading tags: %w", err)
	}
	page.Items = items
	return page, nil
}

// listedTagsCTE は条件に合うタグを listed（id・name・tentative・created_at・
// sort_key・video_count）として返す CTE と、その引数を返す。条件はすべて AND。
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
	where := ``
	if len(conditions) > 0 {
		where = ` where ` + strings.Join(conditions, ` and `)
	}
	return `with tag_counts as (
			select tag_id, count(*) as video_count from (` + taggedVideosSQL("") + `) group by tag_id
		), listed as (
			select t.id as id, tn.name as name, t.tentative as tentative, t.created_at as created_at,
			       tn.sort_key as sort_key, coalesce(c.video_count, 0) as video_count
			  from tags t
			  join tag_names tn on tn.tag_id = t.id and tn.canonical = 1
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
		if err := rows.Scan(&tag.ID, &tag.Name, &tag.Tentative, &createdAt, &sortKey, &tag.VideoCount, &total); err != nil {
			return nil, nil, 0, fmt.Errorf("cannot read tags: %w", err)
		}
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

// tagListOrder はタグの一覧の並び順 1 つ分である（data-model.md §2 の表）。値の
// 向きと名前の向きが違うので、keyset の条件は行値の比較 1 つではなく展開した形で書く。
type tagListOrder struct {
	// value は名前の前に比べる listed の列。名前の順では空。
	value string
	// desc は value を降順に並べる。名前（sort_key, id）はいつも昇順。
	desc    bool
	orderBy string
}

func newTagListOrder(value string, desc bool) tagListOrder {
	order := tagListOrder{value: value, desc: desc, orderBy: `sort_key asc, id asc`}
	if value != "" {
		direction := ` asc`
		if desc {
			direction = ` desc`
		}
		order.orderBy = value + direction + `, ` + order.orderBy
	}
	return order
}

var tagListOrders = map[domain.TagSort]tagListOrder{
	domain.TagSortName:        newTagListOrder("", false),
	domain.TagSortCountDesc:   newTagListOrder("video_count", true),
	domain.TagSortCountAsc:    newTagListOrder("video_count", false),
	domain.TagSortCreatedDesc: newTagListOrder("created_at", true),
	domain.TagSortCreatedAsc:  newTagListOrder("created_at", false),
}

// tagCursorKind はタグの一覧のカーソルの先頭に置き、動画の一覧のカーソルと取り違え
// ないようにする。
const tagCursorKind = "tags"

// tagCursorValue は名前の前に比べる値（本数・作った日の Unix 秒）である。
func (o tagListOrder) tagCursorValue(tag domain.Tag) string {
	switch o.value {
	case "video_count":
		return strconv.Itoa(tag.VideoCount)
	case "created_at":
		return strconv.FormatInt(tag.CreatedAt.Unix(), 10)
	default:
		return ""
	}
}

// encodeCursor は「種類・並び順の名前・値・id・名前の鍵」を包む。名前の鍵は最後に置く。
func (o tagListOrder) encodeCursor(sort domain.TagSort, last domain.Tag, sortKey string) string {
	return packCursor(tagCursorKind, string(sort), o.tagCursorValue(last), strconv.FormatInt(last.ID, 10), sortKey)
}

// cursorCondition はカーソルより後ろだけを取る条件を返す。別の並び順のカーソルや
// 解釈できないものは ErrInvalidCursor。
func (o tagListOrder) cursorCondition(sort domain.TagSort, cursor string) (string, []any, error) {
	parts, err := unpackCursor(cursor, 5)
	if err != nil {
		return "", nil, err
	}
	if parts[0] != tagCursorKind {
		return "", nil, fmt.Errorf("%w: cursor is not for tags", domain.ErrInvalidCursor)
	}
	if parts[1] != string(sort) {
		return "", nil, fmt.Errorf("%w: cursor is for a different sort order", domain.ErrInvalidCursor)
	}
	id, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		return "", nil, fmt.Errorf("%w: cannot parse the id", domain.ErrInvalidCursor)
	}
	sortKey := parts[4]
	nameAfter := `(sort_key, id) > (?, ?)`
	if o.value == "" {
		if parts[2] != "" {
			return "", nil, fmt.Errorf("%w: unexpected sort value", domain.ErrInvalidCursor)
		}
		return nameAfter, []any{sortKey, id}, nil
	}
	value, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return "", nil, fmt.Errorf("%w: cannot parse the sort value", domain.ErrInvalidCursor)
	}
	op := ` > `
	if o.desc {
		op = ` < `
	}
	clause := `(` + o.value + op + `? or (` + o.value + ` = ? and ` + nameAfter + `))`
	return clause, []any{value, value, sortKey, id}, nil
}
