package store

// 走査の結果（ファイルシステムの所在）を索引へ反映する操作。ScanIndexStore が受け持つ。

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// UpsertVideo は走査で分かった1件を索引に反映する。
//
// 突き合わせは content_key を先に見る。内容が同じ別pathは同じvideoの
// locationとして追加する。論理videoを重複させないことが要求であり、
// 再生位置とサムネイルを引き継ぐ前提でもある。
//
// 内容が変わったとき（サイズか mtime が変わる）は、解析結果を捨てて
// probe_state を pending へ戻す。
func (s *ScanIndexStore) UpsertVideo(ctx context.Context, file domain.VideoFile) (domain.UpsertResult, error) {
	now := time.Now().Unix()
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.UpsertResult{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var locationID, oldVideoID, oldVersion, oldSize, oldMtime int64
	var oldKey string
	err = tx.QueryRowContext(ctx, `
		select l.id, l.video_id, l.version, l.size_bytes, l.mtime, v.content_key
		from video_locations l join videos v on v.id = l.video_id where l.path = ?`, file.Path).
		Scan(&locationID, &oldVideoID, &oldVersion, &oldSize, &oldMtime, &oldKey)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return domain.UpsertResult{}, err
	}
	locationExists := err == nil
	if locationExists && oldKey == file.ContentKey && oldSize == file.SizeBytes && oldMtime == file.MTime.Unix() {
		return domain.UpsertResult{ID: oldVideoID, Outcome: domain.OutcomeUnchanged}, tx.Commit()
	}

	var videoID int64
	var probeState, thumbnailState, previewState string
	err = tx.QueryRowContext(ctx, `select id, probe_state, thumbnail_state, preview_state from videos where content_key = ?`, file.ContentKey).
		Scan(&videoID, &probeState, &thumbnailState, &previewState)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return domain.UpsertResult{}, err
	}
	newVideo := errors.Is(err, sql.ErrNoRows)
	addedAt := file.AddedAt
	if addedAt.IsZero() {
		addedAt = time.Now()
	}
	if newVideo {
		probeState = string(domain.ProbeStatePending)
		thumbnailState = string(domain.ThumbnailStatePending)
		previewState = string(domain.PreviewStatePending)
		res, err := tx.ExecContext(ctx, `
		insert into videos
			(added_at, updated_at, content_key, container, playable, probe_state, thumbnail_state)
		values (?, ?, ?, ?, 0, 'pending', 'pending')`,
			addedAt.Unix(), now, file.ContentKey, nullableString(file.Container))
		if err != nil {
			return domain.UpsertResult{}, fmt.Errorf("動画を取り込めません (%s): %w", file.Path, err)
		}
		videoID, err = res.LastInsertId()
		if err != nil {
			return domain.UpsertResult{}, err
		}
	}

	if locationExists {
		_, err = tx.ExecContext(ctx, `update video_locations set video_id = ?, version = version + 1, title = ?, size_bytes = ?, mtime = ?, updated_at = ? where id = ?`,
			videoID, file.Title, file.SizeBytes, file.MTime.Unix(), now, locationID)
	} else {
		_, err = tx.ExecContext(ctx, `insert into video_locations(video_id, path, version, title, size_bytes, mtime, created_at, updated_at) values (?, ?, 1, ?, ?, ?, ?, ?)`,
			videoID, file.Path, file.Title, file.SizeBytes, file.MTime.Unix(), now, now)
	}
	if err != nil {
		return domain.UpsertResult{}, fmt.Errorf("動画の場所を保存できません (%s): %w", file.Path, err)
	}
	// 題名とパスが変わりうるので、照合用の鍵も同じ書き込みの中で作り直す。
	if err := refreshSearchKeysByPath(ctx, tx, file.Path); err != nil {
		return domain.UpsertResult{}, err
	}
	if !locationExists {
		if _, err := tx.ExecContext(ctx, `update videos set location_generation = location_generation + 1 where id = ?`, videoID); err != nil {
			return domain.UpsertResult{}, err
		}
	} else if oldVideoID != videoID {
		if _, err := tx.ExecContext(ctx, `update videos set location_generation = location_generation + 1 where id in (?, ?)`, oldVideoID, videoID); err != nil {
			return domain.UpsertResult{}, err
		}
	}
	if err := syncRepresentativeContainer(ctx, tx, videoID); err != nil {
		return domain.UpsertResult{}, err
	}
	var released []domain.DeletedVideo
	if locationExists && oldVideoID != videoID {
		if err := syncRepresentativeContainer(ctx, tx, oldVideoID); err != nil {
			return domain.UpsertResult{}, err
		}
		released, err = collectDeletedVideos(tx.QueryContext(ctx, `delete from videos where id = ? and not exists (select 1 from video_locations where video_id = ?)
			returning id, content_key`, oldVideoID, oldVideoID))
		if err != nil {
			return domain.UpsertResult{}, err
		}
	}
	// 内容が変わって前の動画が消えたら、前の内容の生成物を片付けさせる。
	var c changes
	c.videosDeleted(released)
	if err := s.db.commit(tx, &c); err != nil {
		return domain.UpsertResult{}, err
	}
	outcome := domain.OutcomeMoved
	if locationExists {
		outcome = domain.OutcomeUpdated
	} else if newVideo {
		outcome = domain.OutcomeAdded
	}
	return domain.UpsertResult{
		ID: videoID, Outcome: outcome,
		NeedsProbe:     probeState != string(domain.ProbeStateDone),
		NeedsThumbnail: thumbnailState != string(domain.ThumbnailStateDone),
		NeedsPreview:   probeState == string(domain.ProbeStateDone) && previewState != string(domain.PreviewStateDone),
	}, nil
}

