package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// A virtual video is a virtual_videos row (user data) projected into the index as a location-less videos row whose
// content key is its user key, so per-video operations and user-data joins serve it unchanged.

// Tests replace the clock to place a virtual video among file videos in time order.
var virtualNow = time.Now

const linkFetchInterrupted = "VVMDM stopped before the fetch finished."

func virtualUserKey(id int64) string {
	return fmt.Sprintf("%s%d", domain.VirtualKeyPrefix, id)
}

// Create stores the virtual video and its projected row in one transaction and returns the projected video.
func (s *VirtualVideoStore) Create(ctx context.Context, title string) (domain.Video, error) {
	title, err := domain.NormalizeVirtualTitle(title)
	if err != nil {
		return domain.Video{}, err
	}
	return s.create(ctx, title, "")
}

// CreateFromLink stores a virtual video whose work information holds link and whose link fetch is running. An empty
// title becomes domain.LinkTitle, which a fill may replace until the title is saved.
func (s *VirtualVideoStore) CreateFromLink(ctx context.Context, title, link string) (domain.Video, error) {
	link, err := domain.NormalizeVirtualLink(link)
	if err != nil {
		return domain.Video{}, err
	}
	if strings.TrimSpace(title) == "" {
		// A blank title is no title, so the link titles the video and a fill may replace it.
		title = ""
	} else if title, err = domain.NormalizeVirtualTitle(title); err != nil {
		return domain.Video{}, err
	}
	return s.create(ctx, title, link)
}

// create stores the row; with a link it also stores the link and marks the fetch running, and an empty title then
// comes from the link.
func (s *VirtualVideoStore) create(ctx context.Context, title, link string) (domain.Video, error) {
	titleFromLink := link != "" && title == ""
	if titleFromLink {
		title = domain.LinkTitle(link)
	}
	var fetch sql.NullString
	if link != "" {
		fetch = sql.NullString{String: string(domain.LinkFetchRunning), Valid: true}
	}
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.Video{}, fmt.Errorf("cannot create the virtual video: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := virtualNow().Unix()
	// The id is known only after the insert; the empty key never outlives this transaction.
	var id int64
	if err := tx.QueryRowContext(ctx, `insert into virtual_videos
		(user_key, title, title_key, search_key, search_version, created_at, updated_at, title_from_link, link_fetch)
		values ('', ?, ?, ?, ?, ?, ?, ?, ?) returning id`,
		title, domain.NaturalSortKey(title), searchKeyPart(title), domain.SearchKeyVersion, now, now,
		titleFromLink, fetch).Scan(&id); err != nil {
		return domain.Video{}, fmt.Errorf("cannot create the virtual video: %w", err)
	}
	key := virtualUserKey(id)
	if _, err := tx.ExecContext(ctx, `update virtual_videos set user_key = ? where id = ?`, key, id); err != nil {
		return domain.Video{}, fmt.Errorf("cannot create the virtual video: %w", err)
	}
	if err := projectVirtualVideos(ctx, tx, `vv.id = ?`, id); err != nil {
		return domain.Video{}, err
	}
	if link != "" {
		if _, err := applyWorkInfo(ctx, tx, []string{key}, map[string]domain.WorkInfo{key: {URLs: []string{link}}}); err != nil {
			return domain.Video{}, err
		}
	}
	video, err := virtualVideoOf(ctx, tx, key)
	if err != nil {
		return domain.Video{}, err
	}
	c := changes{}
	c.matchesChanged()
	if err := s.db.commit(ctx, tx, &c); err != nil {
		return domain.Video{}, fmt.Errorf("cannot create the virtual video: %w", err)
	}
	return video, nil
}

// LinkFetch reads the virtual video's link fetch; false when the video was not created from a link.
func (s *VirtualVideoStore) LinkFetch(ctx context.Context, key string) (domain.LinkFetch, bool, error) {
	var state, failure, plugin sql.NullString
	err := s.db.read.QueryRowContext(ctx, `select link_fetch, link_fetch_failure, link_fetch_plugin
		from virtual_videos where user_key = ?`, key).Scan(&state, &failure, &plugin)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return domain.LinkFetch{}, false, nil
	case err != nil:
		return domain.LinkFetch{}, false, fmt.Errorf("cannot read the link fetch: %w", err)
	}
	return domain.LinkFetch{
		State: domain.LinkFetchState(state.String), Failure: failure.String, Plugin: plugin.String,
	}, state.Valid, nil
}

