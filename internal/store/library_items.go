package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
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

// memberConditions はメンバー単位の絞り込み（再生可否と matchConditions）を、videos に
// 対する条件句の並びと引数にする（GET /api/videos の一覧が使う）。
func memberConditions(spec listSpec) ([]string, []any) {
	var conditions []string
	if spec.playableOnly {
		conditions = append(conditions, playableCondition)
	}
	matches, args := matchConditions(spec)
	return append(conditions, matches...), args
}

// matchConditions は当たった動画（matched）を決める段の絞り込み、つまりタグの AND、
// 人物・団体、追加日と長さの絞り込みを、videos に対する条件句の並びと引数にする
// （specs/047-video-work-info/data-model.md の Lists、specs/052-smart-folders/research.md R-4）。
// spec.and の各段の絞り込みも同じように足す（specs/052-smart-folders/research.md R-5）。
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

// videoUserKey は videos の利用者データの鍵の式を返す。spec.userKeyColumn があればその列で、無ければ
// userKeyExpr である。
func (s listSpec) videoUserKey() string {
	if s.userKeyColumn != "" {
		return s.userKeyColumn
	}
	return userKeyExpr("videos")
}

// levels は spec 自身と、重ねて掛ける絞り込み（spec.and）の各段を返す。
func (s listSpec) levels() []listSpec {
	return append([]listSpec{s}, s.and...)
}

// anyLevel は、どれかの段で test が真かを返す。再生可否とお気に入りのみは、どの段が求めても
// 掛ける。
func (s listSpec) anyLevel(test func(listSpec) bool) bool {
	return slices.ContainsFunc(s.levels(), test)
}

// searchExprs は各段の検索式を返す。
func (s listSpec) searchExprs() []domain.SearchExpr {
	exprs := make([]domain.SearchExpr, 0, 1+len(s.and))
	for _, level := range s.levels() {
		exprs = append(exprs, level.expr)
	}
	return exprs
}

// secondsPerDay は追加日の日数を videos.added_at（Unix 秒）の差にする係数である。
const secondsPerDay = 24 * 60 * 60

// addedAndLengthConditions は追加日と長さの絞り込みを、videos に対する条件句の並びと引数にする。
// 追加日は、一覧を読み始めた時刻（listSpec.now）からその日数前の時刻以後に追加した動画に当たる。
// 件数・ページ・空ページの再集計が同じ基準時刻で数えるよう、SQL の unixepoch() は使わない。
// 長さは画面の表示（web/src/lib/format.ts の formatDuration）と同じく秒未満を切り捨てた秒で比べ、両端を含む。
// 長さの分からない（duration_ms が NULL の）動画はどちらの端にも当たらない
// （specs/052-smart-folders/research.md R-4）。
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
		// 切り捨てた秒が上限以下なのは、ミリ秒が「上限の次の秒」未満であることと同じである。
		conditions = append(conditions, `videos.duration_ms < ?`)
		args = append(args, int64(*spec.maxDurationSec+1)*1000)
	}
	return conditions, args
}

