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

// Credit lists share tag ordering and cursors; detail reads use the same listed relation.

// List includes zero-video entities and reads counts with rows in one snapshot; limit zero ignores cursors and returns all rows.
func (s *CreditStore) List(ctx context.Context, kind domain.CreditKind, query domain.CreditQuery) (domain.CreditPage, error) {
	t, err := creditTablesFor(kind)
	if err != nil {
		return domain.CreditPage{}, err
	}
	sort := query.Sort
	if sort == "" {
		sort = domain.TagSortName
	}
	order, ok := adminListOrders[sort]
	if !ok || !domain.ValidCreditSort(kind, sort) {
		return domain.CreditPage{}, fmt.Errorf("unknown %s sort order: %q", kind, sort)
	}
	limit := min(max(query.Limit, 0), domain.MaxTagPageLimit)
	var cursorClause string
	var cursorArgs []any
	if limit > 0 && query.Cursor != "" {
		cursorClause, cursorArgs, err = order.cursorCondition(t.cursorKind, sort, query.Cursor)
		if err != nil {
			return domain.CreditPage{}, err
		}
	}

	tx, err := s.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.CreditPage{}, fmt.Errorf("cannot start reading %s entities: %w", kind, err)
	}
	defer func() { _ = tx.Rollback() }()

	page := domain.CreditPage{Items: []domain.Credit{}}
	if err := tx.QueryRowContext(ctx, t.q(`select count(*) from {entity}`)).Scan(&page.TotalAll); err != nil {
		return domain.CreditPage{}, fmt.Errorf("cannot count %s entities: %w", kind, err)
	}
	if (query.FavoriteOnly || query.Provisional != nil) && t.kind != domain.CreditPerson {
		return domain.CreditPage{}, fmt.Errorf("the favorite and provisional filters apply to people only, not %s", kind)
	}
	listed, listedArgs := listedCreditsCTE(t, creditFilter{
		search: strings.TrimSpace(domain.FoldForMatch(query.Q)), favoriteOnly: query.FavoriteOnly,
		provisional: query.Provisional,
	})
	statement, args := order.pageStatement(listed, listedArgs, creditListedColumns, cursorClause, cursorArgs, limit)
	items, rows, total, err := readListedCredits(ctx, tx, t, statement, args)
	if err != nil {
		return domain.CreditPage{}, err
	}
	page.Total = total
	if len(items) == 0 && cursorClause != "" {
		// An empty page cannot carry a window count, so count the full filtered result separately.
		if err := tx.QueryRowContext(ctx, listed+` select count(*) from listed`, listedArgs...).Scan(&page.Total); err != nil {
			return domain.CreditPage{}, fmt.Errorf("cannot count %s entities: %w", kind, err)
		}
	}
	if limit > 0 && len(items) > limit {
		items = items[:limit]
		page.NextCursor = order.encodeCursor(t.cursorKind, sort, rows[limit-1])
	}
	if err := addCreditDetails(ctx, tx, t, items); err != nil {
		return domain.CreditPage{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.CreditPage{}, fmt.Errorf("cannot finish reading %s entities: %w", kind, err)
	}
	page.Items = items
	return page, nil
}

// Reject nonpositive IDs before the CTE interprets them as no ID filter.
func creditByID(ctx context.Context, tx *sql.Tx, t creditTables, id int64) (domain.Credit, error) {
	if id <= 0 {
		return domain.Credit{}, domain.ErrNotFound
	}
	listed, args := listedCreditsCTE(t, creditFilter{onlyID: id})
	items, _, _, err := readListedCredits(ctx, tx, t,
		listed+` select `+creditListedColumns+`, 1 from listed`, args)
	if err != nil {
		return domain.Credit{}, err
	}
	if len(items) == 0 {
		return domain.Credit{}, domain.ErrNotFound
	}
	if err := addCreditDetails(ctx, tx, t, items); err != nil {
		return domain.Credit{}, err
	}
	return items[0], nil
}

const creditListedColumns = `id, name, provisional, created_at, sort_key, video_count, parent_id, birth_key`

// creditFilter narrows the listed relation; a zero onlyID means every entity.
type creditFilter struct {
	search       string
	onlyID       int64
	favoriteOnly bool
	// provisional, when set, keeps only people with (false) or without (true) a canonical name.
	provisional *bool
}

// A prefix puts people without a canonical name after every named one in any name order, ties by ID.
const creditSortKey = `case when n.{id} is null then '1' else '0' || n.sort_key end`

// Count visible representatives by user-data key so each bundle contributes one video.
func listedCreditsCTE(t creditTables, filter creditFilter) (string, []any) {
	var countFilter, where string
	var countArgs, whereArgs []any
	var conditions []string
	if filter.onlyID > 0 {
		countFilter = ` and l.{id} = ?`
		countArgs = []any{filter.onlyID}
		conditions = append(conditions, `e.id = ?`)
		whereArgs = append(whereArgs, filter.onlyID)
	}
	if filter.search != "" {
		conditions = append(conditions,
			`exists (select 1 from {names} sn where sn.{id} = e.id and instr(sn.search_key, ?) > 0)`)
		whereArgs = append(whereArgs, filter.search)
	}
	if filter.favoriteOnly {
		conditions = append(conditions, `exists (select 1 from person_favorites pf where pf.person_id = e.id)`)
	}
	if filter.provisional != nil {
		conditions = append(conditions, `(n.{id} is null) = ?`)
		whereArgs = append(whereArgs, boolToInt(*filter.provisional))
	}
	if len(conditions) > 0 {
		where = ` where ` + strings.Join(conditions, ` and `)
	}
	statement := `with credit_counts as (
			select l.{id} as credit_id, count(distinct l.content_key) as video_count
			  from videos v
			  join {links} l on l.content_key = ` + userKeyExpr("v") + `
			 where v.content_key <> '' and ` + registeredVideoCondition("v") + ` and ` +
		shownVideoCondition("v", domain.AudienceOwner) + countFilter + `
			 group by l.{id}
		), listed as (
			select e.id as id, coalesce(n.name, '') as name, n.{id} is null as provisional,
			       e.created_at as created_at, ` + creditSortKey + ` as sort_key,
			       coalesce(c.video_count, 0) as video_count, ` + t.parentColumn() + ` as parent_id,
			       ` + t.birthKeyColumn() + ` as birth_key
			  from {entity} e
			  left join {names} n on n.{id} = e.id and n.canonical = 1
			  left join credit_counts c on c.credit_id = e.id` + where + `
		)`
	return t.q(statement), append(countArgs, whereArgs...)
}