// FinishLinkFetch records the end of a running link fetch, failed when failure is set, and notifies the video's
// screens; a video deleted or merged since has no row and nothing is written.
func (s *VirtualVideoStore) FinishLinkFetch(ctx context.Context, key, plugin, failure string) error {
	state := domain.LinkFetchDone
	if failure != "" {
		state = domain.LinkFetchFailed
	}
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot save the link fetch: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `update virtual_videos set link_fetch = ?, link_fetch_failure = ?, link_fetch_plugin = ?
		where user_key = ? and link_fetch = ?`,
		state, nullString(failure), nullString(plugin), key, domain.LinkFetchRunning)
	if err != nil {
		return fmt.Errorf("cannot save the link fetch: %w", err)
	}
	if n, err := result.RowsAffected(); err != nil || n == 0 {
		return err
	}
	var id int64
	if err := tx.QueryRowContext(ctx, `select id from videos where content_key = ?`, key).Scan(&id); err != nil {
		return fmt.Errorf("cannot read the virtual video (%s): %w", key, err)
	}
	if err := s.db.commitWith(ctx, tx, &changes{}, domain.VideoOverrideChanged{VideoID: id}); err != nil {
		return fmt.Errorf("cannot save the link fetch: %w", err)
	}
	return nil
}

// SaveTitle replaces the title and its keys; contentVersion is the whole user key the editor read.
func (s *VirtualVideoStore) SaveTitle(ctx context.Context, videoID int64, contentVersion, title string) (domain.Video, error) {
	return s.save(ctx, videoID, contentVersion, title, nil)
}

// SaveDetails saves the title and the work information in one transaction, writing neither when either fails;
// contentVersion is the whole user key the editor read.
func (s *VirtualVideoStore) SaveDetails(
	ctx context.Context, videoID int64, contentVersion, title string, info domain.WorkInfo,
) (domain.Video, error) {
	return s.save(ctx, videoID, contentVersion, title, &info)
}

