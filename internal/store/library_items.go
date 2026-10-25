package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/syudead/vv/internal/domain"
)

// Match videos first, collapse wholly matched groups, then filter and page items; group values always include all visible descendants.

// Apply member filters at every stacked spec level before forming items.
func matchConditions(spec listSpec) ([]string, []any) {
	var conditions []string
	var args []any
	if spec.withinVideos != "" {
		conditions = append(conditions, `videos.id in (select value from json_each(?))`)
		args = append(args, spec.withinVideos)
	}
	if spec.matchNothing {
		conditions = append(conditions, `0`)
	}
	userKey := spec.videoUserKey()
	for _, level := range spec.levels() {
		tags, tagArgs := tagConditions(level, userKey)
		works, workArgs := workInfoConditions(level, userKey)
		conditions = append(append(conditions, tags...), works...)
		args = append(append(args, tagArgs...), workArgs...)
	}
	return conditions, args
}

// Use the supplied user-key column when available, otherwise derive it.
func (s listSpec) videoUserKey() string {
	if s.userKeyColumn != "" {
		return s.userKeyColumn
	}
	return userKeyExpr("videos")
}

func (s listSpec) levels() []listSpec {
	return append([]listSpec{s}, s.and...)
}

// Favorite-only filters apply if any stacked level requests them.
func (s listSpec) anyLevel(test func(listSpec) bool) bool {
	return slices.ContainsFunc(s.levels(), test)
}

func (s listSpec) searchExprs() []domain.SearchExpr {
	exprs := make([]domain.SearchExpr, 0, 1+len(s.and))
	for _, level := range s.levels() {
		exprs = append(exprs, level.expr)
	}
	return exprs
}

// Match credits on the user-data key in any role unless personRole is set; missing entity IDs match nothing.
func workInfoConditions(spec listSpec, userKey string) ([]string, []any) {
	var conditions []string
	var args []any
	// Unidentified content has no user data.
	link := func(table, alias, match, extra string) string {
		return `(videos.content_key <> '' and exists (select 1 from ` + table + ` ` + alias + ` where ` + alias +
			`.content_key = ` + userKey + ` and ` + match + extra + `))`
	}
	person := func(match string, value any) {
		role := ""
		args = append(args, value)
		if spec.personRole != "" {
			role = ` and vp.role = ?`
			args = append(args, string(spec.personRole))
		}
		conditions = append(conditions, link("video_work_people", "vp", match, role))
	}
	if spec.personID != 0 {
		person(`vp.person_id = ?`, spec.personID)
	}
	if spec.personName != "" {
		person(creditNameCondition("vp.person_id", "people_names", "person_id"), spec.personName)
	}
	if spec.organizationID != 0 {
		conditions = append(conditions, link("video_work_organizations", "vo", `vo.organization_id = ?`, ""))
		args = append(args, spec.organizationID)
	}
	if spec.organizationName != "" {
		conditions = append(conditions, link("video_work_organizations", "vo",
			creditNameCondition("vo.organization_id", "organization_names", "organization_id"), ""))
		args = append(args, spec.organizationName)
	}
	return conditions, args
}

// Tags match manual or folder-derived attachments through the video's user-data key.
func tagConditions(spec listSpec, userKey string) ([]string, []any) {
	var conditions []string
	var args []any
	for _, tagID := range spec.tagIDs {
		conditions = append(conditions, videoHasTagCondition("videos", userKey))
		args = append(args, tagID, tagID)
	}
	for _, kindID := range []int64{spec.tagKindID, spec.tagKindNameID} {
		if kindID != 0 {
			conditions = append(conditions, videoHasTagOfKindCondition("videos", userKey))
			args = append(args, kindID, kindID)
		}
	}
	return conditions, args
}

// Join the representative location once instead of repeating the same lookup for every selected column.
func representativeLocationJoin(alias string, audience domain.Audience) string {
	return ` left join video_locations rl on rl.id = (select l.id from video_locations l where l.video_id = ` +
		alias + `.video_id and ` + visibleLocationCondition("l", audience) + ` order by l.path limit 1)`
}

const topGroupsSelect = `select g.id from groups g where not exists (select 1 from group_items c where c.child_id = g.id)`

const favoriteGroupsSelect = `select group_id as id from group_favorites`

// Keep watch-state classification equivalent to domain.ClassifyWatch.
const videoWatchStateExpr = `case when p.completed = 1 then 'watched'
				when p.content_key is null or p.position_ms = 0 then 'unwatched'
				else 'inProgress' end`

