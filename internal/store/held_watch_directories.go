package store

import (
	"context"
	"fmt"

	"github.com/syudead/vv/internal/domain"
)

// HeldWatchDirectories lists, in path order, the dirty directories automatic import held through a background pause.
func (s *ScanStore) HeldWatchDirectories(ctx context.Context) ([]domain.DirtyDirectory, error) {
	rows, err := s.db.sql.QueryContext(ctx,
		`select path, recursive from held_watch_directories order by path, recursive`)
	if err != nil {
		return nil, fmt.Errorf("cannot read the held watch directories: %w", err)
	}
	defer func() { _ = rows.Close() }()
	dirs := []domain.DirtyDirectory{}
	for rows.Next() {
		var dir domain.DirtyDirectory
		if err := rows.Scan(&dir.Path, &dir.Recursive); err != nil {
			return nil, fmt.Errorf("cannot read the held watch directories: %w", err)
		}
		dirs = append(dirs, dir)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the held watch directories: %w", err)
	}
	return dirs, nil
}

// HoldWatchDirectories records dirty directories held through a pause; a directory already held is kept once.
func (s *ScanStore) HoldWatchDirectories(ctx context.Context, dirs []domain.DirtyDirectory) error {
	return s.writeHeldWatchDirectories(ctx, dirs,
		`insert into held_watch_directories (path, recursive) values (?, ?) on conflict do nothing`)
}

// ReleaseWatchDirectories deletes the held rows of directories that left the dirty set.
func (s *ScanStore) ReleaseWatchDirectories(ctx context.Context, dirs []domain.DirtyDirectory) error {
	return s.writeHeldWatchDirectories(ctx, dirs,
		`delete from held_watch_directories where path = ? and recursive = ?`)
}

func (s *ScanStore) writeHeldWatchDirectories(ctx context.Context, dirs []domain.DirtyDirectory, query string) error {
	if len(dirs) == 0 {
		return nil
	}
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot write the held watch directories: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, dir := range dirs {
		if _, err := tx.ExecContext(ctx, query, dir.Path, dir.Recursive); err != nil {
			return fmt.Errorf("cannot write the held watch directory %q: %w", dir.Path, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("cannot write the held watch directories: %w", err)
	}
	return nil
}
