package store

import (
	"context"
	"fmt"

	"github.com/syudead/vv/internal/domain"
)

// RefreshSearchKeys は search_version が現在の版より小さいタグ名の行の search_key と
// sort_key、却下した名前の行の sort_key を作り直し、作り直した行の数（両方の表の合計）を
// 返す（specs/014-video-tags/data-model.md §7、specs/036-tag-admin-scale/data-model.md §0・§2）。
// 起動時、LibraryStore.RefreshSearchKeys の隣で呼ぶ。所在の鍵と違ってメディア
// フォルダに依らないので、folderMu は取らない。
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

// staleNames は query（版を 1 つの引数で受け、name を 1 列返す）で、現在の版より小さい行の
// 名前を読む。
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
