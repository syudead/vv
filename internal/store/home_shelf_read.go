package store

// ホームの棚の項目の読み出し（specs/057-home/research.md R-12）。棚の項目はライブラリの項目
// （libraryItemsCTE）と同じ項目・同じ並びだが、ライブラリの問い合わせのように全動画に規則を掛けて項目を
// 組み立ててから並べるのではなく、基底の表の索引を並び順になぞり（home_shelf_top.go）、なぞった動画
// 1 本ごとに見せてよいか・規則に当たるか・どの根（いちばん上のグループ）に入るかを索引で引く。根の判定
// （子孫がすべて当たるか）と根のグループの項目の値は、なぞって現れた根についてだけ、永続のグループの
// 構造の表（group_closure、group_root_values。group_closure.go）から求める。読み出しのたびに全動画や
// 全グループに触れる段は無い。
//
// 並べ替えの値がグループの子孫 1 本から決まらない並び順（合計の長さと大きさ、グループ自身の題名と
// お気に入りにした日時、シャッフル）だけは、当たった動画を全部読んで項目を組み立てて並べる
// （assemblePage）。

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/syudead/vv/internal/domain"
)

// homeShelfRule は棚 1 つの条件を SQL の句にしたものである。句はどれも videos の行（別名 videos）に掛ける。
type homeShelfRule struct {
	spec listSpec
	// condition は当たった動画の条件句（ライブラリの matched と同じ。無ければ空）で、args はその引数。
	condition string
	args      []any
	// favorite は根をお気に入りにしたグループにする（itemGroupsFavorite）。
	favorite bool
	// playable・favoriteOnly・watches は項目に掛ける絞り込みである。
	playable     bool
	favoriteOnly bool
	watches      []domain.WatchFilter
}

// homeShelfReader は 1 回の棚の読み出しである。
type homeShelfReader struct {
	ctx      context.Context
	tx       *sql.Tx
	audience domain.Audience
	now      int64
	// shelves はフォルダの棚ごとの経路の条件（棚にない動画の条件が読む）。nil なら未読。
	shelves []listSpec
	// searches は検索式の段の識別子ごとの、当たる動画の id の JSON の配列である。検索式の段は全文索引
	// から 1 度だけ読み、条件には配列を渡す（まとまりごとに読み直さない）。
	searches map[string]string
}

// videoWatchStateSQL は動画（別名 videos）の視聴状態の式で、videoWatchStateExpr と同じ決め方である。
func videoWatchStateSQL(userKey string) string {
	return `coalesce((select case when p.completed = 1 then 'watched' when p.position_ms = 0 then 'unwatched' else 'inProgress' end
		from playback_progress p where p.content_key = ` + userKey + ` and videos.content_key <> ''), 'unwatched')`
}

// videoFavoriteSQL は動画（別名 videos）がお気に入りかの式である。
func videoFavoriteSQL(userKey string) string {
	return `exists (select 1 from video_favorites fav where fav.content_key = ` + userKey + ` and videos.content_key <> '')`
}

// searchVideosSelect は、見る人 scope の見せる動画のうち検索式 exprs のすべてに当たるものの id を返す
// select 文とその引数を返す（chosenLocationsFrom と同じ決め方）。集まりに属さない動画は所在の表の索引
// （全文索引と事実の名前）から、集まりの動画は同じ集まりのどれかの動画の登録の所在から読む。
func searchVideosSelect(scope locationScope, exprs []domain.SearchExpr) (string, []any) {
	search := searchClausesFor(domain.AudienceOwner, exprs)
	scopeClause, scopeArgs := scope.condition("l")
	args := append(append([]any{}, scopeArgs...), search.locationArgs...)
	args = append(args, search.bundleArgs...)
	return `select l.video_id from video_locations l join videos sv on sv.id = l.video_id
			left join video_bundle_members sbm on sbm.content_key = sv.content_key and sv.content_key <> ''
			where sbm.bundle_id is null and ` + scopeClause + ` and ` + search.location + `
		union
		select sv.id from videos sv join video_bundle_members sbm on sbm.content_key = sv.content_key and sv.content_key <> ''
			where sv.id = ` + effectiveRepresentativeExpr("sbm.bundle_id", scope.audience) + ` and ` +
		visibleVideoCondition("sv", scope.audience) + ` and ` + search.bundleExists(), args
}

