package store

// All group views derive descendant playback order from groupDescendantsCTE.

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

// Resolve bundle keys to the audience's effective representative; inaccessible keys yield NULL.
func shownVideoForKeyExpr(key string, audience domain.Audience) string {
	bundleID := `(select sb.id from video_bundles sb where sb.user_key = ` + key + `)`
	return `(case when exists (select 1 from video_bundles sb where sb.user_key = ` + key + `) then ` +
		effectiveRepresentativeExpr(bundleID, audience) + `
		else (select sv.id from videos sv where sv.content_key = ` + key + ` and sv.content_key <> ''
			and not exists (select 1 from video_bundle_members sm where sm.content_key = sv.content_key)
			and ` + visibleVideoCondition("sv", audience) + ` order by sv.id limit 1) end)`
}

// Fixed-width zero-padded positions make byte ordering match depth-first playback ordering.
const groupSortWidth = 10

// Build one earliest depth-first path per root/descendant pair to avoid exponential expansion of diamond hierarchies.
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

func groupFavoritedAtExpr(alias string) string {
	return `(select gf.favorited_at from group_favorites gf where gf.group_id = ` + alias + `.id)`
}

// Guests never read playback history; return four NULL columns without a join.
func memberProgressSelect(audience domain.Audience) (columns, join string) {
	if !audience.IsOwner() {
		return `null, null, null, null`, ""
	}
	return `p.position_ms, p.duration_ms, p.completed, p.updated_at`,
		` left join playback_progress p on p.content_key = ` + userKeyExpr("videos") + ` and videos.content_key <> ''`
}

type memberProgress struct {
	position, duration, updatedAt sql.NullInt64
	completed                     sql.NullBool
}

func (m *memberProgress) dest() []any {
	return []any{&m.position, &m.duration, &m.completed, &m.updatedAt}
}

func (m *memberProgress) progress() *domain.Progress {
	if !m.position.Valid {
		return nil
	}
	return &domain.Progress{
		PositionMs: m.position.Int64, DurationMs: m.duration.Int64,
		Completed: m.completed.Bool, UpdatedAt: time.Unix(m.updatedAt.Int64, 0),
	}
}

// Load only groups with visible descendants, in playback order; omit playback records for guests.
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

// Group returns the first item page and valid parent trail; hidden and missing groups both return domain.ErrNotFound.
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

// GroupItems uses the same visibility as Group and rejects malformed cursors.
func (s *LibraryStore) GroupItems(ctx context.Context, audience domain.Audience, id int64, cursor string, limit int) (domain.GroupPage, error) {
	return s.readGroupPage(ctx, audience, id, func(tx *sql.Tx, page *domain.GroupPage) error {
		var err error
		page.Items, page.NextCursor, err = groupItemsPage(ctx, tx, audience, id, cursor, limit)
		return err
	})
}

// Read the group, registered folders and page contents from one snapshot.
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

// Keep only the contiguous parent-child suffix of the requested trail, returned outermost first.
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

const groupItemsCursorKind = "group-items"

// Page visible videos and child groups with visible descendants by item position.
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
