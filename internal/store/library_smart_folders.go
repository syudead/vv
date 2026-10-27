package store

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"

	"github.com/syudead/vv/internal/domain"
)

// folderCursorSeed marks a cursor inside the smart-folder section; no seeded order formats its seed this way.
const folderCursorSeed = "folder"

// listLibraryWithFolders pages the owner's pinned smart folders by name ahead of the sorted items, so listing them
// evaluates no folder rule. The folders are top-level ones, or every favorited folder in favorites-only mode.
func listLibraryWithFolders(ctx context.Context, tx *sql.Tx, spec listSpec) (domain.LibraryPage, error) {
	limit := normalizeLimit(spec.limit)
	after, inFolders, err := folderCursorPosition(spec)
	if err != nil {
		return domain.LibraryPage{}, err
	}
	folderCount, err := countPinnedSmartFolders(ctx, tx, spec.favoriteOnly)
	if err != nil {
		return domain.LibraryPage{}, err
	}
	if !inFolders {
		page, err := listLibraryPageTx(ctx, tx, spec)
		page.Total += folderCount
		return page, err
	}
	folders, err := pinnedSmartFolders(ctx, tx, spec.favoriteOnly, after, limit+1)
	if err != nil {
		return domain.LibraryPage{}, err
	}
	spec.cursor = ""
	if len(folders) >= limit {
		// The page holds only folders; read the item count for the total.
		spec.limit = 1
		items, err := listLibraryPageTx(ctx, tx, spec)
		if err != nil {
			return domain.LibraryPage{}, err
		}
		page := domain.LibraryPage{Items: pinnedFolderItems(folders[:limit]), Total: folderCount + items.Total, Limit: limit}
		if len(folders) > limit || items.Total > 0 {
			page.NextCursor = folderCursor(spec, folders[limit-1])
		}
		return page, nil
	}
	spec.limit = limit - len(folders)
	page, err := listLibraryPageTx(ctx, tx, spec)
	if err != nil {
		return domain.LibraryPage{}, err
	}
	page.Items = append(pinnedFolderItems(folders), page.Items...)
	page.Total += folderCount
	page.Limit = limit
	return page, nil
}

type pinnedFolder struct {
	summary  domain.SmartFolderSummary
	titleKey string
}

type folderPosition struct {
	titleKey string
	id       int64
}

// folderCursorPosition reports whether the page starts in the folder section and after which folder.
func folderCursorPosition(spec listSpec) (*folderPosition, bool, error) {
	if spec.cursor == "" {
		return nil, true, nil
	}
	c, err := decodeCursor(spec.cursor)
	if err != nil || c.seed != folderCursorSeed {
		return nil, false, err
	}
	if c.sort != string(spec.sort) {
		return nil, false, fmt.Errorf("%w: cursor is for a different sort order", domain.ErrInvalidCursor)
	}
	return &folderPosition{titleKey: c.value, id: c.id}, true, nil
}

func folderCursor(spec listSpec, last pinnedFolder) string {
	return packCursor(string(spec.sort), folderCursorSeed, "0", strconv.FormatInt(last.summary.ID, 10), last.titleKey)
}

func pinnedFolderCondition(favoriteOnly bool) string {
	if favoriteOnly {
		return `exists (select 1 from smart_folder_favorites ff where ff.smart_folder_id = f.id)`
	}
	return `f.parent_id is null`
}

func countPinnedSmartFolders(ctx context.Context, tx *sql.Tx, favoriteOnly bool) (int, error) {
	var n int
	if err := tx.QueryRowContext(ctx, `select count(*) from smart_folders f where `+pinnedFolderCondition(favoriteOnly)).
		Scan(&n); err != nil {
		return 0, fmt.Errorf("cannot count the smart folders: %w", err)
	}
	return n, nil
}

func pinnedSmartFolders(ctx context.Context, tx *sql.Tx, favoriteOnly bool, after *folderPosition, limit int) ([]pinnedFolder, error) {
	condition, args := pinnedFolderCondition(favoriteOnly), []any{}
	if after != nil {
		condition += ` and (f.title_key, f.id) > (?, ?)`
		args = append(args, after.titleKey, after.id)
	}
	rows, err := tx.QueryContext(ctx, `select f.id, coalesce(f.parent_id, 0), f.name, f.title_key,
		exists (select 1 from smart_folder_favorites ff where ff.smart_folder_id = f.id)
		from smart_folders f where `+condition+` order by f.title_key, f.id limit ?`, append(args, limit)...)
	if err != nil {
		return nil, fmt.Errorf("cannot read the smart folders: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var folders []pinnedFolder
	for rows.Next() {
		var f pinnedFolder
		if err := rows.Scan(&f.summary.ID, &f.summary.ParentID, &f.summary.Name, &f.titleKey, &f.summary.Favorite); err != nil {
			return nil, fmt.Errorf("cannot read the smart folders: %w", err)
		}
		folders = append(folders, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the smart folders: %w", err)
	}
	return folders, nil
}

func pinnedFolderItems(folders []pinnedFolder) []domain.LibraryItem {
	items := make([]domain.LibraryItem, 0, len(folders))
	for i := range folders {
		items = append(items, domain.LibraryItem{SmartFolder: &folders[i].summary})
	}
	return items
}
