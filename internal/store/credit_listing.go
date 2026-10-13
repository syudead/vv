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

// 人物と団体の一覧と 1 件の読み出し（specs/047-video-work-info/data-model.md「Entities」・「Lists」）。
// 一覧はタグの一覧と同じ並び順とカーソル（admin_list_order.go）で、1 件の読み出しも同じ listed を
// id で絞って読む。

// List は条件に合う kind の実体を並び順に 1 ページ返す。本数 0 の実体も返す。query.Limit が 0 なら
// 全件を返し、カーソルは無視する。件数と行はタグの一覧と同じく、読み取り用の接続の 1 つの取引で読む。
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
	if !ok {
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
	listed, listedArgs := listedCreditsCTE(t, strings.TrimSpace(domain.FoldForMatch(query.Q)), 0)
	statement, args := order.pageStatement(listed, listedArgs, creditListedColumns, cursorClause, cursorArgs, limit)
	items, keys, total, err := readListedCredits(ctx, tx, t, statement, args)
	if err != nil {
		return domain.CreditPage{}, err
	}
	page.Total = total
	if len(items) == 0 && cursorClause != "" {
		// カーソルより後ろに行が無いときは、ページの行から数を取れないので、条件に合う数だけを数える。
		if err := tx.QueryRowContext(ctx, listed+` select count(*) from listed`, listedArgs...).Scan(&page.Total); err != nil {
			return domain.CreditPage{}, fmt.Errorf("cannot count %s entities: %w", kind, err)
		}
	}
	if limit > 0 && len(items) > limit {
		items = items[:limit]
		last := items[limit-1]
		page.NextCursor = order.encodeCursor(t.cursorKind, sort,
			adminListRow{id: last.ID, videoCount: last.VideoCount, createdAt: last.CreatedAt, sortKey: keys[limit-1]})
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

// creditByID は取引 tx の中で実体 id を読む。無ければ domain.ErrNotFound。
func creditByID(ctx context.Context, tx *sql.Tx, t creditTables, id int64) (domain.Credit, error) {
	listed, args := listedCreditsCTE(t, "", id)
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

// creditListedColumns は listed から読む列である。readListedCredits が最後の total と合わせて読む。
const creditListedColumns = `id, name, created_at, sort_key, video_count, parent_id`

// listedCreditsCTE は条件に合う実体を listed（id・name・created_at・sort_key・video_count・
// parent_id）として返す CTE と、その引数を返す。search が空でなければ名前か別名の照合形に部分一致
// する実体だけ、onlyID が正ならその実体だけにする。
//
// video_count は表示中の動画（所有者として見える代表）の利用者データの鍵のうち、どれかの役割の
// 結び付きの行がこの実体を指すものの数である（data-model.md「Lists」の最後の段落）。束ねの代表だけが
// 表示中なので、1 つの鍵は 1 本の動画として数える。
func listedCreditsCTE(t creditTables, search string, onlyID int64) (string, []any) {
	var countFilter, where string
	var countArgs, whereArgs []any
	var conditions []string
	if onlyID > 0 {
		countFilter = ` and l.{id} = ?`
		countArgs = []any{onlyID}
		conditions = append(conditions, `e.id = ?`)
		whereArgs = append(whereArgs, onlyID)
	}
	if search != "" {
		conditions = append(conditions,
			`exists (select 1 from {names} sn where sn.{id} = e.id and instr(sn.search_key, ?) > 0)`)
		whereArgs = append(whereArgs, search)
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
			select e.id as id, n.name as name, e.created_at as created_at, n.sort_key as sort_key,
			       coalesce(c.video_count, 0) as video_count, ` + t.parentColumn() + ` as parent_id
			  from {entity} e
			  join {names} n on n.{id} = e.id and n.canonical = 1
			  left join credit_counts c on c.credit_id = e.id` + where + `
		)`
	return t.q(statement), append(countArgs, whereArgs...)
}

// readListedCredits は listed の行を読み、行ごとの名前の鍵（カーソルに包む）と、行が持つ条件に合う
// 数を返す。行が無ければ数は 0。親は id だけを入れ、名前は addCreditDetails が足す。
func readListedCredits(ctx context.Context, tx *sql.Tx, t creditTables, statement string, args []any) ([]domain.Credit, []string, int, error) {
	rows, err := tx.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("cannot read %s entities: %w", t.kind, err)
	}
	defer func() { _ = rows.Close() }()

	items := []domain.Credit{}
	var keys []string
	total := 0
	for rows.Next() {
		credit := domain.Credit{Kind: t.kind, Aliases: []domain.Alias{}, ExternalIDs: []domain.ExternalID{}}
		var createdAt int64
		var sortKey string
		var parent sql.NullInt64
		if err := rows.Scan(&credit.ID, &credit.Name, &createdAt, &sortKey, &credit.VideoCount, &parent, &total); err != nil {
			return nil, nil, 0, fmt.Errorf("cannot read %s entities: %w", t.kind, err)
		}
		credit.CreatedAt = time.Unix(createdAt, 0)
		if parent.Valid {
			credit.Parent = &domain.CreditRef{ID: parent.Int64}
		}
		items = append(items, credit)
		keys = append(keys, sortKey)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, 0, fmt.Errorf("cannot read %s entities: %w", t.kind, err)
	}
	return items, keys, total, nil
}

// addCreditDetails は items の各実体に別名（名前の自然順）・外部 ID（(source, value) の順）・親の
// 名前を足す。ページの実体の id を json_each に 1 つの引数で渡し、それぞれ 1 回で読む（N+1 にしない）。
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
	for i := range items {
		domain.SortAliases(items[i].Aliases)
	}
	return addCreditParentNames(ctx, tx, t, items, parentIDs)
}

// addCreditParentNames は親を持つ実体に親の今の名前を足す。
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

// eachCreditRow は query（id と 2 つの文字列の列を返す）の行ごとに visit を呼ぶ。
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
