package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"modernc.org/sqlite"

	"github.com/syudead/vv/internal/domain"
)

// 一覧の問い合わせの組み立て（specs/013-library-search/plan.md Structural
// Decisions 2・3・7）。ライブラリ・フォルダ直下・フォルダ配下の3つの一覧を、
// 次の1つの流れで返す。
//
//  1. 範囲と検索式を満たす所在を選ぶ
//  2. 動画ごとにパスの最小の1件へまとめる（chosen）
//  3. その所在と動画の属性で、視聴状態と再生可否を絞る
//  4. 並べて keyset で区切る

// locationScopeKind は所在の範囲の形である。
type locationScopeKind int

const (
	// scopeLibrary は登録フォルダの下すべて。
	scopeLibrary locationScopeKind = iota
	// scopeDirect はフォルダの直下だけ。
	scopeDirect
	// scopeSubtree はフォルダとその配下すべて。
	scopeSubtree
)

// locationScope は一覧が対象にする所在の範囲である。どの形でも登録フォルダの
// 下にある所在に限る。
type locationScope struct {
	kind locationScopeKind
	// dir はフォルダの絶対パス。scopeLibrary では使わない。
	dir string
}

func libraryScope() locationScope { return locationScope{kind: scopeLibrary} }

// folderScope はフォルダの問い合わせの範囲を返す。空の指定は直下として扱う。
func folderScope(dir string, scope domain.FolderScope) locationScope {
	if scope == domain.FolderScopeSubtree {
		return locationScope{kind: scopeSubtree, dir: dir}
	}
	return locationScope{kind: scopeDirect, dir: dir}
}

// condition は所在（別名 alias）が範囲にあることを表す条件句と引数を返す。
func (s locationScope) condition(alias string) (string, []any) {
	registered := registeredLocationCondition(alias)
	if s.kind == scopeLibrary {
		return registered, nil
	}
	prefix := folderPrefix(s.dir)
	clause := registered + ` and instr(` + folderPathExpr(alias) + `, ?) = 1`
	args := []any{prefix}
	if s.kind == scopeDirect {
		clause += ` and ` + directChildCondition(alias)
		args = append(args, prefix)
	}
	return clause, args
}

// listSpec は一覧1ページの問い合わせの条件である。
type listSpec struct {
	scope        locationScope
	expr         domain.SearchExpr
	watch        domain.WatchFilter
	playableOnly bool
	// tagIDs はすでに存在を確かめたタグの id（data-model.md §6）。呼び出し側
	// （ListVideos・VideoIDs）が resolveTagIDs で存在しない id を落としたうえで
	// 渡す。
	tagIDs []int64
	sort   domain.VideoSort
	seed   int64
	cursor string
	limit  int
}

