package store

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

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
	sort         VideoSort
	cursor       string
	limit        int
}

// ListVideos はライブラリの一覧1ページを返す。
//
// ページングは keyset（カーソル）方式である。offset を使うと、取り込みで行が
// 増減した瞬間に取りこぼしと重複が起きる。並び順の値と id を境界に使うので、
// 途中で行が動いても続きが安定して取れる。
func (db *DB) ListVideos(ctx context.Context, q VideoQuery) (VideoPage, error) {
	return db.listVideoPage(ctx, listSpec{
		scope: libraryScope(), expr: domain.ParseSearchQuery(q.Query),
		watch: q.Watch, playableOnly: q.PlayableOnly,
		sort: q.Sort, cursor: q.Cursor, limit: q.Limit,
	})
}

// ListFolderVideos はフォルダの動画1ページを返す。範囲は q.Scope で直下か配下
// すべてかを選ぶ。並び順・カーソルの形・絞り込みは ListVideos と同じで、題名は
// その範囲にある所在の題名である。
func (db *DB) ListFolderVideos(ctx context.Context, q domain.FolderVideoQuery) (VideoPage, error) {
	return db.listVideoPage(ctx, listSpec{
		scope: folderScope(q.Dir, q.Scope), expr: domain.ParseSearchQuery(q.Query),
		watch: q.Watch, playableOnly: q.PlayableOnly,
		sort: q.Sort, cursor: q.Cursor, limit: q.Limit,
	})
}

// CountVideos はライブラリで検索語に当たる動画の総件数を返す。1万件規模の
// count(*) は索引走査で数 ms に収まる。
func (db *DB) CountVideos(ctx context.Context, search string) (int, error) {
	return db.countVideos(ctx, listSpec{scope: libraryScope(), expr: domain.ParseSearchQuery(search)})
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
// where 句を返す。withLocation が true なら一覧に出す所在を loc として結ぶ。
func filteredFrom(spec listSpec, withLocation bool) string {
	from := ` from chosen join videos on videos.id = chosen.video_id`
	if withLocation {
		from += ` join video_locations loc on loc.path = chosen.path`
	}
	from += ` left join playback_progress p on p.content_key = videos.content_key`
	var conditions []string
	if condition := watchCondition(spec.watch); condition != "" {
		conditions = append(conditions, condition)
	}
	if spec.playableOnly {
		conditions = append(conditions, playableCondition)
	}
	if len(conditions) > 0 {
		from += ` where ` + strings.Join(conditions, " and ")
	}
	return from
}

// countVideos は範囲・検索式・絞り込みをすべて適用した総件数を返す（要件 13）。
// カーソルには関係しない。
func (db *DB) countVideos(ctx context.Context, spec listSpec) (int, error) {
	cte, args := chosenLocationsCTE(spec.scope, spec.expr)
	var total int
	//nolint:gosec // 組み立てるのは定型の条件句だけで、値はすべて引数で渡す。
	if err := db.sql.QueryRowContext(ctx, cte+` select count(*)`+filteredFrom(spec, false), args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("件数を数えられません: %w", err)
	}
	return total, nil
}

// listVideoPage は条件に合う動画1ページと総件数を返す。
func (db *DB) listVideoPage(ctx context.Context, spec listSpec) (VideoPage, error) {
	limit := normalizeLimit(spec.limit)
	if spec.sort == "" {
		spec.sort = SortAddedDesc
	}
	order := orderFor(spec.sort)

	var cursorClause string
	var cursorArgs []any
	if spec.cursor != "" {
		var err error
		cursorClause, cursorArgs, err = order.cursorCondition(spec.cursor)
		if err != nil {
			return VideoPage{}, err
		}
	}

	total, err := db.countVideos(ctx, spec)
	if err != nil {
		return VideoPage{}, err
	}

	cte, args := chosenLocationsCTE(spec.scope, spec.expr)
	//nolint:gosec // 組み立てるのは列名と定型の条件句だけで、値はすべて引数で渡す。
	query := cte + ` select * from (select ` + listColumns + filteredFrom(spec, true) + `) as videos`
	if cursorClause != "" {
		query += ` where ` + cursorClause
		args = append(args, cursorArgs...)
	}
	// 並び順は検索の有無で変えない。関連度（bm25）にすると、instr で調べる語には
	// 関連度が無いため語の長さで並びが変わり、利用者から見て不可解になる。
	// 次のページがあるかを知るために1件多く取る。件数を数え直すより安い。
	query += ` order by ` + order.orderBy + ` limit ?`

	rows, err := db.sql.QueryContext(ctx, query, append(args, limit+1)...)
	if err != nil {
		return VideoPage{}, fmt.Errorf("一覧を読み出せません: %w", err)
	}
	defer func() { _ = rows.Close() }()

	page := VideoPage{Total: total, Limit: limit, Items: []domain.Video{}}
	for rows.Next() {
		video, err := scanVideo(rows)
		if err != nil {
			return VideoPage{}, fmt.Errorf("一覧を読み出せません: %w", err)
		}
		page.Items = append(page.Items, video)
	}
	if err := rows.Err(); err != nil {
		return VideoPage{}, fmt.Errorf("一覧を読み出せません: %w", err)
	}

	if len(page.Items) > limit {
		page.Items = page.Items[:limit]
		page.NextCursor = order.encodeCursor(page.Items[len(page.Items)-1])
	}
	return page, nil
}

// listOrder は並び順1つ分の order by 句・keyset の条件・カーソルの値の取り出し方
// である。並び順を足すときはここに1件足す。列名は listColumns の外側から見た名前
// （videos.* の列名）で書く。
type listOrder struct {
	// orderBy は order by 句。値が同じ行は id で決着させる。
	orderBy string
	// after はカーソルより後ろを取る条件句で、引数は並びの値と id の2つ。
	after string
	// value はカーソルに包む並びの値を項目から取り出す。
	value func(domain.Video) string
	// parse はカーソルから取り出した並びの値を、after の引数に戻す。
	parse func(string) (any, error)
}

var listOrders = map[VideoSort]listOrder{
	SortAddedDesc: {
		orderBy: `added_at desc, id desc`,
		after:   `(added_at, id) < (?, ?)`,
		value:   func(v domain.Video) string { return strconv.FormatInt(v.AddedAt.Unix(), 10) },
		parse: func(value string) (any, error) {
			addedAt, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("%w: 追加時刻として解釈できません", ErrInvalidCursor)
			}
			return addedAt, nil
		},
	},
	SortTitleAsc: {
		orderBy: `title asc, id asc`,
		after:   `(title, id) > (?, ?)`,
		value:   func(v domain.Video) string { return v.Title },
		parse:   func(value string) (any, error) { return value, nil },
	},
}

