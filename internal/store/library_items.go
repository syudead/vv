package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// ライブラリの項目（specs/017-folder-groups/data-model.md §5〜§7）。013 の一覧の流れ
// （範囲と検索式 → chosen → 絞り込み → keyset）に、当たった動画を項目（動画か
// グループ）へまとめる段を足す。
//
//  1. chosen に再生可否とタグの AND を掛け、当たった動画（matched）を決める
//  2. 当たった動画のうちグループのメンバーはグループへ、それ以外は動画の項目にする
//  3. グループの値は、当たったかどうかに関わらず見せてよい全メンバーから数える
//  4. 視聴状態の絞り込みを項目の視聴状態に掛け、項目の値で並べて keyset で区切る

// minGroupMembers は、見る人にグループとして見せるのに要る、見せてよいメンバーの数である。
// 所有者には、メンバーが消えて1本になったグループも次の作り直しまでグループとして
// 見せる（data-model.md §1）。ゲストには、公開のメンバーが1本だけのグループを動画の
// 項目として見せる（data-model.md §7）。
func minGroupMembers(audience domain.Audience) int {
	if audience.IsOwner() {
		return 1
	}
	return 2
}

// memberConditions はメンバー単位の絞り込み（再生可否とタグの AND）を、videos に
// 対する条件句の並びと引数にする。
func memberConditions(spec listSpec) ([]string, []any) {
	var conditions []string
	var args []any
	if spec.playableOnly {
		conditions = append(conditions, playableCondition)
	}
	// 手で付けた分とフォルダ名から付いている分のどちらでも当たる（data-model.md §4）。
	for _, tagID := range spec.tagIDs {
		conditions = append(conditions, videoHasTagCondition("videos"))
		args = append(args, tagID, tagID)
	}
	return conditions, args
}

// representativeLocationValue は動画（別名 alias の video_id を持つ行）の代表の所在
// （見せてよい所在のうちパスの最小）の列 column を返す副問い合わせである。
func representativeLocationValue(alias, column string, audience domain.Audience) string {
	return `(select rl.` + column + ` from video_locations rl where rl.video_id = ` + alias + `.video_id and ` +
		visibleLocationCondition("rl", audience) + ` order by rl.path limit 1)`
}

// libraryItemsCTE は項目の表 `items(group_id, id, path, added_at, mtime, title_key,
// duration_ms, size_bytes, played_at, watch_state)` と、見せてよいグループのメンバー
// `gm(group_id, video_id, position)`・グループとして見せるもの `live(group_id)` を
// 定める with 句と、その引数を返す。group_id は動画の項目では NULL である。id は
// 動画の項目では動画の id、グループの項目では見せてよいメンバーのうち並びで最初の
// ものの id で、keyset とシャッフルの鍵に使う。
func libraryItemsCTE(spec listSpec) (string, []any) {
	audience := spec.scope.audience
	cte, args := chosenLocationsCTE(spec.scope, spec.expr)
	conditions, memberArgs := memberConditions(spec)
	args = append(args, memberArgs...)
	where := ""
	if len(conditions) > 0 {
		where = ` where ` + strings.Join(conditions, " and ")
	}
	// 内容の識別子が空の動画は再生位置を持たない（filteredFrom と同じ扱い）。
	progressJoin := func(videoAlias string) string {
		return ` left join playback_progress p on p.content_key = ` + videoAlias + `.content_key and ` +
			videoAlias + `.content_key <> ''`
	}
	cte += `,
	matched as (
		select videos.id as video_id, chosen.path as path
		from chosen join videos on videos.id = chosen.video_id` + where + `),
	gm as (
		select m.group_id, m.video_id, m.position from folder_group_members m
		where exists (select 1 from video_locations l where l.video_id = m.video_id and ` +
		visibleLocationCondition("l", audience) + `)),
	live as (
		select group_id from gm group by group_id having count(*) >= ` + strconv.Itoa(minGroupMembers(audience)) + `),
	hit as (
		select distinct gm.group_id from matched
		join gm on gm.video_id = matched.video_id
		join live on live.group_id = gm.group_id),
	mv as (
		select gm.group_id, gm.video_id, gm.position, v.added_at, v.duration_ms,
			` + representativeLocationValue("gm", "mtime", audience) + ` as mtime,
			` + representativeLocationValue("gm", "size_bytes", audience) + ` as size_bytes,
			p.updated_at as played_at,
			coalesce(p.completed, 0) as completed, coalesce(p.position_ms, 0) as position_ms
		from gm join hit on hit.group_id = gm.group_id
		join videos v on v.id = gm.video_id` + progressJoin("v") + `),
	items as (
		select null as group_id, videos.id as id, matched.path as path,
			videos.added_at as added_at, loc.mtime as mtime, loc.title_key as title_key,
			videos.duration_ms as duration_ms, loc.size_bytes as size_bytes, p.updated_at as played_at,
			case when p.completed = 1 then 'watched'
				when p.content_key is null or p.position_ms = 0 then 'unwatched'
				else 'inProgress' end as watch_state
		from matched join videos on videos.id = matched.video_id
		join video_locations loc on loc.path = matched.path` + progressJoin("videos") + `
		where not exists (select 1 from gm join live on live.group_id = gm.group_id where gm.video_id = matched.video_id)
		union all
		select g.id, (select f.video_id from gm f where f.group_id = g.id order by f.position limit 1), null,
			max(mv.added_at), max(mv.mtime), g.title_key,
			sum(mv.duration_ms), sum(mv.size_bytes), max(mv.played_at),
			case when sum(mv.completed = 1 or mv.position_ms > 0) = 0 then 'unwatched'
				when sum(mv.completed = 1) = count(*) then 'watched'
				else 'inProgress' end
		from mv join folder_groups g on g.id = mv.group_id
		group by g.id)`
	return cte, args
}