// DeleteVideos は指定した行を消す。連鎖してジョブも消える。
// 空の指定で全件消さないよう、何も渡されなければ何もしない。
func (s *ScanIndexStore) DeleteVideos(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}

	//nolint:gosec // 組み立てるのはプレースホルダの数だけで、値は引数で渡す。
	released, err := collectDeletedVideos(s.db.sql.QueryContext(ctx,
		`delete from videos where id in (`+placeholders+`) returning id, content_key`, args...))
	if err != nil {
		return fmt.Errorf("動画を削除できません: %w", err)
	}
	var c changes
	c.videosDeleted(released)
	s.db.publish(&c)
	return nil
}

// DeleteVideoLocations removes filesystem facts and then only orphaned logical videos.
func (s *ScanIndexStore) DeleteVideoLocations(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	affected := map[int64]struct{}{}
	for _, id := range ids {
		var videoID int64
		if err := tx.QueryRowContext(ctx, `select video_id from video_locations where id = ?`, id).Scan(&videoID); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		} else if err == nil {
			affected[videoID] = struct{}{}
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
	released, err := deleteOrphanVideos(ctx, tx)
	if err != nil {
		return err
	}
	var c changes
	c.videosDeleted(released)
	return s.db.commit(tx, &c)
}

// IndexedVideosByPath は索引に入っているものをパスで引ける形で返す。
// 走査はこれと実際のファイルを突き合わせて差分を出す。
func (s *ScanIndexStore) IndexedVideosByPath(ctx context.Context) (map[string]domain.IndexedVideo, error) {
	rows, err := s.db.sql.QueryContext(ctx, `select v.id, l.id, l.version, l.path, v.content_key, l.size_bytes, l.mtime, v.probe_state, v.thumbnail_state, v.preview_state from video_locations l join videos v on v.id = l.video_id`)
	if err != nil {
		return nil, fmt.Errorf("索引を読み出せません: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := map[string]domain.IndexedVideo{}
	for rows.Next() {
		var path string
		var video domain.IndexedVideo
		var mtime int64
		if err := rows.Scan(&video.ID, &video.LocationID, &video.LocationVersion, &path, &video.ContentKey, &video.SizeBytes, &mtime, &video.ProbeState, &video.ThumbnailState, &video.PreviewState); err != nil {
			return nil, fmt.Errorf("索引を読み出せません: %w", err)
		}
		video.MTime = time.Unix(mtime, 0)
		out[path] = video
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("索引を読み出せません: %w", err)
	}
	return out, nil
}