// searchSet は spec の検索式の段に当たる動画の id の JSON の配列を返す。同じ検索式は 1 度だけ読む。
func (r *homeShelfReader) searchSet(spec listSpec) (string, error) {
	search := searchClausesFor(domain.AudienceOwner, spec.searchExprs())
	identity := fmt.Sprintf("%v\x00%s\x00%v", r.audience, search.location, search.locationArgs)
	if set, ok := r.searches[identity]; ok {
		return set, nil
	}
	query, args := searchVideosSelect(spec.scope, spec.searchExprs())
	ids, err := queryInt64s(r.ctx, r.tx, query, args...)
	if err != nil {
		return "", fmt.Errorf("cannot read the search shelf videos: %w", err)
	}
	encoded, err := json.Marshal(ids)
	if err != nil {
		return "", fmt.Errorf("cannot encode the search shelf videos: %w", err)
	}
	if r.searches == nil {
		r.searches = map[string]string{}
	}
	r.searches[identity] = string(encoded)
	return string(encoded), nil
}

// matchedCondition は、spec の経路のどの段の条件にも当たる動画（ライブラリの matched）の条件句と引数を
// 返す。条件が無ければ空である。
func (r *homeShelfReader) matchedCondition(spec listSpec) (string, []any, error) {
	conditions, args := matchConditions(spec)
	if spec.hasSearch() {
		set, err := r.searchSet(spec)
		if err != nil {
			return "", nil, err
		}
		conditions = append(conditions, `videos.id in (select value from json_each(?))`)
		args = append(args, set)
	}
	return strings.Join(conditions, " and "), args, nil
}

// perVideoCondition は spec の実効の規則に動画として一致する条件（smartMatchFor と同じ。再生可否・お気に
// 入りのみ・視聴状態も動画に掛ける）の句と引数を返す。
func (r *homeShelfReader) perVideoCondition(spec listSpec) (string, []any, error) {
	condition, args, err := r.matchedCondition(spec)
	if err != nil {
		return "", nil, err
	}
	conditions := []string{}
	if condition != "" {
		conditions = append(conditions, condition)
	}
	userKey := userKeyExpr("videos")
	if spec.anyLevel(func(level listSpec) bool { return level.playableOnly }) {
		conditions = append(conditions, playableCondition)
	}
	if spec.itemGroupsFavorite() {
		conditions = append(conditions, videoFavoriteSQL(userKey))
	}
	for _, level := range spec.levels() {
		if level.watch != "" {
			conditions = append(conditions, videoWatchStateSQL(userKey)+` = ?`)
			args = append(args, string(level.watch))
		}
	}
	if len(conditions) == 0 {
		return `1`, nil, nil
	}
	return strings.Join(conditions, " and "), args, nil
}

// ruleOf は棚 spec の条件を句にする。棚にない動画の棚は、どのフォルダの棚の実効の規則にも動画として一致
// しない条件（R-11。条件が NULL の棚も一致しないと見る）を足す。
func (r *homeShelfReader) ruleOf(spec listSpec) (homeShelfRule, error) {
	rule := homeShelfRule{spec: spec, favorite: spec.itemGroupsFavorite(), favoriteOnly: spec.itemGroupsFavorite(),
		playable: spec.anyLevel(func(level listSpec) bool { return level.playableOnly })}
	var err error
	if rule.condition, rule.args, err = r.matchedCondition(spec); err != nil {
		return homeShelfRule{}, err
	}
	for _, level := range spec.levels() {
		if level.watch != "" {
			rule.watches = append(rule.watches, level.watch)
		}
	}
	if spec.unshelved {
		if r.shelves == nil {
			shelves, err := homeShelfSpecs(r.ctx, r.tx, r.audience, r.now)
			if err != nil {
				return homeShelfRule{}, err
			}
			if shelves == nil {
				shelves = []listSpec{}
			}
			r.shelves = shelves
		}
		conditions := []string{}
		if rule.condition != "" {
			conditions = append(conditions, rule.condition)
		}
		for _, shelf := range r.shelves {
			condition, args, err := r.perVideoCondition(shelf)
			if err != nil {
				return homeShelfRule{}, err
			}
			conditions = append(conditions, `(`+condition+`) is not true`)
			rule.args = append(rule.args, args...)
		}
		rule.condition = strings.Join(conditions, " and ")
	}
	return rule, nil
}

