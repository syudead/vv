package store

// ホームの棚の項目の読み出し（specs/057-home/research.md R-12）。棚の項目はライブラリの項目
// （libraryItemsCTE）と同じ項目・同じ並びだが、ライブラリの問い合わせは棚ごとに見せる動画の表と
// グループの子孫の表を組み立て直し、索引の無い with 句どうしを結ぶたびに自動の索引を作るので、
// 2 万本では画面に入った 8 つの棚で 2 秒を超えた。ここでは 1 回の読み出しの取引の中に、棚によらない
// 表を索引付きの一時表として 1 度だけ作り、棚ごとには当たった動画の id の集合とグループの判定だけを
// 求めて、最初のページの項目だけを読む。
//
//   - hs_base: 見る人に見せる動画 1 本 1 行と、ライブラリの項目の値の列。id が主キーで、利用者データ
//     の鍵（user_key）に索引を持つ。動画が入っているいちばん上のグループ（1 つだけなら group_id、
//     その数が group_count）も持つので、棚ごとにグループの表を引かずに済む
//   - hs_gm・hs_groups: いちばん上のグループごとの見せる子孫の動画（再生順の鍵つき）と、グループの
//     項目の値（本数、最大の日時、合計の長さと大きさ、視聴状態、再生できる子孫の有無）。お気に入りの
//     みの棚があれば、お気に入りにしたグループを根にした _fav の表と列も作る
//   - hs_search: 検索式の条件を持つ棚があるとき、検索式ごとに当たる動画の id。検索式ごとに全文索引
//     から読む
//   - hs_shelved: 棚にない動画の棚を読むとき、どれかのフォルダの棚の実効の規則に動画として一致する
//     動画の id（R-11）
//
// 一時表は取引とともに消える。読み出しの取引は commit せず rollback で終える。

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"

	"github.com/syudead/vv/internal/domain"
)

// homeShelfTables は 1 回の棚の読み出しが作った一時表である。
type homeShelfTables struct {
	// searches は hs_search の検索式の番号ごとの識別子（条件句と引数）である。同じ検索式の段を持つ
	// 棚は 1 つの番号を分け合う。
	searches []string
	// favorite は _fav の表と列を作ったかである。
	favorite bool
}

// homeShelfTableSQL は一時表を作る文 1 つである。
type homeShelfTableSQL struct {
	sql  string
	args []any
}

// newHomeShelfTables は取引 tx に、見る人 audience の棚 specs を読むための一時表を作る。shelves は
// hs_shelved を作るときのフォルダの棚ごとの経路の条件（homeShelfSpecs）で、nil なら作らない。
func newHomeShelfTables(ctx context.Context, tx *sql.Tx, audience domain.Audience, specs, shelves []listSpec) (*homeShelfTables, error) {
	tables := &homeShelfTables{}
	for _, spec := range specs {
		tables.favorite = tables.favorite || spec.itemGroupsFavorite()
	}
	stmts := homeBaseTableSQL(audience, tables.favorite)
	stmts = append(stmts, homeGroupTableSQL(topGroupsSelect, ``)...)
	if tables.favorite {
		stmts = append(stmts, homeGroupTableSQL(favoriteGroupsSelect, `_fav`)...)
	}
	stmts = append(stmts, tables.searchTableSQL(append(append([]listSpec{}, specs...), shelves...))...)
	if shelves != nil {
		stmts = append(stmts, tables.shelvedTableSQL(shelves)...)
	}
	for _, stmt := range stmts {
		if _, err := tx.ExecContext(ctx, stmt.sql, stmt.args...); err != nil {
			return nil, fmt.Errorf("cannot prepare the home shelves read: %w", err)
		}
	}
	return tables, nil
}

