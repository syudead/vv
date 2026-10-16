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

	applied, err := s.attachTagToVideoIDs(ctx, tx, videoIDs, tagID)
	if err != nil {
		return domain.TagRef{}, 0, err
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

	applied, err := s.attachTagToVideoIDs(ctx, tx, videoIDs, ref.ID)
	if err != nil {
		return domain.TagRef{}, 0, err
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

	now := time.Now()
	applied, changed, err := detachTagFromVideoIDs(ctx, tx, videoIDs, tagID, now)
	if err != nil {
		return domain.TagRef{}, 0, err
	}
	if err := s.commitTagChanges(ctx, tx, domain.ActivityTagDetach, changed, tagID, now); err != nil {
		return domain.TagRef{}, 0, fmt.Errorf("cannot remove the tag: %w", err)
	}
	return ref, applied, nil
}

// attachTagToVideoIDs は videoIDs のうちいまライブラリにある動画の利用者データの鍵
// （userKeysForVideoIDs。同じ集まりは 1 つ）へ tagID を insert or ignore で付け、実際に付いた鍵ごとに
// tag_attach の記録を書いて取引 tx を確定し、反映した本数を返す。実際に付いた鍵（集まりなら全メンバー）の
// 更新日時を今にする（specs/033-video-dates/research.md R-3）。
func (s *TagStore) attachTagToVideoIDs(ctx context.Context, tx *sql.Tx, videoIDs []int64, tagID int64) (int, error) {
	keys, err := userKeysForVideoIDs(ctx, tx, videoIDs)
	if err != nil {
		return 0, err
	}
	now := time.Now()
	changed, err := attachTagToUserKeys(ctx, tx, keys, tagID, now)
	if err != nil {
		return 0, err
	}
	if err := s.commitTagChanges(ctx, tx, domain.ActivityTagAttach, changed, tagID, now); err != nil {
		return 0, fmt.Errorf("cannot add the tag: %w", err)
	}
	return len(keys), nil
}

// commitTagChanges は鍵 changed のそれぞれにタグ tagID の kind の記録を書き、取引 tx を確定する
// （specs/055-activity/research.md R-6）。確定後に domain.ActivityChanged を発行する。
func (s *TagStore) commitTagChanges(
	ctx context.Context, tx *sql.Tx, kind domain.ActivityKind, changed []string, tagID int64, now time.Time,
) error {
	c := &changes{}
	if err := recordOwnerVideoActivity(ctx, tx, c, kind, ownerTagChanges(changed, tagID), now); err != nil {
		return err
	}
	return s.db.commit(ctx, tx, c)
}

// attachTagToUserKeys は利用者データの鍵 keys へ tagID を手で付け、実際に付いた鍵を keys の順で返す。
// 仮に付いていたタグは確定する。実際に付いた鍵（集まりなら全メンバー）の更新日時を now にする。
func attachTagToUserKeys(ctx context.Context, tx *sql.Tx, keys []string, tagID int64, now time.Time) ([]string, error) {
	changed := make([]string, 0, len(keys))
	for _, key := range keys {
		result, err := tx.ExecContext(ctx,
			// 自動タグ付けが仮に付けていたタグを手で付けると、確定する。
			`insert into video_tags (content_key, tag_id, created_at) values (?, ?, ?)
			 on conflict (content_key, tag_id) do update set tentative = 0 where video_tags.tentative = 1`,
			key, tagID, now.Unix(),
		)
		if err != nil {
			return nil, fmt.Errorf("cannot add the tag (tag=%d): %w", tagID, err)
		}
		if affected, err := result.RowsAffected(); err != nil {
			return nil, fmt.Errorf("cannot add the tag (tag=%d): %w", tagID, err)
		} else if affected > 0 {
			changed = append(changed, key)
		}
	}
	return changed, touchEditedAtForUserKeys(ctx, tx, changed, now)
}

// detachTagFromVideoIDs は videoIDs のうちいまライブラリにある動画から tagID
// を外し、反映した本数と、実際に外れた鍵を返す。実際に外れた鍵（集まりなら全メンバー）の更新日時を now にする。
func detachTagFromVideoIDs(ctx context.Context, tx *sql.Tx, videoIDs []int64, tagID int64, now time.Time) (int, []string, error) {
	keys, err := userKeysForVideoIDs(ctx, tx, videoIDs)
	if err != nil {
		return 0, nil, err
	}
	changed := make([]string, 0, len(keys))
	for _, key := range keys {
		result, err := tx.ExecContext(ctx,
			`delete from video_tags where content_key = ? and tag_id = ?`, key, tagID,
		)
		if err != nil {
			return 0, nil, fmt.Errorf("cannot remove the tag (tag=%d): %w", tagID, err)
		}
		if affected, err := result.RowsAffected(); err != nil {
			return 0, nil, fmt.Errorf("cannot remove the tag (tag=%d): %w", tagID, err)
		} else if affected > 0 {
			changed = append(changed, key)
		}
	}
	if err := touchEditedAtForUserKeys(ctx, tx, changed, now); err != nil {
		return 0, nil, err
	}
	return len(keys), changed, nil
}

// TagsByContentKeys は利用者データの鍵（Video.UserKey、specs/030-video-versions/data-model.md §3）の
// 集合からそれぞれのタグ（元の名前、
// domain.CompareNatural の順、同じなら id）を出所つきでまとめて引く。手で
// 付けた分とフォルダ名から付いている分の和で、同じタグが両方から付けば1件に
// まとめて出所を両方持つ（017 の data-model.md §4）。タグの無い content_key は
// 結果に現れない。PlaybackStore.ProgressByContentKeys と同じ形で、
// internal/httpapi が progressFor と同じ位置から一覧の項目にタグを足すために
// 使う。
func (s *TagStore) TagsByContentKeys(ctx context.Context, contentKeys []string) (map[string][]domain.VideoTag, error) {
	return tagsByContentKeys(ctx, s.sql, contentKeys)
}

// tagsByContentKeys は TagStore.TagsByContentKeys の本体で、手で付けた分は鍵で、フォルダ名から
// 付く分はその鍵を持つ動画（集まりの鍵ならメンバーの動画）の videos.id から引く。呼び出し側の取引の中でも
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
		keyed_videos(user_key, video_id) as (
			select v.content_key, v.id from videos v
			 where v.content_key in (select content_key from selected) and v.content_key <> ''
			   and not exists (select 1 from video_bundle_members m where m.content_key = v.content_key)
			union all
			select b.user_key, v.id from video_bundles b
			  join video_bundle_members m on m.bundle_id = b.id
			  join videos v on v.content_key = m.content_key and v.content_key <> ''
			 where b.user_key in (select content_key from selected)
		),
		tagged(content_key, tag_id, manual, from_folder, unconfirmed) as (
			select vt.content_key, vt.tag_id, 1, 0, vt.tentative from video_tags vt
			 where vt.content_key in (select content_key from selected)
			union all
			select k.user_key, folder_tn.tag_id, 0, 1, 0 from keyed_videos k
			  join video_folder_names vfn on vfn.video_id = k.video_id
			  join tag_names folder_tn on folder_tn.name = vfn.name
		)
		select t.content_key, t.tag_id, tn.name, tg.tentative, max(t.manual), max(t.from_folder), min(t.unconfirmed),
		       `+tagKindColumns+`
		  from tagged t
		  join tag_names tn on tn.tag_id = t.tag_id and tn.canonical = 1
		  join tags tg on tg.id = t.tag_id`+tagKindJoin+`
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
		var kind scannedTagKind
		if err := rows.Scan(append([]any{&key, &tag.ID, &tag.Name, &tag.Tentative, &tag.Manual, &tag.FromFolder, &tag.Unconfirmed},
			kind.targets()...)...); err != nil {
			return nil, fmt.Errorf("cannot read item tags: %w", err)
		}
		tag.Kind = kind.ref()
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