// watchShown は視聴状態 state が各段の絞り込みのすべてに等しいかを返す（itemWatchConditions と同じ）。
func (rule homeShelfRule) watchShown(state string) bool {
	for _, watch := range rule.watches {
		if string(watch) != state {
			return false
		}
	}
	return true
}

// homeShelfRow はなぞった動画 1 本である。
type homeShelfRow struct {
	videoID    int64
	path       string
	userKey    string
	value      any
	watch      string
	favorited  bool
	playable   bool
	roots      []int64
	addedAt    int64
	mtime      int64
	createdAt  int64
	durationMs sql.NullInt64
	sizeBytes  sql.NullInt64
	titleKey   string
	favoriteAt sql.NullInt64
	playedAt   sql.NullInt64
}

// rowColumns は、なぞる問い合わせが動画 1 本について返す列である（value は呼び出し側が足す）。要らない列
// （絞り込みの無い視聴状態やお気に入り、組み立ての経路だけが使う値）は定数にして、行ごとの副問い合わせを
// 減らす。
func (r *homeShelfReader) rowColumns(rule homeShelfRule, assemble bool) string {
	userKey := userKeyExpr("videos")
	roots := `(select group_concat(c.root_id) from group_closure c where c.content_key = ` + userKey + `
		and not exists (select 1 from group_items pg where pg.child_id = c.root_id))`
	if rule.favorite {
		roots = `(select group_concat(c.root_id) from group_closure c join group_favorites gf on gf.group_id = c.root_id
			where c.content_key = ` + userKey + `)`
	}
	watch, favorite := `''`, `0`
	if len(rule.watches) > 0 {
		watch = videoWatchStateSQL(userKey)
	}
	if rule.favoriteOnly {
		favorite = videoFavoriteSQL(userKey)
	}
	values := `null, null, null, null, null, null, null, null`
	if assemble {
		values = `videos.added_at, loc.mtime, ` + fileCreatedAtExpr("loc") + `, videos.duration_ms, loc.size_bytes, loc.title_key,
		(select fav.favorited_at from video_favorites fav where fav.content_key = ` + userKey + `),
		(select p.updated_at from playback_progress p where p.content_key = ` + userKey + ` and videos.content_key <> '')`
	}
	return `videos.id, loc.path, ` + userKey + `, ` + watch + `, ` + favorite + `, ` + playableCondition + `, ` + roots + `, ` + values
}

// chosenLocationJoin は、見る人に見せる動画（別名 videos）に一覧と同じ所在（見せてよい所在のパスの最小）を
// loc として結ぶ join 句である。
func (r *homeShelfReader) chosenLocationJoin() string {
	return ` join video_locations loc on loc.path = (select min(l.path) from video_locations l where l.video_id = videos.id
		and ` + visibleLocationCondition("l", r.audience) + `)`
}

// shownCondition は動画（別名 videos）を見る人に見せる条件である。
func (r *homeShelfReader) shownCondition() string {
	return visibleVideoCondition("videos", r.audience) + ` and ` + shownVideoCondition("videos", r.audience)
}

