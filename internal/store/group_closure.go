package store

// グループの構造の表 group_closure と、根ごとのグループの項目の値 group_root_values の維持
// （specs/057-home/data-model.md「Group closure」、research.md R-12）。
//
// group_closure は、いちばん上のグループ（根）ごとの子孫の動画の項目の鍵と再生順の鍵で、グループの
// 構造を変える書き込み（GroupStore と OrganizeStore の書き込み、フォルダの自動グループの同期、集まりの
// 鍵の付け替え）が同じ取引で作り直す（rebuildGroupClosure）。groups と group_items への書き込みは引き金で
// group_closure_dirty に印を付けるので、作り直しを通らない書き込みがあっても、書き込みの確定のあとの維持が
// 印を見て作り直す（maintainHomeIndexes）。
//
// group_root_values は見る人ごとに見せる子孫から集めた値で、値を変える書き込みが引き金で根の行を消し、
// 書き込みの確定のあとの維持（home_index_maintenance.go）が欠けた根の行を埋め直す。読み出しは書かず、
// まだ欠けている根だけその場で集める。

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"github.com/syudead/vv/internal/domain"
)

// rebuildGroupClosure は group_closure をすべてのグループについて作り直し（お気に入りのみの棚は入れ子の
// グループも根にする）、根の値を消し、印を下ろす。
func rebuildGroupClosure(ctx context.Context, tx *sql.Tx) error {
	statements := []string{
		`delete from group_closure`,
		`insert into group_closure (root_id, content_key, sort_key) with ` +
			groupDescendantsCTEFrom(`select id from groups`, `gi.content_key`, ``) + ` select root_id, video_id, sort_key from gdesc`,
		`delete from group_root_values`,
		`update group_closure_dirty set dirty = 0`,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("cannot rebuild the group closure: %w", err)
		}
	}
	return nil
}

// groupClosureDirty は、グループの構造が変わってから group_closure を作り直していないかを返す。
func groupClosureDirty(ctx context.Context, q queryRower) (bool, error) {
	var dirty bool
	if err := q.QueryRowContext(ctx, `select dirty from group_closure_dirty where id = 1`).Scan(&dirty); err != nil {
		return false, fmt.Errorf("cannot read whether the group closure is current: %w", err)
	}
	return dirty, nil
}

// rootValuesSelect は、根の集合 roots（1 列 root_id の select 文）の根ごとに、見る人 audience に見せる
// 子孫から集めたグループの項目の値を返す select 文とその引数を返す。condition は videos の行に掛ける棚の
// 条件（空なら数えない）で、matched はそれに当たる子孫の本数である。列は root_id, members, matched,
// added_at, mtime, created_at, played_at, duration_ms, size_bytes, watched, started, playable, added_member,
// mtime_member, created_member, played_member, title_key, favorited_at の順である。
func rootValuesSelect(audience domain.Audience, roots, condition string, args []any) (string, []any) {
	matched := `0`
	if condition != "" {
		matched = `coalesce(sum((` + condition + `) is true), 0)`
	}
	member := func(column string) string {
		return `cast(substr(max(printf('%020d%020d', coalesce(` + column + `, 0), videos.id)), 21) as integer)`
	}
	return `with r(root_id) as (` + roots + `),
		m as (select c.root_id, c.content_key, ` + shownVideoForKeyExpr("c.content_key", audience) + ` as video_id
			from group_closure c join r on r.root_id = c.root_id)
		select m.root_id, count(videos.id), ` + matched + `, max(videos.added_at), max(loc.mtime),
			max(` + fileCreatedAtExpr("loc") + `), max(p.updated_at), sum(videos.duration_ms), sum(loc.size_bytes),
			coalesce(sum(p.completed = 1), 0), coalesce(sum(p.completed = 1 or p.position_ms > 0), 0),
			coalesce(max(` + playableCondition + `), 0),
			` + member(`videos.added_at`) + `, ` + member(`loc.mtime`) + `, ` + member(fileCreatedAtExpr("loc")) + `,
			` + member(`p.updated_at`) + `, (select g.title_key from groups g where g.id = m.root_id),
			(select gf.favorited_at from group_favorites gf where gf.group_id = m.root_id)
		from m join videos on videos.id = m.video_id
		left join video_locations loc on loc.path = (select min(l.path) from video_locations l where l.video_id = videos.id
			and ` + visibleLocationCondition("l", audience) + `)
		left join playback_progress p on p.content_key = m.content_key
		group by m.root_id`, args
}

