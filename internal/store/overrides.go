package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// 動画の上書き（specs/029-video-overrides/data-model.md §1・§3、research.md R-1〜R-3）。
// 上書きは content_key に結ぶ video_overrides の1行で持ち、再スキャン・移動・改名・所在の
// 追加を越えて残る。表示名・位置の両方が null になった行は消す。表示名を書き換えたら、
// 同じ取引でその内容の動画の全所在の title_key・search_key を書き直す。

// overrideColumns は動画を返す読み出し（videoColumns・listColumns）の末尾に足す列で、
// 並びは scanVideo と対応する。表示名・位置・改版番号の順である。有効な題名
// （coalesce(display_name, 所在の題名)）は scanVideo が決める。内容の識別子が空の動画には
// 結ばない。
const overrideColumns = `(select ov.display_name from video_overrides ov
		where ov.content_key = videos.content_key and videos.content_key <> '') as display_name,
	(select ov.thumbnail_position_ms from video_overrides ov
		where ov.content_key = videos.content_key and videos.content_key <> '') as thumbnail_position_ms,
	(select ov.thumbnail_revision from video_overrides ov
		where ov.content_key = videos.content_key and videos.content_key <> '') as thumbnail_revision`

// SetDisplayName は動画 videoID の表示名を name にし、有効な題名を含む動画を返す。name は
// domain.NormalizeDisplayName で整え、空（空白だけを含む）なら解除する。規則に合わなければ
// *domain.InvalidDisplayNameError で、何も書かない。
//
// videoID は content_key へ引き直し（registeredContentKeysForVideoIDs）、登録フォルダの下に
// 所在を持たない動画と内容の識別子が空の動画は domain.ErrNotFound にする。確定後に
// domain.VideoOverrideChanged を1回発行する。
func (s *OverrideStore) SetDisplayName(ctx context.Context, videoID int64, name string) (domain.Video, error) {
	normalized, _, err := domain.NormalizeDisplayName(name)
	if err != nil {
		return domain.Video{}, err
	}

	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.Video{}, fmt.Errorf("cannot update the display name: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	keys, err := registeredContentKeysForVideoIDs(ctx, tx, []int64{videoID})
	if err != nil {
		return domain.Video{}, err
	}
	if len(keys) == 0 {
		return domain.Video{}, domain.ErrNotFound
	}
	if err := writeDisplayName(ctx, tx, keys[0], normalized, time.Now()); err != nil {
		return domain.Video{}, err
	}
	if err := refreshSearchKeysForContentKeys(ctx, tx, keys); err != nil {
		return domain.Video{}, err
	}
	video, err := getVideo(ctx, tx, domain.AudienceOwner, videoID)
	if err != nil {
		return domain.Video{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Video{}, fmt.Errorf("cannot update the display name: %w", err)
	}
	s.db.publishEvents(domain.VideoOverrideChanged{VideoID: videoID})
	return video, nil
}

// SetDisplayNames は外部連携の一括操作で、changes の各動画の表示名を書き換え、反映後の
// 動画を changes の順に返す（同じ動画を2度指せば2度返し、表示名は後の指定が勝つ）。
//
// 全体を1つの取引で行い、1件でも失敗すれば何も残さない。表示名が規則に合わなければ
// *domain.DisplayNameAtError、動画を引けなければ（内容を読めていない動画を含む）
// *domain.VideoRefNotFoundError（domain.ErrNotFound を包む）で、どちらも位置を運ぶ。
// 件数の上限は呼び出し側が確かめる。確定後に、変えた動画ごとに domain.VideoOverrideChanged を
// 発行する。
func (s *OverrideStore) SetDisplayNames(ctx context.Context, changes []domain.DisplayNameChange) ([]domain.Video, error) {
	names := make([]string, 0, len(changes))
	refs := make([]domain.VideoRef, 0, len(changes))
	for i, change := range changes {
		name, _, err := domain.NormalizeDisplayName(change.DisplayName)
		if err != nil {
			return nil, &domain.DisplayNameAtError{Index: i, Err: err}
		}
		names = append(names, name)
		refs = append(refs, change.Video)
	}

	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("cannot update the display names: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	targets, err := resolveTaggableVideos(ctx, tx, refs)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	for i, target := range targets {
		if err := writeDisplayName(ctx, tx, target.contentKey, names[i], now); err != nil {
			return nil, err
		}
	}
	keys := uniqueContentKeys(targets)
	if err := refreshSearchKeysForContentKeys(ctx, tx, keys); err != nil {
		return nil, err
	}

	ids := make([]int64, 0, len(targets))
	seen := make(map[int64]bool, len(targets))
	for _, target := range targets {
		if !seen[target.id] {
			seen[target.id] = true
			ids = append(ids, target.id)
		}
	}
	encoded, err := json.Marshal(ids)
	if err != nil {
		return nil, fmt.Errorf("cannot build video ids: %w", err)
	}
	byID, err := readVideosByIDs(ctx, tx, string(encoded))
	if err != nil {
		return nil, err
	}
	out := make([]domain.Video, 0, len(targets))
	for _, target := range targets {
		video, ok := byID[target.id]
		if !ok {
			return nil, fmt.Errorf("cannot read the video (id=%d): %w", target.id, domain.ErrNotFound)
		}
		out = append(out, video)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("cannot update the display names: %w", err)
	}
	events := make([]domain.Event, 0, len(ids))
	for _, id := range ids {
		events = append(events, domain.VideoOverrideChanged{VideoID: id})
	}
	s.db.publishEvents(events...)
	return out, nil
}

// writeDisplayName は内容 key の表示名を name（空なら解除）にし、表示名も位置も無くなった
// 行を消す。name は整え済みである。
func writeDisplayName(ctx context.Context, tx *sql.Tx, key, name string, now time.Time) error {
	if _, err := tx.ExecContext(ctx,
		`insert into video_overrides (content_key, display_name, updated_at) values (?, ?, ?)
		 on conflict (content_key) do update set display_name = excluded.display_name, updated_at = excluded.updated_at`,
		key, nullableString(name), now.Unix(),
	); err != nil {
		return fmt.Errorf("cannot save the display name: %w", err)
	}
	if err := deleteEmptyOverride(ctx, tx, key); err != nil {
		return err
	}
	return nil
}

// deleteEmptyOverride は内容 key の行が表示名も位置も持たなければ消す（data-model.md §1 の
// 不変条件）。
func deleteEmptyOverride(ctx context.Context, tx *sql.Tx, key string) error {
	if _, err := tx.ExecContext(ctx,
		`delete from video_overrides where content_key = ? and display_name is null and thumbnail_position_ms is null`,
		key,
	); err != nil {
		return fmt.Errorf("cannot save the video override: %w", err)
	}
	return nil
}