// Parent names are populated later by addCreditDetails; the rows carry each item's cursor values.
func readListedCredits(ctx context.Context, tx *sql.Tx, t creditTables, statement string, args []any) ([]domain.Credit, []adminListRow, int, error) {
	rows, err := tx.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("cannot read %s entities: %w", t.kind, err)
	}
	defer func() { _ = rows.Close() }()

	items := []domain.Credit{}
	var listed []adminListRow
	total := 0
	for rows.Next() {
		credit := domain.Credit{Kind: t.kind, Aliases: []domain.Alias{}, ExternalIDs: []domain.ExternalID{}}
		var createdAt int64
		var row adminListRow
		var parent sql.NullInt64
		if err := rows.Scan(&credit.ID, &credit.Name, &credit.Provisional, &createdAt, &row.sortKey, &credit.VideoCount, &parent,
			&row.birthKey, &total); err != nil {
			return nil, nil, 0, fmt.Errorf("cannot read %s entities: %w", t.kind, err)
		}
		credit.CreatedAt = time.Unix(createdAt, 0)
		if parent.Valid {
			credit.Parent = &domain.CreditRef{ID: parent.Int64}
		}
		row.id, row.videoCount, row.createdAt = credit.ID, credit.VideoCount, credit.CreatedAt
		items = append(items, credit)
		listed = append(listed, row)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, 0, fmt.Errorf("cannot read %s entities: %w", t.kind, err)
	}
	return items, listed, total, nil
}

// Batch aliases, external IDs, profiles and parent names for the page to avoid N+1 queries.
func addCreditDetails(ctx context.Context, tx *sql.Tx, t creditTables, items []domain.Credit) error {
	if len(items) == 0 {
		return nil
	}
	index := make(map[int64]int, len(items))
	ids := make([]int64, len(items))
	var parentIDs []int64
	for i, credit := range items {
		index[credit.ID] = i
		ids[i] = credit.ID
		if credit.Parent != nil {
			parentIDs = append(parentIDs, credit.Parent.ID)
		}
	}
	encoded, err := json.Marshal(ids)
	if err != nil {
		return fmt.Errorf("cannot encode %s ids: %w", t.kind, err)
	}
	err = eachCreditRow(ctx, tx, t.q(`select {id}, name, coalesce(language, '') from {names}
		 where canonical = 0 and {id} in (select value from json_each(?))`), string(encoded),
		func(id int64, a, b string) {
			items[index[id]].Aliases = append(items[index[id]].Aliases, domain.Alias{Name: a, Language: b})
		})
	if err != nil {
		return fmt.Errorf("cannot read the aliases of %s entities: %w", t.kind, err)
	}
	err = eachCreditRow(ctx, tx, t.q(`select {id}, source, value from {ext}
		 where {id} in (select value from json_each(?)) order by source, value`), string(encoded),
		func(id int64, a, b string) {
			items[index[id]].ExternalIDs = append(items[index[id]].ExternalIDs, domain.ExternalID{Source: a, Value: b})
		})
	if err != nil {
		return fmt.Errorf("cannot read the external IDs of %s entities: %w", t.kind, err)
	}
	if t.kind == domain.CreditPerson {
		if err := addPersonProfiles(ctx, tx, items, index, string(encoded)); err != nil {
			return err
		}
	}
	for i := range items {
		domain.SortAliases(items[i].Aliases)
	}
	return addCreditParentNames(ctx, tx, t, items, parentIDs)
}

func addCreditParentNames(ctx context.Context, tx *sql.Tx, t creditTables, items []domain.Credit, parentIDs []int64) error {
	if len(parentIDs) == 0 {
		return nil
	}
	encoded, err := json.Marshal(parentIDs)
	if err != nil {
		return fmt.Errorf("cannot encode %s ids: %w", t.kind, err)
	}
	names := make(map[int64]string, len(parentIDs))
	err = eachCreditRow(ctx, tx, t.q(`select {id}, name, '' from {names}
		 where canonical = 1 and {id} in (select value from json_each(?))`), string(encoded),
		func(id int64, name, _ string) { names[id] = name })
	if err != nil {
		return fmt.Errorf("cannot read the parents of %s entities: %w", t.kind, err)
	}
	for i := range items {
		if items[i].Parent != nil {
			items[i].Parent.Name = names[items[i].Parent.ID]
		}
	}
	return nil
}

func eachCreditRow(ctx context.Context, tx *sql.Tx, query string, arg any, visit func(id int64, a, b string)) error {
	rows, err := tx.QueryContext(ctx, query, arg)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id int64
		var a, b string
		if err := rows.Scan(&id, &a, &b); err != nil {
			return err
		}
		visit(id, a, b)
	}
	return rows.Err()
}
