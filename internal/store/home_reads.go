package store

// ホームの読み出し（docs/design-docs/home.md、research.md R-12、R-13）。Home は棚の
// 要約を、ShelfItems は画面に入った棚の最初の項目を、どちらも 1 つの読み取りの
// スナップショットから返す。棚の項目はライブラリの項目と同じ項目・同じ並びで、基底の表の索引を並び順に
// なぞって最初のページだけを読む（home_shelf_read.go）。フォルダの棚にはフォルダの経路の条件
// （smartFolderSpec）を掛ける。子フォルダは混ぜない（R-14）。

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/syudead/vv/internal/domain"
)

// fixedHomeShelves は、見る人 audience のフォルダの棚のあとに並べる決まった棚である（research.md R-13）。
// 続きから見るとおすすめは所有者だけで、おすすめは種 seeded があるときだけ並べる。
func fixedHomeShelves(audience domain.Audience, seeded bool) []domain.HomeShelfKind {
	if !audience.IsOwner() {
		return []domain.HomeShelfKind{domain.HomeShelfRecent, domain.HomeShelfUnshelved}
	}
	kinds := []domain.HomeShelfKind{domain.HomeShelfContinue, domain.HomeShelfRecent, domain.HomeShelfUnshelved}
	if seeded {
		kinds = append(kinds, domain.HomeShelfRecommended)
	}
	return kinds
}

// Home は見る人 audience のホームの要約を返す（GET /api/home）。フォルダの棚を並びの順に、続けて
// 決まった棚を並べる。ゲストには見せる動画の無いフォルダの棚を入れず（research.md R-8 と同じ）、
// 所有者には続きから見る棚と、おすすめの種があれば
// おすすめの棚を並べる。見る人に動画が 1 本も見えなければ
// NoVideos を真にする。
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
	var seeds []int64
	if audience.IsOwner() {
		if seeds, err = recommendSeeds(ctx, tx); err != nil {
			return domain.Home{}, err
		}
	}
	fixed := fixedHomeShelves(audience, len(seeds) > 0)
	home := domain.Home{Shelves: make([]domain.HomeShelf, 0, len(shown)+len(fixed))}
	for _, folder := range folders {
		if visible[folder.ID] {
			home.Shelves = append(home.Shelves, domain.FolderShelf(folder))
		}
	}
	for _, kind := range fixed {
		home.Shelves = append(home.Shelves, domain.HomeShelf{Key: domain.HomeShelfKey{Kind: kind}})
	}
	if home.NoVideos, err = noVisibleVideos(ctx, tx, audience); err != nil {
		return domain.Home{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Home{}, fmt.Errorf("cannot finish reading the home: %w", err)
	}
	return home, nil
}

// ShelfItems は見る人 audience に見せる棚 keys の最初の domain.HomeShelfItemLimit 件の項目と項目の
// 数を、keys の順に返す。もう棚でないフォルダ、無いフォルダ、ゲストに見せる項目の無いフォルダの棚、
// ゲストの続きから見る棚と、この読み出しが受け持たない棚（おすすめ。internal/app の Home が読む）の
// 鍵は落とす。経路の条件が無い実体を名指す
// フォルダの棚は、所有者には項目の無い棚にする（docs/design-docs/catalog-organization.md）。棚ごとの
// 結果は保持し（home_shelf_cache.go）、保持の無い棚だけを 1 つの読み取りのスナップショットから読む
// （home_shelf_read.go）。
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
		if shelf.Total == 0 && shelf.Key.Kind == domain.HomeShelfFolder && !audience.IsOwner() {
			results[i].shown = false
		}
	}
	return nil
}

// homeShelfItemsSpec は棚 key の項目の絞り込みを返す。棚が無ければ found は偽で、フォルダの棚の
// 経路の条件が無い実体を名指せば、何にも一致しないので spec は nil である。
func homeShelfItemsSpec(ctx context.Context, tx *sql.Tx, audience domain.Audience, key domain.HomeShelfKey, now int64) (spec *listSpec, found bool, err error) {
	switch key.Kind {
	case domain.HomeShelfFolder:
		return folderShelfSpec(ctx, tx, audience, key.FolderID, now)
	case domain.HomeShelfContinue:
		// 視聴状態は所有者のものなので、ゲストには棚が無い（research.md R-13）。
		if !audience.IsOwner() {
			return nil, false, nil
		}
		fallthrough
	case domain.HomeShelfRecent, domain.HomeShelfUnshelved:
		// 棚にない動画は hs_shelved で求める（home_shelf_read.go）ので、ここでは shelved を埋めない。
		fixed := libraryListSpec(audience, fixedShelfQuery(key.Kind), nil)
		fixed.now, fixed.limit = now, domain.HomeShelfItemLimit
		return &fixed, true, nil
	}
	return nil, false, nil
}

// fixedShelfQuery は決まった棚 kind の項目のライブラリの絞り込みである。続きから見るはライブラリの
// watch=inProgress と同じ視聴途中の項目を最後に再生した順に、ほかは新しく追加した順に並べる。
func fixedShelfQuery(kind domain.HomeShelfKind) domain.VideoQuery {
	if kind == domain.HomeShelfContinue {
		return domain.VideoQuery{Sort: domain.SortPlayedDesc, Watch: domain.WatchInProgress}
	}
	return domain.VideoQuery{Sort: domain.SortAddedDesc, Unshelved: kind == domain.HomeShelfUnshelved}
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