// Collapse only wholly matched groups; give videos even item IDs and groups odd IDs to avoid keyset collisions.
func libraryItemsCTE(spec listSpec) (string, []any) {
	audience := spec.scope.audience
	searchAudience := audience
	if spec.ownerSearch {
		searchAudience = domain.AudienceOwner
	}
	cte, args := chosenLocationsCTEFor(spec.scope, searchAudience, spec.searchExprs())
	conditions, matchArgs := matchConditions(spec)
	args = append(args, matchArgs...)
	where := ""
	if len(conditions) > 0 {
		where = ` where ` + strings.Join(conditions, " and ")
	}
	// Unidentified content has no playback position.
	progressJoin := func(videoAlias string) string {
		return ` left join playback_progress p on p.content_key = ` + userKeyExpr(videoAlias) + ` and ` +
			videoAlias + `.content_key <> ''`
	}
	folderItems, folderArgs := smartFolderItemsSQL(spec.smartFolders)
	if spec.storedCollections {
		folderItems, folderArgs = storedSmartFolderItemsSQL(spec), nil
	}
	roots := topGroupsSelect
	videoFavorite := ""
	if spec.anyLevel(func(level listSpec) bool { return level.favoriteOnly }) {
		roots = favoriteGroupsSelect
		videoFavorite = ` and fav.content_key is not null`
	}
	groupSQL := libraryGroupSQL(spec, roots, progressJoin("v"))
	whole := `select hits.group_id from hits where hits.n = (` + groupSQL.count + `)`
	// Stored group values count every member, so a kind that drops some members must count the matched ones.
	if spec.storedCollections && len(conditions) == 0 && spec.expr.Empty() && spec.scope.choosesAllKinds() {
		whole = storedWholeGroupsSQL(spec, roots)
	}
	// Partial matches remain individual videos even when another whole group contains them; materialize whole and carded sets to avoid quadratic correlated scans.
	cte += `,
	matched as (
		select videos.id as video_id, chosen.path as path
		from chosen join videos on videos.id = chosen.video_id` + where + `),
	` + groupSQL.members + `,
	hits as (
		select gm.group_id, count(*) as n from matched
		join gm on gm.video_id = matched.video_id
		group by gm.group_id),
	whole as materialized (
		` + whole + `),
	carded as materialized (
		select video_id from gm where group_id in (select group_id from whole)
		except
		select video_id from gm where group_id not in (select group_id from whole)),
` + groupSQL.values + `,
	items as (
		select null as group_id, videos.id * 2 as id, videos.id as video_id, matched.path as path,
			videos.added_at as added_at, ` + locationMTimeExpr("loc") + ` as mtime, ` + locationCreatedAtExpr("loc") + ` as created_at,
			` + locationTitleKeyExpr("loc") + ` as title_key,
			videos.duration_ms as duration_ms, loc.size_bytes as size_bytes, p.updated_at as played_at,
			fav.favorited_at as favorited_at
		from matched join videos on videos.id = matched.video_id` +
		chosenLocationJoin("matched.path", "videos") + progressJoin("videos") + `
		left join video_favorites fav on fav.content_key = ` + userKeyExpr("videos") + ` and videos.content_key <> ''
		where matched.video_id not in (select video_id from carded)` + videoFavorite + `
		union all
` + groupSQL.items + folderItems + `)`
	return cte, append(args, folderArgs...)
}

// Reuse listOrders cursor semantics with item-specific value expressions.
var itemOrderValues = map[domain.VideoSort]string{
	domain.SortAddedAsc: `added_at`, domain.SortAddedDesc: `added_at`,
	domain.SortModifiedAsc: `mtime`, domain.SortModifiedDesc: `mtime`,
	domain.SortCreatedAsc: `created_at`, domain.SortCreatedDesc: `created_at`,
	domain.SortTitleAsc: `title_key`, domain.SortTitleDesc: `title_key`,
	domain.SortDurationAsc: `duration_ms`, domain.SortDurationDesc: `duration_ms`,
	domain.SortSizeAsc: `size_bytes`, domain.SortSizeDesc: `size_bytes`,
	domain.SortPlayedAsc: `played_at`, domain.SortPlayedDesc: `played_at`,
	domain.SortFavoritedAsc: `favorited_at`, domain.SortFavoritedDesc: `favorited_at`,
	domain.SortRandom: shuffleFunction + `(?, id)`,
}

// Put missing sort values last in both directions, including empty smart folders.
func itemOrder(sort domain.VideoSort) listOrder {
	order := listOrders[sort]
	order.value = itemOrderValues[sort]
	order.nullable = true
	return order
}

