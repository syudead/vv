package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// タグそのものの保存（specs/014-video-tags/data-model.md §1〜§4・§7）。
//
// TagStore は PlaybackStore と同じく、共有する SQLite 接続だけを持ち、
// ライブラリ索引の役割の型（LibraryStore）にも通知の発行にも依存しない。
// タグの変更は副作用（生成物の削除・ワーカーの起床・/api/events）を持たない
// ので、トランザクションのコミットだけで済む。

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
		return domain.Tag{}, &domain.TagNameConflict{Tag: lookup.ref()}
	}

	id, err := insertTag(ctx, tx, normalized, false)
	if err != nil {
		return domain.Tag{}, err
	}
	// 作ったばかりのタグでも、同じ名前の祖先フォルダの下の動画にはもう付いて
	// いる（017 の data-model.md §4）ので、本数も数える。作った時刻も載せるため、
	// 手で組み立てずに同じ取引で読み直す（specs/036-tag-admin-scale/data-model.md §2）。
	tag, err := tagByID(ctx, tx, id)
	if err != nil {
		return domain.Tag{}, err
	}

	if err := tx.Commit(); err != nil {
		return domain.Tag{}, fmt.Errorf("cannot create the tag: %w", err)
	}
	return tag, nil
}

// RenameTag は id の元の名前を書き換える。今と同じ名前なら何も変えずに今の
// 状態を返す。名前が変わるときは、同じ取引でタグを確定し、新しい名前を却下した名前から
// 外す（specs/031-tentative-tags/data-model.md §3）。新しい名前が既にあれば（自分のシノニムでも）
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
			return domain.Tag{}, &domain.TagNameConflict{Tag: lookup.ref()}
		}
		if _, err := tx.ExecContext(ctx, `
			update tag_names set name = ?, search_key = ?, search_version = ?
			 where tag_id = ? and canonical = 1`,
			normalized, domain.FoldForMatch(normalized), domain.SearchKeyVersion, id,
		); err != nil {
			return domain.Tag{}, fmt.Errorf("cannot rename the tag (id=%d): %w", id, err)
		}
		if err := forgetRejectedName(ctx, tx, normalized); err != nil {
			return domain.Tag{}, err
		}
		if err := confirmTagInTx(ctx, tx, id); err != nil {
			return domain.Tag{}, err
		}
	}

	tag, err := tagByID(ctx, tx, id)
	if err != nil {
		return domain.Tag{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Tag{}, fmt.Errorf("cannot rename the tag: %w", err)
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
		return fmt.Errorf("cannot delete the tag (id=%d): %w", id, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("cannot delete the tag: %w", err)
	}
	return nil
}

// MergeTags は sourceIDs のタグを targetID へ 1 つの取引で統合する
// （specs/036-tag-admin-scale/data-model.md §2、specs/014-video-tags/data-model.md §4）。統合元の付与は
// insert or ignore で統合先へ写り、統合元の元の名前とシノニムはすべて統合先のシノニムになり、
// 統合元は一覧から消え、統合先は確定したタグになる。統合先が無ければ domain.ErrTagNotFound を返す。
// sourceIDs の重複は 1 つとして扱い、無い id は飛ばして NotFoundIDs に入れる。統合先の id は
// 統合元から除く。統合後の統合先は最後に 1 回だけ読む。取引が失敗したら何も変えない。
func (s *TagStore) MergeTags(ctx context.Context, targetID int64, sourceIDs []int64) (domain.TagMergeOutcome, error) {
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.TagMergeOutcome{}, err
	}
	defer func() { _ = tx.Rollback() }()

	outcome, err := mergeTagsInTx(ctx, tx, targetID, sourceIDs)
	if err != nil {
		return domain.TagMergeOutcome{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.TagMergeOutcome{}, fmt.Errorf("cannot merge the tags: %w", err)
	}
	return outcome, nil
}

// mergeTagsInTx は MergeTags の取引の中身である。統合元の有無と統合先との重なりをここで確かめ、
// 残りを mergeTagsInto に渡し、統合先を tagByID で 1 回だけ読む。
func mergeTagsInTx(ctx context.Context, tx tagTx, targetID int64, sourceIDs []int64) (domain.TagMergeOutcome, error) {
	if _, err := canonicalNameByTagID(ctx, tx, targetID); err != nil {
		return domain.TagMergeOutcome{}, err
	}
	unique := uniqueTagIDs(sourceIDs)
	existing, err := tentativeByTagID(ctx, tx, unique)
	if err != nil {
		return domain.TagMergeOutcome{}, err
	}
	notFound := []int64{}
	merged := make([]int64, 0, len(unique))
	for _, id := range unique {
		switch _, found := existing[id]; {
		case !found:
			notFound = append(notFound, id)
		case id != targetID:
			merged = append(merged, id)
		}
	}
	if len(merged) > 0 {
		if err := mergeTagsInto(ctx, tx, targetID, merged); err != nil {
			return domain.TagMergeOutcome{}, err
		}
	}
	tag, err := tagByID(ctx, tx, targetID)
	if err != nil {
		return domain.TagMergeOutcome{}, err
	}
	return domain.TagMergeOutcome{Tag: tag, NotFoundIDs: notFound}, nil
}

// mergeTagsInto は同じ取引の中で sourceIDs のタグを targetID へ統合し、統合先を確定したタグにする
// （specs/031-tentative-tags/data-model.md §3）。統合元の数によらず決まった数の文で行い、統合後の
// タグは組み立てない（20,000 個の統合で書きの取引を長く握らないため。036 の data-model.md §2）。
// 統合元がすべて今あり、統合先を含まないことは呼び手が先に確かめて渡す。
func mergeTagsInto(ctx context.Context, tx queryExecer, targetID int64, sourceIDs []int64) error {
	encoded, err := json.Marshal(sourceIDs)
	if err != nil {
		return fmt.Errorf("cannot build tag ids: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		insert or ignore into video_tags (content_key, tag_id, created_at)
		select content_key, ?, created_at from video_tags
		 where tag_id in (select value from json_each(?))`,
		targetID, string(encoded),
	); err != nil {
		return fmt.Errorf("cannot copy assignments to the merge target (target=%d): %w", targetID, err)
	}
	if _, err := tx.ExecContext(ctx, `
		update tag_names set tag_id = ?, canonical = 0
		 where tag_id in (select value from json_each(?))`,
		targetID, string(encoded),
	); err != nil {
		return fmt.Errorf("cannot move tag names to the merge target (target=%d): %w", targetID, err)
	}
	if _, err := tx.ExecContext(ctx,
		`delete from tags where id in (select value from json_each(?))`, string(encoded),
	); err != nil {
		return fmt.Errorf("cannot delete the merged tags (target=%d): %w", targetID, err)
	}
	return confirmTagInTx(ctx, tx, targetID)
}

// findOrCreateTag は整えた名前 normalized をシノニムを含めて引き、無ければ同じ
// トランザクションの中で確定したタグとして作る。作ったかどうかも返す。名前でタグを付ける
// 操作と、グループをタグに変える操作（folder_groups.go）が共有する。
func findOrCreateTag(ctx context.Context, tx *sql.Tx, normalized string) (domain.TagRef, bool, error) {
	lookup, found, err := lookupTagName(ctx, tx, normalized)
	if err != nil {
		return domain.TagRef{}, false, err
	}
	if found {
		return lookup.ref(), false, nil
	}
	id, err := insertTag(ctx, tx, normalized, false)
	if err != nil {
		return domain.TagRef{}, false, err
	}
	return domain.TagRef{ID: id, Name: normalized}, true, nil
}

// insertTag は整えた名前 normalized を元の名前に持つタグを作り、その id を返す。
// tentative が真なら仮のタグとして作る（specs/031-tentative-tags/data-model.md §3）。
// 名前がまだ無いことは呼び出し側が確かめる。
func insertTag(ctx context.Context, tx *sql.Tx, normalized string, tentative bool) (int64, error) {
	res, err := tx.ExecContext(ctx, `insert into tags (created_at, tentative) values (?, ?)`,
		time.Now().Unix(), boolToInt(tentative))
	if err != nil {
		return 0, fmt.Errorf("cannot create the tag: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("cannot create the tag: %w", err)
	}
	if err := insertTagName(ctx, tx, normalized, id, true); err != nil {
		return 0, err
	}
	return id, nil
}