// scanRow は rowColumns と value の列を読む。
func scanRow(rows *sql.Rows) (homeShelfRow, error) {
	var row homeShelfRow
	var roots sql.NullString
	var addedAt, mtime, createdAt sql.NullInt64
	var titleKey sql.NullString
	if err := rows.Scan(&row.videoID, &row.path, &row.userKey, &row.watch, &row.favorited, &row.playable, &roots, &addedAt,
		&mtime, &createdAt, &row.durationMs, &row.sizeBytes, &titleKey, &row.favoriteAt, &row.playedAt, &row.value); err != nil {
		return homeShelfRow{}, fmt.Errorf("cannot read the home shelf: %w", err)
	}
	row.addedAt, row.mtime, row.createdAt, row.titleKey = addedAt.Int64, mtime.Int64, createdAt.Int64, titleKey.String
	if roots.Valid && roots.String != "" {
		for _, text := range strings.Split(roots.String, ",") {
			id, err := strconv.ParseInt(text, 10, 64)
			if err != nil {
				return homeShelfRow{}, fmt.Errorf("cannot read the home shelf roots: %w", err)
			}
			row.roots = append(row.roots, id)
		}
	}
	return row, nil
}

// videoShown は動画の項目に再生可否・お気に入り・視聴状態の絞り込みを掛ける。
func (rule homeShelfRule) videoShown(row homeShelfRow) bool {
	if rule.playable && !row.playable || rule.favoriteOnly && !row.favorited {
		return false
	}
	return rule.watchShown(row.watch)
}

// homeShelfRoot は根 1 つの判定と値である。
type homeShelfRoot struct {
	whole  bool
	values rootValuesRow
}

// decideRoots は根 roots（未判定のもの）を判定する。条件の無い棚では見せる子孫があれば whole で、所有者の
// 値は stored なら group_root_values から読む（無ければ集める）。条件のある棚と、ゲストと、グループ自身の
// 値（題名、お気に入りにした日時）が要る組み立ての経路は、子孫から集める。
func (r *homeShelfReader) decideRoots(rule homeShelfRule, roots []int64, known map[int64]homeShelfRoot, stored bool) error {
	var fresh []int64
	for _, root := range roots {
		if _, ok := known[root]; !ok && !slices.Contains(fresh, root) {
			fresh = append(fresh, root)
		}
	}
	if len(fresh) == 0 {
		return nil
	}
	values := map[int64]rootValuesRow{}
	var err error
	if stored && r.audience.IsOwner() {
		if values, err = readStoredRootValues(r.ctx, r.tx, fresh); err != nil {
			return err
		}
		var missing, found []int64
		for _, root := range fresh {
			if _, ok := values[root]; ok {
				found = append(found, root)
			} else {
				missing = append(missing, root)
			}
		}
		if rule.condition != "" && len(found) > 0 {
			// 値は保存したものを使い、子孫のすべてが条件に当たるかだけを確かめる。
			whole, err := readWholeRoots(r.ctx, r.tx, r.audience, found, rule.condition, rule.args)
			if err != nil {
				return err
			}
			for _, root := range found {
				row := values[root]
				row.matched = -1
				if whole[root] {
					row.matched = row.members
				}
				values[root] = row
			}
		}
		fresh = missing
	}
	if len(fresh) > 0 {
		computed, err := readRootValues(r.ctx, r.tx, r.audience, fresh, rule.condition, rule.args)
		if err != nil {
			return err
		}
		for root, row := range computed {
			values[root] = row
		}
	}
	for _, root := range roots {
		if _, ok := known[root]; ok {
			continue
		}
		row := values[root]
		matched := row.matched
		if rule.condition == "" {
			matched = row.members
		}
		whole := row.members > 0 && matched == row.members && (!rule.playable || row.playable)
		known[root] = homeShelfRoot{whole: whole, values: row}
	}
	return nil
}

// homeShelfItem は読んだ項目 1 つである。
type homeShelfItem struct {
	key   itemRow
	value any
	text  string
}

