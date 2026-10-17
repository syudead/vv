package store

// グループの読み出し。
// グループの値・ライブラリの項目・グループの画面は、どれもグループの子孫の動画を再生順で読む
// 1 つの with 句（groupDescendantsCTE）から作る。

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// shownVideoForKeyExpr は、利用者データの鍵の式 key に対して見る人（audience）に見せる動画の id を
// 返す式である。集まりの鍵なら集まりの実効の代表、そうでなければ集まりに属さない同じ content_key の
// 動画で、見る人に見せてよい所在が無ければ NULL になる。
func shownVideoForKeyExpr(key string, audience domain.Audience) string {
	bundleID := `(select sb.id from video_bundles sb where sb.user_key = ` + key + `)`
	return `(case when exists (select 1 from video_bundles sb where sb.user_key = ` + key + `) then ` +
		effectiveRepresentativeExpr(bundleID, audience) + `
		else (select sv.id from videos sv where sv.content_key = ` + key + ` and sv.content_key <> ''
			and not exists (select 1 from video_bundle_members sm where sm.content_key = sv.content_key)
			and ` + visibleVideoCondition("sv", audience) + ` order by sv.id limit 1) end)`
}

// groupSortWidth は再生順の並びの鍵で、位置 1 つを表す桁数である。位置を同じ桁の 0 詰めの
// 文字列にしてつなぐと、鍵のバイト順が深さ優先の順（親の項目の位置、その中の位置、…）になる。
const groupSortWidth = 10

// groupDescendantsCTE は、グループ id を 1 列 id で返す select 文 roots の各グループについて、
// 見る人に見せる子孫の動画を再生順の鍵とともに返す with 句の部品を返す。
// 呼び出し側が読むのは `gdesc(root_id, video_id, sort_key)` だけで、根と動画の組ごとに 1 行、
// sort_key は深さ優先で最初に現れた位置の鍵である。見せる動画の無い根は gdesc に現れない。
// 呼び出し側は先頭に with を付けるか、ほかの with 句の部品に続けてつなぐ。roots は 1 度だけ
// 埋め込むので、位置で結び付ける ? を持ってよい。
//
// 複数の親に入った子グループ（ダイヤモンド状の階層）を、たどれる道ごとに展開すると、行数が段数に
// 対して指数的に増える。そこで道ではなく、根と子孫のグループの組ごとに 1 本だけ、深さ優先で最初に
// 現れる道（位置の鍵がいちばん小さい道）を求める。
//   - gclos(anc, descendant) は根から届くグループの推移閉包（自分自身を含む）。union で重複を
//     捨てるので、行数はグループの組の数で抑えられ、壊れた行の循環があっても終わる。
//   - gpath は根から目的のグループへ、目的に届く項目のうち位置のいちばん小さいものを 1 段ずつ選んで
//     進む。どの道も同じグループを 2 度通らない（通るなら循環）ので、どの位置の鍵も別の道の鍵の
//     接頭辞にならず、最初の道の鍵に中の位置を足した鍵が、すべての道の鍵のうち最小になる。
//     seen は壊れた行の循環で止めるためのもの。
//
// 入れ子の深さに上限は無い。
func groupDescendantsCTE(roots string, audience domain.Audience) string {
	return groupDescendantsCTEFrom(roots, shownVideoForKeyExpr("gi.content_key", audience), "")
}