// itemWatchCondition は項目の視聴状態の絞り込みを items に対する条件句にする。
// 絞り込まないときは空の句を返す。値は検査済みの列挙なので、引数で渡す。
func itemWatchCondition(filter domain.WatchFilter) (string, []any) {
	switch filter {
	case domain.WatchUnwatched, domain.WatchInProgress, domain.WatchWatched:
		return `watch_state = ?`, []any{string(filter)}
	default:
		return "", nil
	}
}

// itemOrderValues は項目の並べ替えの値の式である（data-model.md §5 の 3）。並び順の
// 向き・値の有無・カーソルの形は listOrders と同じで、値の式だけを項目の列にする。
var itemOrderValues = map[domain.VideoSort]string{
	domain.SortAddedAsc: `added_at`, domain.SortAddedDesc: `added_at`,
	domain.SortModifiedAsc: `mtime`, domain.SortModifiedDesc: `mtime`,
	domain.SortTitleAsc: `title_key`, domain.SortTitleDesc: `title_key`,
	domain.SortDurationAsc: `duration_ms`, domain.SortDurationDesc: `duration_ms`,
	domain.SortSizeAsc: `size_bytes`, domain.SortSizeDesc: `size_bytes`,
	domain.SortPlayedAsc: `played_at`, domain.SortPlayedDesc: `played_at`,
	domain.SortRandom: shuffleFunction + `(?, id)`,
}

// itemOrder は並び順 sort の項目の並べ替えを返す。
func itemOrder(sort domain.VideoSort) listOrder {
	order := listOrders[sort]
	order.value = itemOrderValues[sort]
	return order
}

