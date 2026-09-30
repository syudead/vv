package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// AttachTagByID は id で指定したタグを videoIDs の動画へ付ける
// （data-model.md §4）。videoIDs は、いまライブラリにある動画の content_key へ
// 引き直したうえで、content_key ごとに insert or ignore する。既に付いていた
// 動画があっても誤りにしない。引けない id（消えた動画）は飛ばし、反映した
// 本数（applied）を返す。applied には、既に付いていた・付いていなかった動画も
// 数に入る。tagID が無ければ domain.ErrTagNotFound。全体を1つのトランザクション
// で行い、途中で失敗したら何も残さない。
func (s *TagStore) AttachTagByID(ctx context.Context, videoIDs []int64, tagID int64) (domain.TagRef, int, error) {
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.TagRef{}, 0, err
	}
	defer func() { _ = tx.Rollback() }()

	ref, err := tagRefByID(ctx, tx, tagID)
	if err != nil {
		return domain.TagRef{}, 0, err
	}

	applied, err := attachTagToVideoIDs(ctx, tx, videoIDs, tagID)
	if err != nil {
		return domain.TagRef{}, 0, err
	}

	if err := tx.Commit(); err != nil {
		return domain.TagRef{}, 0, fmt.Errorf("cannot add the tag: %w", err)
	}
	return ref, applied, nil
}

// AttachTagByName は名前でタグを付ける。名前はシノニムを含めて引き、無ければ
// 同じトランザクションの中で作る（data-model.md §3・§4、要件 1）。それ以外は
// AttachTagByID と同じ。
func (s *TagStore) AttachTagByName(ctx context.Context, videoIDs []int64, name string) (domain.TagRef, int, error) {
	normalized, err := domain.NormalizeTagName(name)
	if err != nil {
		return domain.TagRef{}, 0, err
	}

	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.TagRef{}, 0, err
	}
	defer func() { _ = tx.Rollback() }()

	ref, _, err := findOrCreateTag(ctx, tx, normalized)
	if err != nil {
		return domain.TagRef{}, 0, err
	}

	applied, err := attachTagToVideoIDs(ctx, tx, videoIDs, ref.ID)
	if err != nil {
		return domain.TagRef{}, 0, err
	}

	if err := tx.Commit(); err != nil {
		return domain.TagRef{}, 0, fmt.Errorf("cannot add the tag: %w", err)
	}
	return ref, applied, nil
}

// DetachTag は id で指定したタグを videoIDs の動画から外す（data-model.md §4）。
// 付いていない動画も誤りにしない。tagID が無ければ domain.ErrTagNotFound。
func (s *TagStore) DetachTag(ctx context.Context, videoIDs []int64, tagID int64) (domain.TagRef, int, error) {
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.TagRef{}, 0, err
	}
	defer func() { _ = tx.Rollback() }()

	ref, err := tagRefByID(ctx, tx, tagID)
	if err != nil {
		return domain.TagRef{}, 0, err
	}

	applied, err := detachTagFromVideoIDs(ctx, tx, videoIDs, tagID)
	if err != nil {
		return domain.TagRef{}, 0, err
	}

	if err := tx.Commit(); err != nil {
		return domain.TagRef{}, 0, fmt.Errorf("cannot remove the tag: %w", err)
	}
	return ref, applied, nil
}

// attachTagToVideoIDs は videoIDs のうちいまライブラリにある動画へ tagID を
// insert or ignore で付け、反映した本数を返す。
func attachTagToVideoIDs(ctx context.Context, tx *sql.Tx, videoIDs []int64, tagID int64) (int, error) {
	keys, err := registeredContentKeysForVideoIDs(ctx, tx, videoIDs)
	if err != nil {
		return 0, err
	}
	now := time.Now().Unix()
	for _, key := range keys {
		if _, err := tx.ExecContext(ctx,
			`insert or ignore into video_tags (content_key, tag_id, created_at) values (?, ?, ?)`,
			key, tagID, now,
		); err != nil {
			return 0, fmt.Errorf("cannot add the tag (tag=%d): %w", tagID, err)
		}
	}
	return len(keys), nil
}

