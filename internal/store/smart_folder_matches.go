package store

// スマートフォルダの一致する動画（specs/052-smart-folders/data-model.md「Contents」の Matching
// videos、research.md R-14）。見る人に見せる動画（内容の鍵ごとに 1 本）のうち、経路のどの段の条件にも
// 一致する動画で、条件は動画そのものに確かめ、グループにまとめる段を持たない。ライブラリの一番上の
// フォルダの並べ替えの値と、カードの読み出しの値をこの動画から作る。
//
// フォルダによらない部分（見せる動画とその所在・再生の記録・お気に入り）は、1 回の読み出しで
// smart_base に 1 度だけ求め、どのフォルダもそれを絞って読む（research.md R-13）。

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/syudead/vv/internal/domain"
)

// smartBaseCTE は、見る人 audience に見せる動画を 1 本 1 行に持つ `smart_base` を定める with 句と
// その引数を返す。列は動画の条件が読む videos の列（id・content_key・added_at・duration_ms・
// playable・probe_state）と、ライブラリの項目と同じ名前の値の列（mtime・created_at・title_key・
// size_bytes・played_at・favorited_at・watch_state）、利用者データの鍵 user_key、thumbnail_state
// である。所在は一覧と同じく見せてよい所在のパスの最小（chosen）で、別名 videos で読めば
// matchConditions と playableCondition の句がそのまま掛かる。
func smartBaseCTE(audience domain.Audience) (string, []any) {
	cte, args := chosenLocationsCTEFor(libraryScope(audience), audience, nil)
	// 利用者データの鍵は動画 1 本に 1 度だけ求め（smart_keys）、再生の記録・お気に入り・条件がそれを使う。
	return cte + `,
	smart_keys as materialized (
		select videos.id as id, videos.content_key as content_key, ` + userKeyExpr("videos") + ` as user_key,
			videos.added_at as added_at, videos.duration_ms as duration_ms, videos.playable as playable,
			videos.probe_state as probe_state, videos.thumbnail_state as thumbnail_state, chosen.path as path
		from chosen join videos on videos.id = chosen.video_id),
	smart_base as materialized (
		select k.id, k.content_key, k.user_key, k.added_at, k.duration_ms, k.playable, k.probe_state, k.thumbnail_state,
			loc.mtime as mtime, ` + fileCreatedAtExpr("loc") + ` as created_at, loc.title_key as title_key,
			loc.size_bytes as size_bytes, p.updated_at as played_at, fav.favorited_at as favorited_at,
			` + videoWatchStateExpr + ` as watch_state
		from smart_keys k join video_locations loc on loc.path = k.path
		left join playback_progress p on p.content_key = k.user_key and k.content_key <> ''
		left join video_favorites fav on fav.content_key = k.user_key and k.content_key <> '')`, args
}

// smartMatch はフォルダ 1 つの一致する動画の条件である。
type smartMatch struct {
	// search は検索式の段に当たる動画の id を持つ with 句の項目（`s<i> as materialized (...)`）と
	// その引数で、検索式が無ければ空である。
	search     string
	searchArgs []any
	// condition は smart_base（別名 videos）に対する条件句で、args はその引数である。
	condition string
	args      []any
}

