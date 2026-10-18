package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"

	"github.com/syudead/vv/internal/domain"
)

func storedGroupListSQL(roots string, audience domain.Audience) groupListSQL {
	a := strconv.Itoa(audienceValue(audience))
	return groupListSQL{
		members: `gm as (select group_id, video_id, position from group_members
			where audience = ` + a + ` and group_id in (` + roots + `))`,
		count:  `select video_count from group_values where group_id = hits.group_id and audience = ` + a,
		values: `mv as (select v.* from group_values v join whole on whole.group_id = v.group_id where v.audience = ` + a + `)`,
		items: `select g.id, g.id * 2 + 1, null, null,
			mv.added_at, mv.mtime, mv.created_at, g.title_key, mv.duration_ms, mv.size_bytes, mv.played_at, ` + groupFavoritedAtExpr("g") + `,
			case when mv.started = 0 then 'unwatched' when mv.watched = mv.video_count then 'watched' else 'inProgress' end
			from mv join groups g on g.id = mv.group_id`,
	}
}

// Load only the summaries whose IDs survived the common page query.
func readLibraryFolderSummaries(ctx context.Context, tx *sql.Tx, audience domain.Audience, keys []itemRow) ([]librarySmartFolder, error) {
	var ids []int64
	for _, key := range keys {
		if key.id < 0 {
			ids = append(ids, -key.id)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	encoded, err := json.Marshal(ids)
	if err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `select f.id, coalesce(f.parent_id, 0), f.name,
		exists (select 1 from smart_folder_favorites ff where ff.smart_folder_id = f.id)
		from smart_folders f where f.id in (select value from json_each(?))`, string(encoded))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var folders []librarySmartFolder
	for rows.Next() {
		var f librarySmartFolder
		if err := rows.Scan(&f.summary.ID, &f.summary.ParentID, &f.summary.Name, &f.summary.Favorite); err != nil {
			return nil, err
		}
		if !audience.IsOwner() {
			f.summary.Favorite = false
		}
		folders = append(folders, f)
	}
	return folders, rows.Err()
}

func storedWholeGroupsSQL(spec listSpec, roots string) string {
	condition := ""
	if spec.playableOnly {
		condition = " and playable = 1"
	}
	return `select group_id from group_values where audience = ` + strconv.Itoa(audienceValue(spec.scope.audience)) +
		` and video_count > 0 and group_id in (` + roots + `)` + condition
}
