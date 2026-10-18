package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// Read saved values and member IDs for the page, then hydrate only the bounded cover videos.
func loadGroupCards(ctx context.Context, tx *sql.Tx, audience domain.Audience, ids []int64) (map[int64]domain.LibraryGroup, error) {
	groups := map[int64]domain.LibraryGroup{}
	if len(ids) == 0 {
		return groups, nil
	}
	encoded, err := json.Marshal(ids)
	if err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `select g.id, coalesce(g.origin_folder, ''), g.name,
		exists (select 1 from group_favorites gf where gf.group_id = g.id),
		v.video_count, v.duration_ms, v.size_bytes, v.added_at, v.played_at, v.watched, v.started, v.open_video_id,
		(select json_group_array(video_id) from (select video_id from group_members m
		 where m.group_id = g.id and m.audience = v.audience order by m.position))
		from groups g join group_values v on v.group_id = g.id
		where v.audience = ? and v.video_count > 0 and g.id in (select value from json_each(?))`, audienceValue(audience), string(encoded))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		group, err := scanGroupCard(rows)
		if err != nil {
			return nil, err
		}
		groups[group.ID] = group
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := loadGroupCardPreviews(ctx, tx, audience, ids, groups); err != nil {
		return nil, err
	}
	if audience.IsOwner() {
		if err := loadGroupCardTags(ctx, tx, ids, groups); err != nil {
			return nil, err
		}
	}
	return groups, nil
}

func scanGroupCard(rows *sql.Rows) (domain.LibraryGroup, error) {
	var group domain.LibraryGroup
	var count, watched, started int
	var duration, added, played sql.NullInt64
	var encoded string
	if err := rows.Scan(&group.ID, &group.Path, &group.Name, &group.Favorite, &count,
		&duration, &group.SizeBytes, &added, &played, &watched, &started, &group.OpenVideoID, &encoded); err != nil {
		return group, err
	}
	group.WatchState, group.WatchedCount = domain.WatchStateOf(count, started, watched), watched
	if duration.Valid {
		group.DurationMs = &duration.Int64
	}
	if added.Valid {
		group.AddedAt = time.Unix(added.Int64, 0)
	}
	if played.Valid {
		at := time.Unix(played.Int64, 0)
		group.LastPlayedAt = &at
	}
	group.Card = &domain.LibraryGroupCard{Tags: []domain.VideoTag{}, Previews: []domain.Video{}}
	if err := json.Unmarshal([]byte(encoded), &group.Card.VideoIDs); err != nil {
		return group, err
	}
	return group, nil
}

func loadGroupCardPreviews(ctx context.Context, tx *sql.Tx, audience domain.Audience, ids []int64, groups map[int64]domain.LibraryGroup) error {
	encoded, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `select g.id,
		(select json_group_array(video_id) from (
		 select m.video_id from group_members m join videos v on v.id = m.video_id
		 where m.group_id = g.id and m.audience = ? and v.thumbnail_state = ?
		 order by m.position limit ?)) from groups g where g.id in (select value from json_each(?))`,
		audienceValue(audience), domain.ThumbnailStateDone, domain.MaxFolderPreviews, string(encoded))
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	var previewIDs []int64
	byGroup := map[int64][]int64{}
	for rows.Next() {
		var id int64
		var list string
		if err := rows.Scan(&id, &list); err != nil {
			return err
		}
		var members []int64
		if err := json.Unmarshal([]byte(list), &members); err != nil {
			return err
		}
		byGroup[id] = members
		previewIDs = append(previewIDs, members...)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	videos, err := groupMembers(ctx, tx, audience, normalizeIDs(previewIDs))
	if err != nil {
		return err
	}
	byID := map[int64]domain.Video{}
	for _, video := range videos {
		byID[video.ID] = video
	}
	for id, members := range byGroup {
		group, ok := groups[id]
		if !ok {
			continue
		}
		for _, videoID := range members {
			group.Card.Previews = append(group.Card.Previews, byID[videoID])
		}
	}
	return nil
}

func loadGroupCardTags(ctx context.Context, tx *sql.Tx, ids []int64, groups map[int64]domain.LibraryGroup) error {
	encoded, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	selected := `select distinct ` + userKeyExpr("v") + ` from group_members m join videos v on v.id = m.video_id
		where m.audience = 1 and v.content_key <> '' and m.group_id in (select value from json_each(?1))`
	rows, err := tx.QueryContext(ctx, taggedUserKeysCTE(selected)+`, members as (
		select m.group_id, `+userKeyExpr("v")+` as user_key, m.position from group_members m join videos v on v.id = m.video_id
		where m.audience = 1 and v.content_key <> '' and m.group_id in (select value from json_each(?1)))
		select m.group_id, t.tag_id, tn.name, tg.tentative, max(t.manual), max(t.from_folder), (select min(first.unconfirmed) from members fm join tagged first on first.content_key = fm.user_key
		 where fm.group_id = m.group_id and first.tag_id = t.tag_id group by fm.position order by fm.position limit 1), `+tagKindColumns+`
		from members m join tagged t on t.content_key = m.user_key
		join tag_names tn on tn.tag_id = t.tag_id and tn.canonical = 1
		join tags tg on tg.id = t.tag_id`+tagKindJoin+` group by m.group_id, t.tag_id`, string(encoded))
	if err != nil {
		return fmt.Errorf("cannot read group card tags: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id int64
		var tag domain.VideoTag
		var kind scannedTagKind
		if err := rows.Scan(append([]any{&id, &tag.ID, &tag.Name, &tag.Tentative, &tag.Manual, &tag.FromFolder, &tag.Unconfirmed}, kind.targets()...)...); err != nil {
			return err
		}
		tag.Kind = kind.ref()
		if group, ok := groups[id]; ok {
			group.Card.Tags = append(group.Card.Tags, tag)
		}
	}
	for _, group := range groups {
		domain.SortVideoTags(group.Card.Tags)
	}
	return rows.Err()
}