// ListVideos はライブラリの一覧1ページを返す。
//
// ページングは keyset（カーソル）方式である。offset を使うと、取り込みで行が
// 増減した瞬間に取りこぼしと重複が起きる。並び順の値と id を境界に使うので、
// 途中で行が動いても続きが安定して取れる。
//
// タグの存在確認・件数・ページの行を同じ読み取りスナップショット（s.db.read の
// 1取引）から返す。別々に読むと、その間の付け外しやタグの削除によって
// MissingTagIDs・Total・Items が食い違いうる。
func (s *LibraryStore) ListVideos(ctx context.Context, q domain.VideoQuery) (domain.VideoPage, error) {
	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.VideoPage{}, fmt.Errorf("一覧の読み取りを始められません: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	tagIDs, missingTagIDs, err := existingTagIDs(ctx, tx, q.TagIDs)
	if err != nil {
		return domain.VideoPage{}, err
	}
	page, err := listVideoPageTx(ctx, tx, listSpec{
		scope: libraryScope(), expr: domain.ParseSearchQuery(q.Query),
		watch: q.Watch, playableOnly: q.PlayableOnly, tagIDs: tagIDs,
		sort: q.Sort, seed: q.Seed, cursor: q.Cursor, limit: q.Limit,
	})
	if err != nil {
		return domain.VideoPage{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.VideoPage{}, fmt.Errorf("一覧の読み取りを終えられません: %w", err)
	}
	page.MissingTagIDs = missingTagIDs
	return page, nil
}

// VideoIDs は ListVideos と同じ条件（並び順・カーソル・件数を除く）に合う
// 全件の id を、ページングせずに返す（「すべて選択」用、Plan の Structural
// Decisions 4）。並びは決めない。MissingTagIDs の意味は ListVideos と同じ
// （contracts/tags-api.md §5 の GET /api/videos/ids）。ListVideos と同じく、
// タグの存在確認と id の読み出しを s.db.read の1取引の中で行う。
func (s *LibraryStore) VideoIDs(ctx context.Context, q domain.VideoQuery) ([]int64, []int64, error) {
	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, nil, fmt.Errorf("id の読み取りを始められません: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	tagIDs, missingTagIDs, err := existingTagIDs(ctx, tx, q.TagIDs)
	if err != nil {
		return nil, nil, err
	}
	ids, err := videoIDsForSpec(ctx, tx, listSpec{
		scope: libraryScope(), expr: domain.ParseSearchQuery(q.Query),
		watch: q.Watch, playableOnly: q.PlayableOnly, tagIDs: tagIDs,
	})
	if err != nil {
		return nil, nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, fmt.Errorf("id の読み取りを終えられません: %w", err)
	}
	return ids, missingTagIDs, nil
}

// videoIDsForSpec は spec に合う全件の id を返す。並びは決めない。
func videoIDsForSpec(ctx context.Context, q queryExecer, spec listSpec) ([]int64, error) {
	cte, args := chosenLocationsCTE(spec.scope, spec.expr)
	from, fromArgs := filteredFrom(spec, false)
	args = append(args, fromArgs...)

	//nolint:gosec // 組み立てるのは定型の条件句だけで、値はすべて引数で渡す。
	rows, err := q.QueryContext(ctx, cte+` select videos.id`+from, args...)
	if err != nil {
		return nil, fmt.Errorf("id を読み出せません: %w", err)
	}
	defer func() { _ = rows.Close() }()

	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("id を読み出せません: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("id を読み出せません: %w", err)
	}
	return ids, nil
}

// ListFolderVideos はフォルダの動画1ページを返す。範囲は q.Scope で直下か配下
// すべてかを選ぶ。並び順・カーソルの形・絞り込みは ListVideos と同じで、題名は
// その範囲にある所在の題名である。
func (s *LibraryStore) ListFolderVideos(ctx context.Context, q domain.FolderVideoQuery) (domain.VideoPage, error) {
	return s.listVideoPage(ctx, listSpec{
		scope: folderScope(q.Dir, q.Scope), expr: domain.ParseSearchQuery(q.Query),
		watch: q.Watch, playableOnly: q.PlayableOnly,
		sort: q.Sort, seed: q.Seed, cursor: q.Cursor, limit: q.Limit,
	})
}

// CountVideos はライブラリで検索語に当たる動画の総件数を返す。一覧と同じ所在の
// まとめ（chosen）を数えるので、1万件規模で数十 ms かかる（一覧の1ページも同程度）。
func (s *LibraryStore) CountVideos(ctx context.Context, search string) (int, error) {
	return s.countVideos(ctx, listSpec{scope: libraryScope(), expr: domain.ParseSearchQuery(search)})
}

// chosenLocationsCTE は、範囲と検索式を満たす所在を動画ごとにパスの最小の1件へ
// まとめる `chosen(video_id, path)` を返す（contracts/list-api.md §4）。
// 式は所在1行に対して評価するので、語ごとに別の所在で満たした動画は当たらない
// （要件 9）。
func chosenLocationsCTE(scope locationScope, expr domain.SearchExpr) (string, []any) {
	scopeClause, args := scope.condition("l")
	clauses := []string{scopeClause}
	if exprClause, exprArgs := searchExprCondition(expr, "l"); exprClause != "" {
		clauses = append(clauses, exprClause)
		args = append(args, exprArgs...)
	}
	return `with chosen as (
		select l.video_id, min(l.path) as path from video_locations l
		where ` + strings.Join(clauses, " and ") + `
		group by l.video_id)`, args
}

// watchCondition は視聴状態の絞り込みを、playback_progress（別名 p、left join）
// に対する条件句にする。定義は domain.ClassifyWatch と同じ
// （data-model.md §6）。絞り込まないときは空の句を返す。
func watchCondition(filter domain.WatchFilter) string {
	switch filter {
	case domain.WatchUnwatched:
		return `(p.content_key is null or (p.completed = 0 and p.position_ms = 0))`
	case domain.WatchInProgress:
		return `(p.content_key is not null and p.completed = 0 and p.position_ms > 0)`
	case domain.WatchWatched:
		return `p.completed = 1`
	default:
		return ""
	}
}

// playableCondition は domain.Video.PlayableInBrowser と同じ条件である。
const playableCondition = `videos.playable = 1 and videos.probe_state = 'done'`

// listColumns は一覧の項目1件を domain.Video に写す列である。並びは scanVideo と
// 対応させる。パス・題名・大きさ・更新時刻は chosen の所在のものを使う。
// パスは後続の単位が Video.folder を組み立てるのに使う。
const listColumns = `videos.id, chosen.path, loc.title, loc.size_bytes, loc.mtime,
	videos.added_at, videos.updated_at, videos.content_key, videos.duration_ms, videos.width,
	videos.height, videos.container, videos.video_codec, videos.audio_codec, videos.playable,
	videos.unplayable_reason, videos.probe_state, videos.probe_error, videos.thumbnail_state, videos.preview_state`

// filteredFrom は chosen に動画と再生の記録を結び、絞り込みを掛けた from 句と
// where 句、その中で使う引数を返す。withLocation が true なら一覧に出す所在を
// loc として結ぶ。
//
// random の並べ替えは選択句の vv_shuffle_key(?, …) の引数を CTE の引数の直後に
// 置いており、呼び出し側はこの関数が返す引数を SQL の文字列の中でこの句が
// 現れる位置（CTE の引数のあと、カーソルの引数の前）にそろえて並べる。
func filteredFrom(spec listSpec, withLocation bool) (string, []any) {
	from := ` from chosen join videos on videos.id = chosen.video_id`
	if withLocation {
		from += ` join video_locations loc on loc.path = chosen.path`
	}
	// 内容の識別子が空の動画は再生位置を持たない（API の progressFor と同じ扱い）。
	from += ` left join playback_progress p on p.content_key = videos.content_key and videos.content_key <> ''`
	var conditions []string
	var args []any
	if condition := watchCondition(spec.watch); condition != "" {
		conditions = append(conditions, condition)
	}
	if spec.playableOnly {
		conditions = append(conditions, playableCondition)
	}
	// タグでの絞り込み（data-model.md §6）。存在の確認は呼び出し側
	// （resolveTagIDs）が済ませているので、ここでは AND を掛けるだけでよい。
	for _, tagID := range spec.tagIDs {
		conditions = append(conditions,
			`exists (select 1 from video_tags vt where vt.content_key = videos.content_key and vt.tag_id = ?)`)
		args = append(args, tagID)
	}
	if len(conditions) > 0 {
		from += ` where ` + strings.Join(conditions, " and ")
	}
	return from, args
}

// countVideos は範囲・検索式・絞り込みをすべて適用した総件数を返す（要件 13）。
// カーソルには関係しない。
type queryRower interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func countVideosWith(ctx context.Context, db queryRower, spec listSpec) (int, error) {
	cte, args := chosenLocationsCTE(spec.scope, spec.expr)
	from, fromArgs := filteredFrom(spec, false)
	args = append(args, fromArgs...)
	var total int
	//nolint:gosec // 組み立てるのは定型の条件句だけで、値はすべて引数で渡す。
	if err := db.QueryRowContext(ctx, cte+` select count(*)`+from, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("件数を数えられません: %w", err)
	}
	return total, nil
}

func (s *LibraryStore) countVideos(ctx context.Context, spec listSpec) (int, error) {
	return countVideosWith(ctx, s.db.sql, spec)
}

// listVideoPage は条件に合う動画1ページと総件数を返す。count とページの行を
// 同じ読み取りスナップショットから返すため、s.db.read に新しい取引を開いて
// listVideoPageTx に委ねる。別々の接続で読むと、その間の取り込みによって
// total と Items が矛盾する。
func (s *LibraryStore) listVideoPage(ctx context.Context, spec listSpec) (domain.VideoPage, error) {
	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.VideoPage{}, fmt.Errorf("一覧の読み取りを始められません: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	page, err := listVideoPageTx(ctx, tx, spec)
	if err != nil {
		return domain.VideoPage{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.VideoPage{}, fmt.Errorf("一覧の読み取りを終えられません: %w", err)
	}
	return page, nil
}

// listVideoPageTx は tx の中で spec に合う動画1ページと総件数を返す。呼び出し側
// が取引の開始・commit・rollback を持つ（ListVideos はタグの存在確認と同じ
// 取引で呼ぶ）。
func listVideoPageTx(ctx context.Context, tx *sql.Tx, spec listSpec) (domain.VideoPage, error) {
	limit := normalizeLimit(spec.limit)
	if !spec.sort.Valid() {
		// 値の検査は入口（internal/httpapi）が行う。ここに来た未知の値は既定にする。
		spec.sort = domain.SortAddedDesc
	}
	order := listOrders[spec.sort]

	var cursorClause string
	var cursorArgs []any
	if spec.cursor != "" {
		var err error
		cursorClause, cursorArgs, err = order.cursorCondition(spec, spec.cursor)
		if err != nil {
			return domain.VideoPage{}, err
		}
	}

	total, err := countVideosWith(ctx, tx, spec)
	if err != nil {
		return domain.VideoPage{}, err
	}

	cte, args := chosenLocationsCTE(spec.scope, spec.expr)
	if order.seeded {
		args = append(args, spec.seed)
	}
	from, fromArgs := filteredFrom(spec, true)
	args = append(args, fromArgs...)
	//nolint:gosec // 組み立てるのは列名と定型の条件句だけで、値はすべて引数で渡す。
	query := cte + ` select * from (select ` + listColumns + `, ` + order.value + ` as sort_value` +
		from + `) as videos`
	if cursorClause != "" {
		query += ` where ` + cursorClause
		args = append(args, cursorArgs...)
	}
	// 並び順は検索の有無で変えない。関連度（bm25）にすると、instr で調べる語には
	// 関連度が無いため語の長さで並びが変わり、利用者から見て不可解になる。
	// 次のページがあるかを知るために1件多く取る。件数を数え直すより安い。
	query += ` order by ` + order.orderBy() + ` limit ?`

	rows, err := tx.QueryContext(ctx, query, append(args, limit+1)...)
	if err != nil {
		return domain.VideoPage{}, fmt.Errorf("一覧を読み出せません: %w", err)
	}
	defer func() { _ = rows.Close() }()

	page := domain.VideoPage{Total: total, Limit: limit, Items: []domain.Video{}}
	var values []any
	for rows.Next() {
		var value any
		video, err := scanVideo(sortValueScanner{rows: rows, value: &value})
		if err != nil {
			return domain.VideoPage{}, fmt.Errorf("一覧を読み出せません: %w", err)
		}
		page.Items = append(page.Items, video)
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return domain.VideoPage{}, fmt.Errorf("一覧を読み出せません: %w", err)
	}
	if err := rows.Close(); err != nil {
		return domain.VideoPage{}, fmt.Errorf("一覧を閉じられません: %w", err)
	}

	if len(page.Items) > limit {
		page.Items = page.Items[:limit]
		last := page.Items[limit-1]
		page.NextCursor, err = order.encodeCursor(spec, values[limit-1], last.ID)
		if err != nil {
			return domain.VideoPage{}, err
		}
	}
	return page, nil
}

// sortValueScanner は一覧の列に続く sort_value を、scanVideo の読み取りに
// 1列足して受け取る。
type sortValueScanner struct {
	rows  *sql.Rows
	value *any
}

func (s sortValueScanner) Scan(dest ...any) error {
	return s.rows.Scan(append(dest, s.value)...)
}

// shuffleFunction は domain.ShuffleKey を SQLite から呼ぶ名前である。
const shuffleFunction = "vv_shuffle_key"

// 登録は以後に開く接続にだけ効き、同じ名前の二重登録は誤りになるので、Open の
// 中ではなくパッケージの初期化で一度だけ行う（plan.md Structural Decisions 5）。
// select の中でしか使わず、スキーマには残らない。
func init() {
	err := sqlite.RegisterDeterministicScalarFunction(shuffleFunction, 2,
		func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			seed, ok := args[0].(int64)
			if !ok {
				return nil, fmt.Errorf("%s: seed が整数ではありません: %T", shuffleFunction, args[0])
			}
			id, ok := args[1].(int64)
			if !ok {
				return nil, fmt.Errorf("%s: id が整数ではありません: %T", shuffleFunction, args[1])
			}
			return domain.ShuffleKey(seed, id), nil
		})
	if err != nil {
		panic(fmt.Sprintf("%s を登録できません: %v", shuffleFunction, err))
	}
}

// sortValueKind はカーソルに包む並べ替えの値の型である。
type sortValueKind int

const (
	sortInteger sortValueKind = iota
	sortText
)

// listOrder は並び順1つ分の定義である（contracts/list-api.md §3）。並び順を
// 足すときは listOrders に1件足す。
type listOrder struct {
	// value は並べ替えの値を作る式。一覧の内側の問い合わせ（videos・loc・p）で
	// 評価し、sort_value として外側から見る。
	value string
	kind  sortValueKind
	// desc は値と id をどちらも降順に並べる。
	desc bool
	// nullable は値が無い（NULL）ことがある。値の無い動画は向きに関係なく末尾に置く。
	nullable bool
	// seeded は value が seed を引数に1つ取る。
	seeded bool
}

var listOrders = map[domain.VideoSort]listOrder{
	domain.SortAddedAsc:     {value: `videos.added_at`},
	domain.SortAddedDesc:    {value: `videos.added_at`, desc: true},
	domain.SortModifiedAsc:  {value: `loc.mtime`},
	domain.SortModifiedDesc: {value: `loc.mtime`, desc: true},
	// 題名は保存した title_key のバイト順で、自然順になる（data-model.md §4）。
	domain.SortTitleAsc:     {value: `loc.title_key`, kind: sortText},
	domain.SortTitleDesc:    {value: `loc.title_key`, kind: sortText, desc: true},
	domain.SortDurationAsc:  {value: `videos.duration_ms`, nullable: true},
	domain.SortDurationDesc: {value: `videos.duration_ms`, desc: true, nullable: true},
	domain.SortSizeAsc:      {value: `loc.size_bytes`},
	domain.SortSizeDesc:     {value: `loc.size_bytes`, desc: true},
	// 再生の記録は filteredFrom の left join（p）から取る。記録が無ければ NULL。
	domain.SortPlayedAsc:  {value: `p.updated_at`, nullable: true},
	domain.SortPlayedDesc: {value: `p.updated_at`, desc: true, nullable: true},
	// ランダムは seed と id だけで決まる値の昇順で、値が同じなら id の昇順。
	domain.SortRandom: {value: shuffleFunction + `(?, videos.id)`, seeded: true},
}

// orderBy は外側の問い合わせの order by 句を返す。値が同じ行は id で決着させる。
func (o listOrder) orderBy() string {
	direction := ` asc`
	if o.desc {
		direction = ` desc`
	}
	clause := `sort_value` + direction + `, id` + direction
	if o.nullable {
		clause = `(sort_value is null) asc, ` + clause
	}
	return clause
}

// after はカーソル（値の有無・値・id）より後ろを取る条件句と引数を返す。値の
// 無い行は末尾にまとまるので、カーソルが値を持てば値の無い行はすべて後ろ、
// カーソルが値を持たなければ値の無い行のうち id が後ろのものだけが後ろになる。
func (o listOrder) after(isNull bool, value any, id int64) (string, []any) {
	op := ` > `
	if o.desc {
		op = ` < `
	}
	if isNull {
		return `sort_value is null and id` + op + `?`, []any{id}
	}
	clause := `(sort_value, id)` + op + `(?, ?)`
	if o.nullable {
		clause = `(sort_value is null or ` + clause + `)`
	}
	return clause, []any{value, id}
}

// cursorCondition はカーソルより後ろだけを取る条件を返す。別の並び順や別の
// seed で作ったカーソルは誤りにする（contracts/list-api.md §5）。
func (o listOrder) cursorCondition(spec listSpec, cursor string) (string, []any, error) {
	c, err := decodeCursor(cursor)
	if err != nil {
		return "", nil, err
	}
	if c.sort != string(spec.sort) {
		return "", nil, fmt.Errorf("%w: 別の並び順のカーソルです", domain.ErrInvalidCursor)
	}
	if c.seed != o.seedText(spec) {
		return "", nil, fmt.Errorf("%w: 別の seed のカーソルです", domain.ErrInvalidCursor)
	}
	if c.isNull {
		if !o.nullable {
			return "", nil, fmt.Errorf("%w: 値の無いカーソルです", domain.ErrInvalidCursor)
		}
		clause, args := o.after(true, nil, c.id)
		return clause, args, nil
	}
	var value any = c.value
	if o.kind == sortInteger {
		parsed, err := strconv.ParseInt(c.value, 10, 64)
		if err != nil {
			return "", nil, fmt.Errorf("%w: 並べ替えの値として解釈できません", domain.ErrInvalidCursor)
		}
		value = parsed
	}
	clause, args := o.after(false, value, c.id)
	return clause, args, nil
}

// seedText はカーソルに包む seed である。seed を使わない並び順では空にする。
func (o listOrder) seedText(spec listSpec) string {
	if !o.seeded {
		return ""
	}
	return strconv.FormatInt(spec.seed, 10)
}

// encodeCursor は「並び順の名前・seed・値の有無・id・値」を不透明な文字列に
// 包む。クライアントは中身を解釈しない。
func (o listOrder) encodeCursor(spec listSpec, value any, id int64) (string, error) {
	nullFlag, text := "0", ""
	switch v := value.(type) {
	case nil:
		nullFlag = "1"
	case int64:
		text = strconv.FormatInt(v, 10)
	case string:
		text = v
	case []byte:
		text = string(v)
	default:
		return "", fmt.Errorf("並べ替えの値を包めません: %T", value)
	}
	fields := []string{string(spec.sort), o.seedText(spec), nullFlag, strconv.FormatInt(id, 10), text}
	return base64.RawURLEncoding.EncodeToString([]byte(strings.Join(fields, cursorSeparator))), nil
}

// cursorSeparator はカーソルの中で項目を区切る。値（題名の鍵）は最後に置くので、
// 値に同じ文字が現れても区切りを誤らない。
const cursorSeparator = "\x1f"

// cursorFields はカーソルから取り出した項目である。
type cursorFields struct {
	sort, seed string
	isNull     bool
	id         int64
	value      string
}

// decodeCursor は包みを解く。解釈できないものは誤りとして返す。
func decodeCursor(cursor string) (cursorFields, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return cursorFields{}, fmt.Errorf("%w: %w", domain.ErrInvalidCursor, err)
	}

	parts := strings.SplitN(string(raw), cursorSeparator, 5)
	if len(parts) != 5 {
		return cursorFields{}, fmt.Errorf("%w: 項目が足りません", domain.ErrInvalidCursor)
	}
	if parts[2] != "0" && parts[2] != "1" {
		return cursorFields{}, fmt.Errorf("%w: 値の有無を解釈できません", domain.ErrInvalidCursor)
	}
	id, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		return cursorFields{}, fmt.Errorf("%w: 識別子として解釈できません", domain.ErrInvalidCursor)
	}
	return cursorFields{sort: parts[0], seed: parts[1], isNull: parts[2] == "1", id: id, value: parts[4]}, nil
}

// normalizeLimit は件数を既定値と上限に丸める。
func normalizeLimit(limit int) int {
	switch {
	case limit <= 0:
		return domain.DefaultLimit
	case limit > domain.MaxLimit:
		return domain.MaxLimit
	default:
		return limit
	}
}
