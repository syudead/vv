package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/syudead/vv/internal/domain"
)

// Version each row so interrupted startup batches resume from remaining stale rows.
const searchKeyBatchSize = 500

type queryExecer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Search keys include title, root-relative path and optional override; unregistered locations have empty keys.
func locationSearchKey(roots []string, path, title, displayName string) string {
	for _, root := range roots {
		rel, ok := registeredRelativePath(root, path)
		if !ok {
			continue
		}
		key := searchKeyPart(title) + "\n" + searchKeyPart(rel)
		if displayName != "" {
			key += "\n" + searchKeyPart(displayName)
		}
		return key
	}
	return ""
}

// Normalize component newlines to spaces, reserving newlines solely for component boundaries.
func searchKeyPart(s string) string {
	return strings.ReplaceAll(domain.FoldForMatch(s), "\n", " ")
}

// Use the same registration rules as listing and normalize path separators to slash, keeping visible locations searchable.
func registeredRelativePath(root, path string) (string, bool) {
	p, r := path, root
	if runtime.GOOS == "windows" {
		// Fold ASCII only, matching SQLite lower().
		p, r = domain.LowerASCII(p), domain.LowerASCII(r)
	}
	if p == r {
		return "", true
	}
	separators := registrationSeparators()
	trimmed := strings.TrimRight(r, string(separators))
	for _, separator := range separators {
		if rest, ok := strings.CutPrefix(p, trimmed+string(separator)); ok {
			return filepath.ToSlash(rest), true
		}
	}
	return "", false
}

func mediaFolderRoots(ctx context.Context, q queryExecer) ([]string, error) {
	folders, err := listMediaFolders(ctx, q)
	if err != nil {
		return nil, err
	}
	roots := make([]string, 0, len(folders))
	for _, folder := range folders {
		roots = append(roots, folder.Path)
	}
	return roots, nil
}

type searchKeyTarget struct {
	id                       int64
	path, title, displayName string
}

const searchKeyTargetsSelect = `select l.id, l.path, l.title, coalesce(ov.display_name, '')
	from video_locations l join videos v on v.id = l.video_id
	left join video_overrides ov on ov.content_key = v.content_key and v.content_key <> ''`

func (t searchKeyTarget) sortTitle() string {
	if t.displayName != "" {
		return t.displayName
	}
	return t.title
}

func writeSearchKeys(ctx context.Context, q queryExecer, roots []string, targets []searchKeyTarget) error {
	for _, target := range targets {
		if _, err := q.ExecContext(ctx,
			`update video_locations set search_key = ?, title_key = ?, search_version = ? where id = ?`,
			locationSearchKey(roots, target.path, target.title, target.displayName),
			domain.NaturalSortKey(target.sortTitle()),
			domain.SearchKeyVersion, target.id,
		); err != nil {
			return fmt.Errorf("cannot save the search key (%s): %w", target.path, err)
		}
	}
	return nil
}

// Refresh keys in the same transaction that adds or changes the location.
func refreshSearchKeysByPath(ctx context.Context, q queryExecer, path string) error {
	targets, err := searchKeyTargets(ctx, q, searchKeyTargetsSelect+` where l.path = ?`, path)
	if err != nil {
		return err
	}
	roots, err := mediaFolderRoots(ctx, q)
	if err != nil {
		return err
	}
	return writeSearchKeys(ctx, q, roots, targets)
}

// Refresh keys in the media-folder registration transaction.
func refreshSearchKeysUnder(ctx context.Context, q queryExecer, root string) error {
	all, err := searchKeyTargets(ctx, q, searchKeyTargetsSelect)
	if err != nil {
		return err
	}
	var targets []searchKeyTarget
	for _, target := range all {
		if _, ok := registeredRelativePath(root, target.path); ok {
			targets = append(targets, target)
		}
	}
	if len(targets) == 0 {
		return nil
	}
	roots, err := mediaFolderRoots(ctx, q)
	if err != nil {
		return err
	}
	return writeSearchKeys(ctx, q, roots, targets)
}

// Refresh all content locations in the display-name transaction.
func refreshSearchKeysForContentKeys(ctx context.Context, q queryExecer, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	encoded, err := json.Marshal(keys)
	if err != nil {
		return fmt.Errorf("cannot build content keys: %w", err)
	}
	targets, err := searchKeyTargets(ctx, q, searchKeyTargetsSelect+
		` where v.content_key in (select value from json_each(?)) and v.content_key <> ''`, string(encoded))
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return nil
	}
	roots, err := mediaFolderRoots(ctx, q)
	if err != nil {
		return err
	}
	return writeSearchKeys(ctx, q, roots, targets)
}

func searchKeyTargets(ctx context.Context, q queryExecer, query string, args ...any) ([]searchKeyTarget, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("cannot read locations: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var targets []searchKeyTarget
	for rows.Next() {
		var target searchKeyTarget
		if err := rows.Scan(&target.id, &target.path, &target.title, &target.displayName); err != nil {
			return nil, fmt.Errorf("cannot read locations: %w", err)
		}
		targets = append(targets, target)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read locations: %w", err)
	}
	return targets, nil
}

// RefreshSearchKeys resumes stale rows in bounded startup transactions; failures must stop startup before serving stale search results.
func (s *LibraryStore) RefreshSearchKeys(ctx context.Context) (int, error) {
	refreshed := 0
	for {
		count, err := s.refreshSearchKeyBatch(ctx)
		if err != nil {
			return refreshed, err
		}
		refreshed += count
		if count < searchKeyBatchSize {
			return refreshed, nil
		}
	}
}

func (s *LibraryStore) refreshSearchKeyBatch(ctx context.Context) (int, error) {
	s.db.folderMu.Lock()
	defer s.db.folderMu.Unlock()
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	targets, err := searchKeyTargets(ctx, tx,
		searchKeyTargetsSelect+` where l.search_version < ? order by l.id limit ?`,
		domain.SearchKeyVersion, searchKeyBatchSize)
	if err != nil {
		return 0, err
	}
	if len(targets) == 0 {
		return 0, nil
	}
	roots, err := mediaFolderRoots(ctx, tx)
	if err != nil {
		return 0, err
	}
	if err := writeSearchKeys(ctx, tx, roots, targets); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("cannot save search keys: %w", err)
	}
	return len(targets), nil
}
