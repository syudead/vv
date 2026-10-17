package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// タグの種類の保存。種類の作成と名前の変更は、タグの変更と
// 同じく副作用（生成物の削除・ワーカーの起床）を持たず、どのスマートフォルダの一致も変えないので、
// 取引のコミットだけで済む。種類を消すと、その種類のタグは tags.kind_id の on delete set null で
// 種類なしになり、その種類を名指す条件は一致を変えるので、db.commitMatchChange で確定して
// domain.SmartFoldersChanged を発行する。

// tagKindColumns はタグの行（別名 tg）に結んだ種類（別名 tk）の id と名前を読む列である。
// 問い合わせには tagKindJoin を添え、読んだ値は scannedTagKind で受ける。
const (
	tagKindColumns = `tk.id, tk.name`
	tagKindJoin    = ` left join tag_kinds tk on tk.id = tg.kind_id`
)

// scannedTagKind は tagKindColumns の 2 列を受ける。種類の無いタグでは両方 null になる。
type scannedTagKind struct {
	id   sql.NullInt64
	name sql.NullString
}

// targets は Scan に渡す 2 つの受け先を返す。
func (k *scannedTagKind) targets() []any {
	return []any{&k.id, &k.name}
}

// ref は読んだ種類を返す。種類が無ければ nil。
func (k scannedTagKind) ref() *domain.TagKindRef {
	if !k.id.Valid {
		return nil
	}
	return &domain.TagKindRef{ID: k.id.Int64, Name: k.name.String}
}

// ListTagKinds は種類をすべて id の順に返す。TagCount は仮のタグも数える。
func (s *TagStore) ListTagKinds(ctx context.Context) ([]domain.TagKind, error) {
	rows, err := s.read.QueryContext(ctx, `
		select k.id, k.name, k.created_at, (select count(*) from tags t where t.kind_id = k.id)
		  from tag_kinds k
		 order by k.id`)
	if err != nil {
		return nil, fmt.Errorf("cannot read tag kinds: %w", err)
	}
	defer func() { _ = rows.Close() }()

	kinds := []domain.TagKind{}
	for rows.Next() {
		var kind domain.TagKind
		var createdAt int64
		if err := rows.Scan(&kind.ID, &kind.Name, &createdAt, &kind.TagCount); err != nil {
			return nil, fmt.Errorf("cannot read tag kinds: %w", err)
		}
		kind.CreatedAt = time.Unix(createdAt, 0)
		kinds = append(kinds, kind)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read tag kinds: %w", err)
	}
	return kinds, nil
}

// CreateTagKind は新しい種類を作る。名前は domain.NormalizeTagName で整え、規則に合わなければ
// その誤り（domain.ErrInvalidTagName）を返す。別の種類が同じ名前なら
// *domain.TagKindNameConflict を返し、何も書かない。
func (s *TagStore) CreateTagKind(ctx context.Context, name string) (domain.TagKind, error) {
	normalized, err := domain.NormalizeTagName(name)
	if err != nil {
		return domain.TagKind{}, err
	}

	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.TagKind{}, err
	}
	defer func() { _ = tx.Rollback() }()

	if err := checkTagKindNameFree(ctx, tx, normalized); err != nil {
		return domain.TagKind{}, err
	}
	res, err := tx.ExecContext(ctx, `insert into tag_kinds (name, created_at) values (?, ?)`,
		normalized, time.Now().Unix())
	if err != nil {
		return domain.TagKind{}, fmt.Errorf("cannot create the tag kind: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return domain.TagKind{}, fmt.Errorf("cannot create the tag kind: %w", err)
	}
	kind, err := tagKindByID(ctx, tx, id)
	if err != nil {
		return domain.TagKind{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.TagKind{}, fmt.Errorf("cannot create the tag kind: %w", err)
	}
	return kind, nil
}

// RenameTagKind は種類 id の名前を書き換える。今と同じ名前なら何も変えずに今の状態を返す。
// 別の種類が同じ名前なら *domain.TagKindNameConflict、id が無ければ domain.ErrTagKindNotFound を
// 返し、何も書かない。
func (s *TagStore) RenameTagKind(ctx context.Context, id int64, name string) (domain.TagKind, error) {
	normalized, err := domain.NormalizeTagName(name)
	if err != nil {
		return domain.TagKind{}, err
	}

	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.TagKind{}, err
	}
	defer func() { _ = tx.Rollback() }()

	current, err := tagKindByID(ctx, tx, id)
	if err != nil {
		return domain.TagKind{}, err
	}
	if current.Name == normalized {
		return current, nil
	}
	if err := checkTagKindNameFree(ctx, tx, normalized); err != nil {
		return domain.TagKind{}, err
	}
	if _, err := tx.ExecContext(ctx, `update tag_kinds set name = ? where id = ?`, normalized, id); err != nil {
		return domain.TagKind{}, fmt.Errorf("cannot rename the tag kind (id=%d): %w", id, err)
	}
	if err := tx.Commit(); err != nil {
		return domain.TagKind{}, fmt.Errorf("cannot rename the tag kind: %w", err)
	}
	current.Name = normalized
	return current, nil
}

// DeleteTagKind は種類を消す。その種類のタグは残り、種類なしになる（on delete set null）。
// その種類を名指すスマートフォルダの条件は書き換えず、欠けた参照になる。id が無ければ domain.ErrTagKindNotFound を
// 返し、何も書かない。
func (s *TagStore) DeleteTagKind(ctx context.Context, id int64) error {
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `delete from tag_kinds where id = ?`, id)
	if err != nil {
		return fmt.Errorf("cannot delete the tag kind (id=%d): %w", id, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("cannot delete the tag kind (id=%d): %w", id, err)
	}
	if affected == 0 {
		return domain.ErrTagKindNotFound
	}
	if err := s.db.commitMatchChange(ctx, tx); err != nil {
		return fmt.Errorf("cannot delete the tag kind (id=%d): %w", id, err)
	}
	return nil
}

// checkTagKindNameFree は name を持つ種類が無いことを確かめる。あれば
// *domain.TagKindNameConflict を返す。今と同じ名前への変更は RenameTagKind が先に返すので、
// 見つかった種類が自分自身であることは無い。
func checkTagKindNameFree(ctx context.Context, q rowQueryer, name string) error {
	var owner domain.TagKindRef
	err := q.QueryRowContext(ctx, `select id, name from tag_kinds where name = ?`, name).Scan(&owner.ID, &owner.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot look up the tag kind by name (%s): %w", name, err)
	}
	return &domain.TagKindNameConflict{Kind: owner}
}

// tagKindByID は種類 1 件をタグの数を添えて返す。無ければ domain.ErrTagKindNotFound を返す。
func tagKindByID(ctx context.Context, q rowQueryer, id int64) (domain.TagKind, error) {
	kind := domain.TagKind{ID: id}
	var createdAt int64
	err := q.QueryRowContext(ctx, `
		select name, created_at, (select count(*) from tags where kind_id = ?)
		  from tag_kinds where id = ?`, id, id).Scan(&kind.Name, &createdAt, &kind.TagCount)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.TagKind{}, domain.ErrTagKindNotFound
	}
	if err != nil {
		return domain.TagKind{}, fmt.Errorf("cannot read the tag kind (id=%d): %w", id, err)
	}
	kind.CreatedAt = time.Unix(createdAt, 0)
	return kind, nil
}
