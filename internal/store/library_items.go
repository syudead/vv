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

func memberConditions(spec listSpec) ([]string, []any) {
	var conditions []string
	if spec.playableOnly {
		conditions = append(conditions, playableCondition)
	}
	matches, args := matchConditions(spec)
	return append(conditions, matches...), args
}

// Apply member filters at every stacked spec level before forming items.
func matchConditions(spec listSpec) ([]string, []any) {
	var conditions []string
	var args []any
	userKey := spec.videoUserKey()
	for _, level := range spec.levels() {
		tags, tagArgs := tagConditions(level, userKey)
		works, workArgs := workInfoConditions(level, userKey)
		facts, factArgs := addedAndLengthConditions(level)
		conditions = append(append(append(conditions, tags...), works...), facts...)
		args = append(append(append(args, tagArgs...), workArgs...), factArgs...)
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

// Playable and favorite-only filters apply if any stacked level requests them.
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

const secondsPerDay = 24 * 60 * 60

// Use one captured query time for age filters; compare inclusive whole-second durations and exclude unknown lengths.
func addedAndLengthConditions(spec listSpec) ([]string, []any) {
	var conditions []string
	var args []any
	if spec.addedWithinDays > 0 {
		conditions = append(conditions, `videos.added_at >= ?`)
		args = append(args, spec.now-int64(spec.addedWithinDays)*secondsPerDay)
	}
	if spec.minDurationSec != nil {
		conditions = append(conditions, `videos.duration_ms >= ?`)
		args = append(args, int64(*spec.minDurationSec)*1000)
	}
	if spec.maxDurationSec != nil {
		// A floored duration is below the inclusive upper bound exactly when milliseconds precede the next second.
		conditions = append(conditions, `videos.duration_ms < ?`)
		args = append(args, int64(*spec.maxDurationSec+1)*1000)
	}
	return conditions, args
}

// Match credits on the user-data key regardless of role; missing entity IDs match nothing.
func workInfoConditions(spec listSpec, userKey string) ([]string, []any) {
	var conditions []string
	var args []any
	// Unidentified content has no user data.
	link := func(table, alias, column string) string {
		return `(videos.content_key <> '' and exists (select 1 from ` + table + ` ` + alias + ` where ` + alias +
			`.content_key = ` + userKey + ` and ` + alias + `.` + column + ` = ?))`
	}
	if spec.personID != 0 {
		conditions = append(conditions, link("video_work_people", "vp", "person_id"))
		args = append(args, spec.personID)
	}
	if spec.organizationID != 0 {
		conditions = append(conditions, link("video_work_organizations", "vo", "organization_id"))
		args = append(args, spec.organizationID)
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
	if spec.tagKindID != 0 {
		conditions = append(conditions, videoHasTagOfKindCondition("videos", userKey))
		args = append(args, spec.tagKindID, spec.tagKindID)
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
	cte, args := chosenLocationsCTEFor(spec.scope, spec.searchAudience(), spec.searchExprs())
	part, partArgs := libraryItemsPart(spec)
	return cte + `,
	` + itemGroupsCTE(spec) + `,
	` + part, append(args, partArgs...)
}

// searchAudience は検索式を照合する見る人である。ownerSearch なら所有者の意味で照合する。
func (s listSpec) searchAudience() domain.Audience {
	if s.ownerSearch {
		return domain.AudienceOwner
	}
	return s.scope.audience
}

// hasSearch は、どれかの段が空でない検索式を持つかを返す。
func (s listSpec) hasSearch() bool {
	return slices.ContainsFunc(s.searchExprs(), func(expr domain.SearchExpr) bool { return !expr.Empty() })
}

// itemGroupsFavorite は、グループの項目になりうるものがお気に入りにしたグループか（どれかの段が
// お気に入りのみ）を返す。そうでなければいちばん上のグループである。
func (s listSpec) itemGroupsFavorite() bool {
	return s.anyLevel(func(level listSpec) bool { return level.favoriteOnly })
}

// itemGroupsCTE は、spec の項目のグループになりうるグループの見せてよい子孫の動画
// `gm(group_id, video_id, position)` を定める with 句の部品である。絞り込みによらず、グループの根
// （itemGroupsFavorite）と見る人だけで決まるので、根と見る人の同じ絞り込みはこれを分け合える。
func itemGroupsCTE(spec listSpec) string {
	roots := topGroupsSelect
	if spec.itemGroupsFavorite() {
		roots = favoriteGroupsSelect
	}
	return groupDescendantsCTE(roots, spec.scope.audience) + `,
	gm as materialized (select root_id as group_id, video_id, sort_key as position from gdesc)`
}

// groupWatchStateExpr はグループの視聴状態を、子孫の動画の行ごとの completed と position_ms の式から
// 集める式である。どれも見ていなければ unwatched、どれも見終えていれば watched、ほかは inProgress。
func groupWatchStateExpr(completed, position string) string {
	return `case when sum(` + completed + ` = 1 or ` + position + ` > 0) = 0 then 'unwatched'
				when sum(` + completed + ` = 1) = count(*) then 'watched'
				else 'inProgress' end`
}

// libraryItemsPart は libraryItemsCTE の項目の表を、所在の表 chosen（`(video_id, path)`）と gm
// から作る with 句の部品とその引数を返す。表の名前は matched・hits・whole・carded・mv・items である。
// ホームの棚の読み出し（home_shelf_read.go）は同じ決め方を一時表の上で行う。
func libraryItemsPart(spec listSpec) (string, []any) {
	audience := spec.scope.audience
	const chosen, matched, hits, whole, carded, mv, items = `chosen`, `matched`, `hits`, `whole`, `carded`, `mv`, `items`
	conditions, args := matchConditions(spec)
	where := ""
	if len(conditions) > 0 {
		where = ` where ` + strings.Join(conditions, " and ")
	}
	// Apply playability after forming items; a group is playable when any descendant is playable.
	videoPlayable := ""
	groupPlayable := ""
	if spec.anyLevel(func(level listSpec) bool { return level.playableOnly }) {
		videoPlayable = ` and ` + playableCondition
		groupPlayable = ` and exists (select 1 from gm pm join videos on videos.id = pm.video_id
			where pm.group_id = ` + hits + `.group_id and ` + playableCondition + `)`
	}
	// Unidentified content has no playback position.
	progressJoin := func(videoAlias string) string {
		return ` left join playback_progress p on p.content_key = ` + userKeyExpr(videoAlias) + ` and ` +
			videoAlias + `.content_key <> ''`
	}
	folderItems, folderArgs := smartFolderItemsSQL(spec.smartFolders)
	videoFavorite := ""
	if spec.itemGroupsFavorite() {
		videoFavorite = ` and fav.content_key is not null`
	}
	part := matched + ` as (
		select videos.id as video_id, ` + chosen + `.path as path
		from ` + chosen + ` join videos on videos.id = ` + chosen + `.video_id` + where + `),
	` + hits + ` as (
		select gm.group_id, count(*) as n from ` + matched + `
		join gm on gm.video_id = ` + matched + `.video_id
		group by gm.group_id),
	` + whole + ` as materialized (
		select ` + hits + `.group_id from ` + hits + `
		where ` + hits + `.n = (select count(*) from gm c where c.group_id = ` + hits + `.group_id)` + groupPlayable + `),
	` + carded + ` as materialized (
		select video_id from gm where group_id in (select group_id from ` + whole + `)
		except
		select video_id from gm where group_id not in (select group_id from ` + whole + `)),
	` + mv + ` as (
		select gm.group_id, gm.video_id, v.added_at, v.duration_ms,
			rl.mtime as mtime, ` + fileCreatedAtExpr("rl") + ` as created_at, rl.size_bytes as size_bytes,
			p.updated_at as played_at,
			coalesce(p.completed, 0) as completed, coalesce(p.position_ms, 0) as position_ms
		from gm join ` + whole + ` on ` + whole + `.group_id = gm.group_id
		join videos v on v.id = gm.video_id` + representativeLocationJoin("gm", audience) + progressJoin("v") + `),
	` + items + ` as (
		select null as group_id, videos.id * 2 as id, videos.id as video_id, ` + matched + `.path as path,
			videos.added_at as added_at, loc.mtime as mtime, ` + fileCreatedAtExpr("loc") + ` as created_at,
			loc.title_key as title_key,
			videos.duration_ms as duration_ms, loc.size_bytes as size_bytes, p.updated_at as played_at,
			fav.favorited_at as favorited_at, ` + videoWatchStateExpr + ` as watch_state
		from ` + matched + ` join videos on videos.id = ` + matched + `.video_id
		join video_locations loc on loc.path = ` + matched + `.path` + progressJoin("videos") + `
		left join video_favorites fav on fav.content_key = ` + userKeyExpr("videos") + ` and videos.content_key <> ''
		where ` + matched + `.video_id not in (select video_id from ` + carded + `)` +
		videoPlayable + videoFavorite + `
		union all
		select g.id, g.id * 2 + 1, null, null,
			max(m.added_at), max(m.mtime), max(m.created_at), g.title_key,
			sum(m.duration_ms), sum(m.size_bytes), max(m.played_at), ` + groupFavoritedAtExpr("g") + `,
			` + groupWatchStateExpr("m.completed", "m.position_ms") + `
		from ` + mv + ` m join groups g on g.id = m.group_id
		group by g.id` + folderItems + `)`
	return part, append(args, folderArgs...)
}

// homeShelfSpecs は、ホームのフォルダの棚ごとの経路の条件を、見る人 audience の範囲と基準時刻 now の
// 絞り込み（smartFolderSpec）にして棚の並びの順に返す。名指す実体の無い棚は何にも一致しないので
// 入れない（docs/design-docs/catalog-organization.md）。
func homeShelfSpecs(ctx context.Context, tx *sql.Tx, audience domain.Audience, now int64) ([]listSpec, error) {
	folders, err := homeShelves(ctx, tx)
	if err != nil {
		return nil, err
	}
	specs := make([]listSpec, 0, len(folders))
	for _, folder := range folders {
		rules, err := smartFolderPathRules(ctx, tx, folder.ID)
		if err != nil {
			return nil, err
		}
		missing, err := missingSmartRuleRefs(ctx, tx, rules)
		if err != nil {
			return nil, err
		}
		if len(missing) > 0 {
			continue
		}
		specs = append(specs, smartFolderSpec(audience, rules, folder.Sort, now))
	}
	return specs, nil
}

func itemWatchCondition(filter domain.WatchFilter) (string, []any) {
	switch filter {
	case domain.WatchUnwatched, domain.WatchInProgress, domain.WatchWatched:
		return `watch_state = ?`, []any{string(filter)}
	default:
		return "", nil
	}
}

func itemWatchConditions(spec listSpec) (string, []any) {
	var clauses []string
	var args []any
	for _, level := range spec.levels() {
		clause, clauseArgs := itemWatchCondition(level.watch)
		if clause == "" {
			continue
		}
		clauses = append(clauses, clause)
		args = append(args, clauseArgs...)
	}
	return strings.Join(clauses, " and "), args
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
	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.LibraryPage{}, fmt.Errorf("cannot start reading the list: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	tagIDs, missingTagIDs, err := existingTagIDs(ctx, tx, q.TagIDs)
	if err != nil {
		return domain.LibraryPage{}, err
	}
	spec := libraryListSpec(audience, q, tagIDs)
	if spec.smartFolders, err = librarySmartFolders(ctx, tx, audience, q, spec.now); err != nil {
		return domain.LibraryPage{}, err
	}
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
	page.MissingTagIDs = missingTagIDs
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
	watchClause, watchArgs := itemWatchConditions(spec)
	whereWatch := ""
	if watchClause != "" {
		whereWatch = ` where ` + watchClause
	}

	// Materialize filtered items once for page and count; count before applying the cursor and bind arguments in SQL order.
	cteArgs := append([]any{}, args...)
	if order.seeded {
		args = append(args, spec.seed)
	}
	args = append(args, watchArgs...)
	query := cte + `, filtered as materialized (select items.*, ` + order.value +
		` as sort_value from items` + whereWatch + `)
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
		if err := tx.QueryRowContext(ctx, cte+` select count(*) from items`+whereWatch,
			append(cteArgs, watchArgs...)...).Scan(&total); err != nil {
			return domain.LibraryPage{}, fmt.Errorf("cannot count items: %w", err)
		}
	}

	page := domain.LibraryPage{Total: total, Limit: limit}
	page.Items, page.NextCursor, err = libraryPageItems(ctx, tx, spec, order, keys, values, limit)
	if err != nil {
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
	items, err := loadLibraryItems(ctx, tx, spec.scope.audience, keys, spec.smartFolders)
	if err != nil {
		return nil, "", err
	}
	return items, next, nil
}

// Video items use their chosen location; group values use all visible descendants and smart folders use prepared summaries.
func loadLibraryItems(ctx context.Context, tx *sql.Tx, audience domain.Audience, keys []itemRow, folders []librarySmartFolder) ([]domain.LibraryItem, error) {
	return assembleLibraryItems(ctx, tx, audience, keys, folders, func(ids []int64) (map[int64]domain.LibraryGroup, error) {
		return loadGroups(ctx, tx, audience, ids)
	})
}

// assembleLibraryItems は loadLibraryItems の本体で、グループの項目を groups で読む。ホームの棚はカードの値
// だけを読む loadGroupCards を渡す（home_group_cards.go）。
func assembleLibraryItems(ctx context.Context, tx *sql.Tx, audience domain.Audience, keys []itemRow, folders []librarySmartFolder,
	groups func([]int64) (map[int64]domain.LibraryGroup, error)) ([]domain.LibraryItem, error) {
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
	loaded, err := groups(groupIDs)
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
			group, ok := loaded[key.groupID.Int64]
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
		select `+listColumns+` from chosen join videos on videos.id = chosen.video_id
		join video_locations loc on loc.path = chosen.path`, string(encoded))
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

// LibraryIDs applies ListLibrary filters without pagination and returns owner-visible selection IDs, with group videos in playback order.
func (s *LibraryStore) LibraryIDs(ctx context.Context, q domain.VideoQuery) (domain.LibrarySelection, []int64, error) {
	selection := domain.LibrarySelection{VideoIDs: []int64{}, Groups: []domain.LibraryGroupSelection{}, SmartFolderIDs: []int64{}}
	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.LibrarySelection{}, nil, fmt.Errorf("cannot start reading ids: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	tagIDs, missingTagIDs, err := existingTagIDs(ctx, tx, q.TagIDs)
	if err != nil {
		return domain.LibrarySelection{}, nil, err
	}
	// Smart-folder cards are unfiltered here, so return their IDs directly, including empty folders for the owner.
	folderIDs, err := librarySmartFolderIDs(ctx, tx, q)
	if err != nil {
		return domain.LibrarySelection{}, nil, err
	}
	selection.SmartFolderIDs = append(selection.SmartFolderIDs, folderIDs...)
	cte, args := libraryItemsCTE(libraryListSpec(domain.AudienceOwner, q, tagIDs))
	watchClause, watchArgs := itemWatchCondition(q.Watch)
	and := ""
	if watchClause != "" {
		and = ` and ` + watchClause
	}
	args = append(args, watchArgs...)
	args = append(args, watchArgs...)
	rows, err := tx.QueryContext(ctx, cte+`
		select null as group_id, video_id, null as position from items
		where group_id is null`+and+`
		union all
		select items.group_id, gm.video_id, gm.position from items
		join gm on gm.group_id = items.group_id
		where items.group_id is not null`+and+`
		order by group_id, position`, args...)
	if err != nil {
		return domain.LibrarySelection{}, nil, fmt.Errorf("cannot read ids: %w", err)
	}
	defer func() { _ = rows.Close() }()
	groupIndex := map[int64]int{}
	for rows.Next() {
		var groupID sql.NullInt64
		var position sql.NullString
		var id int64
		if err := rows.Scan(&groupID, &id, &position); err != nil {
			return domain.LibrarySelection{}, nil, fmt.Errorf("cannot read ids: %w", err)
		}
		if !groupID.Valid {
			selection.VideoIDs = append(selection.VideoIDs, id)
			continue
		}
		index, ok := groupIndex[groupID.Int64]
		if !ok {
			index = len(selection.Groups)
			groupIndex[groupID.Int64] = index
			selection.Groups = append(selection.Groups, domain.LibraryGroupSelection{ID: groupID.Int64, VideoIDs: []int64{}})
		}
		selection.Groups[index].VideoIDs = append(selection.Groups[index].VideoIDs, id)
	}
	if err := rows.Err(); err != nil {
		return domain.LibrarySelection{}, nil, fmt.Errorf("cannot read ids: %w", err)
	}
	if err := rows.Close(); err != nil {
		return domain.LibrarySelection{}, nil, fmt.Errorf("cannot read ids: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return domain.LibrarySelection{}, nil, fmt.Errorf("cannot finish reading ids: %w", err)
	}
	return selection, missingTagIDs, nil
}