// homeBaseTableSQL は hs_base を作る文である。所在は一覧と同じ見せてよい所在のパスの最小（chosen）で、
// 列はライブラリの項目の値（libraryItemsPart の items）と条件が読む videos の列である。グループの列は
// homeGroupTableSQL が埋める。
func homeBaseTableSQL(audience domain.Audience, favorite bool) []homeShelfTableSQL {
	cte, args := chosenLocationsCTEFor(libraryScope(audience), audience, nil)
	groupColumns := `group_id integer, group_count integer not null default 0`
	if favorite {
		groupColumns += `, group_id_fav integer, group_count_fav integer not null default 0`
	}
	return []homeShelfTableSQL{
		{sql: `create temp table hs_base (id integer primary key, content_key text not null, user_key text not null,
			added_at integer, duration_ms integer, playable integer, probe_state text, path text not null,
			mtime integer, created_at integer, title_key text, size_bytes integer, played_at integer,
			favorited_at integer, completed integer not null, position_ms integer not null, watch_state text not null,
			` + groupColumns + `)`},
		{sql: `insert into hs_base (id, content_key, user_key, added_at, duration_ms, playable, probe_state, path, mtime,
			created_at, title_key, size_bytes, played_at, favorited_at, completed, position_ms, watch_state) ` + cte + `,
		keys as (
			select videos.id as id, videos.content_key as content_key, ` + userKeyExpr("videos") + ` as user_key,
				videos.added_at as added_at, videos.duration_ms as duration_ms, videos.playable as playable,
				videos.probe_state as probe_state, chosen.path as path
			from chosen join videos on videos.id = chosen.video_id)
		select k.id, k.content_key, k.user_key, k.added_at, k.duration_ms, k.playable, k.probe_state, k.path,
			loc.mtime, ` + fileCreatedAtExpr("loc") + `, loc.title_key, loc.size_bytes, p.updated_at, fav.favorited_at,
			coalesce(p.completed, 0), coalesce(p.position_ms, 0), ` + videoWatchStateExpr + `
		from keys k join video_locations loc on loc.path = k.path
		left join playback_progress p on p.content_key = k.user_key and k.content_key <> ''
		left join video_favorites fav on fav.content_key = k.user_key and k.content_key <> ''`, args: args},
		{sql: `create index hs_base_user_key on hs_base (user_key)`},
	}
}

// homeGroupTableSQL は、roots（1 列 id の select 文）のグループごとの見せる子孫の動画 hs_gm<suffix> と、
// グループの項目の値 hs_groups<suffix> を作り、hs_base の group_id<suffix>（動画が入っている根が 1 つの
// ときその id）と group_count<suffix>（入っている根の数）を埋める文である。子孫の動画は内容の鍵を
// hs_base の利用者データの鍵に結んで求める（groupDescendantsCTE の shownVideoForKeyExpr と同じ動画に
// なる）。
func homeGroupTableSQL(roots, suffix string) []homeShelfTableSQL {
	gm, groups := `hs_gm`+suffix, `hs_groups`+suffix
	return []homeShelfTableSQL{
		{sql: `create temp table ` + gm + ` (group_id integer not null, video_id integer not null, position text not null)`},
		{sql: `insert into ` + gm + ` with ` + groupDescendantsCTEFrom(roots, `b.id`, ` join hs_base b on b.user_key = gi.content_key`) + `
		select root_id, video_id, sort_key from gdesc`},
		{sql: `create index ` + gm + `_video on ` + gm + ` (video_id, group_id)`},
		{sql: `create index ` + gm + `_group on ` + gm + ` (group_id, video_id)`},
		{sql: `create temp table ` + groups + ` as
		select gm.group_id as group_id, count(*) as n, g.title_key as title_key,
			max(videos.added_at) as added_at, max(videos.mtime) as mtime, max(videos.created_at) as created_at,
			sum(videos.duration_ms) as duration_ms, sum(videos.size_bytes) as size_bytes, max(videos.played_at) as played_at,
			` + groupFavoritedAtExpr("g") + ` as favorited_at,
			` + groupWatchStateExpr("videos.completed", "videos.position_ms") + ` as watch_state,
			max(` + playableCondition + `) as playable
		from ` + gm + ` gm join hs_base videos on videos.id = gm.video_id join groups g on g.id = gm.group_id
		group by gm.group_id`},
		{sql: `update hs_base set group_id` + suffix + ` = vg.group_id, group_count` + suffix + ` = vg.n
		from (select video_id, min(group_id) as group_id, count(*) as n from ` + gm + ` group by video_id) as vg
		where vg.video_id = hs_base.id`},
	}
}

