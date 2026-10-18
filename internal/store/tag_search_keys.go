package store

import (
	"context"
	"fmt"

	"github.com/syudead/vv/internal/domain"
)

// RefreshSearchKeys upgrades tag and rejected-name keys at startup; these keys do not depend on media folders or require folderMu.
func (s *TagStore) RefreshSearchKeys(ctx context.Context) (int, error) {
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	names, err := staleNames(ctx, tx, `select name from tag_names where search_version < ?`)
	if err != nil {
		return 0, fmt.Errorf("cannot read tag names: %w", err)
	}
	for _, name := range names {
		if _, err := tx.ExecContext(ctx,
			`update tag_names set search_key = ?, sort_key = ?, search_version = ? where name = ?`,
			domain.FoldForMatch(name), domain.NaturalSortKey(name), domain.SearchKeyVersion, name,
		); err != nil {
			return 0, fmt.Errorf("cannot save the tag search key (%s): %w", name, err)
		}
	}

	rejected, err := staleNames(ctx, tx, `select name from rejected_tag_names where search_version < ?`)
	if err != nil {
		return 0, fmt.Errorf("cannot read rejected tag names: %w", err)
	}
	for _, name := range rejected {
		if _, err := tx.ExecContext(ctx,
			`update rejected_tag_names set sort_key = ?, search_version = ? where name = ?`,
			domain.NaturalSortKey(name), domain.SearchKeyVersion, name,
		); err != nil {
			return 0, fmt.Errorf("cannot save the rejected tag name sort key (%s): %w", name, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("cannot save tag search keys: %w", err)
	}
	return len(names) + len(rejected), nil
}

func staleNames(ctx context.Context, q queryExecer, query string) ([]string, error) {
	rows, err := q.QueryContext(ctx, query, domain.SearchKeyVersion)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}
