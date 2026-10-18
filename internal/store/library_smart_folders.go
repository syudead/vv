package store

// Unfiltered libraries mix root smart folders with items; favorites includes folders at any depth, with sort values derived from matching videos.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/syudead/vv/internal/domain"
)

type librarySmartFolder struct {
	summary domain.SmartFolderSummary
	// Aggregate timestamps by maximum and sizes/durations by sum; no matches yields NULL values that sort last.
	values [6]any
}

func librarySmartFolderIDs(ctx context.Context, tx *sql.Tx, q domain.VideoQuery) ([]int64, error) {
	if !q.ListsSmartFolders() {
		return nil, nil
	}
	query := `select id from smart_folders where parent_id is null order by id`
	if q.FavoriteOnly {
		query = `select smart_folder_id from smart_folder_favorites order by smart_folder_id`
	}
	ids, err := queryInt64s(ctx, tx, query)
	if err != nil {
		return nil, fmt.Errorf("cannot read the library smart folders: %w", err)
	}
	return ids, nil
}

// Apply audience visibility and the list's captured time; guests cannot see empty playback-order folders.
func librarySmartFolders(ctx context.Context, tx *sql.Tx, audience domain.Audience, q domain.VideoQuery, now int64) ([]librarySmartFolder, error) {
	ids, err := librarySmartFolderIDs(ctx, tx, q)
	if err != nil || len(ids) == 0 {
		return nil, err
	}
	targets, err := shownSmartFolderTargets(ctx, tx, audience, ids, now)
	if err != nil {
		return nil, err
	}
	folders := make([]librarySmartFolder, len(targets))
	for i, target := range targets {
		folders[i].summary = target.summary
	}
	if err := readSmartFolderSortValues(ctx, tx, audience, targets, folders); err != nil {
		return nil, err
	}
	return folders, nil
}

// Compute smart_base once while reading all folder sort values.
func readSmartFolderSortValues(ctx context.Context, tx *sql.Tx, audience domain.Audience, targets []smartFolderTarget, folders []librarySmartFolder) error {
	query := smartFolderBranches(targets, func(query *smartFolderQuery, index int, _ smartFolderTarget, match smartMatch) {
		query.parts = append(query.parts, `select `+strconv.Itoa(index)+`, max(added_at), max(mtime), max(created_at),
			sum(duration_ms), sum(size_bytes), max(played_at) from smart_base videos where `+match.condition)
		query.partArgs = append(query.partArgs, match.args...)
	})
	rows, err := query.run(ctx, tx, audience)
	if err != nil {
		return fmt.Errorf("cannot read the smart folder sort values: %w", err)
	}
	if rows == nil {
		return nil
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var index int
		var values [6]sql.NullInt64
		if err := rows.Scan(&index, &values[0], &values[1], &values[2], &values[3], &values[4], &values[5]); err != nil {
			return fmt.Errorf("cannot read the smart folder sort values: %w", err)
		}
		for i, value := range values {
			if value.Valid {
				folders[index].values[i] = value.Int64
			}
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("cannot read the smart folder sort values: %w", err)
	}
	for index, earlier := range query.same {
		folders[index].values = folders[earlier].values
	}
	return nil
}

// Negative IDs distinguish smart folders from video/group items; folders have no watch state and use matching-video aggregate values.
func smartFolderItemsSQL(folders []librarySmartFolder) (string, []any) {
	if len(folders) == 0 {
		return "", nil
	}
	rows := make([][]any, 0, len(folders))
	for _, folder := range folders {
		rows = append(rows, append([]any{folder.summary.ID}, folder.values[:]...))
	}
	encoded, err := json.Marshal(rows)
	if err != nil {
		panic(fmt.Sprintf("cannot encode smart folder sort values: %v", err))
	}
	return `
		union all
		select null, -f.id, null, null, json_extract(c.value, '$[1]'), json_extract(c.value, '$[2]'),
			json_extract(c.value, '$[3]'), f.title_key, json_extract(c.value, '$[4]'), json_extract(c.value, '$[5]'),
			json_extract(c.value, '$[6]'),
			(select ff.favorited_at from smart_folder_favorites ff where ff.smart_folder_id = f.id),
			null
		from json_each(?) c join smart_folders f on f.id = json_extract(c.value, '$[0]')`, []any{string(encoded)}
}

func librarySmartFolderSummary(folders []librarySmartFolder, id int64) (*domain.SmartFolderSummary, bool) {
	for i := range folders {
		if folders[i].summary.ID == id {
			return &folders[i].summary, true
		}
	}
	return nil, false
}
