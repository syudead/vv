package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/syudead/vv/internal/domain"
)

// ライブラリの項目（specs/027-partial-group-search/contracts/library-api.md §1〜§2、
// specs/048-groups/data-model.md「Library items」）。013 の一覧の流れ（範囲と検索式 → chosen →
// 絞り込み → keyset）に、当たった動画を項目（動画かグループ）へまとめる段を足す。
//
//  1. chosen にタグの AND を掛け、当たった動画（matched）を決める。再生可否は入れない
//  2. いちばん上のグループ（どのグループの子でもないもの）ごとに、見せてよい子孫の動画のうち
//     当たった本数を全部の本数と比べ、等しければグループの項目、一部だけなら当たった動画を
//     1 本ずつ動画の項目にする。どのグループにも無い当たった動画も動画の項目にする
//  3. グループの値は見せてよい子孫の動画の全部から数える
//  4. 再生可否と視聴状態の絞り込みを項目に掛け、項目の値で並べて keyset で区切る

// memberConditions はメンバー単位の絞り込み（再生可否とタグの AND）を、videos に
// 対する条件句の並びと引数にする（GET /api/videos の一覧が使う）。
func memberConditions(spec listSpec) ([]string, []any) {
	var conditions []string
	if spec.playableOnly {
		conditions = append(conditions, playableCondition)
	}
	tags, args := tagConditions(spec)
	return append(conditions, tags...), args
}

// tagConditions はタグの AND を、videos に対する条件句の並びと引数にする。
// 手で付けた分とフォルダ名から付いている分のどちらでも当たる（017 の data-model.md §4）。
func tagConditions(spec listSpec) ([]string, []any) {
	var conditions []string
	var args []any
	for _, tagID := range spec.tagIDs {
		conditions = append(conditions, videoHasTagCondition("videos"))
		args = append(args, tagID, tagID)
	}
	return conditions, args
}

// representativeLocationJoin は、動画（別名 alias の video_id を持つ行）に代表の所在
// （見せてよい所在のうちパスの最小）を rl として結ぶ。列ごとに副問い合わせで代表を
// 選ぶと、1行につき同じ選び方を列の数だけ繰り返す（issue 674）。
func representativeLocationJoin(alias string, audience domain.Audience) string {
	return ` left join video_locations rl on rl.id = (select l.id from video_locations l where l.video_id = ` +
		alias + `.video_id and ` + visibleLocationCondition("l", audience) + ` order by l.path limit 1)`
}

// topGroupsSelect は、いちばん上のグループ（どのグループの子グループの項目でもないもの）の
// id を 1 列 id で返す select 文である。
const topGroupsSelect = `select g.id from groups g where not exists (select 1 from group_items c where c.child_id = g.id)`

// favoriteGroupsSelect は、お気に入りにしたグループ（どの深さでも）の id を 1 列 id で返す
// select 文である。
const favoriteGroupsSelect = `select group_id as id from group_favorites`