// detachTagFromVideoIDs は videoIDs のうちいまライブラリにある動画から tagID
// を外し、反映した本数を返す。
func detachTagFromVideoIDs(ctx context.Context, tx *sql.Tx, videoIDs []int64, tagID int64) (int, error) {
	keys, err := registeredContentKeysForVideoIDs(ctx, tx, videoIDs)
	if err != nil {
		return 0, err
	}
	for _, key := range keys {
		if _, err := tx.ExecContext(ctx,
			`delete from video_tags where content_key = ? and tag_id = ?`, key, tagID,
		); err != nil {
			return 0, fmt.Errorf("cannot remove the tag (tag=%d): %w", tagID, err)
		}
	}
	return len(keys), nil
}

// TagsByContentKeys は content_key の集合からそれぞれのタグ（元の名前、
// domain.CompareNatural の順、同じなら id）を出所つきでまとめて引く。手で
// 付けた分とフォルダ名から付いている分の和で、同じタグが両方から付けば1件に
// まとめて出所を両方持つ（017 の data-model.md §4）。タグの無い content_key は
// 結果に現れない。PlaybackStore.ProgressByContentKeys と同じ形で、
// internal/httpapi が progressFor と同じ位置から一覧の項目にタグを足すために
// 使う。
func (s *TagStore) TagsByContentKeys(ctx context.Context, contentKeys []string) (map[string][]domain.VideoTag, error) {
	return tagsByContentKeys(ctx, s.sql, contentKeys)
}

// tagsByContentKeys は TagStore.TagsByContentKeys の本体で、呼び出し側の取引の中でも
// 読めるように問い合わせ先を取る（外部連携 API の一覧が同じスナップショットで読む）。
func tagsByContentKeys(ctx context.Context, q queryExecer, contentKeys []string) (map[string][]domain.VideoTag, error) {
	if len(contentKeys) == 0 {
		return map[string][]domain.VideoTag{}, nil
	}
	encoded, err := json.Marshal(contentKeys)
	if err != nil {
		return nil, fmt.Errorf("cannot build content_key values: %w", err)
	}

	rows, err := q.QueryContext(ctx, `
		with selected(content_key) as (select value from json_each(?)),
		tagged(content_key, tag_id, manual, from_folder) as (
			select vt.content_key, vt.tag_id, 1, 0 from video_tags vt
			 where vt.content_key in (select content_key from selected)
			union all
			select v.content_key, folder_tn.tag_id, 0, 1 from videos v
			  join video_folder_names vfn on vfn.video_id = v.id
			  join tag_names folder_tn on folder_tn.name = vfn.name
			 where v.content_key in (select content_key from selected) and v.content_key <> ''
		)
		select t.content_key, t.tag_id, tn.name, tg.tentative, max(t.manual), max(t.from_folder)
		  from tagged t
		  join tag_names tn on tn.tag_id = t.tag_id and tn.canonical = 1
		  join tags tg on tg.id = t.tag_id
		 group by t.content_key, t.tag_id`, string(encoded),
	)
	if err != nil {
		return nil, fmt.Errorf("cannot read item tags: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make(map[string][]domain.VideoTag, len(contentKeys))
	for rows.Next() {
		var key string
		var tag domain.VideoTag
		if err := rows.Scan(&key, &tag.ID, &tag.Name, &tag.Tentative, &tag.Manual, &tag.FromFolder); err != nil {
			return nil, fmt.Errorf("cannot read item tags: %w", err)
		}
		out[key] = append(out[key], tag)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read item tags: %w", err)
	}
	for key := range out {
		domain.SortVideoTags(out[key])
	}
	return out, nil
}
