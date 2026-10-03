package store

import (
	"context"
	"database/sql"
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

// MergeTag は sourceID のタグを targetID へ統合する（specs/014-video-tags/data-model.md §4）。source の付与は insert or ignore で target へ写り、
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
		return domain.Tag{}, fmt.Errorf("cannot merge the tags: %w", err)
	}
	return tag, nil
}

// mergeTagInto は同じトランザクションの中で source を target へ統合する。
// target と source が同じ id なら何もせず、今の target をそのまま返す。統合したときは
// target を確定したタグにする（specs/031-tentative-tags/data-model.md §3）。
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
		return domain.Tag{}, fmt.Errorf("cannot copy assignments to the merge target (source=%d target=%d): %w", sourceID, targetID, err)
	}

	if _, err := tx.ExecContext(ctx, `update tag_names set tag_id = ?, canonical = 0 where tag_id = ?`,
		targetID, sourceID,
	); err != nil {
		return domain.Tag{}, fmt.Errorf("cannot move tag names to the merge target (source=%d target=%d): %w", sourceID, targetID, err)
	}

	if _, err := tx.ExecContext(ctx, `delete from tags where id = ?`, sourceID); err != nil {
		return domain.Tag{}, fmt.Errorf("cannot delete the merged tag (id=%d): %w", sourceID, err)
	}
	if err := confirmTagInTx(ctx, tx, targetID); err != nil {
		return domain.Tag{}, err
	}

	return tagByID(ctx, tx, targetID)
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