// libraryItemsCTE は項目の表 `items(group_id, id, video_id, path, added_at, mtime, created_at,
// title_key, duration_ms, size_bytes, played_at, favorited_at, watch_state)` と、グループの項目に
// なりうるグループの見せてよい子孫の動画 `gm(group_id, video_id, position)` を定める with 句と、
// その引数を返す。group_id と video_id は、動画の項目では NULL と動画の id、グループの項目では
// グループの id と NULL である。id は keyset とシャッフルの鍵で、動画の項目は動画の id の 2 倍、
// グループの項目はグループの id の 2 倍に 1 を足した値にして、種類の違う項目の id が重ならない
// ようにする（カーソルは種類を運ぶ）。gm の position は再生順の鍵（groupDescendantsCTE）である。
//
// 全子孫が当たったグループ（whole）だけをグループの項目にし、一部だけが当たったグループの当たった
// 動画は、グループに属さない動画と同じく動画の項目にする
// （specs/027-partial-group-search/contracts/library-api.md §1）。動画は 1 回だけ出る。
//
// お気に入りのみ（favoriteOnly）は、グループの項目になりうるものをお気に入りにしたグループ（どの
// 深さでも）にし、動画の項目はお気に入りの動画のうちそれらの whole のグループに入らないものにする
// （specs/035-favorites/data-model.md §5、specs/048-groups/data-model.md「Library items」）。
func libraryItemsCTE(spec listSpec) (string, []any) {
	audience := spec.scope.audience
	cte, args := chosenLocationsCTE(spec.scope, spec.expr)
	conditions, tagArgs := tagConditions(spec)
	args = append(args, tagArgs...)
	where := ""
	if len(conditions) > 0 {
		where = ` where ` + strings.Join(conditions, " and ")
	}
	// 再生可否は決め手に入れず、できた項目に掛ける（research.md R-2）。動画の項目は
	// その動画、グループの項目は子孫の動画のどれか1本が再生できるかで判定する。
	videoPlayable := ""
	groupPlayable := ""
	if spec.playableOnly {
		videoPlayable = ` and ` + playableCondition
		groupPlayable = ` and exists (select 1 from gm pm join videos on videos.id = pm.video_id
			where pm.group_id = hits.group_id and ` + playableCondition + `)`
	}
	// 内容の識別子が空の動画は再生位置を持たない（filteredFrom と同じ扱い）。
	progressJoin := func(videoAlias string) string {
		return ` left join playback_progress p on p.content_key = ` + userKeyExpr(videoAlias) + ` and ` +
			videoAlias + `.content_key <> ''`
	}
	roots := topGroupsSelect
	videoFavorite := ""
	if spec.favoriteOnly {
		roots = favoriteGroupsSelect
		videoFavorite = ` and fav.content_key is not null`
	}
	// 当たった動画は、どのグループにも無いか、一部だけが当たるグループのどれかにあれば動画の項目に
	// なる。別のグループが丸ごと当たってそのカードに入っていても、部分一致のグループからは 1 本で
	// 並ぶ（research.md R-6）。
	// whole は materialized にする。インライン展開されると、相関副問い合わせ（グループの本数）が
	// mv と not exists から当たった動画1本ごとに評価され、グループの本数の2乗で時間が増える
	// （issue 674）。
	cte += `,
	matched as (
		select videos.id as video_id, chosen.path as path
		from chosen join videos on videos.id = chosen.video_id` + where + `),
	` + groupDescendantsCTE(roots, audience) + `,
	gm as materialized (select root_id as group_id, video_id, sort_key as position from gdesc),
	hits as (
		select gm.group_id, count(*) as n from matched
		join gm on gm.video_id = matched.video_id
		group by gm.group_id),
	whole as materialized (
		select hits.group_id from hits
		where hits.n = (select count(*) from gm c where c.group_id = hits.group_id)` + groupPlayable + `),
	mv as (
		select gm.group_id, gm.video_id, v.added_at, v.duration_ms,
			rl.mtime as mtime, ` + fileCreatedAtExpr("rl") + ` as created_at, rl.size_bytes as size_bytes,
			p.updated_at as played_at,
			coalesce(p.completed, 0) as completed, coalesce(p.position_ms, 0) as position_ms
		from gm join whole on whole.group_id = gm.group_id
		join videos v on v.id = gm.video_id` + representativeLocationJoin("gm", audience) + progressJoin("v") + `),
	items as (
		select null as group_id, videos.id * 2 as id, videos.id as video_id, matched.path as path,
			videos.added_at as added_at, loc.mtime as mtime, ` + fileCreatedAtExpr("loc") + ` as created_at,
			loc.title_key as title_key,
			videos.duration_ms as duration_ms, loc.size_bytes as size_bytes, p.updated_at as played_at,
			fav.favorited_at as favorited_at,
			case when p.completed = 1 then 'watched'
				when p.content_key is null or p.position_ms = 0 then 'unwatched'
				else 'inProgress' end as watch_state
		from matched join videos on videos.id = matched.video_id
		join video_locations loc on loc.path = matched.path` + progressJoin("videos") + `
		left join video_favorites fav on fav.content_key = ` + userKeyExpr("videos") + ` and videos.content_key <> ''
		where (not exists (select 1 from gm where gm.video_id = matched.video_id)
			or exists (select 1 from gm where gm.video_id = matched.video_id
				and gm.group_id not in (select group_id from whole)))` +
		videoPlayable + videoFavorite + `
		union all
		select g.id, g.id * 2 + 1, null, null,
			max(mv.added_at), max(mv.mtime), max(mv.created_at), g.title_key,
			sum(mv.duration_ms), sum(mv.size_bytes), max(mv.played_at), ` + groupFavoritedAtExpr("g") + `,
			case when sum(mv.completed = 1 or mv.position_ms > 0) = 0 then 'unwatched'
				when sum(mv.completed = 1) = count(*) then 'watched'
				else 'inProgress' end
		from mv join groups g on g.id = mv.group_id
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
	domain.SortCreatedAsc: `created_at`, domain.SortCreatedDesc: `created_at`,
	domain.SortTitleAsc: `title_key`, domain.SortTitleDesc: `title_key`,
	domain.SortDurationAsc: `duration_ms`, domain.SortDurationDesc: `duration_ms`,
	domain.SortSizeAsc: `size_bytes`, domain.SortSizeDesc: `size_bytes`,
	domain.SortPlayedAsc: `played_at`, domain.SortPlayedDesc: `played_at`,
	domain.SortFavoritedAsc: `favorited_at`, domain.SortFavoritedDesc: `favorited_at`,
	domain.SortRandom: shuffleFunction + `(?, id)`,
}

// itemOrder は並び順 sort の項目の並べ替えを返す。
func itemOrder(sort domain.VideoSort) listOrder {
	order := listOrders[sort]
	order.value = itemOrderValues[sort]
	return order
}

// ListLibrary は見る人（audience）に見せるライブラリの項目1ページを返す
// （GET /api/library、specs/027-partial-group-search/contracts/library-api.md §1）。条件（視聴状態・並べ替え・
// タグ）をゲストが使えるかは呼び出し側が domain.Audience.CheckVideoQuery で確かめる。
//
// タグの存在確認・件数・ページの項目・グループのメンバー・登録フォルダを同じ読み取り
// スナップショット（s.db.read の1取引）から返す。
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
	page, err := listLibraryPageTx(ctx, tx, listSpec{
		scope: libraryScope(audience), expr: domain.ParseSearchQuery(q.Query),
		watch: q.Watch, playableOnly: q.PlayableOnly, tagIDs: tagIDs, favoriteOnly: q.FavoriteOnly,
		sort: q.Sort, seed: q.Seed, cursor: q.Cursor, limit: q.Limit,
	})
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

// itemRow はページの項目1件の鍵である。id は keyset の鍵（libraryItemsCTE）で、動画の項目の
// 動画の id は videoID が持つ。
type itemRow struct {
	groupID sql.NullInt64
	id      int64
	videoID sql.NullInt64
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

	// 件数とページを1つの問い合わせで読む。絞り込んだ項目（filtered）は materialized で1回だけ
	// 組み立て、件数はカーソルを掛ける前の filtered から数える。2回に分けると、全件の項目を
	// 2回組み立てることになる。
	// 引数は SQL の文字列に現れる順（CTE・seed・視聴状態・カーソル・件数）に並べる。
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
		// カーソルの先に項目が無いときは、件数を別に数える（同じ読み取りのスナップショット）。
		if err := tx.QueryRowContext(ctx, cte+` select count(*) from items`+whereWatch,
			append(cteArgs, watchArgs...)...).Scan(&total); err != nil {
			return domain.LibraryPage{}, fmt.Errorf("cannot count items: %w", err)
		}
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
// 所在（chosen）の題名・大きさ・更新時刻で、グループの項目は見せてよい子孫の動画の全部から作る。
func loadLibraryItems(ctx context.Context, tx *sql.Tx, audience domain.Audience, keys []itemRow) ([]domain.LibraryItem, error) {
	var pairs [][2]any
	var groupIDs []int64
	for _, key := range keys {
		if key.groupID.Valid {
			groupIDs = append(groupIDs, key.groupID.Int64)
		} else {
			pairs = append(pairs, [2]any{key.videoID.Int64, key.path.String})
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

// videosAtLocations は (動画の id, 所在のパス) の組ごとに、その所在の題名・大きさ・
// 更新時刻で動画を読み出す（ListVideos の listColumns と同じ写し方）。
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

// extraScanner は scanVideo の列に続く列を extra へ受け取る。
type extraScanner struct {
	rows  *sql.Rows
	extra []any
}

func (s extraScanner) Scan(dest ...any) error {
	return s.rows.Scan(append(dest, s.extra...)...)
}

// LibraryIDs は ListLibrary と同じ条件（並び順・カーソル・件数を除く）に合う項目を、
// 動画の項目の id と、グループの項目ごとのグループの id と再生順の動画の id に分けて、ページングせずに
// 返す（GET /api/library/ids、specs/027-partial-group-search/contracts/library-api.md §2、
// specs/035-favorites/data-model.md §5、specs/048-groups/data-model.md「Library items」）。動画の
// 項目とグループの並びは決めない。「すべて選択」は所有者だけの操作なので、所有者として読む。
func (s *LibraryStore) LibraryIDs(ctx context.Context, q domain.VideoQuery) (domain.LibrarySelection, []int64, error) {
	selection := domain.LibrarySelection{VideoIDs: []int64{}, Groups: []domain.LibraryGroupSelection{}}
	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.LibrarySelection{}, nil, fmt.Errorf("cannot start reading ids: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	tagIDs, missingTagIDs, err := existingTagIDs(ctx, tx, q.TagIDs)
	if err != nil {
		return domain.LibrarySelection{}, nil, err
	}
	cte, args := libraryItemsCTE(listSpec{
		scope: libraryScope(domain.AudienceOwner), expr: domain.ParseSearchQuery(q.Query),
		watch: q.Watch, playableOnly: q.PlayableOnly, tagIDs: tagIDs, favoriteOnly: q.FavoriteOnly,
	})
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