// classify は判定の済んだ動画 rows を項目にする。動画は、どの根にも入らないか、全部は当たっていない根の
// どれかに入れば動画の項目になり、全部が当たった根は、並び順の値が最大の子孫（sortColumn の *_member）の行
// でグループの項目になる。
func (r *homeShelfReader) classify(rule homeShelfRule, rows []homeShelfRow, known map[int64]homeShelfRoot, sortColumn string) []homeShelfItem {
	var items []homeShelfItem
	for _, row := range rows {
		videoItem := len(row.roots) == 0
		for _, root := range row.roots {
			decided := known[root]
			if !decided.whole {
				videoItem = true
				continue
			}
			if decided.values.member[sortColumn] == row.videoID && rule.watchShown(decided.values.watchState()) {
				items = append(items, homeShelfItem{key: itemRow{groupID: sql.NullInt64{Int64: root, Valid: true}, id: root*2 + 1}, value: row.value})
			}
		}
		if videoItem && rule.videoShown(row) {
			items = append(items, homeShelfItem{key: itemRow{id: row.videoID * 2, videoID: sql.NullInt64{Int64: row.videoID, Valid: true},
				path: sql.NullString{String: row.path, Valid: true}}, value: row.value})
		}
	}
	return items
}

// homeShelfFirstPage は棚 spec の最初のページ（listLibraryPageTx の最初のページと同じ項目と並び）と、
// limit + 1 を上限にした項目の数を読む。
func (r *homeShelfReader) homeShelfFirstPage(spec listSpec) (domain.LibraryPage, error) {
	if !spec.sort.Valid() {
		spec.sort = domain.SortAddedDesc
	}
	order, limit := itemOrder(spec.sort), normalizeLimit(spec.limit)
	rule, err := r.ruleOf(spec)
	if err != nil {
		return domain.LibraryPage{}, err
	}
	var page firstPageRows
	if _, ok := homeShelfTopSorts[spec.sort]; ok {
		page, err = r.topPage(rule, limit)
	} else {
		page, err = r.assemblePage(rule, limit)
	}
	if err != nil {
		return domain.LibraryPage{}, err
	}
	items, _, err := libraryPageItems(r.ctx, r.tx, spec, order, page.keys, page.values, limit)
	if err != nil {
		return domain.LibraryPage{}, err
	}
	return domain.LibraryPage{Items: items, Total: page.total, Limit: limit}, nil
}

// firstPageRows は最初のページの項目の鍵と並べ替えの値と、limit + 1 を上限にした棚の項目の数である。
type firstPageRows struct {
	keys   []itemRow
	values []any
	total  int
}

// pageOf は並べた項目 items から最初の limit 件と、limit + 1 を上限にした数を返す。
func pageOf(items []homeShelfItem, limit int) firstPageRows {
	page := firstPageRows{total: min(len(items), limit+1)}
	for _, item := range items[:min(len(items), limit)] {
		page.keys = append(page.keys, item.key)
		page.values = append(page.values, item.value)
	}
	return page
}

// assemblePage は、並べ替えの値が子孫 1 本から決まらない並び順の棚を、当たった動画をすべて読んで項目に
// してから並べて読む。
func (r *homeShelfReader) assemblePage(rule homeShelfRule, limit int) (firstPageRows, error) {
	where := r.shownCondition()
	if rule.condition != "" {
		where += ` and ` + rule.condition
	}
	rows, err := r.queryRows(`select `+r.rowColumns(rule, true)+`, null from videos`+r.chosenLocationJoin()+` where `+where, rule.args)
	if err != nil {
		return firstPageRows{}, err
	}
	known := map[int64]homeShelfRoot{}
	var roots []int64
	for _, row := range rows {
		roots = append(roots, row.roots...)
	}
	if err := r.decideRoots(rule, roots, known, false); err != nil {
		return firstPageRows{}, err
	}
	sort := rule.spec.sort
	var items []homeShelfItem
	for _, row := range rows {
		videoItem := len(row.roots) == 0
		for _, root := range row.roots {
			decided := known[root]
			if !decided.whole {
				videoItem = true
				continue
			}
			if row.videoID == decided.values.member[`added_at`] && rule.watchShown(decided.values.watchState()) {
				item := homeShelfItem{key: itemRow{groupID: sql.NullInt64{Int64: root, Valid: true}, id: root*2 + 1}}
				item.value, item.text = cardSortValue(sort, rule.spec.seed, decided.values, item.key.id)
				items = append(items, item)
			}
		}
		if videoItem && rule.videoShown(row) {
			item := homeShelfItem{key: itemRow{id: row.videoID * 2, videoID: sql.NullInt64{Int64: row.videoID, Valid: true},
				path: sql.NullString{String: row.path, Valid: true}}}
			item.value, item.text = videoSortValue(sort, rule.spec.seed, row, item.key.id)
			items = append(items, item)
		}
	}
	desc := strings.HasSuffix(string(sort), "Desc")
	slices.SortFunc(items, func(a, b homeShelfItem) int { return compareItems(a, b, desc) })
	return pageOf(items, limit), nil
}

