package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/syudead/vv/internal/domain"
)

// Merge moves the virtual video's user data to the file video's user key, its bundle key when bundled, and deletes
// the virtual video in one transaction (docs/design-docs/catalog-organization.md). Both contentVersions are the
// ones the screen read; the result is the file video with its work information.
func (s *VirtualVideoStore) Merge(
	ctx context.Context, virtualID int64, contentVersion string, videoID int64, videoContentVersion string,
) (domain.Video, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.Video{}, fmt.Errorf("cannot merge the virtual video: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	from, err := virtualKeyFor(ctx, tx, virtualID, contentVersion)
	if err != nil {
		return domain.Video{}, err
	}
	to, err := mergeTargetKey(ctx, tx, videoID, videoContentVersion)
	if err != nil {
		return domain.Video{}, err
	}
	merged, err := mergedWorkInfo(ctx, tx, from, to)
	if err != nil {
		return domain.Video{}, err
	}
	shapes, err := snapshotPlaylistShapes(ctx, tx, []string{from, to})
	if err != nil {
		return domain.Video{}, err
	}
	edited, rekeyedGroups, err := moveMergedUserData(ctx, tx, from, to, merged)
	if err != nil {
		return domain.Video{}, err
	}
	deletedGroups, err := deleteVirtualVideo(ctx, tx, from)
	if err != nil {
		return domain.Video{}, err
	}
	if err := touchEditedAtForUserKeys(ctx, tx, []string{to}, virtualNow()); err != nil {
		return domain.Video{}, err
	}
	if err := touchReshapedPlaylists(ctx, tx, shapes); err != nil {
		return domain.Video{}, err
	}
	video, events, err := savedWorkInfoVideo(ctx, tx, videoID, edited)
	if err != nil {
		return domain.Video{}, err
	}
	targetEvents, err := mergeTargetEvents(ctx, tx, to)
	if err != nil {
		return domain.Video{}, err
	}
	c := changes{}
	c.videosDeleted([]domain.DeletedVideo{{ID: virtualID, ContentKey: from}})
	c.groupsChanged(append(rekeyedGroups, deletedGroups...))
	if err := s.db.commitWith(ctx, tx, &c, append(events, targetEvents...)...); err != nil {
		return domain.Video{}, fmt.Errorf("cannot merge the virtual video: %w", err)
	}
	return video, nil
}

// The target needs a registered file as a bundle member does, so a virtual video is ErrVirtualVideo; a version other
// than the one the screen read is ErrContentChanged. Returns the user key, which is the bundle key when bundled.
func mergeTargetKey(ctx context.Context, tx *sql.Tx, videoID int64, contentVersion string) (string, error) {
	key, err := bundlableContentKey(ctx, tx, videoID)
	if err != nil {
		return "", err
	}
	if domain.ContentVersion(key) != contentVersion {
		return "", domain.ErrContentChanged
	}
	keys, err := userKeysForVideoIDs(ctx, tx, []int64{videoID})
	if err != nil {
		return "", err
	}
	if len(keys) == 0 {
		return "", domain.ErrNotFound
	}
	return keys[0], nil
}

// Tags, the favorite and playlist items move even when the work information does not, so every video under the
// target key refreshes on the screen.
func mergeTargetEvents(ctx context.Context, tx *sql.Tx, key string) ([]domain.Event, error) {
	ids, err := videoIDsForUserKeys(ctx, tx, []string{key})
	if err != nil {
		return nil, err
	}
	return overrideChangedEvents(ids), nil
}

func mergedWorkInfo(ctx context.Context, tx *sql.Tx, from, to string) (domain.WorkInfo, error) {
	infos, err := workInfoByUserKeys(ctx, tx, []string{from, to})
	if err != nil {
		return domain.WorkInfo{}, err
	}
	return domain.MergeWorkInfo(infos[to], infos[from])
}

// Write the merged work information, union favorites and tags, and rekey playlist and group items, keeping the file
// video's item where a playlist holds both. deleteVirtualVideo removes the rest of the virtual key's rows, the public
// flag among them. Returns the edited user keys and the changed groups.
func moveMergedUserData(
	ctx context.Context, tx *sql.Tx, from, to string, merged domain.WorkInfo,
) ([]string, []int64, error) {
	edited, err := applyWorkInfo(ctx, tx, []string{to}, map[string]domain.WorkInfo{to: merged})
	if err != nil {
		return nil, nil, err
	}
	if _, err := tx.ExecContext(ctx, `insert or ignore into video_favorites (content_key, favorited_at)
		select ?, favorited_at from video_favorites where content_key = ?`, to, from); err != nil {
		return nil, nil, fmt.Errorf("cannot move the favorite: %w", err)
	}
	if err := unionTagsAndHistory(ctx, tx, from, to); err != nil {
		return nil, nil, err
	}
	if err := dropItemsBesideKey(ctx, tx, playlistPositions, from, to); err != nil {
		return nil, nil, err
	}
	groups, err := rekeyUserItems(ctx, tx, from, to)
	if err != nil {
		return nil, nil, err
	}
	return edited, groups, nil
}

// Delete the from items whose parent also holds to, compacting those parents, so a later rekey keeps to's item.
func dropItemsBesideKey(ctx context.Context, tx *sql.Tx, t positionTable, from, to string) error {
	beside := ` from ` + t.table + ` where content_key = ?1 and exists (select 1 from ` + t.table + ` o
		where o.` + t.parent + ` = ` + t.table + `.` + t.parent + ` and o.content_key = ?2)`
	parents, err := queryInt64s(ctx, tx, `select distinct `+t.parent+beside, from, to)
	if err != nil {
		return fmt.Errorf("cannot read the %s items: %w", t.noun, err)
	}
	if _, err := tx.ExecContext(ctx, `delete`+beside, from, to); err != nil {
		return fmt.Errorf("cannot delete the %s items: %w", t.noun, err)
	}
	return renumberPositions(ctx, tx, t, parents)
}