// audienceColumn は group_root_values の audience の値である。
func audienceColumn(audience domain.Audience) int {
	if audience.IsOwner() {
		return 1
	}
	return 0
}

// refillGroupRootValues は、見る人 audience の値の行の無いグループの値を group_root_values に入れる。
func refillGroupRootValues(ctx context.Context, tx *sql.Tx, audience domain.Audience) error {
	missing, err := queryInt64s(ctx, tx, `select g.id from groups g
		where not exists (select 1 from group_root_values v where v.root_id = g.id and v.audience = ?)`, audienceColumn(audience))
	if err != nil {
		return fmt.Errorf("cannot find the groups without values: %w", err)
	}
	if len(missing) == 0 {
		return nil
	}
	values, err := readRootValues(ctx, tx, audience, missing, "", nil)
	if err != nil {
		return err
	}
	for _, root := range missing {
		row := values[root]
		if _, err := tx.ExecContext(ctx, `insert or replace into group_root_values (root_id, audience, members, added_at, mtime, created_at, played_at,
			duration_ms, size_bytes, watched, started, playable, added_member, mtime_member, created_member, played_member)
			values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, root, audienceColumn(audience), row.members, row.addedAt, row.mtime, row.createdAt, row.playedAt,
			row.durationMs, row.sizeBytes, row.watched, row.started, row.playable, nullableID(row.member[`added_at`]),
			nullableID(row.member[`mtime`]), nullableID(row.member[`created_at`]), nullableID(row.member[`played_at`])); err != nil {
			return fmt.Errorf("cannot refill the group values: %w", err)
		}
	}
	return nil
}

// rebuildFolderIndex はフォルダの自動グループを同期し（rebuildFolderIndexOnly）、グループの構造の表を
// 作り直す。
func rebuildFolderIndex(ctx context.Context, tx *sql.Tx) ([]int64, error) {
	changed, err := rebuildFolderIndexOnly(ctx, tx)
	if err != nil {
		return nil, err
	}
	return changed, rebuildGroupClosure(ctx, tx)
}

// rekeyGroupItems は鍵 from の項目を鍵 to へ付け替え（rekeyGroupItemsOnly）、グループの構造の表を
// 作り直す。
func rekeyGroupItems(ctx context.Context, tx *sql.Tx, from, to string) ([]int64, error) {
	changed, err := rekeyGroupItemsOnly(ctx, tx, from, to)
	if err != nil {
		return nil, err
	}
	return changed, rebuildGroupClosure(ctx, tx)
}

// rootValuesRow は根 1 つの値（rootValuesSelect の 1 行）である。
type rootValuesRow struct {
	rootID     int64
	members    int
	matched    int
	addedAt    sql.NullInt64
	mtime      sql.NullInt64
	createdAt  sql.NullInt64
	playedAt   sql.NullInt64
	durationMs sql.NullInt64
	sizeBytes  sql.NullInt64
	watched    int
	started    int
	playable   bool
	member     map[string]int64
	// titleKey と favoritedAt はグループ自身の題名の鍵とお気に入りにした日時である（集めた行だけが持つ）。
	titleKey    string
	favoritedAt sql.NullInt64
}

// scanRootValues は rootValuesSelect の行を読む。
func scanRootValues(rows *sql.Rows) (rootValuesRow, error) {
	var row rootValuesRow
	var added, mtime, created, played sql.NullInt64
	var title sql.NullString
	if err := rows.Scan(&row.rootID, &row.members, &row.matched, &row.addedAt, &row.mtime, &row.createdAt, &row.playedAt,
		&row.durationMs, &row.sizeBytes, &row.watched, &row.started, &row.playable, &added, &mtime, &created, &played,
		&title, &row.favoritedAt); err != nil {
		return rootValuesRow{}, fmt.Errorf("cannot read the group values: %w", err)
	}
	row.member = map[string]int64{`added_at`: added.Int64, `mtime`: mtime.Int64, `created_at`: created.Int64, `played_at`: played.Int64}
	row.titleKey = title.String
	return row, nil
}

// watchState はグループの視聴状態（groupWatchStateExpr と同じ決め方）である。
func (r rootValuesRow) watchState() string {
	switch {
	case r.started == 0:
		return string(domain.WatchUnwatched)
	case r.watched == r.members:
		return string(domain.WatchWatched)
	}
	return string(domain.WatchInProgress)
}

// readRootValues は、見る人 audience の根 roots の値と、条件 condition（引数 args）に当たる子孫の本数を読む。
func readRootValues(ctx context.Context, tx *sql.Tx, audience domain.Audience, roots []int64, condition string, args []any) (map[int64]rootValuesRow, error) {
	ids := make([]string, len(roots))
	for i, root := range roots {
		ids[i] = strconv.FormatInt(root, 10)
	}
	query, queryArgs := rootValuesSelect(audience, `select value from json_each(?)`, condition, args)
	rows, err := tx.QueryContext(ctx, query, append([]any{`[` + strings.Join(ids, ",") + `]`}, queryArgs...)...)
	if err != nil {
		return nil, fmt.Errorf("cannot read the group values: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make(map[int64]rootValuesRow, len(roots))
	for rows.Next() {
		row, err := scanRootValues(rows)
		if err != nil {
			return nil, err
		}
		out[row.rootID] = row
	}
	return out, rows.Err()
}

// readStoredRootValues は見る人 audience の根 roots の値を group_root_values から読む。行の無い根は結果に無い。
func readStoredRootValues(ctx context.Context, tx *sql.Tx, audience domain.Audience, roots []int64) (map[int64]rootValuesRow, error) {
	ids := make([]string, len(roots))
	for i, root := range roots {
		ids[i] = strconv.FormatInt(root, 10)
	}
	rows, err := tx.QueryContext(ctx, `select root_id, members, 0, added_at, mtime, created_at, played_at, duration_ms, size_bytes,
		watched, started, playable, added_member, mtime_member, created_member, played_member, null, null
		from group_root_values where audience = ? and root_id in (select value from json_each(?))`, audienceColumn(audience), `[`+strings.Join(ids, ",")+`]`)
	if err != nil {
		return nil, fmt.Errorf("cannot read the group values: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make(map[int64]rootValuesRow, len(roots))
	for rows.Next() {
		row, err := scanRootValues(rows)
		if err != nil {
			return nil, err
		}
		out[row.rootID] = row
	}
	return out, rows.Err()
}

// readWholeRoots は、見る人 audience の根 roots のうち、見せる子孫のすべてが条件 condition（引数 args）に
// 当たる根を返す。条件が NULL になる子孫は当たらない。当たらない子孫が 1 本見つかれば、その根の子孫は
// それ以上見ない。
func readWholeRoots(ctx context.Context, tx *sql.Tx, audience domain.Audience, roots []int64, condition string, args []any) (map[int64]bool, error) {
	ids := make([]string, len(roots))
	for i, root := range roots {
		ids[i] = strconv.FormatInt(root, 10)
	}
	rows, err := tx.QueryContext(ctx, `with r(root_id) as (select value from json_each(?))
		select r.root_id from r where not exists (
			select 1 from group_closure c join videos on videos.id = `+shownVideoForKeyExpr("c.content_key", audience)+`
			where c.root_id = r.root_id and (`+condition+`) is not true)`,
		append([]any{`[` + strings.Join(ids, ",") + `]`}, args...)...)
	if err != nil {
		return nil, fmt.Errorf("cannot judge the group members: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make(map[int64]bool, len(roots))
	for rows.Next() {
		var root int64
		if err := rows.Scan(&root); err != nil {
			return nil, fmt.Errorf("cannot judge the group members: %w", err)
		}
		out[root] = true
	}
	return out, rows.Err()
}