// groupDescendantsCTEFrom は groupDescendantsCTE の本体で、グループの項目 gi の動画を videoExpr で
// 求める。videoJoin は gi に続けて結ぶ join 句（無ければ空）で、見せる動画を 1 行に持つ表があれば、
// 鍵で結んで動画ごとの副問い合わせを省ける（home_shelf_read.go）。
func groupDescendantsCTEFrom(roots, videoExpr, videoJoin string) string {
	width := strconv.Itoa(groupSortWidth)
	return `groots(id) as (select distinct id from (` + roots + `)),
	gnodes(id) as (
		select id from groots
		union
		select gi.child_id from gnodes n join group_items gi on gi.group_id = n.id
		where gi.child_id is not null),
	gclos(anc, descendant) as materialized (
		select id, id from gnodes
		union
		select c.anc, gi.child_id from gclos c join group_items gi on gi.group_id = c.descendant
		where gi.child_id is not null),
	gpath(root_id, group_id, target, sort_path, seen) as (
		select r.id, r.id, c.descendant, '', ',' || r.id || ','
		from groots r join gclos c on c.anc = r.id
		union all
		select p.root_id, gi.child_id, p.target, p.sort_path || printf('%0` + width + `d', gi.position),
			p.seen || gi.child_id || ','
		from gpath p join group_items gi on gi.group_id = p.group_id
		where p.group_id <> p.target and gi.child_id is not null
			and instr(p.seen, ',' || gi.child_id || ',') = 0
			and gi.position = (select min(n.position) from group_items n
				join gclos nc on nc.anc = n.child_id and nc.descendant = p.target
				where n.group_id = p.group_id)),
	gdesc(root_id, video_id, sort_key) as materialized (
		select root_id, video_id, min(sort_key) from (
			select t.root_id, ` + videoExpr + ` as video_id,
				t.sort_path || printf('%0` + width + `d', gi.position) as sort_key
			from gpath t join group_items gi on gi.group_id = t.target` + videoJoin + `
			where t.group_id = t.target and gi.content_key is not null)
		where video_id is not null
		group by root_id, video_id)`
}

// groupFavoritedAtExpr は、グループ（別名 alias の groups）をお気に入りにした時刻を返す式である。
// お気に入りでなければ NULL になる（group_favorites、docs/design-docs/catalog-organization.md）。
func groupFavoritedAtExpr(alias string) string {
	return `(select gf.favorited_at from group_favorites gf where gf.group_id = ` + alias + `.id)`
}

// memberProgressSelect は、videos の行に再生の記録を並べる列 4 つと、その結合を返す。ゲストには
// 再生の記録を読まない（016 の guest-api §1）ので、列はどれも NULL で結合は空である。列は
// memberProgress.dest の順に読む。
func memberProgressSelect(audience domain.Audience) (columns, join string) {
	if !audience.IsOwner() {
		return `null, null, null, null`, ""
	}
	return `p.position_ms, p.duration_ms, p.completed, p.updated_at`,
		` left join playback_progress p on p.content_key = ` + userKeyExpr("videos") + ` and videos.content_key <> ''`
}

// memberProgress は memberProgressSelect の列 4 つを受け取る。
type memberProgress struct {
	position, duration, updatedAt sql.NullInt64
	completed                     sql.NullBool
}

func (m *memberProgress) dest() []any {
	return []any{&m.position, &m.duration, &m.completed, &m.updatedAt}
}

// progress は受け取った再生の記録を返す。記録が無ければ nil である。
func (m *memberProgress) progress() *domain.Progress {
	if !m.position.Valid {
		return nil
	}
	return &domain.Progress{
		PositionMs: m.position.Int64, DurationMs: m.duration.Int64,
		Completed: m.completed.Bool, UpdatedAt: time.Unix(m.updatedAt.Int64, 0),
	}
}