// queryRows は rowColumns と並べ替えの値の列を返す問い合わせを読む。
func (r *homeShelfReader) queryRows(query string, args []any) ([]homeShelfRow, error) {
	rows, err := r.tx.QueryContext(r.ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("cannot read the home shelf: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []homeShelfRow
	for rows.Next() {
		row, err := scanRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// videoSortValue は動画の項目の並べ替えの値（数か文字列。無ければ nil と空）である。
func videoSortValue(sort domain.VideoSort, seed int64, row homeShelfRow, itemID int64) (any, string) {
	switch sort {
	case domain.SortDurationAsc, domain.SortDurationDesc:
		return nullable(row.durationMs), ""
	case domain.SortSizeAsc, domain.SortSizeDesc:
		return nullable(row.sizeBytes), ""
	case domain.SortTitleAsc, domain.SortTitleDesc:
		return nil, row.titleKey
	case domain.SortFavoritedAsc, domain.SortFavoritedDesc:
		return nullable(row.favoriteAt), ""
	case domain.SortPlayedAsc, domain.SortPlayedDesc:
		return nullable(row.playedAt), ""
	case domain.SortAddedAsc, domain.SortAddedDesc:
		return row.addedAt, ""
	case domain.SortModifiedAsc, domain.SortModifiedDesc:
		return row.mtime, ""
	case domain.SortCreatedAsc, domain.SortCreatedDesc:
		return row.createdAt, ""
	case domain.SortRandom:
		return domain.ShuffleKey(seed, itemID), ""
	}
	return nil, ""
}

// cardSortValue はグループの項目の並べ替えの値である。
func cardSortValue(sort domain.VideoSort, seed int64, values rootValuesRow, itemID int64) (any, string) {
	switch sort {
	case domain.SortDurationAsc, domain.SortDurationDesc:
		return nullable(values.durationMs), ""
	case domain.SortSizeAsc, domain.SortSizeDesc:
		return nullable(values.sizeBytes), ""
	case domain.SortTitleAsc, domain.SortTitleDesc:
		return nil, values.titleKey
	case domain.SortFavoritedAsc, domain.SortFavoritedDesc:
		return nullable(values.favoritedAt), ""
	case domain.SortPlayedAsc, domain.SortPlayedDesc:
		return nullable(values.playedAt), ""
	case domain.SortAddedAsc, domain.SortAddedDesc:
		return nullable(values.addedAt), ""
	case domain.SortModifiedAsc, domain.SortModifiedDesc:
		return nullable(values.mtime), ""
	case domain.SortCreatedAsc, domain.SortCreatedDesc:
		return nullable(values.createdAt), ""
	case domain.SortRandom:
		return domain.ShuffleKey(seed, itemID), ""
	}
	return nil, ""
}

// nullable は NULL を nil にする。
func nullable(value sql.NullInt64) any {
	if !value.Valid {
		return nil
	}
	return value.Int64
}

// compareItems は項目をライブラリの並び（値の無い項目は末尾、値、項目の id）で比べる。題名は文字列で、
// ほかは数で比べる。
func compareItems(a, b homeShelfItem, desc bool) int {
	aNull, bNull := a.value == nil && a.text == "", b.value == nil && b.text == ""
	if aNull != bNull {
		if aNull {
			return 1
		}
		return -1
	}
	c := 0
	if a.value != nil {
		c = cmp.Compare(a.value.(int64), b.value.(int64))
	} else {
		c = cmp.Compare(a.text, b.text)
	}
	c = cmp.Or(c, cmp.Compare(a.key.id, b.key.id))
	if desc {
		return -c
	}
	return c
}
