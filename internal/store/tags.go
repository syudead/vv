package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// タグそのものの保存（specs/014-video-tags/data-model.md §1〜§4・§7）。
//
// TagStore は PlaybackStore と同じく、共有する SQLite 接続だけを持ち、
// ライブラリ索引の役割の型（LibraryStore）にも通知の発行にも依存しない。
// タグの変更は副作用（生成物の削除・ワーカーの起床・/api/events）を持たない
// ので、トランザクションのコミットだけで済む（Plan の Constitution Check）。

// tagTx は1つのトランザクションの中で名前とタグの行を読み書きするための
// 共通部分である。*sql.Tx はこれを満たす。
type tagTx interface {
	queryExecer
	rowQueryer
}

// CreateTag は新しいタグを作る。名前が既に別のタグの元の名前かシノニムなら
// *domain.TagNameConflict（domain.ErrTagNameTaken）を返す。
func (s *TagStore) CreateTag(ctx context.Context, name string) (domain.Tag, error) {
	normalized, err := domain.NormalizeTagName(name)
	if err != nil {
		return domain.Tag{}, err
	}

	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.Tag{}, err
	}
	defer func() { _ = tx.Rollback() }()

	lookup, found, err := lookupTagName(ctx, tx, normalized)
	if err != nil {
		return domain.Tag{}, err
	}
	if found {
		return domain.Tag{}, &domain.TagNameConflict{Tag: domain.TagRef{ID: lookup.tagID, Name: lookup.canonicalName}}
	}

	res, err := tx.ExecContext(ctx, `insert into tags (created_at) values (?)`, time.Now().Unix())
	if err != nil {
		return domain.Tag{}, fmt.Errorf("タグを作成できません: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return domain.Tag{}, fmt.Errorf("タグを作成できません: %w", err)
	}
	if err := insertTagName(ctx, tx, normalized, id, true); err != nil {
		return domain.Tag{}, err
	}
	// 作ったばかりのタグでも、同じ名前の祖先フォルダの下の動画にはもう付いて
	// いる（017 の data-model.md §4）ので、本数は数える。
	count, err := videoCountByTagID(ctx, tx, id)
	if err != nil {
		return domain.Tag{}, err
	}

	if err := tx.Commit(); err != nil {
		return domain.Tag{}, fmt.Errorf("タグを作成できません: %w", err)
	}
	return domain.Tag{ID: id, Name: normalized, Synonyms: []string{}, VideoCount: count}, nil
}

// RenameTag は id の元の名前を書き換える。今と同じ名前なら何も変えずに今の
// 状態を返す。新しい名前が既にあれば（自分のシノニムでも）
// *domain.TagNameConflict を返す。id が無ければ domain.ErrTagNotFound を返す。
func (s *TagStore) RenameTag(ctx context.Context, id int64, name string) (domain.Tag, error) {
	normalized, err := domain.NormalizeTagName(name)
	if err != nil {
		return domain.Tag{}, err
	}

	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.Tag{}, err
	}
	defer func() { _ = tx.Rollback() }()

	current, err := canonicalNameByTagID(ctx, tx, id)
	if err != nil {
		return domain.Tag{}, err
	}

	if current != normalized {
		lookup, found, err := lookupTagName(ctx, tx, normalized)
		if err != nil {
			return domain.Tag{}, err
		}
		if found {
			return domain.Tag{}, &domain.TagNameConflict{Tag: domain.TagRef{ID: lookup.tagID, Name: lookup.canonicalName}}
		}
		if _, err := tx.ExecContext(ctx, `
			update tag_names set name = ?, search_key = ?, search_version = ?
			 where tag_id = ? and canonical = 1`,
			normalized, domain.FoldForMatch(normalized), domain.SearchKeyVersion, id,
		); err != nil {
			return domain.Tag{}, fmt.Errorf("タグを改名できません (id=%d): %w", id, err)
		}
	}

	tag, err := tagByID(ctx, tx, id)
	if err != nil {
		return domain.Tag{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Tag{}, fmt.Errorf("タグを改名できません: %w", err)
	}
	return tag, nil
}

