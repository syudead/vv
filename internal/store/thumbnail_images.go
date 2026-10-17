package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// 所有者が代表サムネイルとして適用した画像。画像は content_key に結ぶ thumbnail_images の1行で持ち、
// 再スキャン・取り込み直しを越えて残る。代表サムネイルのファイルは internal/app の Ingest が
// この行から公開し直す。

// ApplyThumbnailImage は、内容 contentKey の動画 videoID の代表サムネイルとして画像 image を
// 記録し、反映後の動画を返す。image.Image は動画の縦横比に合わせた JPEG、image.Source は同じ
// 切り抜きの長辺最大 2048 px の JPEG、image.Origin はその出どころである
//
// 呼ぶのは internal/app の Ingest だけで、同じ画像を公開したあと、生成の錠の中で呼ぶ。1 つの
// 取引で、行（画像・元の画像・出どころ。改版番号は video_overrides.thumbnail_revision と同じ規則）
// を書き、その内容の動画の thumbnail_state を done にして代表サムネイルの代用と失敗の問題を消し、
// 更新日時を進める。video_overrides の位置は残す。
//
// 画像か元の画像が空か、出どころが既知の値でなければ誤りで、何も書かない。videoID の動画が
// 無いか、登録フォルダの下に所在が無いか、内容が contentKey でなければ domain.ErrContentChanged
// で、何も書かない。確定後に domain.VideoOverrideChanged を発行し、問題を消したときは
// domain.ScanChanged も発行する。
func (s *OverrideStore) ApplyThumbnailImage(
	ctx context.Context, videoID int64, contentKey string, image domain.AppliedThumbnailImage,
) (domain.Video, error) {
	if contentKey == "" || len(image.Image) == 0 || len(image.Source) == 0 {
		return domain.Video{}, errors.New("cannot apply an empty thumbnail image")
	}
	if !image.Origin.Valid() {
		return domain.Video{}, fmt.Errorf("cannot apply a thumbnail image of origin %q", image.Origin)
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
		`insert into thumbnail_images (content_key, image, source, origin, revision, applied_at)
		 values (?, ?, ?, ?, ?, ?)
		 on conflict (content_key) do update set
		     image = excluded.image, source = excluded.source, origin = excluded.origin,
		     revision = excluded.revision, applied_at = excluded.applied_at`,
		contentKey, image.Image, image.Source, string(image.Origin), revision, now.UnixMilli(),
	); err != nil {
		return domain.Video{}, fmt.Errorf("cannot save the thumbnail image: %w", err)
	}
	return s.finishThumbnailImageChange(ctx, tx, videoID, contentKey, now)
}

// RevertThumbnailImage は動画 videoID の内容に適用した画像の行を消し、反映後の動画を返す。
//
// 呼ぶのは internal/app の Ingest だけで、位置（無ければ自動の位置）のコマを公開したあと、
// 生成の錠の中で呼ぶ。1 つの取引で、行を消し、その内容の動画の thumbnail_state を done にして
// 代表サムネイルの代用と失敗の問題を消し、更新日時を進める。画像を適用していなくても成功し、
// コマを作り直したので更新日時を進める。
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
	if _, err := deleteThumbnailImage(ctx, tx, keys[0]); err != nil {
		return domain.Video{}, err
	}
	return s.finishThumbnailImageChange(ctx, tx, videoID, keys[0], time.Now())
}

// finishThumbnailImageChange は適用・取り消しの取引の残り（更新日時、thumbnail_state と問題、
// 確定、知らせ）を行う。
func (s *OverrideStore) finishThumbnailImageChange(
	ctx context.Context, tx *sql.Tx, videoID int64, key string, now time.Time,
) (domain.Video, error) {
	if err := touchEditedAt(ctx, tx, []string{key}, now); err != nil {
		return domain.Video{}, err
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

// ThumbnailImageSource は内容 contentKey に適用した画像の元の画像（長辺最大 2048 px の JPEG。
// docs/design-docs/thumbnail-editing.md）を返す。適用していないか、元の画像を持たない
// 行なら nil を返す。
func (s *OverrideStore) ThumbnailImageSource(ctx context.Context, contentKey string) ([]byte, error) {
	var source []byte
	err := s.db.sql.QueryRowContext(ctx,
		`select source from thumbnail_images where content_key = ? and content_key <> ''`, contentKey,
	).Scan(&source)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cannot read the thumbnail image source: %w", err)
	}
	return source, nil
}

// CurrentThumbnailSelection は内容 contentKey の今の代表サムネイルの選び方（適用した画像の元の
// 画像、元の画像を持たない行では代表サムネイル、と指定した位置）を返す。1 つの文で読むので、画像と位置は同じ
// 時点のもので、行が置き換わる途中の組み合わせにはならない。
func (s *OverrideStore) CurrentThumbnailSelection(
	ctx context.Context, contentKey string,
) (domain.CurrentThumbnailSelection, error) {
	if contentKey == "" {
		return domain.CurrentThumbnailSelection{}, nil
	}
	var image []byte
	var position sql.NullInt64
	err := s.db.sql.QueryRowContext(ctx,
		`select (select coalesce(source, image) from thumbnail_images where content_key = ?1),
		        (select thumbnail_position_ms from video_overrides where content_key = ?1)`,
		contentKey,
	).Scan(&image, &position)
	if err != nil {
		return domain.CurrentThumbnailSelection{}, fmt.Errorf("cannot read the current thumbnail: %w", err)
	}
	selection := domain.CurrentThumbnailSelection{Image: image}
	if position.Valid {
		selection.PositionMs = &position.Int64
	}
	return selection, nil
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

// ApplyThumbnailImage・RevertThumbnailImage・ThumbnailImage・ThumbnailImageSource は
// OverrideStore の同名の操作と同じである。代表サムネイルの生成と記録を受け持つ internal/app の
// Ingest が、取り込みの保存先（app.IngestStore）として呼ぶ。

func (s *IngestStore) ApplyThumbnailImage(
	ctx context.Context, videoID int64, contentKey string, image domain.AppliedThumbnailImage,
) (domain.Video, error) {
	return (&OverrideStore{db: s.db}).ApplyThumbnailImage(ctx, videoID, contentKey, image)
}

func (s *IngestStore) RevertThumbnailImage(ctx context.Context, videoID int64) (domain.Video, error) {
	return (&OverrideStore{db: s.db}).RevertThumbnailImage(ctx, videoID)
}

func (s *IngestStore) ThumbnailImage(ctx context.Context, contentKey string) ([]byte, error) {
	return (&OverrideStore{db: s.db}).ThumbnailImage(ctx, contentKey)
}

func (s *IngestStore) ThumbnailImageSource(ctx context.Context, contentKey string) ([]byte, error) {
	return (&OverrideStore{db: s.db}).ThumbnailImageSource(ctx, contentKey)
}
