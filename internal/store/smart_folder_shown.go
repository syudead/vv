package store

// ゲストに見せるスマートフォルダ（specs/052-smart-folders/data-model.md「Contents」の Shown、
// research.md R-8）。ゲストにはフォルダの再生順が空でないときだけ見せる。再生順が空でないのは、
// ライブラリの項目の問い合わせ（libraryItemsCTE）の項目が 1 つでもあるときである。フォルダごとに
// その問い合わせを読むと、フォルダの数だけ見せる動画とグループの子孫の動画を求めることになる
// （ホームの 50 の棚で十数秒、specs/057-home/research.md R-12）。ここではカードの読み出しと同じく
// smart_base を 1 度だけ求め、どのフォルダの答えも 1 つの問い合わせで、項目の問い合わせと同じ
// 規則で求める。
//
//   - 当たった動画（matched）は smart_base のうち経路の条件（smartRuleMatch）に一致する動画である。
//   - 視聴状態・再生可否の絞り込みの無いフォルダは、当たった動画があれば項目がある（当たった動画は
//     動画の項目か、丸ごと当たったグループの項目に必ず入る）。
//   - そうでなければ、丸ごと当たったグループ（whole）を求め、絞り込みに合う動画の項目かグループの
//     項目があるかを確かめる。動画の項目は、丸ごと当たったグループのカードにだけ出る動画
//     （libraryItemsPart の carded）でない当たった動画である。グループの視聴状態と
//     再生可否は子孫の動画のすべてから求める（mv と whole の groupPlayable と同じ）。
//
// お気に入りのみのフォルダは、グループの項目になりうるものが違う（itemGroupsFavorite）ので、
// フォルダごとに smartFolderHasItems で確かめる。

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strconv"

	"github.com/syudead/vv/internal/domain"
)

// smartFoldersHaveItems は、見る人 audience のフォルダ targets のそれぞれの再生順が空でないかを
// targets の順に返す。missing のフォルダは偽である。
func smartFoldersHaveItems(ctx context.Context, tx *sql.Tx, audience domain.Audience, targets []smartFolderTarget) ([]bool, error) {
	found := make([]bool, len(targets))
	batched := make([]smartFolderTarget, len(targets))
	for i, target := range targets {
		batched[i] = target
		if target.missing || !target.spec.itemGroupsFavorite() {
			continue
		}
		batched[i].missing = true
		has, err := smartFolderHasItems(ctx, tx, target.spec)
		if err != nil {
			return nil, err
		}
		found[i] = has
	}
	groups := false
	query := smartFolderBranches(batched, func(query *smartFolderQuery, index int, target smartFolderTarget, _ smartMatch) {
		part, args, needsGroups := smartFolderShownPart(query, index, target.spec)
		query.parts = append(query.parts, part)
		query.partArgs = append(query.partArgs, args...)
		groups = groups || needsGroups
	})
	if groups {
		query.ctes = append([]string{shownGroupsCTE(audience)}, query.ctes...)
	}
	rows, err := query.run(ctx, tx, audience)
	if err != nil {
		return nil, fmt.Errorf("cannot read whether the smart folders have items: %w", err)
	}
	if rows == nil {
		return found, nil
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var index int
		var has bool
		if err := rows.Scan(&index, &has); err != nil {
			return nil, fmt.Errorf("cannot read whether the smart folders have items: %w", err)
		}
		found[index] = has
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read whether the smart folders have items: %w", err)
	}
	for index, earlier := range query.same {
		found[index] = found[earlier]
	}
	return found, rows.Close()
}

