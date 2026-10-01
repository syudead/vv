package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/syudead/vv/internal/domain"
)

// tagTx は1つのトランザクションの中で名前とタグの行を読み書きするための
// 共通部分である。*sql.Tx はこれを満たす。
type tagTx interface {
	queryExecer
	rowQueryer
}

// nameLookup は tag_names を name で引いた結果である。
type nameLookup struct {
	tagID int64
	// canonicalName はその名前を持つタグの元の名前（表示名）。name 自身が
	// シノニムのときは、name とは異なる。
	canonicalName string
	isCanonical   bool
	// tentative はその名前を持つタグが仮のタグであること（specs/031-tentative-tags/data-model.md §5）。
	tentative bool
}

// lookupTagName は名前からタグを引く（data-model.md §3）。元の名前でも
// シノニムでも同じタグに着く。無ければ found = false を返す。
func lookupTagName(ctx context.Context, q tagTx, name string) (nameLookup, bool, error) {
	var lookup nameLookup
	var canonicalInt int
	err := q.QueryRowContext(ctx, `
		select tn.tag_id, tn.canonical, canon.name, t.tentative
		  from tag_names tn
		  join tag_names canon on canon.tag_id = tn.tag_id and canon.canonical = 1
		  join tags t on t.id = tn.tag_id
		 where tn.name = ?`, name,
	).Scan(&lookup.tagID, &canonicalInt, &lookup.canonicalName, &lookup.tentative)
	if errors.Is(err, sql.ErrNoRows) {
		return nameLookup{}, false, nil
	}
	if err != nil {
		return nameLookup{}, false, fmt.Errorf("cannot look up the tag by name (%s): %w", name, err)
	}
	lookup.isCanonical = canonicalInt != 0
	return lookup, true, nil
}

// ref は引いた名前のタグを TagRef にする（名前は元の名前）。
func (l nameLookup) ref() domain.TagRef {
	return domain.TagRef{ID: l.tagID, Name: l.canonicalName, Tentative: l.tentative}
}

// tagRefByID はタグ id の元の名前と仮かどうかを返す。無ければ domain.ErrTagNotFound を返す。
func tagRefByID(ctx context.Context, q rowQueryer, id int64) (domain.TagRef, error) {
	ref := domain.TagRef{ID: id}
	err := q.QueryRowContext(ctx, `
		select tn.name, t.tentative
		  from tags t
		  join tag_names tn on tn.tag_id = t.id and tn.canonical = 1
		 where t.id = ?`, id).Scan(&ref.Name, &ref.Tentative)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.TagRef{}, domain.ErrTagNotFound
	}
	if err != nil {
		return domain.TagRef{}, fmt.Errorf("cannot read the tag (id=%d): %w", id, err)
	}
	return ref, nil
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
		return "", fmt.Errorf("cannot read the tag names (id=%d): %w", id, err)
	}
	return name, nil
}

// tagByID はタグ1件を、シノニムと本数を添えて返す。無ければ
// domain.ErrTagNotFound を返す。
func tagByID(ctx context.Context, q tagTx, id int64) (domain.Tag, error) {
	ref, err := tagRefByID(ctx, q, id)
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
	return domain.Tag{ID: id, Name: ref.Name, Synonyms: synonyms, VideoCount: count, Tentative: ref.Tentative}, nil
}

// videoCountByTagID はいまライブラリにある動画のうち id が付いている本数を
// 数える（data-model.md §5）。フォルダ名から付いている分も含む（017 の
// data-model.md §4）。
func videoCountByTagID(ctx context.Context, q rowQueryer, id int64) (int, error) {
	query := `select count(*) from (` + taggedVideosSQL(` and tag_id = ?`) + `)`
	var count int
	if err := q.QueryRowContext(ctx, query, id, id).Scan(&count); err != nil {
		return 0, fmt.Errorf("cannot count tag uses (id=%d): %w", id, err)
	}
	return count, nil
}

// synonymsByTagID はタグ id のシノニムを名前の自然順で返す。
func synonymsByTagID(ctx context.Context, q queryExecer, id int64) ([]string, error) {
	rows, err := q.QueryContext(ctx, `select name from tag_names where tag_id = ? and canonical = 0`, id)
	if err != nil {
		return nil, fmt.Errorf("cannot read synonyms (id=%d): %w", id, err)
	}
	defer func() { _ = rows.Close() }()
	names := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("cannot read synonyms (id=%d): %w", id, err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read synonyms (id=%d): %w", id, err)
	}
	domain.SortTagNames(names)
	return names, nil
}

// insertTagName は tag_names に1行足し、照合用の鍵を同じトランザクションで
// 書く（data-model.md §7）。同じ取引でその名前を却下した名前から外す
// （specs/031-tentative-tags/research.md R-3）。
func insertTagName(ctx context.Context, tx *sql.Tx, name string, tagID int64, canonical bool) error {
	if _, err := tx.ExecContext(ctx, `
		insert into tag_names (name, tag_id, canonical, search_key, search_version)
		values (?, ?, ?, ?, ?)`,
		name, tagID, boolToInt(canonical), domain.FoldForMatch(name), domain.SearchKeyVersion,
	); err != nil {
		return fmt.Errorf("cannot save the tag name (%s): %w", name, err)
	}
	return forgetRejectedName(ctx, tx, name)
}

// forgetRejectedName は name を却下した名前から外す。無ければ何もしない。
func forgetRejectedName(ctx context.Context, tx *sql.Tx, name string) error {
	if _, err := tx.ExecContext(ctx, `delete from rejected_tag_names where name = ?`, name); err != nil {
		return fmt.Errorf("cannot forget the rejected tag name (%s): %w", name, err)
	}
	return nil
}

// confirmTagInTx はタグ id を確定したタグにする（tentative = 0）。既に 0 なら何も変えない
// （specs/031-tentative-tags/research.md R-4）。
func confirmTagInTx(ctx context.Context, tx *sql.Tx, id int64) error {
	if _, err := tx.ExecContext(ctx, `update tags set tentative = 0 where id = ? and tentative = 1`, id); err != nil {
		return fmt.Errorf("cannot confirm the tag (id=%d): %w", id, err)
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

	rows, err := q.QueryContext(ctx, `select id from tags where id in (`+placeholders+`)`, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot check whether the tag exists: %w", err)
	}
	defer func() { _ = rows.Close() }()

	found := make(map[int64]bool, len(deduped))
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, nil, fmt.Errorf("cannot check whether the tag exists: %w", err)
		}
		found[id] = true
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("cannot check whether the tag exists: %w", err)
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
