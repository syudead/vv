package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// 所有者が代表サムネイルとして適用した画像（specs/045-comfyui-thumbnails/data-model.md、
// research.md R-6・R-10）。画像は content_key に結ぶ thumbnail_images の1行で持ち、
// 再スキャン・取り込み直しを越えて残る。代表サムネイルのファイルは internal/app の Ingest が
// この行から公開し直す。

// ApplyThumbnailImage は、内容 contentKey の動画 videoID の代表サムネイルとして画像 image
// （動画の縦横比に合わせた JPEG）を記録し、反映後の動画を返す。
//
// 呼ぶのは internal/app の Ingest だけで、同じ画像を公開したあと、生成の錠の中で呼ぶ。1 つの
// 取引で、行を書き（改版番号は video_overrides.thumbnail_revision と同じ規則）、その内容の動画の
// thumbnail_state を done にして代表サムネイルの代用と失敗の問題を消し、更新日時を進める。
// video_overrides の位置は残す（R-10）。
//
// videoID の動画が無いか、登録フォルダの下に所在が無いか、内容が contentKey でなければ
// domain.ErrContentChanged で、何も書かない。確定後に domain.VideoOverrideChanged を発行し、
// 問題を消したときは domain.ScanChanged も発行する。
func (s *OverrideStore) ApplyThumbnailImage(
	ctx context.Context, videoID int64, contentKey string, image []byte,
) (domain.Video, error) {
	if contentKey == "" || len(image) == 0 {
		return domain.Video{}, errors.New("cannot apply an empty thumbnail image")
	}
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.Video{}, fmt.Errorf("cannot apply the thumbnail image: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	keys, err := registeredContentKeysForVideoIDs(ctx, tx, []int64{videoID})
	if err != nil {
		return domain.Video{}, err
	}
	if len(keys) == 0 || keys[0] != contentKey {
		return domain.Video{}, domain.ErrContentChanged
	}
	now := time.Now()
	var previous sql.NullInt64
	err = tx.QueryRowContext(ctx,
		`select revision from thumbnail_images where content_key = ?`, contentKey,
	).Scan(&previous)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return domain.Video{}, fmt.Errorf("cannot read the thumbnail image revision: %w", err)
	}
	revision := s.db.nextThumbnailRevision(now, previous.Int64)
	if _, err := tx.ExecContext(ctx,
		`insert into thumbnail_images (content_key, image, revision, applied_at) values (?, ?, ?, ?)
		 on conflict (content_key) do update set
		     image = excluded.image, revision = excluded.revision, applied_at = excluded.applied_at`,
		contentKey, image, revision, now.UnixMilli(),
	); err != nil {
		return domain.Video{}, fmt.Errorf("cannot save the thumbnail image: %w", err)
	}
	return s.finishThumbnailImageChange(ctx, tx, videoID, contentKey, true, now)
}

// RevertThumbnailImage は動画 videoID の内容に適用した画像の行を消し、反映後の動画を返す。
//
// 呼ぶのは internal/app の Ingest だけで、位置（無ければ自動の位置）のコマを公開したあと、
// 生成の錠の中で呼ぶ。1 つの取引で、行を消し、その内容の動画の thumbnail_state を done にして
// 代表サムネイルの代用と失敗の問題を消す。更新日時は行を消したときだけ進める。画像を適用して
// いなくても成功する。
//
// videoID は content_key へ引き直し、引けなければ domain.ErrNotFound にする。確定後に
// domain.VideoOverrideChanged を発行し、問題を消したときは domain.ScanChanged も発行する。
func (s *OverrideStore) RevertThumbnailImage(ctx context.Context, videoID int64) (domain.Video, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.Video{}, fmt.Errorf("cannot revert the thumbnail image: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	keys, err := registeredContentKeysForVideoIDs(ctx, tx, []int64{videoID})
	if err != nil {
		return domain.Video{}, err
	}
	if len(keys) == 0 {
		return domain.Video{}, domain.ErrNotFound
	}
	removed, err := deleteThumbnailImage(ctx, tx, keys[0])
	if err != nil {
		return domain.Video{}, err
	}
	return s.finishThumbnailImageChange(ctx, tx, videoID, keys[0], removed, time.Now())
}

// finishThumbnailImageChange は適用・取り消しの取引の残り（更新日時、thumbnail_state と問題、
// 確定、知らせ）を行う。edited が偽なら更新日時を進めない。
func (s *OverrideStore) finishThumbnailImageChange(
	ctx context.Context, tx *sql.Tx, videoID int64, key string, edited bool, now time.Time,
) (domain.Video, error) {
	if edited {
		if err := touchEditedAt(ctx, tx, []string{key}, now); err != nil {
			return domain.Video{}, err
		}
	}
	issuesCleared, err := markThumbnailDoneForContent(ctx, tx, key, now)
	if err != nil {
		return domain.Video{}, err
	}
	video, err := getVideo(ctx, tx, domain.AudienceOwner, videoID)
	if err != nil {
		return domain.Video{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Video{}, fmt.Errorf("cannot record the thumbnail image: %w", err)
	}
	events := []domain.Event{domain.VideoOverrideChanged{VideoID: videoID}}
	if issuesCleared {
		events = append(events, domain.ScanChanged{})
	}
	s.db.publishEvents(events...)
	return video, nil
}

// ThumbnailImage は内容 contentKey に適用した画像を返す。適用していなければ nil を返す。
func (s *OverrideStore) ThumbnailImage(ctx context.Context, contentKey string) ([]byte, error) {
	var image []byte
	err := s.db.sql.QueryRowContext(ctx,
		`select image from thumbnail_images where content_key = ? and content_key <> ''`, contentKey,
	).Scan(&image)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cannot read the thumbnail image: %w", err)
	}
	return image, nil
}

// deleteThumbnailImage は内容 key に適用した画像の行を消し、消したかを返す。
func deleteThumbnailImage(ctx context.Context, tx *sql.Tx, key string) (bool, error) {
	result, err := tx.ExecContext(ctx, `delete from thumbnail_images where content_key = ?`, key)
	if err != nil {
		return false, fmt.Errorf("cannot remove the thumbnail image: %w", err)
	}
	removed, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("cannot remove the thumbnail image: %w", err)
	}
	return removed > 0, nil
}

// ApplyThumbnailImage・RevertThumbnailImage・ThumbnailImage は OverrideStore の同名の操作と
// 同じである。代表サムネイルの生成と記録を受け持つ internal/app の Ingest が、取り込みの
// 保存先（app.IngestStore）として呼ぶ。

func (s *IngestStore) ApplyThumbnailImage(
	ctx context.Context, videoID int64, contentKey string, image []byte,
) (domain.Video, error) {
	return (&OverrideStore{db: s.db}).ApplyThumbnailImage(ctx, videoID, contentKey, image)
}

func (s *IngestStore) RevertThumbnailImage(ctx context.Context, videoID int64) (domain.Video, error) {
	return (&OverrideStore{db: s.db}).RevertThumbnailImage(ctx, videoID)
}

func (s *IngestStore) ThumbnailImage(ctx context.Context, contentKey string) ([]byte, error) {
	return (&OverrideStore{db: s.db}).ThumbnailImage(ctx, contentKey)
}
