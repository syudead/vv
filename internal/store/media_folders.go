package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// ListMediaFolders は登録済みのメディアフォルダを返す。走査と設定と閲覧が
// 同じものを読むので、SQL は listMediaFolders の1か所に置く。
func (s *SettingsStore) ListMediaFolders(ctx context.Context) ([]domain.MediaFolder, error) {
	return listMediaFolders(ctx, s.db.sql)
}

func (s *ScanIndexStore) ListMediaFolders(ctx context.Context) ([]domain.MediaFolder, error) {
	return listMediaFolders(ctx, s.db.sql)
}

func (s *LibraryStore) ListMediaFolders(ctx context.Context) ([]domain.MediaFolder, error) {
	return listMediaFolders(ctx, s.db.sql)
}

func listMediaFolders(ctx context.Context, q queryExecer) ([]domain.MediaFolder, error) {
	rows, err := q.QueryContext(ctx, `select id, path, version, created_at, updated_at from media_folders order by id`)
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
		return domain.MediaFolder{}, fmt.Errorf("メディアフォルダを追加できません: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return domain.MediaFolder{}, err
	}
	// 登録の無いまま残っていた所在は鍵が空なので、新しい登録の下にある分を作り直す。
	if err := refreshSearchKeysUnder(ctx, tx, cleaned); err != nil {
		return domain.MediaFolder{}, err
	}
	// 新しい登録の下の所在がグループに入りうる。
	if err := rebuildFolderIndex(ctx, tx); err != nil {
		return domain.MediaFolder{}, err
	}
	// 登録外の所在しか無かった待ちの仕事が、この登録で取り出せるようになる。
	// 眠っているワーカーを起こさないと、次に仕事が積まれるまで止まったままになる。
	var c changes
	c.jobsQueued(domain.JobKinds...)
	if err := s.db.commit(tx, &c); err != nil {
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
	// 相対パスは登録フォルダからの位置なので、新しい登録の下の所在の鍵を作り直す。
	if err := refreshSearchKeysUnder(ctx, tx, cleaned); err != nil {
		return domain.MediaFolder{}, err
	}
	if err := rebuildFolderIndex(ctx, tx); err != nil {
		return domain.MediaFolder{}, err
	}
	var c changes
	c.videosDeleted(released)
	// 付け替え先に所在を持つ待ちの仕事が取り出せるようになる（AddMediaFolder と同じ）。
	c.jobsQueued(domain.JobKinds...)
	if err := s.db.commit(tx, &c); err != nil {
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
	released, err := removeLocationsUnder(ctx, tx, path)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `delete from media_folders where id = ?`, id); err != nil {
		return err
	}
	// 登録外になった所在はグループにもフォルダ名にも入らない。
	if err := rebuildFolderIndex(ctx, tx); err != nil {
		return err
	}
	var c changes
	// 登録を外して消えた動画の生成物を片付けさせる。
	c.videosDeleted(released)
	// 動画の行が残っても、登録外になった所在の仕事は残りとして数えなくなる。
	c.processingChanged()
	return s.db.commit(tx, &c)
}

// ensureFolderPlacementAllowed は取引の中で走査の有無と登録済みのフォルダを
// 読み、path へフォルダを置いてよいかを domain の規則で判断する。取引の外で
// 同じ判断を済ませていても、ここで読み直す。その間に別の要求が走査を始めたり
// フォルダを登録したりしうるためである。
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

// scanRunning は実行中の走査があるかを返す。
func scanRunning(ctx context.Context, tx *sql.Tx) (bool, error) {
	var running int
	if err := tx.QueryRowContext(ctx, `select count(*) from scans where state = 'running'`).Scan(&running); err != nil {
		return false, err
	}
	return running != 0, nil
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

// removeLocationsUnder は root 以下の所在を消し、所在が無くなった動画の行も消す。
// 消した動画を返す。
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