// ListLibrary reads counts, items, members and registered folders from one snapshot; callers validate audience permissions for filters.
func (s *LibraryStore) ListLibrary(ctx context.Context, audience domain.Audience, q domain.VideoQuery) (domain.LibraryPage, error) {
	if err := s.db.RefreshCollectionValues(ctx); err != nil {
		return domain.LibraryPage{}, err
	}
	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.LibraryPage{}, fmt.Errorf("cannot start reading the list: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	filters, err := resolveListFilters(ctx, tx, q)
	if err != nil {
		return domain.LibraryPage{}, err
	}
	spec := filters.narrow(libraryListSpec(audience, q, filters.tagIDs))
	spec.storedCollections = true
	spec.includeSmartFolders = q.ListsSmartFolders()
	page, err := listLibraryPageTx(ctx, tx, spec)
	if err != nil {
		return domain.LibraryPage{}, err
	}
	roots, err := listMediaFolders(ctx, tx)
	if err != nil {
		return domain.LibraryPage{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.LibraryPage{}, fmt.Errorf("cannot finish reading the list: %w", err)
	}
	page.MissingTagIDs = filters.missingTagIDs
	page.Roots = roots
	return page, nil
}

// Item IDs are keyset keys; negative IDs encode smart folders, while videoID holds a video's actual ID.
type itemRow struct {
	groupID sql.NullInt64
	id      int64
	videoID sql.NullInt64
	path    sql.NullString
}

func listLibraryPageTx(ctx context.Context, tx *sql.Tx, spec listSpec) (domain.LibraryPage, error) {
	limit := normalizeLimit(spec.limit)
	if !spec.sort.Valid() {
		// The HTTP boundary validates enums; unknown internal values fall back to defaults.
		spec.sort = domain.SortAddedDesc
	}
	order := itemOrder(spec.sort)

	var cursorClause string
	var cursorArgs []any
	if spec.cursor != "" {
		var err error
		cursorClause, cursorArgs, err = order.cursorCondition(spec, spec.cursor)
		if err != nil {
			return domain.LibraryPage{}, err
		}
	}

	cte, args := libraryItemsCTE(spec)

	// Materialize items once for page and count; count before applying the cursor and bind arguments in SQL order.
	cteArgs := append([]any{}, args...)
	if order.seeded {
		args = append(args, spec.seed)
	}
	query := cte + `, filtered as materialized (select items.*, ` + order.value +
		` as sort_value from items)
	select group_id, id, video_id, path, sort_value, (select count(*) from filtered) from filtered`
	if cursorClause != "" {
		query += ` where ` + cursorClause
		args = append(args, cursorArgs...)
	}
	query += ` order by ` + order.orderBy() + ` limit ?`

	rows, err := tx.QueryContext(ctx, query, append(args, limit+1)...)
	if err != nil {
		return domain.LibraryPage{}, fmt.Errorf("cannot read the list: %w", err)
	}
	defer func() { _ = rows.Close() }()
	total := -1
	var keys []itemRow
	var values []any
	for rows.Next() {
		var key itemRow
		var value any
		if err := rows.Scan(&key.groupID, &key.id, &key.videoID, &key.path, &value, &total); err != nil {
			return domain.LibraryPage{}, fmt.Errorf("cannot read the list: %w", err)
		}
		keys = append(keys, key)
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return domain.LibraryPage{}, fmt.Errorf("cannot read the list: %w", err)
	}
	if err := rows.Close(); err != nil {
		return domain.LibraryPage{}, fmt.Errorf("cannot close the list: %w", err)
	}
	if total < 0 {
		// An empty page needs a separate count within the same snapshot.
		if err := tx.QueryRowContext(ctx, cte+` select count(*) from items`, cteArgs...).Scan(&total); err != nil {
			return domain.LibraryPage{}, fmt.Errorf("cannot count items: %w", err)
		}
	}

	page := domain.LibraryPage{Total: total, Limit: limit}
	page.Items, page.NextCursor, err = libraryPageItems(ctx, tx, spec, order, keys, values, limit)
	if err != nil {
		return domain.LibraryPage{}, err
	}
	ids := make([]int64, len(page.Items))
	for i := range ids {
		ids[i] = keys[i].id
	}
	if page.ItemCursors, err = order.itemCursors(spec, values, ids); err != nil {
		return domain.LibraryPage{}, err
	}
	return page, nil
}

func libraryPageItems(ctx context.Context, tx *sql.Tx, spec listSpec, order listOrder, keys []itemRow, values []any, limit int) ([]domain.LibraryItem, string, error) {
	next := ""
	if len(keys) > limit {
		keys = keys[:limit]
		var err error
		next, err = order.encodeCursor(spec, values[limit-1], keys[limit-1].id)
		if err != nil {
			return nil, "", err
		}
	}
	items, err := loadLibraryItems(ctx, tx, spec.scope.audience, keys, spec.smartFolders, spec.storedCollections)
	if err != nil {
		return nil, "", err
	}
	return items, next, nil
}

// Video items use their chosen location; group values use all visible descendants and smart folders use prepared summaries.
func loadLibraryItems(ctx context.Context, tx *sql.Tx, audience domain.Audience, keys []itemRow, folders []librarySmartFolder, compact bool) ([]domain.LibraryItem, error) {
	var pairs [][2]any
	var groupIDs []int64
	for _, key := range keys {
		switch {
		case key.id < 0:
		case key.groupID.Valid:
			groupIDs = append(groupIDs, key.groupID.Int64)
		default:
			pairs = append(pairs, [2]any{key.videoID.Int64, key.path.String})
		}
	}
	videos, err := videosAtLocations(ctx, tx, pairs)
	if err != nil {
		return nil, err
	}
	if err := attachVersionsIn(ctx, tx, audience, videos); err != nil {
		return nil, err
	}
	if compact {
		folders, err = readLibraryFolderSummaries(ctx, tx, audience, keys)
		if err != nil {
			return nil, err
		}
	}
	groups, err := loadLibraryGroups(ctx, tx, audience, groupIDs, compact)
	if err != nil {
		return nil, err
	}

	items := make([]domain.LibraryItem, 0, len(keys))
	for _, key := range keys {
		if key.id < 0 {
			summary, ok := librarySmartFolderSummary(folders, -key.id)
			if !ok {
				return nil, fmt.Errorf("cannot read the smart folder (id=%d)", -key.id)
			}
			items = append(items, domain.LibraryItem{SmartFolder: summary})
			continue
		}
		if key.groupID.Valid {
			group, ok := groups[key.groupID.Int64]
			if !ok {
				return nil, fmt.Errorf("cannot read the group (id=%d)", key.groupID.Int64)
			}
			items = append(items, domain.LibraryItem{Group: &group})
			continue
		}
		video, ok := videos[key.videoID.Int64]
		if !ok {
			return nil, fmt.Errorf("cannot read the video (id=%d)", key.videoID.Int64)
		}
		items = append(items, domain.LibraryItem{Video: &video})
	}
	return items, nil
}

func loadLibraryGroups(ctx context.Context, tx *sql.Tx, audience domain.Audience, ids []int64, compact bool) (map[int64]domain.LibraryGroup, error) {
	if compact {
		return loadGroupCards(ctx, tx, audience, ids)
	}
	return loadGroups(ctx, tx, audience, ids)
}

func videosAtLocations(ctx context.Context, tx *sql.Tx, pairs [][2]any) (map[int64]domain.Video, error) {
	out := map[int64]domain.Video{}
	if len(pairs) == 0 {
		return out, nil
	}
	encoded, err := json.Marshal(pairs)
	if err != nil {
		return nil, fmt.Errorf("cannot build item keys: %w", err)
	}
	rows, err := tx.QueryContext(ctx, `with chosen(video_id, path) as (
			select json_extract(value, '$[0]'), json_extract(value, '$[1]') from json_each(?))
		select `+listColumns+` from chosen join videos on videos.id = chosen.video_id`+
		chosenLocationJoin("chosen.path", "videos"), string(encoded))
	if err != nil {
		return nil, fmt.Errorf("cannot read item videos: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		video, err := scanVideo(rows)
		if err != nil {
			return nil, fmt.Errorf("cannot read item videos: %w", err)
		}
		out[video.ID] = video
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read item videos: %w", err)
	}
	return out, nil
}

type extraScanner struct {
	rows  *sql.Rows
	extra []any
}

func (s extraScanner) Scan(dest ...any) error {
	return s.rows.Scan(append(dest, s.extra...)...)
}

type groupListSQL struct{ members, count, values, items string }

func libraryGroupSQL(spec listSpec, roots, progress string) groupListSQL {
	audience := spec.scope.audience
	if spec.storedCollections {
		return storedGroupListSQL(roots, audience)
	}
	return groupListSQL{
		members: groupDescendantsCTE(roots, audience) + `,
		gm as materialized (select root_id as group_id, video_id, sort_key as position from gdesc)`,
		count: `select count(*) from gm c where c.group_id = hits.group_id`,
		values: `mv as (
		select gm.group_id, gm.video_id, v.added_at, v.duration_ms,
			rl.mtime as mtime, ` + fileCreatedAtExpr("rl") + ` as created_at, rl.size_bytes as size_bytes,
			p.updated_at as played_at
		from gm join whole on whole.group_id = gm.group_id
		join videos v on v.id = gm.video_id` + representativeLocationJoin("gm", audience) + progress + `)`,
		items: `select g.id, g.id * 2 + 1, null, null,
			max(mv.added_at), max(mv.mtime), max(mv.created_at), g.title_key,
			sum(mv.duration_ms), sum(mv.size_bytes), max(mv.played_at), ` + groupFavoritedAtExpr("g") + `
		from mv join groups g on g.id = mv.group_id
		group by g.id`,
	}
}
