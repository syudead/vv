package store

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"

	"github.com/syudead/vv/internal/domain"
)

func hasCollectionConditions(q domain.VideoQuery) bool {
	return q.Query != "" || q.FavoriteOnly || q.Sort != ""
}

type groupItemEntry struct {
	id, position     int64
	videoID, childID sql.NullInt64
}

// Match descendant titles before paging; the favorite filter applies to each displayed item's own value.
func groupItemCriteriaSQL(audience domain.Audience, id int64, query domain.VideoQuery) (string, []any) {
	spec := libraryListSpec(audience, query, nil)
	spec.scope.withinGroup = id
	cte, args := chosenLocationsCTE(spec.scope, spec.expr)
	conditions, matchArgs := matchConditions(spec)
	where := ""
	for _, condition := range conditions {
		where += " and " + condition
	}
	args = append(args, matchArgs...)
	a := strconv.Itoa(audienceValue(audience))
	cte += `, matched as (select videos.id from chosen join videos on videos.id = chosen.video_id where 1=1` + where + `),
 entries as (select gi.id, gi.position, gi.child_id,
  case when gi.content_key is not null then ` + shownVideoForKeyExpr("gi.content_key", audience) + ` end as video_id
  from group_items gi where gi.group_id = ?),
 items as (
  select e.*, v.added_at, ` + locationMTimeExpr("loc") + ` as mtime, ` + locationCreatedAtExpr("loc") + ` as created_at, ` +
		locationTitleKeyExpr("loc") + ` as title_key,
   v.duration_ms, loc.size_bytes, p.updated_at as played_at, fav.favorited_at
  from entries e join videos v on v.id = e.video_id
  join chosen on chosen.video_id = v.id` + chosenLocationJoin("chosen.path", "v") + `
  left join playback_progress p on p.content_key = ` + userKeyExpr("v") + ` and v.content_key <> ''
  left join video_favorites fav on fav.content_key = ` + userKeyExpr("v") + ` and v.content_key <> ''
  where e.video_id in (select id from matched)
  union all
  select e.*, gv.added_at, gv.mtime, gv.created_at, g.title_key, gv.duration_ms, gv.size_bytes, gv.played_at,
   ` + groupFavoritedAtExpr("g") + `
  from entries e join groups g on g.id = e.child_id
  join group_values gv on gv.group_id = g.id and gv.audience = ` + a + `
  where exists (select 1 from group_members gm where gm.group_id = e.child_id and gm.audience = ` + a + ` and gm.video_id in (select id from matched)))`
	return cte, append(args, id)
}

func groupPageOrder(query domain.VideoQuery) (listOrder, listSpec) {
	spec := libraryListSpec(domain.AudienceOwner, query, nil)
	if query.Sort.Valid() {
		return itemOrder(query.Sort), spec
	}
	spec.sort = domain.VideoSort(groupItemsCursorKind)
	return listOrder{value: "position"}, spec
}

func groupItemPageStatement(audience domain.Audience, id int64, cursor string, limit int, query domain.VideoQuery) (string, []any, listOrder, listSpec, error) {
	order, spec := groupPageOrder(query)
	clause, cursorArgs := "", []any(nil)
	if cursor != "" {
		var err error
		clause, cursorArgs, err = order.cursorCondition(spec, cursor)
		if err != nil {
			return "", nil, order, spec, err
		}
	}
	cte, args := groupItemCriteriaSQL(audience, id, query)
	where := ""
	if query.FavoriteOnly {
		where = " where favorited_at is not null"
	}
	if order.seeded {
		args = append(args, query.Seed)
	}
	statement := cte + `, filtered as (select items.*, ` + order.value + ` as sort_value from items` + where + `)
  select id, position, video_id, child_id, sort_value from filtered`
	if clause != "" {
		statement += " where " + clause
		args = append(args, cursorArgs...)
	}
	statement += " order by " + order.orderBy() + " limit ?"
	return statement, append(args, limit+1), order, spec, nil
}

func readGroupItemKeys(ctx context.Context, tx *sql.Tx, audience domain.Audience, id int64, cursor string, limit int, query domain.VideoQuery) ([]groupItemEntry, string, error) {
	if limit <= 0 {
		limit = domain.GroupItemsDefaultLimit
	}
	limit = min(limit, domain.GroupItemsMaxLimit)
	statement, args, order, spec, err := groupItemPageStatement(audience, id, cursor, limit, query)
	if err != nil {
		return nil, "", err
	}
	rows, err := tx.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, "", fmt.Errorf("cannot read the group items: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var entries []groupItemEntry
	var values []any
	for rows.Next() {
		var e groupItemEntry
		var value any
		if err := rows.Scan(&e.id, &e.position, &e.videoID, &e.childID, &value); err != nil {
			return nil, "", err
		}
		entries = append(entries, e)
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	next := ""
	if len(entries) > limit {
		entries = entries[:limit]
		next, err = order.encodeCursor(spec, values[limit-1], entries[limit-1].id)
	}
	return entries, next, err
}
