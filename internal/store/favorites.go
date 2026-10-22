package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// Favorites are persistent user data; index rebuilds leave them intact, while deleting a group, smart folder or person cascades its favorite.

var favoriteColumn = `exists (select 1 from video_favorites fav where fav.content_key = ` + userKeyExpr("videos") +
	` and videos.content_key <> '')`

var favoriteNow = time.Now

// SetFavorites counts existing video IDs separately even when they share a key; one monotonic timestamp covers the atomic change without advancing video edits.
func (s *FavoriteStore) SetFavorites(ctx context.Context, change domain.FavoriteChange) (domain.FavoriteApplied, error) {
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.FavoriteApplied{}, fmt.Errorf("cannot update favorites: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	userKeys, err := userKeysForVideoIDs(ctx, tx, change.VideoIDs)
	if err != nil {
		return domain.FavoriteApplied{}, err
	}
	videos, err := countMappedVideoIDs(ctx, tx, change.VideoIDs)
	if err != nil {
		return domain.FavoriteApplied{}, err
	}
	groups, err := existingGroupIDs(ctx, tx, change.GroupIDs)
	if err != nil {
		return domain.FavoriteApplied{}, err
	}
	folders, err := existingIDs(ctx, tx, "smart_folders", change.SmartFolderIDs)
	if err != nil {
		return domain.FavoriteApplied{}, err
	}
	people, err := existingIDs(ctx, tx, "people", change.PersonIDs)
	if err != nil {
		return domain.FavoriteApplied{}, err
	}

	encodedKeys, err := json.Marshal(nonNil(userKeys))
	if err != nil {
		return domain.FavoriteApplied{}, fmt.Errorf("cannot build user keys: %w", err)
	}
	targets := []favoriteTable{
		{table: "video_favorites", column: "content_key", values: string(encodedKeys)},
	}
	for _, ids := range []struct {
		table, column string
		ids           []int64
	}{
		{"group_favorites", "group_id", groups}, {"smart_folder_favorites", "smart_folder_id", folders},
		{"person_favorites", "person_id", people},
	} {
		encoded, err := json.Marshal(nonNil(ids.ids))
		if err != nil {
			return domain.FavoriteApplied{}, fmt.Errorf("cannot build %s ids: %w", ids.column, err)
		}
		targets = append(targets, favoriteTable{table: ids.table, column: ids.column, values: string(encoded)})
	}

	if change.Favorite {
		var at int64
		if err := tx.QueryRowContext(ctx, `select max(?, coalesce((select max(m) from (
				select max(favorited_at) as m from video_favorites
				union all select max(favorited_at) from group_favorites
				union all select max(favorited_at) from smart_folder_favorites
				union all select max(favorited_at) from person_favorites)), 0) + 1)`,
			favoriteNow().UnixMilli()).Scan(&at); err != nil {
			return domain.FavoriteApplied{}, fmt.Errorf("cannot decide the favorited time: %w", err)
		}
		// Never write empty user keys; numeric IDs safely share this predicate.
		for _, target := range targets {
			if _, err := tx.ExecContext(ctx, `insert or ignore into `+target.table+` (`+target.column+`, favorited_at)
				select distinct value, ? from json_each(?) where value <> ''`, at, target.values); err != nil {
				return domain.FavoriteApplied{}, fmt.Errorf("cannot update %s: %w", target.table, err)
			}
		}
	} else {
		for _, target := range targets {
			if _, err := tx.ExecContext(ctx, `delete from `+target.table+`
				where `+target.column+` in (select value from json_each(?))`, target.values); err != nil {
				return domain.FavoriteApplied{}, fmt.Errorf("cannot update %s: %w", target.table, err)
			}
		}
	}

	// No smart rule reads a person favorite, so a people-only change refreshes no folder.
	matched := len(change.VideoIDs)+len(change.GroupIDs)+len(change.SmartFolderIDs) > 0
	if err := s.db.commitMatchChangeIf(ctx, tx, matched); err != nil {
		return domain.FavoriteApplied{}, fmt.Errorf("cannot update favorites: %w", err)
	}
	return domain.FavoriteApplied{Videos: videos, Groups: len(groups), SmartFolders: len(folders)}, nil
}

type favoriteTable struct {
	table, column, values string
}

// Count distinct video IDs, not shared bundle keys.
func countMappedVideoIDs(ctx context.Context, tx *sql.Tx, videoIDs []int64) (int, error) {
	if len(videoIDs) == 0 {
		return 0, nil
	}
	encoded, err := json.Marshal(videoIDs)
	if err != nil {
		return 0, fmt.Errorf("cannot build video ids: %w", err)
	}
	var count int
	if err := tx.QueryRowContext(ctx, `select count(distinct v.id) from videos v
		where v.id in (select value from json_each(?)) and v.content_key <> '' and `+
		libraryVideoCondition("v"), string(encoded)).Scan(&count); err != nil {
		return 0, fmt.Errorf("cannot count the videos: %w", err)
	}
	return count, nil
}

func existingGroupIDs(ctx context.Context, tx *sql.Tx, ids []int64) ([]int64, error) {
	return existingIDs(ctx, tx, "groups", ids)
}

func existingIDs(ctx context.Context, tx *sql.Tx, table string, ids []int64) ([]int64, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	encoded, err := json.Marshal(ids)
	if err != nil {
		return nil, fmt.Errorf("cannot build %s ids: %w", table, err)
	}
	found, err := queryInt64s(ctx, tx, `select id from `+table+` where id in (select value from json_each(?))`, string(encoded))
	if err != nil {
		return nil, fmt.Errorf("cannot read the %s: %w", table, err)
	}
	// Use set lookups to keep requests of up to 20,000 IDs linear.
	pending := make(map[int64]bool, len(found))
	for _, id := range found {
		pending[id] = true
	}
	var out []int64
	for _, id := range ids {
		if pending[id] {
			out = append(out, id)
			delete(pending, id)
		}
	}
	return out, nil
}

// Encode nil slices as [] instead of null for json_each.
func nonNil[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return items
}
