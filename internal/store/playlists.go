package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// Playlist content keys survive missing videos; hide unresolved entries without deleting them, compact positions on writes, and emit no domain events.

var playlistNow = time.Now

func playlistItemVideoExpr(alias string) string {
	return `(case when ` + alias + `.content_key is null then null
		else ` + shownVideoForKeyExpr(alias+`.content_key`, domain.AudienceOwner) + ` end)`
}

// Share the group descendant CTE so playlist visibility and playback order match library groups.
func playlistGroupVideos(alias string) string {
	return `with ` + groupDescendantsCTE(`select `+alias+`.group_id as id`, domain.AudienceOwner)
}

// Counts and item reads share visibility; existing smart folders remain visible even when empty.
func playlistItemShownCondition(alias string) string {
	return `(` + alias + `.smart_folder_id is not null
		or (` + alias + `.group_id is not null and exists (` + playlistGroupVideos(alias) + ` select 1 from gdesc))
		or ` + playlistItemVideoExpr(alias) + ` is not null)`
}

var playlistSummaryColumns = `p.id, p.name, p.updated_at, (select count(*) from playlist_items i
	where i.playlist_id = p.id and ` + playlistItemShownCondition("i") + `)`

func scanPlaylist(row interface{ Scan(...any) error }) (domain.Playlist, error) {
	var playlist domain.Playlist
	var updatedAt int64
	if err := row.Scan(&playlist.ID, &playlist.Name, &updatedAt, &playlist.ItemCount); err != nil {
		return domain.Playlist{}, err
	}
	playlist.UpdatedAt = time.UnixMilli(updatedAt)
	return playlist, nil
}