// workInfoConditions は人物と団体の絞り込みを、videos に対する条件句の並びと引数にする。
// 役割は問わず、利用者データの鍵（userKey、listSpec.videoUserKey）に結び付いた行があれば当たる。
// 実体の無い id は何にも当たらない（specs/047-video-work-info/data-model.md の Lists）。
func workInfoConditions(spec listSpec, userKey string) ([]string, []any) {
	var conditions []string
	var args []any
	// 内容の識別子が空の動画は利用者データを持たない（videoHasTagCondition と同じ扱い）。
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

// tagConditions はタグの AND とタグの種類の絞り込みを、videos に対する条件句の並びと引数にする。
// 手で付けた分とフォルダ名から付いている分のどちらでも当たる（017 の data-model.md §4、
// specs/051-tag-kinds/data-model.md の Rules）。userKey は videos の利用者データの鍵の式
// （listSpec.videoUserKey）である。
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

// videoWatchStateExpr は動画 1 本の視聴状態（domain.ClassifyWatch と同じ定義）を、再生の記録
// （別名 p、left join）から 'unwatched'・'inProgress'・'watched' にする式である。
const videoWatchStateExpr = `case when p.completed = 1 then 'watched'
				when p.content_key is null or p.position_ms = 0 then 'unwatched'
				else 'inProgress' end`

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
	searchAudience := audience
	if spec.ownerSearch {
		searchAudience = domain.AudienceOwner
	}
	cte, args := chosenLocationsCTEFor(spec.scope, searchAudience, spec.searchExprs())
	shelved, shelvedArgs, unshelved := unshelvedCTE(spec)
	cte += shelved
	args = append(args, shelvedArgs...)
	conditions, matchArgs := matchConditions(spec)
	if unshelved != "" {
		conditions = append(conditions, unshelved)
	}
	args = append(args, matchArgs...)
	where := ""
	if len(conditions) > 0 {
		where = ` where ` + strings.Join(conditions, " and ")
	}
	// 再生可否は決め手に入れず、できた項目に掛ける（research.md R-2）。動画の項目は
	// その動画、グループの項目は子孫の動画のどれか1本が再生できるかで判定する。
	videoPlayable := ""
	groupPlayable := ""
	if spec.anyLevel(func(level listSpec) bool { return level.playableOnly }) {
		videoPlayable = ` and ` + playableCondition
		groupPlayable = ` and exists (select 1 from gm pm join videos on videos.id = pm.video_id
			where pm.group_id = hits.group_id and ` + playableCondition + `)`
	}
	// 内容の識別子が空の動画は再生位置を持たない（filteredFrom と同じ扱い）。
	progressJoin := func(videoAlias string) string {
		return ` left join playback_progress p on p.content_key = ` + userKeyExpr(videoAlias) + ` and ` +
			videoAlias + `.content_key <> ''`
	}
	folderItems, folderArgs := smartFolderItemsSQL(spec.smartFolders)
	roots := topGroupsSelect
	videoFavorite := ""
	if spec.anyLevel(func(level listSpec) bool { return level.favoriteOnly }) {
		roots = favoriteGroupsSelect
		videoFavorite = ` and fav.content_key is not null`
	}
	// 当たった動画は、どのグループにも無いか、一部だけが当たるグループのどれかにあれば動画の項目に
	// なる。別のグループが丸ごと当たってそのカードに入っていても、部分一致のグループからは 1 本で
	// 並ぶ（research.md R-6）。
	// carded は、グループに入り、そのどれもが丸ごと当たった（カードにだけ出る）動画である。
	// 動画の項目はこれに入らない当たった動画で、動画ごとに gm を読む相関副問い合わせにすると、
	// グループに入った動画の本数の 2 乗で時間が増える（specs/052-smart-folders/research.md R-13）。
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
	carded as materialized (
		select video_id from gm where group_id in (select group_id from whole)
		except
		select video_id from gm where group_id not in (select group_id from whole)),
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
			fav.favorited_at as favorited_at, ` + videoWatchStateExpr + ` as watch_state
		from matched join videos on videos.id = matched.video_id
		join video_locations loc on loc.path = matched.path` + progressJoin("videos") + `
		left join video_favorites fav on fav.content_key = ` + userKeyExpr("videos") + ` and videos.content_key <> ''
		where matched.video_id not in (select video_id from carded)` +
		videoPlayable + videoFavorite + `
		union all
		select g.id, g.id * 2 + 1, null, null,
			max(mv.added_at), max(mv.mtime), max(mv.created_at), g.title_key,
			sum(mv.duration_ms), sum(mv.size_bytes), max(mv.played_at), ` + groupFavoritedAtExpr("g") + `,
			case when sum(mv.completed = 1 or mv.position_ms > 0) = 0 then 'unwatched'
				when sum(mv.completed = 1) = count(*) then 'watched'
				else 'inProgress' end
		from mv join groups g on g.id = mv.group_id
		group by g.id` + folderItems + `)`
	return cte, append(args, folderArgs...)
}

// unshelvedCTE は、spec.unshelved のときに当たった動画（matched）に掛ける、ホームのどの棚にも
// 一致しない動画の条件を作る（specs/057-home/research.md R-11）。返すのは with 句に続けて足す項目
// （先頭の `,` を含む）とその引数、videos に対する条件句で、絞り込まなければどれも空である。
// 棚ごとの一致はカードの読み出しと同じ smartMatchFor で求め、その和を shelved に 1 度だけ求めて
// 否定する。ライブラリの chosen は検索式で絞るので、smart_base の所在は別の名前で定める。
func unshelvedCTE(spec listSpec) (string, []any, string) {
	if !spec.unshelved || len(spec.shelves) == 0 {
		return "", nil, ""
	}
	cte, args := smartBaseItems(spec.scope.audience, `unshelved_chosen`)
	cte = `,
	` + cte
	matches := make([]string, 0, len(spec.shelves))
	var matchArgs []any
	for i, shelf := range spec.shelves {
		match := smartMatchFor(shelf, `us`+strconv.Itoa(i))
		if match.search != "" {
			cte += `,
	` + match.search
			args = append(args, match.searchArgs...)
		}
		matches = append(matches, `(`+match.condition+`)`)
		matchArgs = append(matchArgs, match.args...)
	}
	cte += `,
	shelved as materialized (select videos.id as id from smart_base videos where ` + strings.Join(matches, " or ") + `)`
	return cte, append(args, matchArgs...), `videos.id not in (select id from shelved)`
}

// homeShelfSpecs は、ホームのフォルダの棚ごとの経路の条件を、見る人 audience の範囲と基準時刻 now の
// 絞り込み（smartFolderSpec）にして棚の並びの順に返す。名指す実体の無い棚は何にも一致しないので
// 入れない（specs/052-smart-folders/research.md R-9）。
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

// itemWatchConditions は、spec と重ねて掛ける各段（spec.and）の視聴状態の絞り込みを、items に
// 対する 1 つの条件句にする（各段の句の AND）。どの段も絞り込まないときは空の句を返す。
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

// itemOrder は並び順 sort の項目の並べ替えを返す。どの並び順でも値の無い項目を向きに関係なく末尾に
// 置く（nullable）。動画の項目で値の無いものは listOrders と同じく長さ・再生日時・お気に入りにした
// 日時だけで、ほかには一致する動画の無いスマートフォルダの項目がある（specs/052-smart-folders/
// data-model.md「Library items」）。
func itemOrder(sort domain.VideoSort) listOrder {
	order := listOrders[sort]
	order.value = itemOrderValues[sort]
	order.nullable = true
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
	spec := libraryListSpec(audience, q, tagIDs)
	if spec.smartFolders, err = librarySmartFolders(ctx, tx, audience, q, spec.now); err != nil {
		return domain.LibraryPage{}, err
	}
	if spec.unshelved {
		if spec.shelves, err = homeShelfSpecs(ctx, tx, audience, spec.now); err != nil {
			return domain.LibraryPage{}, err
		}
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

// itemRow はページの項目1件の鍵である。id は keyset の鍵（libraryItemsCTE）で、動画の項目の
// 動画の id は videoID が持つ。スマートフォルダの項目は id が負で、フォルダの id はその符号を
// 変えた値である（smartFolderItemsSQL）。
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
	watchClause, watchArgs := itemWatchConditions(spec)
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

	page := domain.LibraryPage{Total: total, Limit: limit}
	page.Items, page.NextCursor, err = libraryPageItems(ctx, tx, spec, order, keys, values, limit)
	if err != nil {
		return domain.LibraryPage{}, err
	}
	return page, nil
}

// libraryPageItems は、カーソルより後ろの項目の鍵 keys（並べ順に limit+1 件まで）と、それぞれの
// 並べ替えの値 values から、limit 件の項目と次のページのカーソル（無ければ空）を返す。
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

// loadLibraryItems はページの項目の鍵から項目を組み立てる。動画の項目は一覧に出す
// 所在（chosen）の題名・大きさ・更新時刻で、グループの項目は見せてよい子孫の動画の全部から作る。
// スマートフォルダの項目は、一覧が混ぜたフォルダ folders の要約から取る。
func loadLibraryItems(ctx context.Context, tx *sql.Tx, audience domain.Audience, keys []itemRow, folders []librarySmartFolder) ([]domain.LibraryItem, error) {
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
	groups, err := loadGroups(ctx, tx, audience, groupIDs)
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
// 動画の項目の id と、グループの項目ごとのグループの id と再生順の動画の id と、スマートフォルダの
// 項目の id に分けて、ページングせずに返す（GET /api/library/ids、specs/027-partial-group-search/contracts/library-api.md §2、
// specs/035-favorites/data-model.md §5、specs/048-groups/data-model.md「Library items」）。動画の
// 項目とグループの並びは決めない。「すべて選択」は所有者だけの操作なので、所有者として読む。
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
	// スマートフォルダの項目は条件で落ちない（出すのは絞り込みが無いかお気に入りのみのときだけ）ので、
	// 項目の問い合わせに混ぜず、出すフォルダの id をそのまま返す。所有者には空のフォルダも出す。
	folderIDs, err := librarySmartFolderIDs(ctx, tx, q)
	if err != nil {
		return domain.LibrarySelection{}, nil, err
	}
	selection.SmartFolderIDs = append(selection.SmartFolderIDs, folderIDs...)
	// 並び順・カーソル・件数は使わない（libraryItemsCTE は読まない）。
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