// ListLibrary は見る人（audience）に見せるライブラリの項目1ページを返す
// （GET /api/library、contracts/library-api.md §1）。条件（視聴状態・並べ替え・
// タグ）をゲストが使えるかは呼び出し側が domain.Audience.CheckVideoQuery で確かめる。
//
// タグの存在確認・件数・ページの項目・グループのメンバーを同じ読み取り
// スナップショット（s.db.read の1取引）から返す。
func (s *LibraryStore) ListLibrary(ctx context.Context, audience domain.Audience, q domain.VideoQuery) (domain.LibraryPage, error) {
	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.LibraryPage{}, fmt.Errorf("一覧の読み取りを始められません: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	tagIDs, missingTagIDs, err := existingTagIDs(ctx, tx, q.TagIDs)
	if err != nil {
		return domain.LibraryPage{}, err
	}
	page, err := listLibraryPageTx(ctx, tx, listSpec{
		scope: libraryScope(audience), expr: domain.ParseSearchQuery(q.Query),
		watch: q.Watch, playableOnly: q.PlayableOnly, tagIDs: tagIDs,
		sort: q.Sort, seed: q.Seed, cursor: q.Cursor, limit: q.Limit,
	})
	if err != nil {
		return domain.LibraryPage{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.LibraryPage{}, fmt.Errorf("一覧の読み取りを終えられません: %w", err)
	}
	page.MissingTagIDs = missingTagIDs
	return page, nil
}

// itemRow はページの項目1件の鍵である。
type itemRow struct {
	groupID sql.NullInt64
	id      int64
	path    sql.NullString
}

func listLibraryPageTx(ctx context.Context, tx *sql.Tx, spec listSpec) (domain.LibraryPage, error) {
	limit := normalizeLimit(spec.limit)
	if !spec.sort.Valid() {
		// 値の検査は入口（internal/httpapi）が行う。ここに来た未知の値は既定にする。
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
	watchClause, watchArgs := itemWatchCondition(spec.watch)
	whereWatch := ""
	if watchClause != "" {
		whereWatch = ` where ` + watchClause
	}

	var total int
	//nolint:gosec // 組み立てるのは定型の条件句だけで、値はすべて引数で渡す。
	if err := tx.QueryRowContext(ctx, cte+` select count(*) from items`+whereWatch,
		append(append([]any{}, args...), watchArgs...)...).Scan(&total); err != nil {
		return domain.LibraryPage{}, fmt.Errorf("件数を数えられません: %w", err)
	}

	// 引数は SQL の文字列に現れる順（CTE・seed・視聴状態・カーソル・件数）に並べる。
	if order.seeded {
		args = append(args, spec.seed)
	}
	args = append(args, watchArgs...)
	//nolint:gosec // 組み立てるのは列名と定型の条件句だけで、値はすべて引数で渡す。
	query := cte + ` select group_id, id, path, sort_value from (select items.*, ` + order.value +
		` as sort_value from items` + whereWatch + `) as items`
	if cursorClause != "" {
		query += ` where ` + cursorClause
		args = append(args, cursorArgs...)
	}
	query += ` order by ` + order.orderBy() + ` limit ?`

	rows, err := tx.QueryContext(ctx, query, append(args, limit+1)...)
	if err != nil {
		return domain.LibraryPage{}, fmt.Errorf("一覧を読み出せません: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var keys []itemRow
	var values []any
	for rows.Next() {
		var key itemRow
		var value any
		if err := rows.Scan(&key.groupID, &key.id, &key.path, &value); err != nil {
			return domain.LibraryPage{}, fmt.Errorf("一覧を読み出せません: %w", err)
		}
		keys = append(keys, key)
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return domain.LibraryPage{}, fmt.Errorf("一覧を読み出せません: %w", err)
	}
	if err := rows.Close(); err != nil {
		return domain.LibraryPage{}, fmt.Errorf("一覧を閉じられません: %w", err)
	}

	page := domain.LibraryPage{Total: total, Limit: limit, Items: []domain.LibraryItem{}}
	if len(keys) > limit {
		keys = keys[:limit]
		page.NextCursor, err = order.encodeCursor(spec, values[limit-1], keys[limit-1].id)
		if err != nil {
			return domain.LibraryPage{}, err
		}
	}
	page.Items, err = loadLibraryItems(ctx, tx, spec.scope.audience, keys)
	if err != nil {
		return domain.LibraryPage{}, err
	}
	return page, nil
}

// loadLibraryItems はページの項目の鍵から項目を組み立てる。動画の項目は一覧に出す
// 所在（chosen）の題名・大きさ・更新時刻で、グループの項目は見せてよい全メンバーから作る。
func loadLibraryItems(ctx context.Context, tx *sql.Tx, audience domain.Audience, keys []itemRow) ([]domain.LibraryItem, error) {
	var pairs [][2]any
	var groupIDs []int64
	for _, key := range keys {
		if key.groupID.Valid {
			groupIDs = append(groupIDs, key.groupID.Int64)
		} else {
			pairs = append(pairs, [2]any{key.id, key.path.String})
		}
	}
	videos, err := videosAtLocations(ctx, tx, pairs)
	if err != nil {
		return nil, err
	}
	groups, err := loadGroups(ctx, tx, audience, groupIDs)
	if err != nil {
		return nil, err
	}

	items := make([]domain.LibraryItem, 0, len(keys))
	for _, key := range keys {
		if key.groupID.Valid {
			group, ok := groups[key.groupID.Int64]
			if !ok {
				return nil, fmt.Errorf("グループを読み出せません (id=%d)", key.groupID.Int64)
			}
			items = append(items, domain.LibraryItem{Group: &group})
			continue
		}
		video, ok := videos[key.id]
		if !ok {
			return nil, fmt.Errorf("動画を読み出せません (id=%d)", key.id)
		}
		items = append(items, domain.LibraryItem{Video: &video})
	}
	return items, nil
}

// videosAtLocations は (動画の id, 所在のパス) の組ごとに、その所在の題名・大きさ・
// 更新時刻で動画を読み出す（ListVideos の listColumns と同じ写し方）。
func videosAtLocations(ctx context.Context, tx *sql.Tx, pairs [][2]any) (map[int64]domain.Video, error) {
	out := map[int64]domain.Video{}
	if len(pairs) == 0 {
		return out, nil
	}
	encoded, err := json.Marshal(pairs)
	if err != nil {
		return nil, fmt.Errorf("項目の鍵を組み立てられません: %w", err)
	}
	//nolint:gosec // listColumns は定型の列だけで、値は引数で渡す。
	rows, err := tx.QueryContext(ctx, `with chosen(video_id, path) as (
			select json_extract(value, '$[0]'), json_extract(value, '$[1]') from json_each(?))
		select `+listColumns+` from chosen join videos on videos.id = chosen.video_id
		join video_locations loc on loc.path = chosen.path`, string(encoded))
	if err != nil {
		return nil, fmt.Errorf("項目の動画を読み出せません: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		video, err := scanVideo(rows)
		if err != nil {
			return nil, fmt.Errorf("項目の動画を読み出せません: %w", err)
		}
		out[video.ID] = video
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("項目の動画を読み出せません: %w", err)
	}
	return out, nil
}

// extraScanner は scanVideo の列に続く列を extra へ受け取る。
type extraScanner struct {
	rows  *sql.Rows
	extra []any
}

func (s extraScanner) Scan(dest ...any) error {
	return s.rows.Scan(append(dest, s.extra...)...)
}

// loadGroups はグループの id ごとに、見せてよいメンバーを並びの順に読み、グループの
// 項目を作る。ゲストには再生の記録を読まない（016 の guest-api §1）。見せてよい
// メンバーが無いグループは結果に入らない。
func loadGroups(ctx context.Context, q queryExecer, audience domain.Audience, groupIDs []int64) (map[int64]domain.LibraryGroup, error) {
	out := map[int64]domain.LibraryGroup{}
	if len(groupIDs) == 0 {
		return out, nil
	}
	encoded, err := json.Marshal(groupIDs)
	if err != nil {
		return nil, fmt.Errorf("グループの id を組み立てられません: %w", err)
	}
	progressColumns := `null, null, null, null`
	progressJoin := ""
	if audience.IsOwner() {
		progressColumns = `p.position_ms, p.duration_ms, p.completed, p.updated_at`
		progressJoin = ` left join playback_progress p on p.content_key = videos.content_key and videos.content_key <> ''`
	}
	//nolint:gosec // 組み立てるのは定型の列と条件句だけで、値は引数で渡す。
	rows, err := q.QueryContext(ctx, `select `+videoColumns(audience)+`, g.id, g.path, g.name, `+progressColumns+`
		from folder_groups g join folder_group_members m on m.group_id = g.id
		join videos on videos.id = m.video_id`+progressJoin+`
		where g.id in (select value from json_each(?)) and `+visibleVideoCondition("videos", audience)+`
		order by g.id, m.position`, string(encoded))
	if err != nil {
		return nil, fmt.Errorf("グループのメンバーを読み出せません: %w", err)
	}
	defer func() { _ = rows.Close() }()

	type collected struct {
		path, name string
		members    []domain.Video
		progress   []*domain.Progress
	}
	var order []int64
	groups := map[int64]*collected{}
	for rows.Next() {
		var groupID int64
		var path, name string
		var position, duration, updatedAt sql.NullInt64
		var completed sql.NullBool
		video, err := scanVideo(extraScanner{rows: rows, extra: []any{
			&groupID, &path, &name, &position, &duration, &completed, &updatedAt,
		}})
		if err != nil {
			return nil, fmt.Errorf("グループのメンバーを読み出せません: %w", err)
		}
		group := groups[groupID]
		if group == nil {
			group = &collected{path: path, name: name}
			groups[groupID] = group
			order = append(order, groupID)
		}
		var progress *domain.Progress
		if position.Valid {
			progress = &domain.Progress{
				PositionMs: position.Int64, DurationMs: duration.Int64,
				Completed: completed.Bool, UpdatedAt: time.Unix(updatedAt.Int64, 0),
			}
		}
		group.members = append(group.members, video)
		group.progress = append(group.progress, progress)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("グループのメンバーを読み出せません: %w", err)
	}
	for _, id := range order {
		group := groups[id]
		out[id] = domain.NewLibraryGroup(group.path, group.name, group.members, group.progress)
	}
	return out, nil
}

// LibraryIDs は ListLibrary と同じ条件（並び順・カーソル・件数を除く）に合う項目の、
// 動画の id とグループの全メンバーの id を、ページングせずに返す
// （GET /api/library/ids、contracts/library-api.md §2・data-model.md §5 の 7）。並びは
// 決めない。「すべて選択」は所有者だけの操作なので、所有者として読む。
func (s *LibraryStore) LibraryIDs(ctx context.Context, q domain.VideoQuery) ([]int64, []int64, error) {
	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, nil, fmt.Errorf("id の読み取りを始められません: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	tagIDs, missingTagIDs, err := existingTagIDs(ctx, tx, q.TagIDs)
	if err != nil {
		return nil, nil, err
	}
	cte, args := libraryItemsCTE(listSpec{
		scope: libraryScope(domain.AudienceOwner), expr: domain.ParseSearchQuery(q.Query),
		watch: q.Watch, playableOnly: q.PlayableOnly, tagIDs: tagIDs,
	})
	watchClause, watchArgs := itemWatchCondition(q.Watch)
	and := ""
	if watchClause != "" {
		and = ` and ` + watchClause
	}
	args = append(args, watchArgs...)
	args = append(args, watchArgs...)
	//nolint:gosec // 組み立てるのは定型の条件句だけで、値はすべて引数で渡す。
	rows, err := tx.QueryContext(ctx, cte+`
		select id from items where group_id is null`+and+`
		union all
		select gm.video_id from items join gm on gm.group_id = items.group_id
		where items.group_id is not null`+and, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("id を読み出せません: %w", err)
	}
	defer func() { _ = rows.Close() }()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, nil, fmt.Errorf("id を読み出せません: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("id を読み出せません: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, nil, fmt.Errorf("id を読み出せません: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, fmt.Errorf("id の読み取りを終えられません: %w", err)
	}
	return ids, missingTagIDs, nil
}

// FolderGroup はフォルダ dir（絶対パス）のグループを、絞り込みに関係なく見せてよい
// 全メンバーから作って返す（GET /api/folders/{rootId}/group、contracts/library-api.md §3）。
// そのフォルダが今グループでない、無い、または見る人にグループとして見せられない
// （ゲストに公開のメンバーが2本以上無い、data-model.md §7）ときは domain.ErrNotFound である。
func (s *LibraryStore) FolderGroup(ctx context.Context, audience domain.Audience, dir string) (domain.LibraryGroup, error) {
	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.LibraryGroup{}, fmt.Errorf("グループの読み取りを始められません: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var groupID int64
	err = tx.QueryRowContext(ctx, `select id from folder_groups where path_key = ?`, domain.FolderKey(dir)).Scan(&groupID)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.LibraryGroup{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.LibraryGroup{}, fmt.Errorf("グループを読み出せません: %w", err)
	}
	groups, err := loadGroups(ctx, tx, audience, []int64{groupID})
	if err != nil {
		return domain.LibraryGroup{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.LibraryGroup{}, fmt.Errorf("グループの読み取りを終えられません: %w", err)
	}
	group, ok := groups[groupID]
	if !ok || len(group.Members) < minGroupMembers(audience) {
		return domain.LibraryGroup{}, domain.ErrNotFound
	}
	return group, nil
}
