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

// ListTags reads counts, rows and batched synonyms in one deferred snapshot; zero limit returns all matches and ignores the cursor.
func (s *TagStore) ListTags(ctx context.Context, query domain.TagListQuery) (domain.TagPage, error) {
	sort := query.Sort
	if sort == "" {
		sort = domain.TagSortName
	}
	order, ok := adminListOrders[sort]
	if !ok || !sort.Valid() {
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
		`id, name, tentative, created_at, sort_key, video_count, kind_id, kind_name, kind_card_first`, cursorClause, cursorArgs, limit)

	items, keys, total, err := readListedTags(ctx, tx, statement, args)
	if err != nil {
		return domain.TagPage{}, err
	}
	page.Total = total
	if len(items) == 0 && cursorClause != "" {
		// An empty page cannot carry the window count, so count the filtered result separately.
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

// An exact raw name/synonym match can fall outside the naturally sorted first page; retrieve it separately.
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

func listedTagsCTE(query domain.TagListQuery) (string, []any) {
	var conditions []string
	var args []any
	if search := strings.TrimSpace(domain.FoldForMatch(query.Search)); search != "" {
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
			       tk.id as kind_id, tk.name as kind_name, tk.card_first as kind_card_first
			  from tags t
			  join tag_names tn on tn.tag_id = t.id and tn.canonical = 1
			  left join tag_kinds tk on tk.id = t.kind_id
			  left join tag_counts c on c.tag_id = t.id` + where + `
		)`, args
}

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
			&kind.id, &kind.name, &kind.cardFirst, &total); err != nil {
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

// Load all page synonyms in one JSON-keyed query to avoid N+1 reads.
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

// Prefix tag cursors to reject video or credit cursors.
const tagCursorKind = "tags"
