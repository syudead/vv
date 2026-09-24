package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/syudead/vv/internal/domain"
)

var (
	ErrScanRunning       = domain.ErrScanRunning
	ErrFolderConflict    = domain.ErrFolderConflict
	ErrVersionConflict   = domain.ErrVersionConflict
	ErrInvalidFolder     = domain.ErrInvalidMediaFolder
	ErrUnsupportedFolder = domain.ErrUnsupportedMediaFolder
)

// NormalizePath returns a clean absolute path while preserving the filesystem's
// exact Unicode spelling. Rewriting that spelling can point at another entry on
// filesystems where normalization forms are distinct.
func NormalizePath(path string) (string, error) {
	if path == "" || !filepath.IsAbs(path) {
		return "", ErrInvalidFolder
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalidFolder, err)
	}
	return filepath.Clean(absolute), nil
}

// PathWithinRoot reports whether path is root itself or one of its descendants.
// filepath.Rel supplies the platform's volume, separator and case rules.
func PathWithinRoot(root, path string) bool {
	return domain.PathWithinRoot(root, path)
}

func validateMediaFolder(path string) (string, error) {
	cleaned, err := NormalizePath(path)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(cleaned)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalidFolder, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		if info.Mode()&os.ModeSymlink != 0 {
			return "", ErrUnsupportedFolder
		}
		return "", ErrInvalidFolder
	}
	resolved, err := filepath.EvalSymlinks(cleaned)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalidFolder, err)
	}
	resolved, err = NormalizePath(resolved)
	if err != nil || !domain.PathWithinRoot(cleaned, resolved) || !domain.PathWithinRoot(resolved, cleaned) {
		return "", fmt.Errorf("%w: symbolic link", ErrUnsupportedFolder)
	}
	dir, err := os.Open(cleaned)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalidFolder, err)
	}
	defer func() { _ = dir.Close() }()
	if _, err := dir.ReadDir(1); err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("%w: %w", ErrInvalidFolder, err)
	}
	return cleaned, nil
}

func (db *DB) ListMediaFolders(ctx context.Context) ([]domain.MediaFolder, error) {
	rows, err := db.sql.QueryContext(ctx, `select id, path, version, created_at, updated_at from media_folders order by id`)
	if err != nil {
		return nil, fmt.Errorf("メディアフォルダを読み出せません: %w", err)
	}
	defer func() { _ = rows.Close() }()
	folders := []domain.MediaFolder{}
	for rows.Next() {
		var folder domain.MediaFolder
		var createdAt, updatedAt int64
		if err := rows.Scan(&folder.ID, &folder.Path, &folder.Version, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("メディアフォルダを読み出せません: %w", err)
		}
		folder.CreatedAt = time.Unix(createdAt, 0)
		folder.UpdatedAt = time.Unix(updatedAt, 0)
		folders = append(folders, folder)
	}
	return folders, rows.Err()
}

