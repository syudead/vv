package store

import (
	"context"
	"fmt"

	"github.com/syudead/vv/internal/domain"
)

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
			return 0, fmt.Errorf("cannot save the tag search key (%s): %w", name, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("cannot save tag search keys: %w", err)
	}
	return len(names), nil
}

func staleTagNames(ctx context.Context, q queryExecer) ([]string, error) {
	rows, err := q.QueryContext(ctx, `select name from tag_names where search_version < ?`, domain.SearchKeyVersion)
	if err != nil {
		return nil, fmt.Errorf("cannot read tag names: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("cannot read tag names: %w", err)
		}
		names = append(names, name)
	}
	return names, rows.Err()
}
