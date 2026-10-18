package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/syudead/vv/internal/domain"
)

// Home returns saved shelves in owner order, omitting empty guest shelves.
func (s *HomeStore) Home(ctx context.Context, audience domain.Audience) (domain.Home, error) {
	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.Home{}, fmt.Errorf("cannot start reading the home: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	folders, err := homeShelves(ctx, tx)
	if err != nil {
		return domain.Home{}, err
	}
	shown, err := shownSmartFolderTargets(ctx, tx, audience, shelfIDs(folders), listNow().Unix())
	if err != nil {
		return domain.Home{}, err
	}
	visible := make(map[int64]bool, len(shown))
	for _, target := range shown {
		visible[target.summary.ID] = true
	}
	home := domain.Home{Shelves: make([]domain.HomeShelf, 0, len(shown))}
	for _, folder := range folders {
		if visible[folder.ID] {
			home.Shelves = append(home.Shelves, domain.FolderShelf(folder))
		}
	}
	if home.NoVideos, err = noVisibleVideos(ctx, tx, audience); err != nil {
		return domain.Home{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Home{}, fmt.Errorf("cannot finish reading the home: %w", err)
	}
	return home, nil
}

// ShelfItems reads cached smart-folder pages, omitting missing shelves and empty guest shelves.
func (s *HomeStore) ShelfItems(ctx context.Context, audience domain.Audience, keys []domain.HomeShelfKey) (domain.HomeShelfPage, error) {
	generation := s.db.homeShelves.start()
	results := make([]homeShelfCacheEntry, len(keys))
	var missing []int
	for i, key := range keys {
		entry, ok := s.db.homeShelves.get(homeShelfCacheKey{shelf: key, audience: audience})
		if ok {
			results[i] = entry
		} else {
			missing = append(missing, i)
		}
	}
	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.HomeShelfPage{}, fmt.Errorf("cannot start reading the home shelves: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := readHomeShelves(ctx, tx, audience, keys, missing, results); err != nil {
		return domain.HomeShelfPage{}, err
	}
	page := domain.HomeShelfPage{Shelves: make([]domain.HomeShelfItems, 0, len(keys))}
	if page.Roots, err = listMediaFolders(ctx, tx); err != nil {
		return domain.HomeShelfPage{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.HomeShelfPage{}, fmt.Errorf("cannot finish reading the home shelves: %w", err)
	}
	for _, i := range missing {
		s.db.homeShelves.put(generation, homeShelfCacheKey{shelf: keys[i], audience: audience}, results[i])
	}
	for _, result := range results {
		if result.shown {
			page.Shelves = append(page.Shelves, result.shelf)
		}
	}
	return page, nil
}

// readHomeShelves は棚 keys のうち位置 indexes の棚を読み、同じ位置の results に入れる。棚の項目は基底の
// 表の索引をなぞって読む（home_shelf_read.go）。
func readHomeShelves(ctx context.Context, tx *sql.Tx, audience domain.Audience, keys []domain.HomeShelfKey, indexes []int, results []homeShelfCacheEntry) error {
	reader, err := newHomeShelfReader(ctx, tx, audience, listNow().Unix())
	if err != nil {
		return err
	}
	for _, i := range indexes {
		spec, found, err := homeShelfItemsSpec(ctx, tx, audience, keys[i], reader.now)
		if err != nil {
			return err
		}
		results[i] = homeShelfCacheEntry{shelf: domain.HomeShelfItems{Key: keys[i], Items: []domain.LibraryItem{}}, shown: found, isShelf: found}
		shelf := &results[i].shelf
		if spec != nil {
			page, err := reader.homeShelfFirstPage(*spec)
			if err != nil {
				return err
			}
			shelf.Items, shelf.Total = page.Items, page.Total
		}
		if shelf.Total == 0 && !audience.IsOwner() {
			results[i].shown = false
		}
	}
	return nil
}

// homeShelfItemsSpec は棚 key の項目の絞り込みを返す。棚が無ければ found は偽で、フォルダの棚の
// 経路の条件が無い実体を名指せば、何にも一致しないので spec は nil である。
func homeShelfItemsSpec(ctx context.Context, tx *sql.Tx, audience domain.Audience, key domain.HomeShelfKey, now int64) (spec *listSpec, found bool, err error) {
	return folderShelfSpec(ctx, tx, audience, key.FolderID, now)
}

// folderShelfSpec はフォルダ id の棚の項目の絞り込みを、フォルダと祖先の条件とフォルダの並べ順で
// 返す。フォルダが無いか棚でなければ found は偽である。
func folderShelfSpec(ctx context.Context, tx *sql.Tx, audience domain.Audience, id int64, now int64) (*listSpec, bool, error) {
	folder, err := readSmartFolder(ctx, tx, id)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if folder.HomePosition == 0 {
		return nil, false, nil
	}
	rules, err := smartFolderPathRules(ctx, tx, id)
	if err != nil {
		return nil, false, err
	}
	missing, err := missingSmartRuleRefs(ctx, tx, rules)
	if err != nil {
		return nil, false, err
	}
	if len(missing) > 0 {
		return nil, true, nil
	}
	spec := smartFolderSpec(audience, rules, folder.Sort, now)
	spec.limit = domain.HomeShelfItemLimit
	return &spec, true, nil
}

// noVisibleVideos は、見る人 audience に見せてよい所在を持つ動画が 1 本も無いかを返す。見せてよい
// 所在を持つ動画はどれも、動画かグループの項目としてライブラリに出る。
func noVisibleVideos(ctx context.Context, tx *sql.Tx, audience domain.Audience) (bool, error) {
	var found bool
	if err := tx.QueryRowContext(ctx, `select exists (select 1 from video_locations l where `+
		visibleLocationCondition("l", audience)+`)`).Scan(&found); err != nil {
		return false, fmt.Errorf("cannot read whether the library has videos: %w", err)
	}
	return !found, nil
}