// orderFor は並び順の定義を返す。未知の値は既定（addedDesc）にする。値の検査は
// 入口（internal/httpapi）が行う。
func orderFor(sort VideoSort) listOrder {
	if order, ok := listOrders[sort]; ok {
		return order
	}
	return listOrders[SortAddedDesc]
}

// cursorCondition はカーソルより後ろだけを取る条件を返す。
func (o listOrder) cursorCondition(cursor string) (string, []any, error) {
	value, id, err := decodeCursor(cursor)
	if err != nil {
		return "", nil, err
	}
	parsed, err := o.parse(value)
	if err != nil {
		return "", nil, err
	}
	return o.after, []any{parsed, id}, nil
}

// cursorSeparator はカーソルの中で並び順の値と id を区切る。題名に現れない
// 制御文字を選ぶ。
const cursorSeparator = "\x1f"

// encodeCursor は「並び順の値 + id」を不透明な文字列に包む。クライアントは
// 中身を解釈しない。
func (o listOrder) encodeCursor(last domain.Video) string {
	return base64.RawURLEncoding.EncodeToString(
		[]byte(o.value(last) + cursorSeparator + strconv.FormatInt(last.ID, 10)))
}

// decodeCursor は包みを解く。解釈できないものは誤りとして返す。
func decodeCursor(cursor string) (value string, id int64, err error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return "", 0, fmt.Errorf("%w: %w", ErrInvalidCursor, err)
	}

	parts := strings.SplitN(string(raw), cursorSeparator, 2)
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("%w: 区切りがありません", ErrInvalidCursor)
	}

	id, err = strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return "", 0, fmt.Errorf("%w: 識別子として解釈できません", ErrInvalidCursor)
	}
	return parts[0], id, nil
}

// normalizeLimit は件数を既定値と上限に丸める。
func normalizeLimit(limit int) int {
	switch {
	case limit <= 0:
		return DefaultLimit
	case limit > MaxLimit:
		return MaxLimit
	default:
		return limit
	}
}