// shownGroupsCTE は、いちばん上のグループの見せてよい子孫の動画 gm（itemGroupsCTE）と、グループごとの
// 子孫の動画の数・視聴状態・再生できる動画があるか shown_groups(group_id, n, watch_state, playable)
// を定める with 句の項目である。
func shownGroupsCTE(audience domain.Audience) string {
	return itemGroupsCTE(listSpec{scope: libraryScope(audience)}) + `,
	shown_groups as materialized (
		select gm.group_id, count(*) as n,
			` + groupWatchStateExpr("coalesce(p.completed, 0)", "coalesce(p.position_ms, 0)") + ` as watch_state,
			max(` + playableCondition + `) as playable
		from gm join videos on videos.id = gm.video_id
		left join playback_progress p on p.content_key = ` + userKeyExpr("videos") + ` and videos.content_key <> ''
		group by gm.group_id)`
}

// smartFolderShownPart は、フォルダ index（経路の条件 spec）の再生順が空でないかを返す select 文と
// その引数を返し、当たった動画 sm<index>、丸ごと当たったグループ sw<index>、それらのカードにだけ出る
// 動画 sc<index>（libraryItemsPart の carded）を query.ctes に足す。
// needsGroups は文が shownGroupsCTE を読むかである。
func smartFolderShownPart(query *smartFolderQuery, index int, spec listSpec) (part string, args []any, needsGroups bool) {
	name := strconv.Itoa(index)
	matched, whole, carded := `sm`+name, `sw`+name, `sc`+name
	rule := smartRuleMatch(spec, `s`+name)
	query.ctes = append(query.ctes, matched+` as materialized (select id from smart_base videos where `+rule.condition+`)`)
	query.cteArgs = append(query.cteArgs, rule.args...)
	playable := spec.anyLevel(func(level listSpec) bool { return level.playableOnly })
	watch, watchArgs := itemWatchConditions(spec)
	if !playable && watch == "" {
		return smartFolderMatchesPart(name, spec, rule)
	}
	query.ctes = append(query.ctes, whole+` as materialized (
		select h.group_id from (select gm.group_id, count(*) as n from `+matched+` join gm on gm.video_id = `+matched+`.id
			group by gm.group_id) h
		join shown_groups g on g.group_id = h.group_id and g.n = h.n`+andIf(playable, `g.playable`)+`)`)
	videoWhere, groupWhere := andIf(watch != "", watch), andIf(watch != "", watch)
	if playable {
		videoWhere += ` and ` + playableCondition
	}
	query.ctes = append(query.ctes, carded+` as materialized (
		select video_id from gm where group_id in (select group_id from `+whole+`)
		except
		select video_id from gm where group_id not in (select group_id from `+whole+`))`)
	part = `select ` + name + `, exists (select 1 from ` + matched + ` m join smart_base videos on videos.id = m.id
			where videos.id not in (select video_id from ` + carded + `)` + videoWhere + `)
		or exists (select 1 from ` + whole + ` w join shown_groups g on g.group_id = w.group_id where 1` + groupWhere + `)`
	return part, append(append(args, watchArgs...), watchArgs...), true
}

// smartFolderMatchesPart は、視聴状態・再生可否の絞り込みの無いフォルダ name（経路の条件 spec、
// 一致の条件 rule）に当たった動画があるかを返す select 文とその引数である。検索式は動画ごとに
// 所在を確かめ、最初に当たった動画で止める（検索式の with 句をすべて求めない）。
func smartFolderMatchesPart(name string, spec listSpec, rule smartMatch) (string, []any, bool) {
	if !spec.hasSearch() {
		return `select ` + name + `, exists (select 1 from sm` + name + `)`, nil, false
	}
	from, args := chosenLocationsFrom(spec.scope, domain.AudienceOwner, spec.searchExprs())
	return `select ` + name + `, exists (select 1 from smart_base videos where ` + rule.filter + `
		and exists (select 1 ` + from + ` and l.video_id = videos.id))`, append(slices.Clone(rule.args), args...), false
}

// andIf は ok なら ` and ` に続けた clause を、そうでなければ空を返す。
func andIf(ok bool, clause string) string {
	if !ok {
		return ""
	}
	return ` and ` + clause
}