func (db *DB) AddMediaFolder(ctx context.Context, path string) (domain.MediaFolder, error) {
	db.folderMu.Lock()
	defer db.folderMu.Unlock()
	cleaned, err := validateMediaFolder(path)
	if err != nil {
		return domain.MediaFolder{}, err
	}
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.MediaFolder{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := ensureFolderMutationAllowed(ctx, tx, 0, cleaned); err != nil {
		return domain.MediaFolder{}, err
	}
	now := time.Now().Unix()
	res, err := tx.ExecContext(ctx, `insert into media_folders(path, version, created_at, updated_at) values (?, 1, ?, ?)`, cleaned, now, now)
	if err != nil {
		return domain.MediaFolder{}, fmt.Errorf("メディアフォルダを追加できません: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return domain.MediaFolder{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.MediaFolder{}, err
	}
	// 登録外の所在しか無かった待ちの仕事が、この登録で取り出せるようになる。
	// 眠っているワーカーを起こさないと、次に仕事が積まれるまで止まったままになる。
	db.notifyJobsQueued(domain.JobKinds...)
	return domain.MediaFolder{ID: id, Path: cleaned, Version: 1, CreatedAt: time.Unix(now, 0), UpdatedAt: time.Unix(now, 0)}, nil
}

func (db *DB) ReplaceMediaFolder(ctx context.Context, id, expectedVersion int64, path string) (domain.MediaFolder, error) {
	db.folderMu.Lock()
	defer db.folderMu.Unlock()
	cleaned, err := validateMediaFolder(path)
	if err != nil {
		return domain.MediaFolder{}, err
	}
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.MediaFolder{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var oldPath string
	var createdAt, previousUpdatedAt int64
	var version int64
	if err := tx.QueryRowContext(ctx, `select path, version, created_at, updated_at from media_folders where id = ?`, id).Scan(&oldPath, &version, &createdAt, &previousUpdatedAt); errors.Is(err, sql.ErrNoRows) {
		return domain.MediaFolder{}, ErrNotFound
	} else if err != nil {
		return domain.MediaFolder{}, err
	}
	if version != expectedVersion {
		return domain.MediaFolder{}, ErrVersionConflict
	}
	if cleaned == oldPath {
		if err := tx.Commit(); err != nil {
			return domain.MediaFolder{}, err
		}
		return domain.MediaFolder{ID: id, Path: oldPath, Version: version, CreatedAt: time.Unix(createdAt, 0), UpdatedAt: time.Unix(previousUpdatedAt, 0)}, nil
	}
	if err := ensureFolderMutationAllowed(ctx, tx, id, cleaned); err != nil {
		return domain.MediaFolder{}, err
	}
	now := time.Now().Unix()
	if _, err := tx.ExecContext(ctx, `update media_folders set path = ?, version = version + 1, updated_at = ? where id = ?`, cleaned, now, id); err != nil {
		return domain.MediaFolder{}, err
	}
	if err := removeLocationsUnder(ctx, tx, oldPath); err != nil {
		return domain.MediaFolder{}, err
	}
	if err := syncLocationsUnder(ctx, tx, cleaned); err != nil {
		return domain.MediaFolder{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.MediaFolder{}, err
	}
	// 付け替え先に所在を持つ待ちの仕事が取り出せるようになる（AddMediaFolder と同じ）。
	db.notifyJobsQueued(domain.JobKinds...)
	return domain.MediaFolder{ID: id, Path: cleaned, Version: version + 1, CreatedAt: time.Unix(createdAt, 0), UpdatedAt: time.Unix(now, 0)}, nil
}

func (db *DB) DeleteMediaFolder(ctx context.Context, id, expectedVersion int64) error {
	db.folderMu.Lock()
	defer db.folderMu.Unlock()
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var path string
	var version int64
	if err := tx.QueryRowContext(ctx, `select path, version from media_folders where id = ?`, id).Scan(&path, &version); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	if version != expectedVersion {
		return ErrVersionConflict
	}
	if err := ensureNoRunningScan(ctx, tx); err != nil {
		return err
	}
	if err := removeLocationsUnder(ctx, tx, path); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `delete from media_folders where id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func ensureFolderMutationAllowed(ctx context.Context, tx *sql.Tx, exceptID int64, path string) error {
	if err := ensureNoRunningScan(ctx, tx); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `select path from media_folders where id <> ?`, exceptID)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var existing string
		if err := rows.Scan(&existing); err != nil {
			return err
		}
		if PathWithinRoot(existing, path) || PathWithinRoot(path, existing) {
			return ErrFolderConflict
		}
	}
	return rows.Err()
}

func ensureNoRunningScan(ctx context.Context, tx *sql.Tx) error {
	var running int
	if err := tx.QueryRowContext(ctx, `select count(*) from scans where state = 'running'`).Scan(&running); err != nil {
		return err
	}
	if running != 0 {
		return ErrScanRunning
	}
	return nil
}

// locationsUnder は root 配下の場所の id と、その場所を持つ動画の id を返す。
//
// 反復の打ち切りを検査せずにこの集合を使うと、途中で失敗しても部分集合のまま
// 成功として扱われ、フォルダの削除や同期が黙って一部にしか適用されない。
// 呼び出し側ごとに書くと検査の漏れも各所に散るので、1か所に集める。
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

func removeLocationsUnder(ctx context.Context, tx *sql.Tx, root string) error {
	ids, affected, err := locationsUnder(ctx, tx, root)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, `update jobs set location_id = null, location_version = null, location_path = null where state = 'queued' and location_id = ?`, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `delete from video_locations where id = ?`, id); err != nil {
			return err
		}
	}
	for videoID := range affected {
		if _, err := tx.ExecContext(ctx, `update videos set location_generation = location_generation + 1 where id = ?`, videoID); err != nil {
			return err
		}
		if err := syncRepresentativeContainer(ctx, tx, videoID); err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `delete from videos where not exists (select 1 from video_locations where video_locations.video_id = videos.id)`)
	return err
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