// searchTableSQL は specs のうち検索式の段を持つ棚の検索式を tables.searches に集め、hs_search を作る文を
// 返す。検索式ごとに、見せる動画のうちその式のすべてに当たる動画の id を、集まりに属さない動画は登録の
// 所在から、集まりの動画は同じ集まりのどれかの動画の登録の所在から読む（chosenLocationsFrom と同じ
// 決め方）。ゲストに見せる動画は公開なので、見せてよい所在の条件のうち公開の分は hs_base に入って
// いることで満たされている。検索式の棚が無ければ文は無い。
func (t *homeShelfTables) searchTableSQL(specs []listSpec) []homeShelfTableSQL {
	var stmts []homeShelfTableSQL
	for _, spec := range specs {
		if !spec.hasSearch() {
			continue
		}
		search, identity := homeShelfSearchOf(spec)
		if t.searchIndex(identity) >= 0 {
			continue
		}
		number := strconv.Itoa(len(t.searches))
		t.searches = append(t.searches, identity)
		stmts = append(stmts,
			homeShelfTableSQL{sql: `insert or ignore into hs_search
			select l.video_id, ` + number + ` from video_locations l join hs_base b on b.id = l.video_id
			left join video_bundle_members sbm on sbm.content_key = b.content_key and b.content_key <> ''
			where sbm.bundle_id is null and ` + registeredLocationCondition("l") + ` and ` + search.location, args: search.locationArgs},
			homeShelfTableSQL{sql: `insert or ignore into hs_search
			select b.id, ` + number + ` from hs_base b join video_bundle_members sbm on sbm.content_key = b.content_key
			where b.content_key <> '' and ` + search.bundleExists(), args: search.bundleArgs})
	}
	if len(stmts) == 0 {
		return nil
	}
	create := homeShelfTableSQL{sql: `create temp table hs_search (video_id integer not null, search integer not null,
		primary key (video_id, search)) without rowid`}
	return append([]homeShelfTableSQL{create}, stmts...)
}

// homeShelfSearchOf は棚 spec の検索式の段の条件句と、その識別子を返す。検索式は所有者の意味で照合する
// （specs/052-smart-folders/research.md R-11）。
func homeShelfSearchOf(spec listSpec) (searchClauses, string) {
	search := searchClausesFor(domain.AudienceOwner, spec.searchExprs())
	return search, fmt.Sprintf("%s\x00%v", search.location, search.locationArgs)
}

// searchIndex は識別子 identity の検索式の番号を返す。無ければ -1 である。
func (t *homeShelfTables) searchIndex(identity string) int {
	for i, known := range t.searches {
		if known == identity {
			return i
		}
	}
	return -1
}

// videoCondition は、hs_base の行（別名 videos）が棚 spec の条件 match（smartMatchOf）に当たる条件句と
// その引数を返す。検索式の段は hs_search で確かめる。
func (t *homeShelfTables) videoCondition(spec listSpec, match smartMatch) (string, []any) {
	if !spec.hasSearch() {
		return match.filter, match.args
	}
	_, identity := homeShelfSearchOf(spec)
	number := t.searchIndex(identity)
	if number < 0 {
		panic("home shelf search is not in hs_search: " + identity)
	}
	return match.filter + ` and videos.id in (select video_id from hs_search where search = ` + strconv.Itoa(number) + `)`, match.args
}