// ListPlaylists reads summaries, covers, total and version from one snapshot, ordered by descending edit time and ID.
func (s *PlaylistStore) ListPlaylists(ctx context.Context, cursor string, limit int) (domain.PlaylistPage, error) {
	limit = max(1, limit)
	var conditions []string
	var args []any
	if cursor != "" {
		after, err := domain.DecodePlaylistCursor(cursor)
		if err != nil {
			return domain.PlaylistPage{}, err
		}
		// Row-value comparison seeks into the order index; an OR predicate would scan it from the start.
		conditions = append(conditions, `(p.updated_at, p.id) < (?, ?)`)
		args = append(args, after.UpdatedAtMs, after.ID)
	}
	args = append(args, limit+1)

	tx, err := s.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.PlaylistPage{}, fmt.Errorf("cannot read the playlists: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	page := domain.PlaylistPage{Items: []domain.Playlist{}}
	var updatedAtSum int64
	if err := tx.QueryRowContext(ctx, `select count(*), coalesce(sum(updated_at), 0) from playlists`).Scan(
		&page.Total, &updatedAtSum); err != nil {
		return domain.PlaylistPage{}, fmt.Errorf("cannot read the playlists: %w", err)
	}
	page.Version = playlistListVersion(page.Total, updatedAtSum)
	rows, err := tx.QueryContext(ctx, `select `+playlistSummaryColumns+` from playlists p`+whereClause(conditions)+`
		order by p.updated_at desc, p.id desc limit ?`, args...)
	if err != nil {
		return domain.PlaylistPage{}, fmt.Errorf("cannot read the playlists: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		playlist, err := scanPlaylist(rows)
		if err != nil {
			return domain.PlaylistPage{}, fmt.Errorf("cannot read the playlists: %w", err)
		}
		page.Items = append(page.Items, playlist)
	}
	if err := rows.Err(); err != nil {
		return domain.PlaylistPage{}, fmt.Errorf("cannot read the playlists: %w", err)
	}
	_ = rows.Close()
	if len(page.Items) > limit {
		page.Items = page.Items[:limit]
		last := page.Items[limit-1]
		page.NextCursor = domain.EncodePlaylistCursor(domain.PlaylistCursor{
			UpdatedAtMs: last.UpdatedAt.UnixMilli(), ID: last.ID,
		})
	}
	for i := range page.Items {
		covers, err := playlistCovers(ctx, tx, page.Items[i].ID)
		if err != nil {
			return domain.PlaylistPage{}, err
		}
		page.Items[i].Covers = covers
	}
	if err := tx.Commit(); err != nil {
		return domain.PlaylistPage{}, fmt.Errorf("cannot read the playlists: %w", err)
	}
	return page, nil
}

// Combine count and summed monotonic edit times to detect same-millisecond changes that a maximum timestamp would miss.
func playlistListVersion(total int, updatedAtSum int64) string {
	return strconv.Itoa(total) + "." + strconv.FormatInt(updatedAtSum, 10)
}

// Use the first generated thumbnail in visible group playback order; smart folders contribute no SQL cover expression.
func playlistCoverVideoExpr(alias string) string {
	return `(case when ` + alias + `.content_key is not null then ` + playlistItemVideoExpr(alias) + `
		else (` + playlistGroupVideos(alias) + ` select d.video_id from gdesc d join videos pgv on pgv.id = d.video_id
			where pgv.thumbnail_state = ? order by d.sort_key limit 1) end)`
}

func playlistCovers(ctx context.Context, tx *sql.Tx, id int64) ([]domain.FolderPreview, error) {
	rows, err := tx.QueryContext(ctx, `select v.id, v.content_key, v.preview_state,
			(select ov.thumbnail_position_ms from video_overrides ov where ov.content_key = v.content_key and v.content_key <> ''),
			(select ov.thumbnail_revision from video_overrides ov where ov.content_key = v.content_key and v.content_key <> ''),
			v.thumbnail_build, v.seek_thumbnail_build, v.preview_build
		from playlist_items i join videos v on v.id = `+playlistCoverVideoExpr("i")+`
		where i.playlist_id = ? and v.thumbnail_state = ?
		order by i.position, i.id limit ?`,
		string(domain.ThumbnailStateDone), id, string(domain.ThumbnailStateDone), domain.MaxFolderPreviews)
	if err != nil {
		return nil, fmt.Errorf("cannot read the playlist covers: %w", err)
	}
	defer func() { _ = rows.Close() }()
	covers := []domain.FolderPreview{}
	for rows.Next() {
		var cover domain.FolderPreview
		var preview string
		var position, revision sql.NullInt64
		if err := rows.Scan(&cover.VideoID, &cover.ContentKey, &preview, &position, &revision,
			&cover.Builds.Thumbnail, &cover.Builds.SeekThumbnail, &cover.Builds.Preview); err != nil {
			return nil, fmt.Errorf("cannot read the playlist covers: %w", err)
		}
		cover.PreviewState = domain.PreviewState(preview)
		if position.Valid {
			ms := position.Int64
			cover.ThumbnailPositionMs = &ms
			cover.ThumbnailRevision = revision.Int64
		}
		covers = append(covers, cover)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the playlist covers: %w", err)
	}
	return covers, nil
}

// CreatePlaylist allows duplicate normalized names.
func (s *PlaylistStore) CreatePlaylist(ctx context.Context, name string) (domain.Playlist, error) {
	name, err := domain.ValidatePlaylistName(name)
	if err != nil {
		return domain.Playlist{}, err
	}
	now := playlistNow().UnixMilli()
	result, err := s.sql.ExecContext(ctx, `insert into playlists (name, created_at, updated_at) values (?, ?, ?)`,
		name, now, now)
	if err != nil {
		return domain.Playlist{}, fmt.Errorf("cannot create the playlist: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return domain.Playlist{}, fmt.Errorf("cannot create the playlist: %w", err)
	}
	return domain.Playlist{ID: id, Name: name, UpdatedAt: time.UnixMilli(now)}, nil
}

func (s *PlaylistStore) RenamePlaylist(ctx context.Context, id int64, name string) (domain.Playlist, error) {
	name, err := domain.ValidatePlaylistName(name)
	if err != nil {
		return domain.Playlist{}, err
	}
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.Playlist{}, fmt.Errorf("cannot rename the playlist: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `update playlists set name = ?, updated_at = `+nextUpdatedAt+` where id = ?`,
		name, playlistNow().UnixMilli(), id)
	if err != nil {
		return domain.Playlist{}, fmt.Errorf("cannot rename the playlist: %w", err)
	}
	if affected, err := result.RowsAffected(); err != nil {
		return domain.Playlist{}, fmt.Errorf("cannot rename the playlist: %w", err)
	} else if affected == 0 {
		return domain.Playlist{}, domain.ErrPlaylistNotFound
	}
	playlist, err := playlistSummary(ctx, tx, id)
	if err != nil {
		return domain.Playlist{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Playlist{}, fmt.Errorf("cannot rename the playlist: %w", err)
	}
	return playlist, nil
}

// DeletePlaylist removes only the playlist and its items, preserving videos, groups and user data.
func (s *PlaylistStore) DeletePlaylist(ctx context.Context, id int64) error {
	result, err := s.sql.ExecContext(ctx, `delete from playlists where id = ?`, id)
	if err != nil {
		return fmt.Errorf("cannot delete the playlist: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("cannot delete the playlist: %w", err)
	}
	if affected == 0 {
		return domain.ErrPlaylistNotFound
	}
	return nil
}

// Playlist reads owner-visible summaries, items and registered folders from one snapshot so counts and locations agree.
func (s *PlaylistStore) Playlist(ctx context.Context, id int64) (domain.Playlist, []domain.PlaylistItem, []domain.MediaFolder, error) {
	tx, err := s.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.Playlist{}, nil, nil, fmt.Errorf("cannot read the playlist: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	playlist, err := playlistSummary(ctx, tx, id)
	if err != nil {
		return domain.Playlist{}, nil, nil, err
	}
	items, err := shownPlaylistItems(ctx, tx, id)
	if err != nil {
		return domain.Playlist{}, nil, nil, err
	}
	roots, err := listMediaFolders(ctx, tx)
	if err != nil {
		return domain.Playlist{}, nil, nil, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Playlist{}, nil, nil, fmt.Errorf("cannot read the playlist: %w", err)
	}
	return playlist, items, roots, nil
}

// PlaylistPlayback resolves the current position against the latest order, reading hidden items and visible details in one snapshot.
func (s *PlaylistStore) PlaylistPlayback(ctx context.Context, id, itemID, videoID int64) (domain.PlaylistPlayback, error) {
	tx, err := s.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.PlaylistPlayback{}, fmt.Errorf("cannot read the playlist playback: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	playlist, err := playlistSummary(ctx, tx, id)
	if err != nil {
		return domain.PlaylistPlayback{}, err
	}
	order, err := playlistItemOrder(ctx, tx, id)
	if err != nil {
		return domain.PlaylistPlayback{}, err
	}
	shown, err := shownPlaylistItems(ctx, tx, id)
	if err != nil {
		return domain.PlaylistPlayback{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.PlaylistPlayback{}, fmt.Errorf("cannot read the playlist playback: %w", err)
	}

	byID := make(map[int64]domain.PlaylistItem, len(shown))
	for _, item := range shown {
		byID[item.ID] = item
	}
	items := make([]domain.PlaylistItem, len(order))
	for i, itemID := range order {
		item, ok := byID[itemID]
		if !ok {
			item = domain.PlaylistItem{ID: itemID}
		}
		items[i] = item
	}
	playback, err := domain.PlaylistPlaybackOf(items, domain.PlaylistCurrent{ItemID: itemID, VideoID: videoID})
	if err != nil {
		return domain.PlaylistPlayback{}, err
	}
	playback.Playlist = playlist
	return playback, nil
}

// AddPlaylistItems appends videos, groups then smart folders; shared bundle keys collapse, existing items stay unchanged, and only additions advance time.
func (s *PlaylistStore) AddPlaylistItems(ctx context.Context, id int64, add domain.PlaylistAdd) (domain.PlaylistAdded, error) {
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.PlaylistAdded{}, fmt.Errorf("cannot add to the playlist: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := ensurePlaylist(ctx, tx, id); err != nil {
		return domain.PlaylistAdded{}, err
	}

	videoKeys, err := orderedUserKeys(ctx, tx, add.VideoIDs)
	if err != nil {
		return domain.PlaylistAdded{}, err
	}
	groupIDs, err := existingGroupIDs(ctx, tx, add.GroupIDs)
	if err != nil {
		return domain.PlaylistAdded{}, err
	}
	folderIDs, err := existingIDs(ctx, tx, "smart_folders", add.SmartFolderIDs)
	if err != nil {
		return domain.PlaylistAdded{}, err
	}

	var next int
	if err := tx.QueryRowContext(ctx, `select count(*) from playlist_items where playlist_id = ?`, id).Scan(&next); err != nil {
		return domain.PlaylistAdded{}, fmt.Errorf("cannot add to the playlist: %w", err)
	}
	now := playlistNow().UnixMilli()
	var added domain.PlaylistAdded
	insert := func(column string, key any) error {
		result, err := tx.ExecContext(ctx, `insert or ignore into playlist_items
			(playlist_id, `+column+`, position, added_at) values (?, ?, ?, ?)`, id, key, next, now)
		if err != nil {
			return fmt.Errorf("cannot add to the playlist: %w", err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("cannot add to the playlist: %w", err)
		}
		if affected == 0 {
			added.Existing++
			return nil
		}
		added.Added++
		next++
		return nil
	}
	for _, key := range videoKeys {
		if err := insert("content_key", key); err != nil {
			return domain.PlaylistAdded{}, err
		}
	}
	for _, groupID := range groupIDs {
		if err := insert("group_id", groupID); err != nil {
			return domain.PlaylistAdded{}, err
		}
	}
	for _, folderID := range folderIDs {
		if err := insert("smart_folder_id", folderID); err != nil {
			return domain.PlaylistAdded{}, err
		}
	}
	if added.Added > 0 {
		if err := touchPlaylist(ctx, tx, id); err != nil {
			return domain.PlaylistAdded{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return domain.PlaylistAdded{}, fmt.Errorf("cannot add to the playlist: %w", err)
	}
	return added, nil
}

// MovePlaylistItem retains hidden entries in the order; nil means first and moving after itself is a no-op.
func (s *PlaylistStore) MovePlaylistItem(ctx context.Context, id, itemID int64, afterItemID *int64) error {
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot move the playlist item: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := ensurePlaylist(ctx, tx, id); err != nil {
		return err
	}
	moved, err := moveItemAfter(ctx, tx, playlistPositions, id, itemID, afterItemID)
	if errors.Is(err, errPositionItemNotFound) {
		return domain.ErrPlaylistItemNotFound
	}
	if err != nil {
		return err
	}
	if !moved {
		return tx.Commit()
	}
	if err := touchPlaylist(ctx, tx, id); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("cannot move the playlist item: %w", err)
	}
	return nil
}

// RemovePlaylistItem compacts positions without changing the referenced content or user data.
func (s *PlaylistStore) RemovePlaylistItem(ctx context.Context, id, itemID int64) error {
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot remove the playlist item: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := ensurePlaylist(ctx, tx, id); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `delete from playlist_items where id = ? and playlist_id = ?`, itemID, id)
	if err != nil {
		return fmt.Errorf("cannot remove the playlist item: %w", err)
	}
	if affected, err := result.RowsAffected(); err != nil {
		return fmt.Errorf("cannot remove the playlist item: %w", err)
	} else if affected == 0 {
		return domain.ErrPlaylistItemNotFound
	}
	if err := renumberPlaylists(ctx, tx, []int64{id}); err != nil {
		return err
	}
	if err := touchPlaylist(ctx, tx, id); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("cannot remove the playlist item: %w", err)
	}
	return nil
}

// Move keys and retain the earliest duplicate; callers compare playlist shapes after all transaction changes before advancing timestamps.
func rekeyPlaylistItems(ctx context.Context, tx *sql.Tx, from, to string) error {
	if from == "" || to == "" || from == to {
		return nil
	}
	affected, err := queryInt64s(ctx, tx, `select distinct playlist_id from playlist_items where content_key = ?`, from)
	if err != nil {
		return fmt.Errorf("cannot move the playlist items: %w", err)
	}
	if len(affected) == 0 {
		return nil
	}
	if err := deleteRekeyDuplicates(ctx, tx, playlistPositions, from, to); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `update playlist_items set content_key = ? where content_key = ?`, to, from); err != nil {
		return fmt.Errorf("cannot move the playlist items: %w", err)
	}
	return renumberPlaylists(ctx, tx, affected)
}

// The ordered (item ID, visibility) pairs determine the visible playback shape.
type playlistShapes map[int64]string

// Capture shapes before changing bundles or videos in a rekey transaction.
func snapshotPlaylistShapes(ctx context.Context, tx *sql.Tx, keys []string) (playlistShapes, error) {
	keys = slices.DeleteFunc(slices.Clone(keys), func(key string) bool { return key == "" })
	shapes := playlistShapes{}
	if len(keys) == 0 {
		return shapes, nil
	}
	encoded, err := json.Marshal(keys)
	if err != nil {
		return nil, fmt.Errorf("cannot read the playlists: %w", err)
	}
	ids, err := queryInt64s(ctx, tx, `select distinct playlist_id from playlist_items
		where content_key in (select value from json_each(?))`, string(encoded))
	if err != nil {
		return nil, fmt.Errorf("cannot read the playlists: %w", err)
	}
	for _, id := range ids {
		shape, err := playlistShape(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		shapes[id] = shape
	}
	return shapes, nil
}

// Advance timestamps only after visible shape changes; key-only replacements must not reorder the playlist list.
func touchReshapedPlaylists(ctx context.Context, tx *sql.Tx, before playlistShapes) error {
	for _, id := range slices.Sorted(maps.Keys(before)) {
		after, err := playlistShape(ctx, tx, id)
		if err != nil {
			return err
		}
		if after == before[id] {
			continue
		}
		if err := touchPlaylist(ctx, tx, id); err != nil {
			return err
		}
	}
	return nil
}

func playlistShape(ctx context.Context, tx *sql.Tx, id int64) (string, error) {
	rows, err := tx.QueryContext(ctx, `select i.id, `+playlistItemShownCondition("i")+`
		from playlist_items i where i.playlist_id = ? order by i.position, i.id`, id)
	if err != nil {
		return "", fmt.Errorf("cannot read the playlist items: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var shape strings.Builder
	for rows.Next() {
		var itemID int64
		var shown bool
		if err := rows.Scan(&itemID, &shown); err != nil {
			return "", fmt.Errorf("cannot read the playlist items: %w", err)
		}
		fmt.Fprintf(&shape, "%d:%t,", itemID, shown)
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("cannot read the playlist items: %w", err)
	}
	return shape.String(), nil
}

func renumberPlaylists(ctx context.Context, tx *sql.Tx, playlistIDs []int64) error {
	return renumberPositions(ctx, tx, playlistPositions, playlistIDs)
}

func ensurePlaylist(ctx context.Context, tx *sql.Tx, id int64) error {
	var exists int
	if err := tx.QueryRowContext(ctx, `select exists (select 1 from playlists where id = ?)`, id).Scan(&exists); err != nil {
		return fmt.Errorf("cannot read the playlist: %w", err)
	}
	if exists == 0 {
		return domain.ErrPlaylistNotFound
	}
	return nil
}

// Advance beyond the previous millisecond value even if the clock moves backward, making equal timestamps imply equal order.
const nextUpdatedAt = `max(?, updated_at + 1)`

func touchPlaylist(ctx context.Context, tx *sql.Tx, id int64) error {
	if _, err := tx.ExecContext(ctx, `update playlists set updated_at = `+nextUpdatedAt+` where id = ?`,
		playlistNow().UnixMilli(), id); err != nil {
		return fmt.Errorf("cannot update the playlist: %w", err)
	}
	return nil
}

func playlistSummary(ctx context.Context, tx *sql.Tx, id int64) (domain.Playlist, error) {
	playlist, err := scanPlaylist(tx.QueryRowContext(ctx, `select `+playlistSummaryColumns+` from playlists p where p.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Playlist{}, domain.ErrPlaylistNotFound
	}
	if err != nil {
		return domain.Playlist{}, fmt.Errorf("cannot read the playlist: %w", err)
	}
	return playlist, nil
}

func playlistItemOrder(ctx context.Context, tx *sql.Tx, id int64) ([]int64, error) {
	return positionOrder(ctx, tx, playlistPositions, id)
}

// Resolve user keys in request order, deduplicating and omitting unresolved videos.
func orderedUserKeys(ctx context.Context, tx *sql.Tx, videoIDs []int64) ([]string, error) {
	byID, err := userKeysByVideoID(ctx, tx, videoIDs, true)
	if err != nil {
		return nil, err
	}
	var keys []string
	for _, id := range videoIDs {
		if key, ok := byID[id]; ok && key != "" && !slices.Contains(keys, key) {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

// Exactly one target ID is valid.
type playlistEntry struct {
	id       int64
	videoID  sql.NullInt64
	groupID  sql.NullInt64
	folderID sql.NullInt64
}

// Resolve groups and smart folders using their current owner-visible playback order.
func shownPlaylistItems(ctx context.Context, tx *sql.Tx, id int64) ([]domain.PlaylistItem, error) {
	entries, err := shownPlaylistEntries(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	var videoIDs, groupIDs []int64
	for _, e := range entries {
		if e.videoID.Valid {
			videoIDs = append(videoIDs, e.videoID.Int64)
		} else if e.groupID.Valid {
			groupIDs = append(groupIDs, e.groupID.Int64)
		}
	}
	videos, err := ownerVideosByID(ctx, tx, videoIDs)
	if err != nil {
		return nil, err
	}
	groups, err := loadGroups(ctx, tx, domain.AudienceOwner, groupIDs)
	if err != nil {
		return nil, err
	}
	items := make([]domain.PlaylistItem, 0, len(entries))
	for _, e := range entries {
		item := domain.PlaylistItem{ID: e.id}
		switch {
		case e.videoID.Valid:
			video, ok := videos[e.videoID.Int64]
			if !ok {
				continue
			}
			item.Video = &video
		case e.groupID.Valid:
			group, ok := groups[e.groupID.Int64]
			if !ok {
				continue
			}
			item.Group = &group
		default:
			card, err := ownerSmartFolderCard(ctx, tx, e.folderID.Int64)
			if err != nil {
				return nil, err
			}
			item.SmartFolder = &card
		}
		items = append(items, item)
	}
	return items, nil
}

func shownPlaylistEntries(ctx context.Context, tx *sql.Tx, id int64) ([]playlistEntry, error) {
	rows, err := tx.QueryContext(ctx, `select i.id, `+playlistItemVideoExpr("i")+`, i.group_id, i.smart_folder_id
		from playlist_items i where i.playlist_id = ? and `+playlistItemShownCondition("i")+`
		order by i.position, i.id`, id)
	if err != nil {
		return nil, fmt.Errorf("cannot read the playlist items: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var entries []playlistEntry
	for rows.Next() {
		var e playlistEntry
		if err := rows.Scan(&e.id, &e.videoID, &e.groupID, &e.folderID); err != nil {
			return nil, fmt.Errorf("cannot read the playlist items: %w", err)
		}
		if e.videoID.Valid || e.groupID.Valid || e.folderID.Valid {
			entries = append(entries, e)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the playlist items: %w", err)
	}
	return entries, nil
}

func ownerVideosByID(ctx context.Context, tx *sql.Tx, ids []int64) (map[int64]domain.Video, error) {
	return videosByID(ctx, tx, domain.AudienceOwner, ids)
}

func videosByID(ctx context.Context, tx *sql.Tx, audience domain.Audience, ids []int64) (map[int64]domain.Video, error) {
	out := map[int64]domain.Video{}
	if len(ids) == 0 {
		return out, nil
	}
	encoded, err := json.Marshal(ids)
	if err != nil {
		return nil, fmt.Errorf("cannot build video ids: %w", err)
	}
	rows, err := tx.QueryContext(ctx, `select `+videoColumns(audience)+` from videos`+
		representativeJoin(audience)+` where videos.id in (select value from json_each(?))`, string(encoded))
	if err != nil {
		return nil, fmt.Errorf("cannot read the videos: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		video, err := scanVideo(rows)
		if err != nil {
			return nil, fmt.Errorf("cannot read the videos: %w", err)
		}
		out[video.ID] = video
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the videos: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("cannot read the videos: %w", err)
	}
	if err := attachVersionsIn(ctx, tx, audience, out); err != nil {
		return nil, err
	}
	return out, nil
}

func queryInt64s(ctx context.Context, tx *sql.Tx, query string, args ...any) ([]int64, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []int64
	for rows.Next() {
		var value int64
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	return out, rows.Err()
}
