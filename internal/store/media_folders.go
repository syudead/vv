package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/syudead/vv/internal/domain"
)

func (s *SettingsStore) ListMediaFolders(ctx context.Context) ([]domain.MediaFolder, error) {
	return listMediaFolders(ctx, s.db.sql)
}

func (s *ScanIndexStore) ListMediaFolders(ctx context.Context) ([]domain.MediaFolder, error) {
	return listMediaFolders(ctx, s.db.sql)
}

func (s *LibraryStore) ListMediaFolders(ctx context.Context) ([]domain.MediaFolder, error) {
	return listMediaFolders(ctx, s.db.sql)
}

// HasLocationsUnder reports whether a video's location lies under the media folder; an unknown folder has none.
func (s *ScanIndexStore) HasLocationsUnder(ctx context.Context, folderID int64) (bool, error) {
	var populated bool
	if err := s.db.sql.QueryRowContext(ctx, `select exists (select 1 from media_folders mf
		join video_locations l on `+inMediaFolderCondition("l.path")+` where mf.id = ?)`, folderID).
		Scan(&populated); err != nil {
		return false, fmt.Errorf("cannot check whether the media folder holds videos: %w", err)
	}
	return populated, nil
}

func listMediaFolders(ctx context.Context, q queryExecer) ([]domain.MediaFolder, error) {
	rows, err := q.QueryContext(ctx, `select id, path, version, created_at, updated_at from media_folders order by id`)
	if err != nil {
		return nil, fmt.Errorf("cannot read media folders: %w", err)
	}
	defer func() { _ = rows.Close() }()
	folders := []domain.MediaFolder{}
	for rows.Next() {
		var folder domain.MediaFolder
		var createdAt, updatedAt int64
		if err := rows.Scan(&folder.ID, &folder.Path, &folder.Version, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("cannot read media folders: %w", err)
		}
		folder.CreatedAt = time.Unix(createdAt, 0)
		folder.UpdatedAt = time.Unix(updatedAt, 0)
		folders = append(folders, folder)
	}
	return folders, rows.Err()
}