// smartMatchFor は経路の条件 spec（smartFolderSpec）の一致する動画の条件を作る。name は検索式の
// with 句の名前である。どの段の絞り込みも動画 1 本に掛ける。タグ・人物と団体・追加日と長さは
// matchConditions、再生可否とお気に入りのみはどれかの段が求めれば掛け、視聴状態は段ごとの
// itemWatchConditions を動画の視聴状態に掛ける。検索式は所有者の意味で、どの段の式も同じ所在 1 行で
// 満たす動画に限る（chosenLocationsSelect、research.md R-11）。検索式は別の with 句に 1 度だけ
// 求める（条件句の副問い合わせにすると、フォルダが増えるほど遅くなる）。
func smartMatchFor(spec listSpec, name string) smartMatch {
	spec.userKeyColumn = `videos.user_key`
	conditions, args := matchConditions(spec)
	if spec.anyLevel(func(level listSpec) bool { return level.playableOnly }) {
		conditions = append(conditions, playableCondition)
	}
	if spec.anyLevel(func(level listSpec) bool { return level.favoriteOnly }) {
		conditions = append(conditions, `videos.favorited_at is not null`)
	}
	if watch, watchArgs := itemWatchConditions(spec); watch != "" {
		conditions = append(conditions, watch)
		args = append(args, watchArgs...)
	}
	var match smartMatch
	if slices.ContainsFunc(spec.searchExprs(), func(expr domain.SearchExpr) bool { return !expr.Empty() }) {
		search, searchArgs := chosenLocationsSelect(spec.scope, domain.AudienceOwner, spec.searchExprs())
		match.search, match.searchArgs = name+` as materialized (`+search+`)`, searchArgs
		conditions = append(conditions, `videos.id in (select video_id from `+name+`)`)
	}
	match.condition, match.args = `1`, args
	if len(conditions) > 0 {
		match.condition = strings.Join(conditions, " and ")
	}
	return match
}

// smartFolderTarget は 1 回の読み出しで値を求めるフォルダ 1 つである。
type smartFolderTarget struct {
	summary domain.SmartFolderSummary
	sort    domain.VideoSort
	// spec は経路の条件の絞り込み（smartFolderSpec）である。
	spec listSpec
	// missing は経路の条件が無い実体を名指すことで、そのときフォルダは何にも一致しない
	// （research.md R-9）。
	missing bool
}

// smartFolderTargetOf はフォルダ folder を経路の条件 rules（外側の祖先から順、最後が folder の条件）
// で読む準備をし、見る人 audience に見せるかとともに返す。所有者には常に、ゲストには再生順が空で
// ないときだけ見せる。ゲストには再生順の最初の項目があるかを 1 回の問い合わせで確かめる
// （data-model.md「Contents」の Shown、research.md R-8）。
func smartFolderTargetOf(ctx context.Context, tx *sql.Tx, audience domain.Audience, folder domain.SmartFolder, rules []domain.SmartRule, now int64) (smartFolderTarget, bool, error) {
	target := smartFolderTarget{
		summary: domain.SmartFolderSummary{ID: folder.ID, ParentID: folder.ParentID, Name: folder.Name},
		sort:    folder.Sort,
		spec:    smartFolderSpec(audience, rules, folder.Sort, now),
	}
	missing, err := missingSmartRuleRefs(ctx, tx, rules)
	if err != nil {
		return smartFolderTarget{}, false, err
	}
	target.missing = len(missing) > 0
	if err := tx.QueryRowContext(ctx, `select exists (select 1 from smart_folder_favorites where smart_folder_id = ?)`,
		folder.ID).Scan(&target.summary.Favorite); err != nil {
		return smartFolderTarget{}, false, fmt.Errorf("cannot read the smart folder favorite: %w", err)
	}
	if audience.IsOwner() {
		return target, true, nil
	}
	if target.missing {
		return target, false, nil
	}
	shown, err := smartFolderHasItems(ctx, tx, target.spec)
	return target, shown, err
}

// smartFolderTargetByID はフォルダ id を、その経路の条件で smartFolderTargetOf と同じく読む。
// フォルダが無ければ見せないものとして false を返す。
func smartFolderTargetByID(ctx context.Context, tx *sql.Tx, audience domain.Audience, id int64, now int64) (smartFolderTarget, bool, error) {
	folder, err := readSmartFolder(ctx, tx, id)
	if errors.Is(err, domain.ErrNotFound) {
		return smartFolderTarget{}, false, nil
	}
	if err != nil {
		return smartFolderTarget{}, false, err
	}
	rules, err := smartFolderPathRules(ctx, tx, id)
	if err != nil {
		return smartFolderTarget{}, false, err
	}
	return smartFolderTargetOf(ctx, tx, audience, folder, rules, now)
}