// save writes the title and, when info is set, the work information; the returned video carries work information
// only then, as the file-video saves do.
func (s *VirtualVideoStore) save(
	ctx context.Context, videoID int64, contentVersion, title string, info *domain.WorkInfo,
) (domain.Video, error) {
	title, err := domain.NormalizeVirtualTitle(title)
	if err != nil {
		return domain.Video{}, err
	}
	var normalized domain.WorkInfo
	if info != nil {
		if normalized, _, err = domain.NormalizeWorkInfo(*info); err != nil {
			return domain.Video{}, err
		}
	}
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.Video{}, fmt.Errorf("cannot save the virtual video: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	key, err := virtualKeyFor(ctx, tx, videoID, contentVersion)
	if err != nil {
		return domain.Video{}, err
	}
	now := virtualNow()
	titleChanged, err := writeVirtualTitle(ctx, tx, key, title, now)
	if err != nil {
		return domain.Video{}, err
	}
	var edited []string
	if info != nil {
		if edited, err = writeVirtualWorkInfo(ctx, tx, key, *info, normalized); err != nil {
			return domain.Video{}, err
		}
	}
	if titleChanged || len(edited) > 0 {
		if err := touchEditedAt(ctx, tx, []string{key}, now); err != nil {
			return domain.Video{}, err
		}
	}
	// The owner's save answers a failed link fetch, so its note leaves the page.
	if _, err := tx.ExecContext(ctx, `update virtual_videos set link_fetch = null, link_fetch_failure = null,
		link_fetch_plugin = null where user_key = ? and link_fetch = ?`, key, domain.LinkFetchFailed); err != nil {
		return domain.Video{}, fmt.Errorf("cannot save the virtual video: %w", err)
	}
	var video domain.Video
	if info != nil {
		video, err = ownerVideoWithWorkInfo(ctx, tx, videoID)
	} else {
		video, err = getVideo(ctx, tx, domain.AudienceOwner, videoID)
	}
	if err != nil {
		return domain.Video{}, err
	}
	events, err := workInfoChangedEvents(ctx, tx, edited)
	if err != nil {
		return domain.Video{}, err
	}
	if titleChanged {
		events = append([]domain.Event{domain.VideoOverrideChanged{VideoID: videoID}}, events...)
	}
	if err := s.db.commitMatchChangeIf(ctx, tx, titleChanged, events...); err != nil {
		return domain.Video{}, fmt.Errorf("cannot save the virtual video: %w", err)
	}
	return video, nil
}

// Check the credits the input references, then replace the key's work information; returns the edited keys.
func writeVirtualWorkInfo(ctx context.Context, tx *sql.Tx, key string, input, normalized domain.WorkInfo) ([]string, error) {
	if _, err := checkCreditRefs(ctx, tx, []domain.WorkInfo{input}); err != nil {
		return nil, err
	}
	return applyWorkInfo(ctx, tx, []string{key}, map[string]domain.WorkInfo{key: normalized})
}

// Write a changed title with its sort and search keys; reports whether the title changed.
func writeVirtualTitle(ctx context.Context, tx *sql.Tx, key, title string, now time.Time) (bool, error) {
	result, err := tx.ExecContext(ctx, `update virtual_videos
		set title = ?, title_key = ?, search_key = ?, search_version = ?, updated_at = ?, title_from_link = 0
		where user_key = ? and title <> ?`,
		title, domain.NaturalSortKey(title), searchKeyPart(title), domain.SearchKeyVersion, now.Unix(), key, title)
	if err != nil {
		return false, fmt.Errorf("cannot save the virtual video: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("cannot save the virtual video: %w", err)
	}
	return changed > 0, nil
}

// Delete removes the virtual video, its projected row and every user-data row under its key in one transaction, and
// reports the key unreferenced after commit so its artifacts are released.
func (s *VirtualVideoStore) Delete(ctx context.Context, videoID int64, contentVersion string) error {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot delete the virtual video: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	key, err := virtualKeyFor(ctx, tx, videoID, contentVersion)
	if err != nil {
		return err
	}
	groups, err := deleteVirtualVideo(ctx, tx, key)
	if err != nil {
		return err
	}
	c := changes{}
	c.videosDeleted([]domain.DeletedVideo{{ID: videoID, ContentKey: key}})
	c.groupsChanged(groups)
	if err := s.db.commit(ctx, tx, &c); err != nil {
		return fmt.Errorf("cannot delete the virtual video: %w", err)
	}
	return nil
}

// Delete the virtual video's row, its projection and the user data still under its key; returns the changed groups.
func deleteVirtualVideo(ctx context.Context, tx *sql.Tx, key string) ([]int64, error) {
	groups, err := deleteKeyUserData(ctx, tx, key)
	if err != nil {
		return nil, err
	}
	for _, statement := range []string{
		`delete from virtual_videos where user_key = ?`,
		`delete from videos where content_key = ?`,
	} {
		if _, err := tx.ExecContext(ctx, statement, key); err != nil {
			return nil, fmt.Errorf("cannot delete the virtual video: %w", err)
		}
	}
	return groups, nil
}

// A missing video is ErrNotFound, a file video ErrNotVirtual, and a key other than contentVersion ErrContentChanged,
// which also catches a stale editor after SQLite reused the videos rowid.
func virtualKeyFor(ctx context.Context, tx *sql.Tx, videoID int64, contentVersion string) (string, error) {
	var key sql.NullString
	err := tx.QueryRowContext(ctx, `select vv.user_key from videos v
		left join virtual_videos vv on vv.user_key = v.content_key where v.id = ?`, videoID).Scan(&key)
	if errors.Is(err, sql.ErrNoRows) {
		return "", domain.ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("cannot read the virtual video (id=%d): %w", videoID, err)
	}
	if !key.Valid {
		return "", domain.ErrNotVirtual
	}
	if key.String != contentVersion {
		return "", domain.ErrContentChanged
	}
	return key.String, nil
}

func virtualVideoOf(ctx context.Context, tx *sql.Tx, key string) (domain.Video, error) {
	var id int64
	if err := tx.QueryRowContext(ctx, `select id from videos where content_key = ?`, key).Scan(&id); err != nil {
		return domain.Video{}, fmt.Errorf("cannot read the virtual video (%s): %w", key, err)
	}
	return getVideo(ctx, tx, domain.AudienceOwner, id)
}

// Insert the missing projected rows of the selected virtual videos. No job is queued, so no worker claims the
// pending stages; the thumbnail is done while an uploaded image exists.
func projectVirtualVideos(ctx context.Context, tx *sql.Tx, where string, args ...any) error {
	if _, err := tx.ExecContext(ctx, `insert into videos (added_at, indexed_at, content_key, playable, thumbnail_state)
		select vv.created_at, vv.created_at, vv.user_key, 0,
			case when exists (select 1 from thumbnail_images ti where ti.content_key = vv.user_key) then 'done' else 'pending' end
		from virtual_videos vv
		where not exists (select 1 from videos v where v.content_key = vv.user_key) and `+where, args...); err != nil {
		return fmt.Errorf("cannot project the virtual videos: %w", err)
	}
	return nil
}

// Delete every user-data row under the key, compacting the playlists and groups that held it; returns the changed
// groups with their ancestors. The activity log keeps its history.
func deleteKeyUserData(ctx context.Context, tx *sql.Tx, key string) ([]int64, error) {
	tables := []string{"video_edits", "video_overrides", "thumbnail_images", "auto_tag_rejections", "group_exclusions"}
	for _, t := range userDataTables {
		tables = append(tables, t.table)
	}
	for _, table := range tables {
		if _, err := tx.ExecContext(ctx, `delete from `+table+` where content_key = ?`, key); err != nil {
			return nil, fmt.Errorf("cannot delete the %s values: %w", table, err)
		}
	}
	if err := dropAutoTagKeys(ctx, tx, key); err != nil {
		return nil, err
	}
	playlists, err := deleteKeyItems(ctx, tx, playlistPositions, key)
	if err != nil {
		return nil, err
	}
	for _, id := range playlists {
		if err := touchPlaylist(ctx, tx, id); err != nil {
			return nil, err
		}
	}
	groups, err := deleteKeyItems(ctx, tx, groupPositions, key)
	if err != nil {
		return nil, err
	}
	if err := touchGroups(ctx, tx, groups, time.Now().UnixMilli()); err != nil {
		return nil, err
	}
	return groupsWithAncestors(ctx, tx, groups)
}

// Delete the key's items from a positioned table and compact the parents' positions; returns the parents.
func deleteKeyItems(ctx context.Context, tx *sql.Tx, t positionTable, key string) ([]int64, error) {
	parents, err := queryInt64s(ctx, tx, `select distinct `+t.parent+` from `+t.table+` where content_key = ?`, key)
	if err != nil {
		return nil, fmt.Errorf("cannot read the %s items: %w", t.noun, err)
	}
	if _, err := tx.ExecContext(ctx, `delete from `+t.table+` where content_key = ?`, key); err != nil {
		return nil, fmt.Errorf("cannot delete the %s items: %w", t.noun, err)
	}
	return parents, renumberPositions(ctx, tx, t, parents)
}

// Refresh stale virtual title keys and recreate missing projected rows; startup runs this with the location keys.
// A link fetch still running belonged to the previous process, so it is recorded as interrupted.
func refreshVirtualVideos(ctx context.Context, tx *sql.Tx) (int, error) {
	if _, err := tx.ExecContext(ctx, `update virtual_videos set link_fetch = ?, link_fetch_failure = ?
		where link_fetch = ?`, domain.LinkFetchFailed, linkFetchInterrupted, domain.LinkFetchRunning); err != nil {
		return 0, fmt.Errorf("cannot save the interrupted link fetches: %w", err)
	}
	titles, err := staleVirtualTitles(ctx, tx)
	if err != nil {
		return 0, err
	}
	for id, title := range titles {
		if _, err := tx.ExecContext(ctx, `update virtual_videos set title_key = ?, search_key = ?, search_version = ?
			where id = ?`, domain.NaturalSortKey(title), searchKeyPart(title), domain.SearchKeyVersion, id); err != nil {
			return 0, fmt.Errorf("cannot save the virtual video keys: %w", err)
		}
	}
	return len(titles), projectVirtualVideos(ctx, tx, `1 = 1`)
}

func staleVirtualTitles(ctx context.Context, tx *sql.Tx) (map[int64]string, error) {
	rows, err := tx.QueryContext(ctx, `select id, title from virtual_videos where search_version < ?`, domain.SearchKeyVersion)
	if err != nil {
		return nil, fmt.Errorf("cannot read the virtual videos: %w", err)
	}
	defer func() { _ = rows.Close() }()
	titles := map[int64]string{}
	for rows.Next() {
		var id int64
		var title string
		if err := rows.Scan(&id, &title); err != nil {
			return nil, fmt.Errorf("cannot read the virtual videos: %w", err)
		}
		titles[id] = title
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the virtual videos: %w", err)
	}
	return titles, nil
}