func (s *SettingsStore) AddMediaFolder(ctx context.Context, path string) (domain.MediaFolder, error) {
	s.db.folderMu.Lock()
	defer s.db.folderMu.Unlock()
	cleaned, err := domain.NormalizeMediaFolderPath(path)
	if err != nil {
		return domain.MediaFolder{}, err
	}
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.MediaFolder{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := ensureFolderPlacementAllowed(ctx, tx, 0, cleaned); err != nil {
		return domain.MediaFolder{}, err
	}
	now := time.Now().Unix()
	res, err := tx.ExecContext(ctx, `insert into media_folders(path, version, created_at, updated_at) values (?, 1, ?, ?)`, cleaned, now, now)
	if err != nil {
		return domain.MediaFolder{}, fmt.Errorf("cannot add the media folder: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return domain.MediaFolder{}, err
	}
	// Previously unregistered locations have empty keys; rebuild those covered by the new root.
	if err := refreshSearchKeysUnder(ctx, tx, cleaned); err != nil {
		return domain.MediaFolder{}, err
	}
	changedGroups, err := rebuildFolderIndex(ctx, tx)
	if err != nil {
		return domain.MediaFolder{}, err
	}
	// Newly registered locations can make queued jobs claimable; include their videos in the latest import.
	if err := addScanVideosWithRemainingJobs(ctx, tx); err != nil {
		return domain.MediaFolder{}, err
	}
	// Wake idle workers now rather than waiting for another enqueue.
	var c changes
	c.groupsChanged(changedGroups)
	c.jobsQueued(domain.JobKinds...)
	c.issuesChanged()
	c.matchesChanged()
	if err := s.db.commit(ctx, tx, &c); err != nil {
		return domain.MediaFolder{}, err
	}
	return domain.MediaFolder{ID: id, Path: cleaned, Version: 1, CreatedAt: time.Unix(now, 0), UpdatedAt: time.Unix(now, 0)}, nil
}

func (s *SettingsStore) ReplaceMediaFolder(ctx context.Context, id, expectedVersion int64, path string) (domain.MediaFolder, error) {
	s.db.folderMu.Lock()
	defer s.db.folderMu.Unlock()
	cleaned, err := domain.NormalizeMediaFolderPath(path)
	if err != nil {
		return domain.MediaFolder{}, err
	}
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.MediaFolder{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var oldPath string
	var createdAt, previousUpdatedAt int64
	var version int64
	if err := tx.QueryRowContext(ctx, `select path, version, created_at, updated_at from media_folders where id = ?`, id).Scan(&oldPath, &version, &createdAt, &previousUpdatedAt); errors.Is(err, sql.ErrNoRows) {
		return domain.MediaFolder{}, domain.ErrNotFound
	} else if err != nil {
		return domain.MediaFolder{}, err
	}
	if version != expectedVersion {
		return domain.MediaFolder{}, domain.ErrVersionConflict
	}
	if cleaned == oldPath {
		if err := tx.Commit(); err != nil {
			return domain.MediaFolder{}, err
		}
		return domain.MediaFolder{ID: id, Path: oldPath, Version: version, CreatedAt: time.Unix(createdAt, 0), UpdatedAt: time.Unix(previousUpdatedAt, 0)}, nil
	}
	if err := ensureFolderPlacementAllowed(ctx, tx, id, cleaned); err != nil {
		return domain.MediaFolder{}, err
	}
	now := time.Now().Unix()
	if _, err := tx.ExecContext(ctx, `update media_folders set path = ?, version = version + 1, updated_at = ? where id = ?`, cleaned, now, id); err != nil {
		return domain.MediaFolder{}, err
	}
	released, err := removeLocationsUnder(ctx, tx, oldPath)
	if err != nil {
		return domain.MediaFolder{}, err
	}
	if err := syncLocationsUnder(ctx, tx, cleaned); err != nil {
		return domain.MediaFolder{}, err
	}
	// Relative-path keys depend on the registered root and must be rebuilt after relocation.
	if err := refreshSearchKeysUnder(ctx, tx, cleaned); err != nil {
		return domain.MediaFolder{}, err
	}
	changedGroups, err := rebuildFolderIndex(ctx, tx)
	if err != nil {
		return domain.MediaFolder{}, err
	}
	if err := addScanVideosWithRemainingJobs(ctx, tx); err != nil {
		return domain.MediaFolder{}, err
	}
	var c changes
	c.groupsChanged(changedGroups)
	c.videosDeleted(released)
	c.jobsQueued(domain.JobKinds...)
	c.issuesChanged()
	c.matchesChanged()
	if err := s.db.commit(ctx, tx, &c); err != nil {
		return domain.MediaFolder{}, err
	}
	return domain.MediaFolder{ID: id, Path: cleaned, Version: version + 1, CreatedAt: time.Unix(createdAt, 0), UpdatedAt: time.Unix(now, 0)}, nil
}

func (s *SettingsStore) DeleteMediaFolder(ctx context.Context, id, expectedVersion int64) error {
	s.db.folderMu.Lock()
	defer s.db.folderMu.Unlock()
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var path string
	var version int64
	if err := tx.QueryRowContext(ctx, `select path, version from media_folders where id = ?`, id).Scan(&path, &version); errors.Is(err, sql.ErrNoRows) {
		return domain.ErrNotFound
	} else if err != nil {
		return err
	}
	if version != expectedVersion {
		return domain.ErrVersionConflict
	}
	running, err := scanRunning(ctx, tx)
	if err != nil {
		return err
	}
	if err := domain.CheckMediaFolderMutation(running); err != nil {
		return err
	}
	// Read affected videos before deleting locations to record how many leave the library.
	_, underFolder, err := locationsUnder(ctx, tx, path)
	if err != nil {
		return err
	}
	released, err := removeLocationsUnder(ctx, tx, path)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `delete from media_folders where id = ?`, id); err != nil {
		return err
	}
	removed, err := countVideosOutOfLibrary(ctx, tx, underFolder)
	if err != nil {
		return err
	}
	changedGroups, err := rebuildFolderIndex(ctx, tx)
	if err != nil {
		return err
	}
	var c changes
	c.groupsChanged(changedGroups)
	c.videosDeleted(released)
	// Jobs without registered locations no longer count as remaining, even if their video row survives.
	c.processingChanged()
	c.issuesChanged()
	c.matchesChanged()
	// Removing thumbnail work can unblock seek jobs; surviving videos emit no ingest event, so wake workers explicitly.
	c.jobsQueued(domain.JobSeekThumbnail)
	if err := recordMediaFolderRemoval(ctx, tx, &c, path, removed, time.Now()); err != nil {
		return err
	}
	return s.db.commit(ctx, tx, &c)
}

// Recheck scans and registered folders inside the transaction because concurrent requests can invalidate earlier placement checks.
func ensureFolderPlacementAllowed(ctx context.Context, tx *sql.Tx, id int64, path string) error {
	running, err := scanRunning(ctx, tx)
	if err != nil {
		return err
	}
	folders, err := listMediaFolders(ctx, tx)
	if err != nil {
		return err
	}
	return domain.CheckMediaFolderPlacement(running, folders, id, path)
}

func scanRunning(ctx context.Context, tx *sql.Tx) (bool, error) {
	var running int
	if err := tx.QueryRowContext(ctx, `select count(*) from scans where state = 'running'`).Scan(&running); err != nil {
		return false, err
	}
	return running != 0, nil
}

// Check iteration errors centrally so callers never treat a partial location set as a successful deletion or synchronization.
func locationsUnder(ctx context.Context, tx *sql.Tx, root string) (ids []int64, videoIDs map[int64]struct{}, err error) {
	rows, err := tx.QueryContext(ctx, `select id, video_id, path from video_locations`)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = rows.Close() }()

	videoIDs = map[int64]struct{}{}
	for rows.Next() {
		var id, videoID int64
		var path string
		if err := rows.Scan(&id, &videoID, &path); err != nil {
			return nil, nil, err
		}
		if domain.PathWithinRoot(root, path) {
			ids = append(ids, id)
			videoIDs[videoID] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return ids, videoIDs, nil
}

// Count both deleted videos and surviving videos left with only unregistered locations.
func countVideosOutOfLibrary(ctx context.Context, tx *sql.Tx, videoIDs map[int64]struct{}) (int, error) {
	ids := make([]int64, 0, len(videoIDs))
	for id := range videoIDs {
		ids = append(ids, id)
	}
	encoded, err := json.Marshal(ids)
	if err != nil {
		return 0, fmt.Errorf("cannot build video ids: %w", err)
	}
	var count int
	if err := tx.QueryRowContext(ctx, `select count(*) from json_each(?) v
		where not exists (select 1 from video_locations l where l.video_id = v.value and `+
		registeredLocationCondition("l")+`)`, string(encoded)).Scan(&count); err != nil {
		return 0, fmt.Errorf("cannot count the videos the folder removed: %w", err)
	}
	return count, nil
}

func removeLocationsUnder(ctx context.Context, tx *sql.Tx, root string) ([]domain.DeletedVideo, error) {
	ids, affected, err := locationsUnder(ctx, tx, root)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, `update jobs set location_id = null, location_version = null, location_path = null where state = 'queued' and location_id = ?`, id); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `delete from video_locations where id = ?`, id); err != nil {
			return nil, err
		}
	}
	for videoID := range affected {
		if _, err := tx.ExecContext(ctx, `update videos set location_generation = location_generation + 1 where id = ?`, videoID); err != nil {
			return nil, err
		}
		if err := syncRepresentativeContainer(ctx, tx, videoID); err != nil {
			return nil, err
		}
	}
	return deleteOrphanVideos(ctx, tx)
}

func syncLocationsUnder(ctx context.Context, tx *sql.Tx, root string) error {
	_, videoIDs, err := locationsUnder(ctx, tx, root)
	if err != nil {
		return err
	}
	for videoID := range videoIDs {
		if _, err := tx.ExecContext(ctx, `update videos set location_generation = location_generation + 1 where id = ?`, videoID); err != nil {
			return err
		}
		if err := syncRepresentativeContainer(ctx, tx, videoID); err != nil {
			return err
		}
	}
	return nil
}

// MediaFolderVideoBytes sums every indexed file below dir, each location once, whether or not it represents its video.
func (s *SettingsStore) MediaFolderVideoBytes(ctx context.Context, dir string) (int64, error) {
	inFolder, args := folderPrefixCondition("l", folderPrefix(dir))
	var total int64
	if err := s.db.sql.QueryRowContext(ctx, `select coalesce(sum(l.size_bytes), 0) from video_locations l where `+inFolder,
		args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("cannot sum the media folder's video sizes: %w", err)
	}
	return total, nil
}