// shownSmartFolderTargets はフォルダ ids のうち、あって見る人に見せるものを ids の順に読む。
// 同じ id は最初の 1 回だけ読む。
func shownSmartFolderTargets(ctx context.Context, tx *sql.Tx, audience domain.Audience, ids []int64, now int64) ([]smartFolderTarget, error) {
	targets := make([]smartFolderTarget, 0, len(ids))
	seen := make(map[int64]bool, len(ids))
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		target, shown, err := smartFolderTargetByID(ctx, tx, audience, id, now)
		if err != nil {
			return nil, err
		}
		if shown {
			targets = append(targets, target)
		}
	}
	return targets, nil
}

// smartFolderHasItems は、経路の条件 spec の中身（ライブラリの項目の問い合わせ）に項目が 1 つでも
// あるか、つまり再生順が空でないかを返す。
func smartFolderHasItems(ctx context.Context, tx *sql.Tx, spec listSpec) (bool, error) {
	cte, args := libraryItemsCTE(spec)
	where, watchArgs := itemsWhere(spec)
	var found bool
	if err := tx.QueryRowContext(ctx, cte+` select exists (select 1 from items`+where+`)`,
		append(args, watchArgs...)...).Scan(&found); err != nil {
		return false, fmt.Errorf("cannot read whether the smart folder has items: %w", err)
	}
	return found, nil
}

// itemsWhere は spec の各段の視聴状態の絞り込みを、items に対する where 句（絞り込まなければ空）と
// その引数にする。
func itemsWhere(spec listSpec) (string, []any) {
	clause, args := itemWatchConditions(spec)
	if clause == "" {
		return "", nil
	}
	return ` where ` + clause, args
}

// smartFolderQuery は、フォルダごとの断片を smart_base の後ろに並べた問い合わせの組み立てである。
type smartFolderQuery struct {
	// ctes は with 句に足す項目（検索式とフォルダごとの項目）で、cteArgs はその引数である。
	ctes    []string
	cteArgs []any
	// parts は with 句の後ろに union all で並べる select 文で、partArgs はその引数である。
	parts    []string
	partArgs []any
	// same は、前のフォルダと条件も並べ順も同じで断片を足さなかったフォルダの位置から、その前の
	// フォルダの位置への対応である。呼び出し側は値を写す。
	same map[int]int
}

// smartFolderBranches は、値を求めるフォルダ targets のうち何かに一致しうるもの（missing でない
// もの）ごとに、branch で問い合わせの断片を足す。branch はフォルダの位置 index と一致する動画の条件
// match を受け取り、query.ctes・query.parts とその引数に足す。検索式の with 句はここで足す。経路の
// 条件と並べ順が前のフォルダと同じフォルダは、断片を足さず query.same に記す（同じ絞り込みを 2 度
// 求めない）。
func smartFolderBranches(targets []smartFolderTarget, branch func(query *smartFolderQuery, index int, target smartFolderTarget, match smartMatch)) smartFolderQuery {
	query := smartFolderQuery{same: map[int]int{}}
	first := map[string]int{}
	for i, target := range targets {
		if target.missing {
			continue
		}
		key := smartMatchFor(target.spec, `s`)
		identity := fmt.Sprintf("%s\x00%s\x00%v\x00%s\x00%v", target.sort, key.search, key.searchArgs, key.condition, key.args)
		if earlier, ok := first[identity]; ok {
			query.same[i] = earlier
			continue
		}
		first[identity] = i
		match := smartMatchFor(target.spec, `s`+strconv.Itoa(i))
		if match.search != "" {
			query.ctes = append(query.ctes, match.search)
			query.cteArgs = append(query.cteArgs, match.searchArgs...)
		}
		branch(&query, i, target, match)
	}
	return query
}

// run は audience に見せる動画の smart_base に続けて断片を並べた問い合わせを実行する。断片が無ければ
// nil を返す。
func (q smartFolderQuery) run(ctx context.Context, tx *sql.Tx, audience domain.Audience) (*sql.Rows, error) {
	if len(q.parts) == 0 {
		return nil, nil
	}
	cte, args := smartBaseCTE(audience)
	for _, item := range q.ctes {
		cte += `, ` + item
	}
	args = append(append(args, q.cteArgs...), q.partArgs...)
	return tx.QueryContext(ctx, cte+` `+strings.Join(q.parts, ` union all `), args...)
}