// loadGroups はグループの id ごとに、見せてよい子孫の動画を再生順に読み、グループの項目を作る。
// ゲストには再生の記録を読まない（016 の guest-api §1）。見せてよい動画が無いグループと、無い
// グループは結果に入らない。
func loadGroups(ctx context.Context, q queryExecer, audience domain.Audience, groupIDs []int64) (map[int64]domain.LibraryGroup, error) {
	out := map[int64]domain.LibraryGroup{}
	if len(groupIDs) == 0 {
		return out, nil
	}
	encoded, err := json.Marshal(groupIDs)
	if err != nil {
		return nil, fmt.Errorf("cannot build group ids: %w", err)
	}
	progressColumns, progressJoin := memberProgressSelect(audience)
	rows, err := q.QueryContext(ctx, `with `+groupDescendantsCTE(`select distinct value as id from json_each(?)`, audience)+`
		select `+videoColumns(audience)+`, g.id, coalesce(g.origin_folder, ''), g.name, `+
		groupFavoritedAtExpr("g")+` is not null, `+progressColumns+`
		from gdesc d join groups g on g.id = d.root_id
		join videos on videos.id = d.video_id`+representativeJoin(audience)+progressJoin+`
		order by g.id, d.sort_key`, string(encoded))
	if err != nil {
		return nil, fmt.Errorf("cannot read the group videos: %w", err)
	}
	defer func() { _ = rows.Close() }()

	type collected struct {
		path, name string
		favorite   bool
		members    []domain.Video
		progress   []*domain.Progress
	}
	var order []int64
	groups := map[int64]*collected{}
	for rows.Next() {
		var groupID int64
		var path, name string
		var favorite bool
		var scanned memberProgress
		video, err := scanVideo(extraScanner{rows: rows, extra: append([]any{
			&groupID, &path, &name, &favorite}, scanned.dest()...)})
		if err != nil {
			return nil, fmt.Errorf("cannot read the group videos: %w", err)
		}
		group := groups[groupID]
		if group == nil {
			group = &collected{path: path, name: name, favorite: favorite}
			groups[groupID] = group
			order = append(order, groupID)
		}
		group.members = append(group.members, video)
		group.progress = append(group.progress, scanned.progress())
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the group videos: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("cannot read the group videos: %w", err)
	}
	memberLists := make([][]domain.Video, 0, len(order))
	for _, id := range order {
		memberLists = append(memberLists, groups[id].members)
	}
	if err := attachVersionsEach(ctx, q, audience, memberLists); err != nil {
		return nil, err
	}
	for _, id := range order {
		group := groups[id]
		item := domain.NewLibraryGroup(id, group.path, group.name, group.members, group.progress)
		item.Favorite = group.favorite
		out[id] = item
	}
	return out, nil
}

// Group は見る人（audience）に見せるグループ id と、その項目の最初の 1 ページ、要求の trail の
// 親と子の組が続く部分を返す（GET /api/groups/{id}、docs/design-docs/catalog-organization.md）。
// 無いグループと見る人に見せないグループは domain.ErrNotFound である。
func (s *LibraryStore) Group(ctx context.Context, audience domain.Audience, id int64, trail []int64) (domain.GroupPage, error) {
	return s.readGroupPage(ctx, audience, id, func(tx *sql.Tx, page *domain.GroupPage) error {
		var err error
		page.Trail, err = groupTrail(ctx, tx, id, trail)
		if err != nil {
			return err
		}
		page.Items, page.NextCursor, err = groupItemsPage(ctx, tx, audience, id, "", domain.GroupItemsDefaultLimit)
		return err
	})
}

// GroupItems はグループ id の項目 1 ページを返す（GET /api/groups/{id}/items）。cursor は前の
// ページの NextCursor で、読めなければ domain.ErrInvalidCursor である。グループの扱いは Group と同じ。
func (s *LibraryStore) GroupItems(ctx context.Context, audience domain.Audience, id int64, cursor string, limit int) (domain.GroupPage, error) {
	return s.readGroupPage(ctx, audience, id, func(tx *sql.Tx, page *domain.GroupPage) error {
		var err error
		page.Items, page.NextCursor, err = groupItemsPage(ctx, tx, audience, id, cursor, limit)
		return err
	})
}