// shelvedTableSQL は hs_shelved を作る文である。棚ごとに、見せる動画のうちその棚の実効の規則に動画として
// 一致する動画（smartMatchFor。shelvedVideos と同じ）を足し、同じ条件の棚は 1 度だけ足す。
func (t *homeShelfTables) shelvedTableSQL(shelves []listSpec) []homeShelfTableSQL {
	stmts := []homeShelfTableSQL{{sql: `create temp table hs_shelved (id integer primary key)`}}
	seen := map[string]bool{}
	for _, shelf := range shelves {
		condition, args := t.videoCondition(shelf, smartMatchFor(shelf, `s`))
		identity := fmt.Sprintf("%s\x00%v", condition, args)
		if seen[identity] {
			continue
		}
		seen[identity] = true
		stmts = append(stmts, homeShelfTableSQL{
			sql:  `insert or ignore into hs_shelved select videos.id from hs_base videos where ` + condition,
			args: args,
		})
	}
	return stmts
}

// homeShelfGroupNames は棚 1 つが読むグループの表と列の名前である。
type homeShelfGroupNames struct {
	gm, groups, groupID, groupCount string
}

// groupNames は棚 spec のグループの表と列の名前（いちばん上のグループか、お気に入りの根のもの）を返す。
func (t *homeShelfTables) groupNames(spec listSpec) homeShelfGroupNames {
	suffix := ``
	if spec.itemGroupsFavorite() {
		suffix = `_fav`
	}
	return homeShelfGroupNames{gm: `hs_gm` + suffix, groups: `hs_groups` + suffix,
		groupID: `group_id` + suffix, groupCount: `group_count` + suffix}
}

// homeShelfItemsSQL は棚 spec の最初のページを読む文とその引数を返す。項目の決め方はライブラリの項目
// （libraryItemsPart）と同じである。当たった動画（matched）を、入っている根が 1 つならその id とともに
// 求め、根ごとの当たった本数（hits）が子孫の本数と等しい根を全部が当たったグループ（whole）にする。
// 動画の項目は、どの根にも入らないか、全部が当たっていない根のどれかに入る当たった動画で、グループの
// 項目は whole の根の値（hs_groups）である。入っている根が 2 つ以上の動画だけ、根の表（hs_gm）を引く。
// 項目は並べ替えの値を添えて 1 度だけ組み立て（filtered）、数と最初の limit 件を読む（listLibraryPageTx
// と同じ形）。
func (t *homeShelfTables) homeShelfItemsSQL(spec listSpec, order listOrder, limit int) (string, []any) {
	names := t.groupNames(spec)
	condition, args := t.videoCondition(spec, smartRuleMatch(spec, `s`))
	if spec.unshelved {
		condition += ` and videos.id not in (select id from hs_shelved)`
	}
	videoFilter, groupFilter := "", ""
	if spec.anyLevel(func(level listSpec) bool { return level.playableOnly }) {
		videoFilter += ` and ` + playableCondition
		groupFilter += ` and g.playable`
	}
	if spec.itemGroupsFavorite() {
		videoFilter += ` and videos.favorited_at is not null`
	}
	whole, videos := homeShelfSetsSQL(names, condition, groupFilter)
	if order.seeded {
		args = append(args, spec.seed)
	}
	watch, watchArgs := itemsWhere(spec)
	args = append(append(args, watchArgs...), limit)
	return `with ` + whole + `,
	items as (
		select null as group_id, videos.id * 2 as id, videos.id as video_id, videos.path as path, videos.added_at as added_at,
			videos.mtime as mtime, videos.created_at as created_at, videos.title_key as title_key, videos.duration_ms as duration_ms,
			videos.size_bytes as size_bytes, videos.played_at as played_at, videos.favorited_at as favorited_at,
			videos.watch_state as watch_state
		from ` + videos + videoFilter + `
		union all
		select g.group_id, g.group_id * 2 + 1, null, null, g.added_at, g.mtime, g.created_at, g.title_key, g.duration_ms,
			g.size_bytes, g.played_at, g.favorited_at, g.watch_state
		from whole w join ` + names.groups + ` g on g.group_id = w.group_id),
	filtered as materialized (select items.*, ` + order.value + ` as sort_value from items` + watch + `)
	select group_id, id, video_id, path, sort_value, (select count(*) from filtered) from filtered
	order by ` + order.orderBy() + ` limit ?`, args
}

