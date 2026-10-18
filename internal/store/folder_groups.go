package store

// Folder index rebuilds run inside each owning store's transaction; groups are synchronized from the resulting index.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// SetOverride persists one override per absolute path, even if the folder is currently absent, and rebuilds atomically.
func (s *FolderGroupStore) SetOverride(ctx context.Context, folderPath string, mode domain.FolderGroupMode) error {
	if !mode.Valid() {
		return fmt.Errorf("invalid folder grouping: %q", mode)
	}
	_, err := s.SetFolderGrouping(ctx, folderPath, mode)
	return err
}

// ClearOverride rebuilds the index atomically even if no override exists.
func (s *FolderGroupStore) ClearOverride(ctx context.Context, folderPath string) error {
	_, err := s.SetFolderGrouping(ctx, folderPath, "")
	return err
}

// SetFolderGrouping atomically updates the override and index; an empty mode restores automatic grouping.
func (s *FolderGroupStore) SetFolderGrouping(ctx context.Context, folderPath string, mode domain.FolderGroupMode) (domain.FolderGrouping, error) {
	if mode != "" && !mode.Valid() {
		return domain.FolderGrouping{}, fmt.Errorf("invalid folder grouping: %q", mode)
	}
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.FolderGrouping{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := writeOverride(ctx, tx, folderPath, mode); err != nil {
		return domain.FolderGrouping{}, err
	}
	changedGroups, err := dissolveUngroupedAndRebuild(ctx, tx, folderPath, mode)
	if err != nil {
		return domain.FolderGrouping{}, err
	}
	groupings, err := folderGroupings(ctx, tx, []string{folderPath})
	if err != nil {
		return domain.FolderGrouping{}, err
	}
	if err := s.db.commitEvents(ctx, tx, groupsChangedEvents(changedGroups)...); err != nil {
		return domain.FolderGrouping{}, fmt.Errorf("cannot save the folder grouping: %w", err)
	}
	return groupings[0], nil
}

// FolderGroupings preserves input order and reads one snapshot.
func (s *FolderGroupStore) FolderGroupings(ctx context.Context, folderPaths []string) ([]domain.FolderGrouping, error) {
	if len(folderPaths) == 0 {
		return []domain.FolderGrouping{}, nil
	}
	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("cannot start reading folder groupings: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	groupings, err := folderGroupings(ctx, tx, folderPaths)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("cannot finish reading folder groupings: %w", err)
	}
	return groupings, nil
}

func folderGroupings(ctx context.Context, tx *sql.Tx, folderPaths []string) ([]domain.FolderGrouping, error) {
	keys := make([]string, len(folderPaths))
	for i, path := range folderPaths {
		keys[i] = domain.FolderKey(path)
	}
	encoded, err := json.Marshal(keys)
	if err != nil {
		return nil, fmt.Errorf("cannot build folder keys: %w", err)
	}
	rows, err := tx.QueryContext(ctx, `
		with wanted(folder_key) as (select distinct value from json_each(?))
		select w.folder_key, o.mode, g.id
		  from wanted w
		  left join folder_group_overrides o on o.path = w.folder_key
		  left join groups g on g.origin_folder = w.folder_key`, string(encoded))
	if err != nil {
		return nil, fmt.Errorf("cannot read folder groupings: %w", err)
	}
	defer func() { _ = rows.Close() }()
	byKey := map[string]domain.FolderGrouping{}
	for rows.Next() {
		var key string
		var mode sql.NullString
		var groupID sql.NullInt64
		if err := rows.Scan(&key, &mode, &groupID); err != nil {
			return nil, fmt.Errorf("cannot read folder groupings: %w", err)
		}
		byKey[key] = domain.FolderGrouping{Mode: domain.FolderGroupMode(mode.String), GroupID: groupID.Int64}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read folder groupings: %w", err)
	}
	out := make([]domain.FolderGrouping, len(keys))
	for i, key := range keys {
		out[i] = byKey[key]
	}
	return out, nil
}

// Ungroup dissolves through the normal path; automatic/groupDirect modes allow synchronization to recreate eligible groups.
func dissolveUngroupedAndRebuild(ctx context.Context, tx *sql.Tx, folderPath string, mode domain.FolderGroupMode) ([]int64, error) {
	var dissolved []int64
	if mode == domain.FolderGroupUngroup {
		var err error
		if dissolved, err = dissolveFolderGroup(ctx, tx, folderPath); err != nil {
			return nil, err
		}
	}
	rebuilt, err := rebuildFolderIndex(ctx, tx)
	if err != nil {
		return nil, err
	}
	return normalizeIDs(append(dissolved, rebuilt...)), nil
}

func writeOverride(ctx context.Context, tx *sql.Tx, folderPath string, mode domain.FolderGroupMode) error {
	var err error
	if mode == "" {
		_, err = tx.ExecContext(ctx, `delete from folder_group_overrides where path = ?`, domain.FolderKey(folderPath))
	} else {
		_, err = tx.ExecContext(ctx, `insert into folder_group_overrides (path, mode, updated_at) values (?, ?, ?)
			on conflict (path) do update set mode = excluded.mode, updated_at = excluded.updated_at`,
			domain.FolderKey(folderPath), string(mode), time.Now().Unix())
	}
	if err != nil {
		return fmt.Errorf("cannot save the folder grouping: %w", err)
	}
	return nil
}

// RebuildFolderIndex preserves the previous index on failure and marks it stale in a separate transaction for startup recovery.
func (s *ScanIndexStore) RebuildFolderIndex(ctx context.Context) error {
	err := s.rebuildFolderIndexTx(ctx)
	if err == nil {
		return nil
	}
	if markErr := markFolderIndexStale(ctx, s.db.sql); markErr != nil {
		return errors.Join(err, markErr)
	}
	return err
}

// RefreshFolderIndex rebuilds only absent, stale or outdated indexes before HTTP and workers start.
func (s *ScanIndexStore) RefreshFolderIndex(ctx context.Context) (bool, error) {
	var version, searchVersion, stale int
	err := s.db.sql.QueryRowContext(ctx,
		`select version, search_version, stale from folder_index_state where id = 1`).
		Scan(&version, &searchVersion, &stale)
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return false, fmt.Errorf("cannot read the folder index state: %w", err)
	case version == domain.FolderIndexVersion && searchVersion == domain.SearchKeyVersion && stale == 0:
		return false, nil
	}
	if err := s.RebuildFolderIndex(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func (s *ScanIndexStore) rebuildFolderIndexTx(ctx context.Context) error {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	changedGroups, err := rebuildFolderIndex(ctx, tx)
	if err != nil {
		return err
	}
	return s.db.commitEvents(ctx, tx, groupsChangedEvents(changedGroups)...)
}

// A missing state row is inserted at version zero to force the next startup rebuild.
func markFolderIndexStale(ctx context.Context, q queryExecer) error {
	if _, err := q.ExecContext(ctx, `insert into folder_index_state (id, version, search_version, stale) values (1, 0, 0, 1)
		on conflict (id) do update set stale = 1`); err != nil {
		return fmt.Errorf("cannot save the folder index state: %w", err)
	}
	return nil
}

func rebuildFolderIndexOnly(ctx context.Context, tx *sql.Tx) ([]int64, error) {
	roots, err := listMediaFolders(ctx, tx)
	if err != nil {
		return nil, err
	}
	locations, err := folderIndexLocations(ctx, tx)
	if err != nil {
		return nil, err
	}
	overrides, err := folderGroupOverrides(ctx, tx)
	if err != nil {
		return nil, err
	}
	index := domain.BuildFolderIndex(roots, locations, overrides)
	if err := writeFolderIndex(ctx, tx, index); err != nil {
		return nil, fmt.Errorf("cannot save the folder index: %w", err)
	}
	changed, err := syncFolderGroups(ctx, tx, index, overrides)
	if err != nil {
		return nil, fmt.Errorf("cannot sync the folder groups: %w", err)
	}
	return changed, nil
}

// Only visible representatives contribute locations; nonrepresentative versions never enter the folder index.
func folderIndexLocations(ctx context.Context, tx *sql.Tx) ([]domain.FolderIndexLocation, error) {
	rows, err := tx.QueryContext(ctx, `select l.video_id, l.path from video_locations l
		join videos on videos.id = l.video_id
		where `+shownVideoCondition("videos", domain.AudienceOwner))
	if err != nil {
		return nil, fmt.Errorf("cannot read locations: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var locations []domain.FolderIndexLocation
	for rows.Next() {
		var location domain.FolderIndexLocation
		if err := rows.Scan(&location.VideoID, &location.Path); err != nil {
			return nil, fmt.Errorf("cannot read locations: %w", err)
		}
		locations = append(locations, location)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read locations: %w", err)
	}
	return locations, nil
}

func folderGroupOverrides(ctx context.Context, tx *sql.Tx) (map[string]domain.FolderGroupMode, error) {
	rows, err := tx.QueryContext(ctx, `select path, mode from folder_group_overrides`)
	if err != nil {
		return nil, fmt.Errorf("cannot read folder groupings: %w", err)
	}
	defer func() { _ = rows.Close() }()
	overrides := map[string]domain.FolderGroupMode{}
	for rows.Next() {
		var path, mode string
		if err := rows.Scan(&path, &mode); err != nil {
			return nil, fmt.Errorf("cannot read folder groupings: %w", err)
		}
		overrides[path] = domain.FolderGroupMode(mode)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read folder groupings: %w", err)
	}
	return overrides, nil
}

func writeFolderIndex(ctx context.Context, tx *sql.Tx, index domain.FolderIndex) error {
	if _, err := tx.ExecContext(ctx, `delete from video_folder_names`); err != nil {
		return err
	}

	insertName, err := tx.PrepareContext(ctx, `insert into video_folder_names (video_id, name) values (?, ?)`)
	if err != nil {
		return err
	}
	defer func() { _ = insertName.Close() }()
	for videoID, names := range index.FolderNames {
		for _, name := range names {
			if _, err := insertName.ExecContext(ctx, videoID, name); err != nil {
				return err
			}
		}
	}

	_, err = tx.ExecContext(ctx, `insert into folder_index_state (id, version, search_version, stale) values (1, ?, ?, 0)
		on conflict (id) do update set version = excluded.version, search_version = excluded.search_version, stale = 0`,
		domain.FolderIndexVersion, domain.SearchKeyVersion)
	return err
}
