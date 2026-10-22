package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// RelatedCandidates reads one snapshot of visible representatives plus self, excluding self's other versions; the domain ranks fingerprints and metadata.
func (s *LibraryStore) RelatedCandidates(ctx context.Context, audience domain.Audience, selfID int64) ([]domain.RelatedCandidate, error) {
	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("cannot start reading related video candidates: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, `select videos.id, coalesce(rep.path, ''), coalesce(rep.title, vv.title),
		(select ov.display_name from video_overrides ov
		  where ov.content_key = videos.content_key and videos.content_key <> ''),
		videos.added_at, f.version, f.interval_ms, f.hashes
		from videos`+representativeJoin(audience)+`
		left join video_fingerprints f on f.content_key = videos.content_key and videos.content_key <> ''
		where `+visibleVideoCondition("videos", audience)+`
		  and (videos.id = ?1 or (`+shownVideoCondition("videos", audience)+`
		  and not exists (select 1 from videos self
		       join video_bundle_members sm on sm.content_key = self.content_key and self.content_key <> ''
		       join video_bundle_members om on om.bundle_id = sm.bundle_id
		      where self.id = ?1 and om.content_key = videos.content_key)))`, selfID)
	if err != nil {
		return nil, fmt.Errorf("cannot read related video candidates: %w", err)
	}
	candidates, byID, err := scanRelatedCandidates(rows)
	if err != nil {
		return nil, err
	}

	tagRows, err := tx.QueryContext(ctx, `select v.id, vt.tag_id from videos v
		join video_tags vt on vt.content_key = `+userKeyExpr("v")+`
		where v.content_key <> ''
		union
		select v.id, tn.tag_id from video_folder_names vfn
		join tag_names tn on tn.name = vfn.name
		join videos v on v.id = vfn.video_id and v.content_key <> ''`)
	if err != nil {
		return nil, fmt.Errorf("cannot read tags of related video candidates: %w", err)
	}
	defer func() { _ = tagRows.Close() }()
	for tagRows.Next() {
		var videoID, tagID int64
		if err := tagRows.Scan(&videoID, &tagID); err != nil {
			return nil, fmt.Errorf("cannot read tags of related video candidates: %w", err)
		}
		if index, ok := byID[videoID]; ok {
			candidates[index].TagIDs = append(candidates[index].TagIDs, tagID)
		}
	}
	if err := tagRows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read tags of related video candidates: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("cannot finish reading related video candidates: %w", err)
	}
	return candidates, nil
}

func scanRelatedCandidates(rows *sql.Rows) ([]domain.RelatedCandidate, map[int64]int, error) {
	defer func() { _ = rows.Close() }()
	candidates := []domain.RelatedCandidate{}
	byID := map[int64]int{}
	for rows.Next() {
		var item domain.RelatedCandidate
		var displayName sql.NullString
		var addedAt int64
		var version, intervalMs sql.NullInt64
		var hashes []byte
		if err := rows.Scan(&item.VideoID, &item.Path, &item.Title, &displayName, &addedAt,
			&version, &intervalMs, &hashes); err != nil {
			return nil, nil, fmt.Errorf("cannot read related video candidates: %w", err)
		}
		if displayName.Valid && displayName.String != "" {
			item.Title = displayName.String
		}
		item.AddedAt = time.Unix(addedAt, 0)
		if version.Valid && intervalMs.Valid {
			// Treat unreadable fingerprints as absent so ranking can continue with a zero fingerprint score.
			if fingerprint, err := domain.DecodeFingerprint(hashes, int(version.Int64), intervalMs.Int64); err == nil {
				item.Fingerprint = &fingerprint
			}
		}
		byID[item.VideoID] = len(candidates)
		candidates = append(candidates, item)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("cannot read related video candidates: %w", err)
	}
	return candidates, byID, nil
}

// VideosByIDs preserves input order while omitting hidden videos and nonrepresentative versions.
func (s *LibraryStore) VideosByIDs(ctx context.Context, audience domain.Audience, ids []int64) ([]domain.Video, error) {
	if len(ids) == 0 {
		return []domain.Video{}, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}

	rows, err := s.db.sql.QueryContext(ctx, `select `+videoColumns(audience)+` from videos`+representativeJoin(audience)+` where videos.id in (`+
		placeholders+`) and `+visibleVideoCondition("videos", audience)+` and `+shownVideoCondition("videos", audience), args...)
	if err != nil {
		return nil, fmt.Errorf("cannot read related videos: %w", err)
	}
	defer func() { _ = rows.Close() }()

	byID := make(map[int64]domain.Video, len(ids))
	for rows.Next() {
		video, err := scanVideo(rows)
		if err != nil {
			return nil, fmt.Errorf("cannot read related videos: %w", err)
		}
		byID[video.ID] = video
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read related videos: %w", err)
	}
	_ = rows.Close()
	if err := attachVersionsIn(ctx, s.db.sql, audience, byID); err != nil {
		return nil, err
	}

	videos := make([]domain.Video, 0, len(ids))
	for _, id := range ids {
		if video, ok := byID[id]; ok {
			videos = append(videos, video)
		}
	}
	return videos, nil
}

// VideoGroup returns all playback IDs but loads details only for the requested window, bounding detail work for large groups.
func (s *LibraryStore) VideoGroup(ctx context.Context, audience domain.Audience, id int64, opened domain.VideoContext, window domain.GroupWindow) (domain.VideoGroup, bool, error) {
	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.VideoGroup{}, false, fmt.Errorf("cannot start reading the video's group: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	group, found, err := videoContext(ctx, tx, audience, id, opened)
	if err != nil || !found {
		return domain.VideoGroup{}, false, err
	}
	position := group.Position(id)
	if position == 0 {
		return domain.VideoGroup{}, false, nil
	}
	if window.Limit > 0 {
		group.Offset = window.Start(len(group.MemberIDs), position)
		end := min(group.Offset+window.Limit, len(group.MemberIDs))
		group.Members, err = groupMembers(ctx, tx, audience, group.MemberIDs[group.Offset:end])
		if err != nil {
			return domain.VideoGroup{}, false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return domain.VideoGroup{}, false, fmt.Errorf("cannot finish reading the video's group: %w", err)
	}
	return group, true, nil
}

// Prefer an explicitly opened group over a smart folder, then fall back to the lowest-ID direct parent containing the visible video.
func videoContext(ctx context.Context, tx *sql.Tx, audience domain.Audience, id int64, opened domain.VideoContext) (domain.VideoGroup, bool, error) {
	if opened.GroupID > 0 {
		var held bool
		err := tx.QueryRowContext(ctx, `with `+groupDescendantsCTE(`select ?1 as id`, audience)+`
			select exists (select 1 from gdesc where video_id = ?2)`, opened.GroupID, id).Scan(&held)
		if err != nil {
			return domain.VideoGroup{}, false, fmt.Errorf("cannot read the video's group: %w", err)
		}
		if held {
			return groupPlayOrder(ctx, tx, audience, opened.GroupID)
		}
	} else if opened.SmartFolderID > 0 {
		folder, found, err := smartFolderVideoGroup(ctx, tx, audience, opened.SmartFolderID)
		if err != nil {
			return domain.VideoGroup{}, false, err
		}
		if found && folder.Position(id) > 0 {
			return folder, true, nil
		}
	}
	groupID, found, err := firstGroupOf(ctx, tx, audience, id)
	if err != nil || !found {
		return domain.VideoGroup{}, false, err
	}
	return groupPlayOrder(ctx, tx, audience, groupID)
}

// Require the direct item to resolve to this exact video, ensuring the chosen group is visible.
func firstGroupOf(ctx context.Context, tx *sql.Tx, audience domain.Audience, id int64) (int64, bool, error) {
	var groupID sql.NullInt64
	err := tx.QueryRowContext(ctx, `select min(gi.group_id) from group_items gi
		where gi.content_key = (select `+userKeyExpr("videos")+` from videos where videos.id = ?1 and videos.content_key <> '')
		and `+shownVideoForKeyExpr("gi.content_key", audience)+` = ?1`, id).Scan(&groupID)
	if err != nil {
		return 0, false, fmt.Errorf("cannot read the video's group: %w", err)
	}
	return groupID.Int64, groupID.Valid, nil
}

func groupPlayOrder(ctx context.Context, tx *sql.Tx, audience domain.Audience, groupID int64) (domain.VideoGroup, bool, error) {
	// The root folder has an empty automatic key; distinguish it from a manual group's NULL.
	var origin sql.NullString
	var name string
	if err := tx.QueryRowContext(ctx, `select origin_folder, name from groups where id = ?`,
		groupID).Scan(&origin, &name); err != nil {
		return domain.VideoGroup{}, false, fmt.Errorf("cannot read the video's group: %w", err)
	}
	memberIDs, err := groupPlayOrderIDs(ctx, tx, audience, groupID)
	if err != nil {
		return domain.VideoGroup{}, false, err
	}
	group := domain.VideoGroup{ID: groupID, Name: name, MemberIDs: memberIDs}
	if origin.Valid {
		roots, err := listMediaFolders(ctx, tx)
		if err != nil {
			return domain.VideoGroup{}, false, err
		}
		if folder, located := domain.LocateFolderKey(roots, origin.String); located {
			group.Folder = &folder
		}
	}
	return group, true, nil
}

func groupPlayOrderIDs(ctx context.Context, tx *sql.Tx, audience domain.Audience, groupID int64) ([]int64, error) {
	rows, err := tx.QueryContext(ctx, `with `+groupDescendantsCTE(`select ? as id`, audience)+`
		select video_id from gdesc order by sort_key`, groupID)
	if err != nil {
		return nil, fmt.Errorf("cannot read the group play order: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("cannot read the group play order: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the group play order: %w", err)
	}
	return ids, nil
}

// IDs came from the same transaction's visible playback order and need no second visibility check.
func groupMembers(ctx context.Context, tx *sql.Tx, audience domain.Audience, ids []int64) ([]domain.Video, error) {
	encoded, err := json.Marshal(ids)
	if err != nil {
		return nil, fmt.Errorf("cannot build group member ids: %w", err)
	}
	rows, err := tx.QueryContext(ctx, `select `+videoColumns(audience)+` from videos`+representativeJoin(audience)+`
		where videos.id in (select value from json_each(?))`, string(encoded))
	if err != nil {
		return nil, fmt.Errorf("cannot read group members: %w", err)
	}
	defer func() { _ = rows.Close() }()
	byID := make(map[int64]domain.Video, len(ids))
	for rows.Next() {
		video, err := scanVideo(rows)
		if err != nil {
			return nil, fmt.Errorf("cannot read group members: %w", err)
		}
		byID[video.ID] = video
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read group members: %w", err)
	}
	_ = rows.Close()
	if err := attachVersionsIn(ctx, tx, audience, byID); err != nil {
		return nil, err
	}
	members := make([]domain.Video, 0, len(ids))
	for _, id := range ids {
		if video, ok := byID[id]; ok {
			members = append(members, video)
		}
	}
	return members, nil
}