// homeShelfSetsSQL は、条件 condition（hs_base の行 videos に掛ける）の棚の、全部が当たったグループ
// whole を定める with 句の部品と、動画の項目になる動画（別名 videos）の from 句を返す。当たった動画
// （matched）は id と根の列だけで 1 度求め、根ごとの当たった本数（hits）は根が 1 つの動画はその列で、
// 根が 2 つ以上の動画だけ hs_gm を引いて数える。動画の項目は、どの根にも入らないか、全部は当たっていない
// 根のどれかに入る当たった動画である。条件が無い棚（最近追加した棚）では見せる動画がすべて当たるので、
// 再生可否の絞り込みが無ければどのグループも whole で、動画の項目はどの根にも入らない動画である。
func homeShelfSetsSQL(names homeShelfGroupNames, condition, groupFilter string) (whole, videos string) {
	if condition == `1` && groupFilter == "" {
		return `whole as (select group_id from ` + names.groups + `)`,
			`hs_base videos where videos.` + names.groupCount + ` = 0`
	}
	return `matched as materialized (
		select videos.id as id, videos.` + names.groupID + ` as group_id, videos.` + names.groupCount + ` as group_count
		from hs_base videos where ` + condition + `),
	hits as (
		select group_id, count(*) as n from (
			select group_id from matched where group_count = 1
			union all
			select gm.group_id from matched join ` + names.gm + ` gm on gm.video_id = matched.id where matched.group_count > 1)
		group by group_id),
	whole as materialized (
		select g.group_id from hits join ` + names.groups + ` g on g.group_id = hits.group_id where hits.n = g.n` + groupFilter + `)`,
		`matched join hs_base videos on videos.id = matched.id
		where (matched.group_count = 0
			or (matched.group_count = 1 and matched.group_id not in (select group_id from whole))
			or (matched.group_count > 1 and exists (select 1 from ` + names.gm + ` gm where gm.video_id = matched.id
				and gm.group_id not in (select group_id from whole))))`
}

// homeShelfFirstPage は一時表 tables から棚 spec の最初のページ（listLibraryPageTx の最初のページと同じ
// 項目と数。次のページのカーソルは持たない）を読む。
func (t *homeShelfTables) homeShelfFirstPage(ctx context.Context, tx *sql.Tx, spec listSpec) (domain.LibraryPage, error) {
	if !spec.sort.Valid() {
		spec.sort = domain.SortAddedDesc
	}
	order, limit := itemOrder(spec.sort), normalizeLimit(spec.limit)
	query, args := t.homeShelfItemsSQL(spec, order, limit)
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return domain.LibraryPage{}, fmt.Errorf("cannot read the home shelf: %w", err)
	}
	defer func() { _ = rows.Close() }()
	total := 0
	var keys []itemRow
	var values []any
	for rows.Next() {
		var key itemRow
		var value any
		if err := rows.Scan(&key.groupID, &key.id, &key.videoID, &key.path, &value, &total); err != nil {
			return domain.LibraryPage{}, fmt.Errorf("cannot read the home shelf: %w", err)
		}
		keys = append(keys, key)
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return domain.LibraryPage{}, fmt.Errorf("cannot read the home shelf: %w", err)
	}
	if err := rows.Close(); err != nil {
		return domain.LibraryPage{}, fmt.Errorf("cannot close the home shelf: %w", err)
	}
	items, _, err := libraryPageItems(ctx, tx, spec, order, keys, values, limit)
	if err != nil {
		return domain.LibraryPage{}, err
	}
	return domain.LibraryPage{Items: items, Total: total, Limit: limit}, nil
}