// readGroupPage はグループ id の値と登録フォルダを 1 つの読み取りスナップショットで読み、同じ
// 取引で fill にページの残りを埋めさせる。
func (s *LibraryStore) readGroupPage(ctx context.Context, audience domain.Audience, id int64, fill func(*sql.Tx, *domain.GroupPage) error) (domain.GroupPage, error) {
	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.GroupPage{}, fmt.Errorf("cannot start reading the group: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	groups, err := loadGroups(ctx, tx, audience, []int64{id})
	if err != nil {
		return domain.GroupPage{}, err
	}
	group, ok := groups[id]
	if !ok {
		return domain.GroupPage{}, domain.ErrNotFound
	}
	page := domain.GroupPage{Group: group}
	if err := fill(tx, &page); err != nil {
		return domain.GroupPage{}, err
	}
	page.Roots, err = listMediaFolders(ctx, tx)
	if err != nil {
		return domain.GroupPage{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.GroupPage{}, fmt.Errorf("cannot finish reading the group: %w", err)
	}
	return page, nil
}

// groupTrail は trail（外側から順のグループの id）のうち、グループ id から外へ向かって親と子の組が
// 続く部分を、外側から順に名前とともに返す。最後の要素は id の親で、組が崩れた
// 所より外側は捨てる。
func groupTrail(ctx context.Context, tx *sql.Tx, id int64, trail []int64) ([]domain.GroupRef, error) {
	out := []domain.GroupRef{}
	child := id
	for i := len(trail) - 1; i >= 0; i-- {
		var name string
		err := tx.QueryRowContext(ctx, `select g.name from groups g
			where g.id = ? and exists (select 1 from group_items i where i.group_id = g.id and i.child_id = ?)`,
			trail[i], child).Scan(&name)
		if errors.Is(err, sql.ErrNoRows) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("cannot read the group trail: %w", err)
		}
		out = append(out, domain.GroupRef{ID: trail[i], Name: name})
		child = trail[i]
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

// groupItemsCursorKind はグループの項目のカーソルの最初の項目である。ほかの一覧のカーソルを
// 渡されたときに読めないカーソルとして断る。
const groupItemsCursorKind = "group-items"

// groupItemsPage はグループ id の見せる項目を、位置が cursor の項目より後ろから limit 件、位置の
// 順に返す。動画の項目は見る人に見せる動画（shownVideoForKeyExpr）、子グループの項目は見る人に
// 見せる子孫の動画を持つものだけを数える。
func groupItemsPage(ctx context.Context, tx *sql.Tx, audience domain.Audience, id int64, cursor string, limit int) ([]domain.GroupItem, string, error) {
	after := int64(-1)
	if cursor != "" {
		parts, err := unpackCursor(cursor, 2)
		if err != nil {
			return nil, "", err
		}
		position, err := strconv.ParseInt(parts[1], 10, 64)
		if parts[0] != groupItemsCursorKind || err != nil || position < 0 {
			return nil, "", fmt.Errorf("%w: not a group item cursor", domain.ErrInvalidCursor)
		}
		after = position
	}
	if limit <= 0 {
		limit = domain.GroupItemsDefaultLimit
	}
	limit = min(limit, domain.GroupItemsMaxLimit)

	rows, err := tx.QueryContext(ctx, `with `+groupDescendantsCTE(`select child_id as id from group_items
			where group_id = ?1 and child_id is not null`, audience)+`
		select id, position, video_id, child_id from (
			select gi.id, gi.position, gi.child_id,
				case when gi.content_key is not null then `+shownVideoForKeyExpr("gi.content_key", audience)+` end as video_id
			from group_items gi where gi.group_id = ?1 and gi.position > ?2)
		where video_id is not null or child_id in (select root_id from gdesc)
		order by position limit ?3`, id, after, limit+1)
	if err != nil {
		return nil, "", fmt.Errorf("cannot read the group items: %w", err)
	}
	defer func() { _ = rows.Close() }()
	type entry struct {
		id, position     int64
		videoID, childID sql.NullInt64
	}
	var entries []entry
	var videoIDs, childIDs []int64
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.id, &e.position, &e.videoID, &e.childID); err != nil {
			return nil, "", fmt.Errorf("cannot read the group items: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("cannot read the group items: %w", err)
	}
	_ = rows.Close()

	next := ""
	if len(entries) > limit {
		entries = entries[:limit]
		next = packCursor(groupItemsCursorKind, strconv.FormatInt(entries[limit-1].position, 10))
	}
	for _, e := range entries {
		if e.videoID.Valid {
			videoIDs = append(videoIDs, e.videoID.Int64)
		} else {
			childIDs = append(childIDs, e.childID.Int64)
		}
	}
	videos, err := videosByID(ctx, tx, audience, videoIDs)
	if err != nil {
		return nil, "", err
	}
	children, err := loadGroups(ctx, tx, audience, childIDs)
	if err != nil {
		return nil, "", err
	}
	items := make([]domain.GroupItem, 0, len(entries))
	for _, e := range entries {
		item := domain.GroupItem{ID: e.id}
		if e.videoID.Valid {
			video, ok := videos[e.videoID.Int64]
			if !ok {
				return nil, "", fmt.Errorf("cannot read the group video (id=%d)", e.videoID.Int64)
			}
			item.Video = &video
		} else {
			child, ok := children[e.childID.Int64]
			if !ok {
				return nil, "", fmt.Errorf("cannot read the child group (id=%d)", e.childID.Int64)
			}
			item.Group = &child
		}
		items = append(items, item)
	}
	return items, next, nil
}