// DeleteTag はタグを消す。tag_names と video_tags は外部キーの ON DELETE
// CASCADE で連鎖して消える。いまライブラリに無い動画への付与も消える。
// id が無ければ domain.ErrTagNotFound を返す。
func (s *TagStore) DeleteTag(ctx context.Context, id int64) error {
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := canonicalNameByTagID(ctx, tx, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `delete from tags where id = ?`, id); err != nil {
		return fmt.Errorf("タグを削除できません (id=%d): %w", id, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("タグを削除できません: %w", err)
	}
	return nil
}

// MergeTag は sourceID のタグを targetID へ統合する（data-model.md §4、Plan の
// Structural Decisions 11）。source の付与は insert or ignore で target へ写り、
// source の元の名前とシノニムはすべて target のシノニムになり、source は
// 一覧から消える。どちらかが無ければ domain.ErrTagNotFound を返す。
func (s *TagStore) MergeTag(ctx context.Context, targetID, sourceID int64) (domain.Tag, error) {
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.Tag{}, err
	}
	defer func() { _ = tx.Rollback() }()

	tag, err := mergeTagInto(ctx, tx, targetID, sourceID)
	if err != nil {
		return domain.Tag{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Tag{}, fmt.Errorf("タグを統合できません: %w", err)
	}
	return tag, nil
}

// mergeTagInto は同じトランザクションの中で source を target へ統合する。
// target と source が同じ id なら何もせず、今の target をそのまま返す。
func mergeTagInto(ctx context.Context, tx *sql.Tx, targetID, sourceID int64) (domain.Tag, error) {
	if _, err := canonicalNameByTagID(ctx, tx, targetID); err != nil {
		return domain.Tag{}, err
	}
	if targetID == sourceID {
		return tagByID(ctx, tx, targetID)
	}
	if _, err := canonicalNameByTagID(ctx, tx, sourceID); err != nil {
		return domain.Tag{}, err
	}

	if _, err := tx.ExecContext(ctx, `
		insert or ignore into video_tags (content_key, tag_id, created_at)
		select content_key, ?, created_at from video_tags where tag_id = ?`,
		targetID, sourceID,
	); err != nil {
		return domain.Tag{}, fmt.Errorf("付与を統合先へ写せません (source=%d target=%d): %w", sourceID, targetID, err)
	}

	if _, err := tx.ExecContext(ctx, `update tag_names set tag_id = ?, canonical = 0 where tag_id = ?`,
		targetID, sourceID,
	); err != nil {
		return domain.Tag{}, fmt.Errorf("タグ名を統合先へ付け替えられません (source=%d target=%d): %w", sourceID, targetID, err)
	}

	if _, err := tx.ExecContext(ctx, `delete from tags where id = ?`, sourceID); err != nil {
		return domain.Tag{}, fmt.Errorf("統合元のタグを削除できません (id=%d): %w", sourceID, err)
	}

	return tagByID(ctx, tx, targetID)
}

// AddSynonym は名前 name をタグ tagID のシノニムにする（data-model.md §4）。
//
//   - name が無ければ canonical = 0 で1行足す。
//   - name が既に tagID のシノニムなら何も変えない。
//   - name が tagID 自身の元の名前なら *domain.TagNameConflict を返す。
//   - name が別のタグ S のシノニムなら *domain.TagNameConflict を返す（S を示す）。
//   - name が別のタグ S の元の名前なら、mergeTagID が S の id と一致すれば
//     S を tagID へ統合し、一致しなければ（無い場合を含む）
//     *domain.TagMergeRequired を返す（S を示す）。
func (s *TagStore) AddSynonym(ctx context.Context, tagID int64, name string, mergeTagID *int64) (domain.Tag, error) {
	normalized, err := domain.NormalizeTagName(name)
	if err != nil {
		return domain.Tag{}, err
	}

	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.Tag{}, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := canonicalNameByTagID(ctx, tx, tagID); err != nil {
		return domain.Tag{}, err
	}

	lookup, found, err := lookupTagName(ctx, tx, normalized)
	if err != nil {
		return domain.Tag{}, err
	}

	switch {
	case !found:
		if err := insertTagName(ctx, tx, normalized, tagID, false); err != nil {
			return domain.Tag{}, err
		}
	case lookup.tagID == tagID && lookup.isCanonical:
		return domain.Tag{}, &domain.TagNameConflict{Tag: domain.TagRef{ID: tagID, Name: lookup.canonicalName}}
	case lookup.tagID == tagID:
		// 既にこのタグのシノニム。何も変えない。
	case !lookup.isCanonical:
		return domain.Tag{}, &domain.TagNameConflict{Tag: domain.TagRef{ID: lookup.tagID, Name: lookup.canonicalName}}
	case mergeTagID == nil || *mergeTagID != lookup.tagID:
		return domain.Tag{}, &domain.TagMergeRequired{Tag: domain.TagRef{ID: lookup.tagID, Name: lookup.canonicalName}}
	default:
		if _, err := mergeTagInto(ctx, tx, tagID, lookup.tagID); err != nil {
			return domain.Tag{}, err
		}
	}

	tag, err := tagByID(ctx, tx, tagID)
	if err != nil {
		return domain.Tag{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Tag{}, fmt.Errorf("シノニムを登録できません: %w", err)
	}
	return tag, nil
}

// RemoveSynonym はタグ tagID のシノニム name を解除する。name が tagID の
// シノニムでなければ何も変えない。tagID が無ければ domain.ErrTagNotFound を
// 返す。
func (s *TagStore) RemoveSynonym(ctx context.Context, tagID int64, name string) error {
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := canonicalNameByTagID(ctx, tx, tagID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`delete from tag_names where name = ? and tag_id = ? and canonical = 0`, name, tagID,
	); err != nil {
		return fmt.Errorf("シノニムを解除できません (tag=%d): %w", tagID, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("シノニムを解除できません: %w", err)
	}
	return nil
}

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

	name, err := canonicalNameByTagID(ctx, tx, tagID)
	if err != nil {
		return domain.TagRef{}, 0, err
	}

	applied, err := attachTagToVideoIDs(ctx, tx, videoIDs, tagID)
	if err != nil {
		return domain.TagRef{}, 0, err
	}

	if err := tx.Commit(); err != nil {
		return domain.TagRef{}, 0, fmt.Errorf("タグを付けられません: %w", err)
	}
	return domain.TagRef{ID: tagID, Name: name}, applied, nil
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

	lookup, found, err := lookupTagName(ctx, tx, normalized)
	if err != nil {
		return domain.TagRef{}, 0, err
	}

	var ref domain.TagRef
	if found {
		ref = domain.TagRef{ID: lookup.tagID, Name: lookup.canonicalName}
	} else {
		res, err := tx.ExecContext(ctx, `insert into tags (created_at) values (?)`, time.Now().Unix())
		if err != nil {
			return domain.TagRef{}, 0, fmt.Errorf("タグを作成できません: %w", err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			return domain.TagRef{}, 0, fmt.Errorf("タグを作成できません: %w", err)
		}
		if err := insertTagName(ctx, tx, normalized, id, true); err != nil {
			return domain.TagRef{}, 0, err
		}
		ref = domain.TagRef{ID: id, Name: normalized}
	}

	applied, err := attachTagToVideoIDs(ctx, tx, videoIDs, ref.ID)
	if err != nil {
		return domain.TagRef{}, 0, err
	}

	if err := tx.Commit(); err != nil {
		return domain.TagRef{}, 0, fmt.Errorf("タグを付けられません: %w", err)
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

	name, err := canonicalNameByTagID(ctx, tx, tagID)
	if err != nil {
		return domain.TagRef{}, 0, err
	}

	applied, err := detachTagFromVideoIDs(ctx, tx, videoIDs, tagID)
	if err != nil {
		return domain.TagRef{}, 0, err
	}

	if err := tx.Commit(); err != nil {
		return domain.TagRef{}, 0, fmt.Errorf("タグを外せません: %w", err)
	}
	return domain.TagRef{ID: tagID, Name: name}, applied, nil
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
			return 0, fmt.Errorf("タグを付けられません (tag=%d): %w", tagID, err)
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
			return 0, fmt.Errorf("タグを外せません (tag=%d): %w", tagID, err)
		}
	}
	return len(keys), nil
}

// Summary は videoIDs のうちいまライブラリにある動画の数（Total）と、1本以上に
// 付いているタグごとの本数（Items）を、名前の自然順で返す
// （contracts/tags-api.md §4 の summary）。
func (s *TagStore) Summary(ctx context.Context, videoIDs []int64) (domain.TagSummary, error) {
	// videoIDs → content_key の解決と本数の集計を同じ読み取りスナップショットで
	// 行う。別々に読むと、その間の付け外しで Total と Items の本数が食い違いうる。
	tx, err := s.sql.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.TagSummary{}, err
	}
	defer func() { _ = tx.Rollback() }()

	keys, err := registeredContentKeysForVideoIDs(ctx, tx, videoIDs)
	if err != nil {
		return domain.TagSummary{}, err
	}
	summary := domain.TagSummary{Total: len(keys), Items: []domain.TagSummaryItem{}}
	if len(keys) == 0 {
		return summary, nil
	}

	encoded, err := json.Marshal(keys)
	if err != nil {
		return domain.TagSummary{}, fmt.Errorf("content_key を組み立てられません: %w", err)
	}

	// count はどちらかの出所で、manualCount は手で付けた分だけで数える（017 の
	// data-model.md §4）。1本の動画に同じタグが複数のフォルダ名（元の名前と
	// シノニムなど）から当たりうるので、content_key の重複を除いて数える。
	rows, err := tx.QueryContext(ctx, `
		with selected(content_key) as (select value from json_each(?)),
		tagged(content_key, tag_id, manual) as (
			select vt.content_key, vt.tag_id, 1 from video_tags vt
			 where vt.content_key in (select content_key from selected)
			union all
			select v.content_key, folder_tn.tag_id, 0 from videos v
			  join video_folder_names vfn on vfn.video_id = v.id
			  join tag_names folder_tn on folder_tn.name = vfn.name
			 where v.content_key in (select content_key from selected)
		)
		select t.tag_id, tn.name, count(distinct t.content_key),
		       count(distinct case when t.manual = 1 then t.content_key end)
		  from tagged t
		  join tag_names tn on tn.tag_id = t.tag_id and tn.canonical = 1
		 group by t.tag_id`, string(encoded),
	)
	if err != nil {
		return domain.TagSummary{}, fmt.Errorf("タグの要約を読み出せません: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var item domain.TagSummaryItem
		if err := rows.Scan(&item.Tag.ID, &item.Tag.Name, &item.Count, &item.ManualCount); err != nil {
			return domain.TagSummary{}, fmt.Errorf("タグの要約を読み出せません: %w", err)
		}
		summary.Items = append(summary.Items, item)
	}
	if err := rows.Err(); err != nil {
		return domain.TagSummary{}, fmt.Errorf("タグの要約を読み出せません: %w", err)
	}
	if err := rows.Close(); err != nil {
		return domain.TagSummary{}, fmt.Errorf("タグの要約を読み出せません: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return domain.TagSummary{}, fmt.Errorf("タグの要約を読み出せません: %w", err)
	}
	domain.SortTagSummaryItems(summary.Items)
	return summary, nil
}

// TagsByContentKeys は content_key の集合からそれぞれのタグ（元の名前、
// domain.CompareNatural の順、同じなら id）を出所つきでまとめて引く。手で
// 付けた分とフォルダ名から付いている分の和で、同じタグが両方から付けば1件に
// まとめて出所を両方持つ（017 の data-model.md §4）。タグの無い content_key は
// 結果に現れない。PlaybackStore.ProgressByContentKeys と同じ形で、
// internal/httpapi が progressFor と同じ位置から一覧の項目にタグを足すために
// 使う（Plan の Structural Decisions 5・14）。
func (s *TagStore) TagsByContentKeys(ctx context.Context, contentKeys []string) (map[string][]domain.VideoTag, error) {
	if len(contentKeys) == 0 {
		return map[string][]domain.VideoTag{}, nil
	}
	encoded, err := json.Marshal(contentKeys)
	if err != nil {
		return nil, fmt.Errorf("content_key を組み立てられません: %w", err)
	}

	rows, err := s.sql.QueryContext(ctx, `
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
		select t.content_key, t.tag_id, tn.name, max(t.manual), max(t.from_folder)
		  from tagged t
		  join tag_names tn on tn.tag_id = t.tag_id and tn.canonical = 1
		 group by t.content_key, t.tag_id`, string(encoded),
	)
	if err != nil {
		return nil, fmt.Errorf("項目のタグを読み出せません: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make(map[string][]domain.VideoTag, len(contentKeys))
	for rows.Next() {
		var key string
		var tag domain.VideoTag
		if err := rows.Scan(&key, &tag.ID, &tag.Name, &tag.Manual, &tag.FromFolder); err != nil {
			return nil, fmt.Errorf("項目のタグを読み出せません: %w", err)
		}
		out[key] = append(out[key], tag)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("項目のタグを読み出せません: %w", err)
	}
	for key := range out {
		domain.SortVideoTags(out[key])
	}
	return out, nil
}

// ListTags はタグを名前の自然順ですべて返す。本数 0 のタグも返す
// （data-model.md §5）。本数は video_tags を videos.content_key と結び、いま
// ライブラリにある動画だけを数える。3つの問い合わせ（タグ、シノニム、本数）に
// 分けるのは、タグごとに引き直すと N+1 になるためである。
func (s *TagStore) ListTags(ctx context.Context) ([]domain.Tag, error) {
	tags, index, err := listCanonicalTags(ctx, s.sql)
	if err != nil {
		return nil, err
	}
	if err := addSynonymsToTags(ctx, s.sql, tags, index); err != nil {
		return nil, err
	}
	if err := addVideoCountsToTags(ctx, s.sql, tags, index); err != nil {
		return nil, err
	}
	for i := range tags {
		domain.SortTagNames(tags[i].Synonyms)
	}
	domain.SortTags(tags)
	return tags, nil
}

// listCanonicalTags はタグごとの元の名前を読み、id から一覧内の位置への
// 対応も返す。
func listCanonicalTags(ctx context.Context, q queryExecer) ([]domain.Tag, map[int64]int, error) {
	rows, err := q.QueryContext(ctx, `
		select t.id, tn.name from tags t
		  join tag_names tn on tn.tag_id = t.id and tn.canonical = 1`)
	if err != nil {
		return nil, nil, fmt.Errorf("タグを読み出せません: %w", err)
	}
	defer func() { _ = rows.Close() }()

	tags := []domain.Tag{}
	index := make(map[int64]int)
	for rows.Next() {
		var tag domain.Tag
		if err := rows.Scan(&tag.ID, &tag.Name); err != nil {
			return nil, nil, fmt.Errorf("タグを読み出せません: %w", err)
		}
		tag.Synonyms = []string{}
		index[tag.ID] = len(tags)
		tags = append(tags, tag)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("タグを読み出せません: %w", err)
	}
	return tags, index, nil
}

// addSynonymsToTags は tags[index[tag_id]].Synonyms にシノニムを足す。
func addSynonymsToTags(ctx context.Context, q queryExecer, tags []domain.Tag, index map[int64]int) error {
	rows, err := q.QueryContext(ctx, `select tag_id, name from tag_names where canonical = 0`)
	if err != nil {
		return fmt.Errorf("シノニムを読み出せません: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var tagID int64
		var name string
		if err := rows.Scan(&tagID, &name); err != nil {
			return fmt.Errorf("シノニムを読み出せません: %w", err)
		}
		if i, ok := index[tagID]; ok {
			tags[i].Synonyms = append(tags[i].Synonyms, name)
		}
	}
	return rows.Err()
}

// addVideoCountsToTags は tags[index[tag_id]].VideoCount に、いまライブラリに
// ある動画だけを数えた本数を足す。手で付けた分とフォルダ名から付いている分の
// どちらかで付いていれば1本と数える（017 の data-model.md §4）。
func addVideoCountsToTags(ctx context.Context, q queryExecer, tags []domain.Tag, index map[int64]int) error {
	//nolint:gosec // registeredVideoCondition は定型SQLだけを返す。
	query := `select tag_id, count(*) from (` + taggedVideosSQL("") + `) group by tag_id`
	rows, err := q.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("タグの本数を数えられません: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var tagID int64
		var count int
		if err := rows.Scan(&tagID, &count); err != nil {
			return fmt.Errorf("タグの本数を数えられません: %w", err)
		}
		if i, ok := index[tagID]; ok {
			tags[i].VideoCount = count
		}
	}
	return rows.Err()
}

// RefreshSearchKeys は search_version が現在の版より小さいタグ名の行の
// search_key を作り直し、作り直した件数を返す（data-model.md §7）。起動時、
// LibraryStore.RefreshSearchKeys の隣で呼ぶ。所在の鍵と違ってメディア
// フォルダに依らないので、folderMu は取らない。
func (s *TagStore) RefreshSearchKeys(ctx context.Context) (int, error) {
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	names, err := staleTagNames(ctx, tx)
	if err != nil {
		return 0, err
	}
	for _, name := range names {
		if _, err := tx.ExecContext(ctx,
			`update tag_names set search_key = ?, search_version = ? where name = ?`,
			domain.FoldForMatch(name), domain.SearchKeyVersion, name,
		); err != nil {
			return 0, fmt.Errorf("タグの照合用の鍵を保存できません (%s): %w", name, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("タグの照合用の鍵を保存できません: %w", err)
	}
	return len(names), nil
}

func staleTagNames(ctx context.Context, q queryExecer) ([]string, error) {
	rows, err := q.QueryContext(ctx, `select name from tag_names where search_version < ?`, domain.SearchKeyVersion)
	if err != nil {
		return nil, fmt.Errorf("タグ名を読み出せません: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("タグ名を読み出せません: %w", err)
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

// nameLookup は tag_names を name で引いた結果である。
type nameLookup struct {
	tagID int64
	// canonicalName はその名前を持つタグの元の名前（表示名）。name 自身が
	// シノニムのときは、name とは異なる。
	canonicalName string
	isCanonical   bool
}

// lookupTagName は名前からタグを引く（data-model.md §3）。元の名前でも
// シノニムでも同じタグに着く。無ければ found = false を返す。
func lookupTagName(ctx context.Context, q tagTx, name string) (nameLookup, bool, error) {
	var lookup nameLookup
	var canonicalInt int
	err := q.QueryRowContext(ctx, `
		select tn.tag_id, tn.canonical, canon.name
		  from tag_names tn
		  join tag_names canon on canon.tag_id = tn.tag_id and canon.canonical = 1
		 where tn.name = ?`, name,
	).Scan(&lookup.tagID, &canonicalInt, &lookup.canonicalName)
	if errors.Is(err, sql.ErrNoRows) {
		return nameLookup{}, false, nil
	}
	if err != nil {
		return nameLookup{}, false, fmt.Errorf("タグを名前で引けません (%s): %w", name, err)
	}
	lookup.isCanonical = canonicalInt != 0
	return lookup, true, nil
}

// canonicalNameByTagID はタグ id の元の名前を返す。無ければ
// domain.ErrTagNotFound を返す。
func canonicalNameByTagID(ctx context.Context, q rowQueryer, id int64) (string, error) {
	var name string
	err := q.QueryRowContext(ctx, `select name from tag_names where tag_id = ? and canonical = 1`, id).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return "", domain.ErrTagNotFound
	}
	if err != nil {
		return "", fmt.Errorf("タグの名前を読み出せません (id=%d): %w", id, err)
	}
	return name, nil
}

// tagByID はタグ1件を、シノニムと本数を添えて返す。無ければ
// domain.ErrTagNotFound を返す。
func tagByID(ctx context.Context, q tagTx, id int64) (domain.Tag, error) {
	name, err := canonicalNameByTagID(ctx, q, id)
	if err != nil {
		return domain.Tag{}, err
	}
	count, err := videoCountByTagID(ctx, q, id)
	if err != nil {
		return domain.Tag{}, err
	}
	synonyms, err := synonymsByTagID(ctx, q, id)
	if err != nil {
		return domain.Tag{}, err
	}
	return domain.Tag{ID: id, Name: name, Synonyms: synonyms, VideoCount: count}, nil
}

// videoCountByTagID はいまライブラリにある動画のうち id が付いている本数を
// 数える（data-model.md §5）。フォルダ名から付いている分も含む（017 の
// data-model.md §4）。
func videoCountByTagID(ctx context.Context, q rowQueryer, id int64) (int, error) {
	//nolint:gosec // registeredVideoCondition は定型SQLだけを返す。
	query := `select count(*) from (` + taggedVideosSQL(` and tag_id = ?`) + `)`
	var count int
	if err := q.QueryRowContext(ctx, query, id, id).Scan(&count); err != nil {
		return 0, fmt.Errorf("タグの本数を数えられません (id=%d): %w", id, err)
	}
	return count, nil
}

// synonymsByTagID はタグ id のシノニムを名前の自然順で返す。
func synonymsByTagID(ctx context.Context, q queryExecer, id int64) ([]string, error) {
	rows, err := q.QueryContext(ctx, `select name from tag_names where tag_id = ? and canonical = 0`, id)
	if err != nil {
		return nil, fmt.Errorf("シノニムを読み出せません (id=%d): %w", id, err)
	}
	defer func() { _ = rows.Close() }()
	names := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("シノニムを読み出せません (id=%d): %w", id, err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("シノニムを読み出せません (id=%d): %w", id, err)
	}
	domain.SortTagNames(names)
	return names, nil
}

// insertTagName は tag_names に1行足し、照合用の鍵を同じトランザクションで
// 書く（data-model.md §7）。
func insertTagName(ctx context.Context, tx *sql.Tx, name string, tagID int64, canonical bool) error {
	if _, err := tx.ExecContext(ctx, `
		insert into tag_names (name, tag_id, canonical, search_key, search_version)
		values (?, ?, ?, ?, ?)`,
		name, tagID, boolToInt(canonical), domain.FoldForMatch(name), domain.SearchKeyVersion,
	); err != nil {
		return fmt.Errorf("タグ名を保存できません (%s): %w", name, err)
	}
	return nil
}

// existingTagIDs は ids のうち tags テーブルにあるものと無いものを、それぞれ
// ids の並びのまま返す（data-model.md §6）。LibraryStore がタグでの絞り込み・
// 「すべて選択」で使う（Structural Decisions 13：複数の型が読む問い合わせの共有）。
// ids の重複は1つにまとめる。ids が空なら問い合わせずに両方空を返す（空の
// in () は誤りになる）。
func existingTagIDs(ctx context.Context, q queryExecer, ids []int64) (existing, missing []int64, err error) {
	if len(ids) == 0 {
		return nil, nil, nil
	}
	seen := make(map[int64]bool, len(ids))
	deduped := make([]int64, 0, len(ids))
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		deduped = append(deduped, id)
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(deduped)), ",")
	args := make([]any, 0, len(deduped))
	for _, id := range deduped {
		args = append(args, id)
	}

	//nolint:gosec // 組み立てるのはプレースホルダの数だけで、値は引数で渡す。
	rows, err := q.QueryContext(ctx, `select id from tags where id in (`+placeholders+`)`, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("タグの存在を確かめられません: %w", err)
	}
	defer func() { _ = rows.Close() }()

	found := make(map[int64]bool, len(deduped))
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, nil, fmt.Errorf("タグの存在を確かめられません: %w", err)
		}
		found[id] = true
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("タグの存在を確かめられません: %w", err)
	}

	for _, id := range deduped {
		if found[id] {
			existing = append(existing, id)
		} else {
			missing = append(missing, id)
		}
	}
	return existing, missing, nil
}
