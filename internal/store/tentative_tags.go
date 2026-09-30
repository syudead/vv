package store

import (
	"context"
	"fmt"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// 仮のタグの確定・却下と、却下した名前（specs/031-tentative-tags/data-model.md §3・§5）。
// 仮のタグを作るのは tentative が真の一括操作（external_video_tags.go の resolveTagNames）で、
// 名前を tag_names に書く入口（insertTagName・RenameTag）が却下した名前から外す。

// ConfirmTag はタグ id を確定したタグにする。既に確定していれば何も変えない。名前と付与は
// 変えない。id が無ければ domain.ErrTagNotFound を返す。
func (s *TagStore) ConfirmTag(ctx context.Context, id int64) (domain.Tag, error) {
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.Tag{}, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tagRefByID(ctx, tx, id); err != nil {
		return domain.Tag{}, err
	}
	if err := confirmTagInTx(ctx, tx, id); err != nil {
		return domain.Tag{}, err
	}
	tag, err := tagByID(ctx, tx, id)
	if err != nil {
		return domain.Tag{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Tag{}, fmt.Errorf("cannot confirm the tag: %w", err)
	}
	return tag, nil
}

// RejectTag は仮のタグ id を消し、その元の名前を却下した名前として覚え、その名前を返す。
// tag_names と video_tags は外部キーの ON DELETE CASCADE で連鎖して消える。id が無ければ
// domain.ErrTagNotFound、仮でなければ domain.ErrTagNotTentative を返し、何も変えない
// （specs/031-tentative-tags/research.md R-5）。
func (s *TagStore) RejectTag(ctx context.Context, id int64) (string, error) {
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()

	ref, err := tagRefByID(ctx, tx, id)
	if err != nil {
		return "", err
	}
	if !ref.Tentative {
		return "", domain.ErrTagNotTentative
	}
	if _, err := tx.ExecContext(ctx, `delete from tags where id = ?`, id); err != nil {
		return "", fmt.Errorf("cannot reject the tag (id=%d): %w", id, err)
	}
	if _, err := tx.ExecContext(ctx,
		`insert or ignore into rejected_tag_names (name, created_at) values (?, ?)`,
		ref.Name, time.Now().Unix(),
	); err != nil {
		return "", fmt.Errorf("cannot remember the rejected tag name (%s): %w", ref.Name, err)
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("cannot reject the tag: %w", err)
	}
	return ref.Name, nil
}

// ListRejectedTagNames は却下した名前を名前の自然順（domain.SortTagNames）ですべて返す。
func (s *TagStore) ListRejectedTagNames(ctx context.Context) ([]string, error) {
	rows, err := s.sql.QueryContext(ctx, `select name from rejected_tag_names`)
	if err != nil {
		return nil, fmt.Errorf("cannot read rejected tag names: %w", err)
	}
	defer func() { _ = rows.Close() }()

	names := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("cannot read rejected tag names: %w", err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read rejected tag names: %w", err)
	}
	domain.SortTagNames(names)
	return names, nil
}

// ForgetRejectedTagName は name を却下した名前から外す。無ければ何も変えない。name は
// domain.NormalizeTagName で整えてから照合し、整えられない入力はそのまま照合する
// （RemoveSynonym と同じ扱い）。
func (s *TagStore) ForgetRejectedTagName(ctx context.Context, name string) error {
	if normalized, err := domain.NormalizeTagName(name); err == nil {
		name = normalized
	}
	if _, err := s.sql.ExecContext(ctx, `delete from rejected_tag_names where name = ?`, name); err != nil {
		return fmt.Errorf("cannot forget the rejected tag name: %w", err)
	}
	return nil
}
