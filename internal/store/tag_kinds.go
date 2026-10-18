package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// Kind creation and renaming do not affect matches; deletion clears tag kinds and invalidates smart folders referencing the deleted kind.

const (
	tagKindColumns = `tk.id, tk.name`
	tagKindJoin    = ` left join tag_kinds tk on tk.id = tg.kind_id`
)

// Both columns are NULL for an unclassified tag.
type scannedTagKind struct {
	id   sql.NullInt64
	name sql.NullString
}

func (k *scannedTagKind) targets() []any {
	return []any{&k.id, &k.name}
}

func (k scannedTagKind) ref() *domain.TagKindRef {
	if !k.id.Valid {
		return nil
	}
	return &domain.TagKindRef{ID: k.id.Int64, Name: k.name.String}
}

// ListTagKinds includes tentative tags in TagCount.
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

// CreateTagKind normalizes names and rejects conflicts without writing.
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
	if err := s.db.commitEvents(ctx, tx); err != nil {
		return domain.TagKind{}, fmt.Errorf("cannot create the tag kind: %w", err)
	}
	return kind, nil
}

// RenameTagKind is a no-op for the current name and rejects missing kinds or name conflicts.
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
	if err := s.db.commitEvents(ctx, tx); err != nil {
		return domain.TagKind{}, fmt.Errorf("cannot rename the tag kind: %w", err)
	}
	current.Name = normalized
	return current, nil
}

// DeleteTagKind preserves tags as unclassified and leaves smart-folder references missing rather than rewriting rules.
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

// The caller handles unchanged names first, so any matching kind is a conflict.
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
