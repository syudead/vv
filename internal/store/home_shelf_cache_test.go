package store

import (
	"context"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// shelfTotal treats an omitted guest shelf as empty.
func shelfTotal(t *testing.T, db *DB, audience domain.Audience, id int64) int {
	t.Helper()
	page, err := db.Home().ShelfItems(context.Background(), audience, []domain.HomeShelfKey{domain.FolderShelfKey(id)})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Shelves) == 0 {
		return 0
	}
	if len(page.Shelves) != 1 {
		t.Fatalf("shelves = %+v", page.Shelves)
	}
	return page.Shelves[0].Total
}

// 棚の項目は見る人ごとに保持し、ホームを読み直させる知らせか保持の期限で読み直す。知らせの無い変化
// （ここでは公開の行を直接足す）は保持のあいだ見えない。
func TestHomeShelfItemsAreHeldUntilANoticeOrTheTTL(t *testing.T) {
	db, _ := homeFixture(t, "猫", "犬")
	folder := createRuleFolder(t, db, "All", domain.SmartRule{})
	if err := db.Home().SetShelves(context.Background(), []int64{folder.ID}); err != nil {
		t.Fatal(err)
	}
	clock := time.Unix(1_000_000, 0)
	homeShelfCacheNow = func() time.Time { return clock }
	t.Cleanup(func() { homeShelfCacheNow = time.Now })
	publish := func(key string) {
		t.Helper()
		if _, err := db.sql.Exec(`insert into public_videos (content_key, published_at) values (?, 1)`, key); err != nil {
			t.Fatal(err)
		}
	}

	if got := shelfTotal(t, db, domain.AudienceGuest, folder.ID); got != 0 {
		t.Fatalf("ゲストの棚 = %d, want 0", got)
	}
	publish("key-0")
	if got := shelfTotal(t, db, domain.AudienceGuest, folder.ID); got != 0 {
		t.Errorf("知らせの前のゲストの棚 = %d, want 0（保持した値）", got)
	}
	if got := shelfTotal(t, db, domain.AudienceOwner, folder.ID); got != 2 {
		t.Errorf("所有者の棚 = %d, want 2（見る人ごとに保持する）", got)
	}

	// スマートフォルダの知らせ（ホームの画面を読み直させる）で捨てる。
	db.publishEvents(domain.SmartFoldersChanged{IDs: []int64{1}})
	if got := shelfTotal(t, db, domain.AudienceGuest, folder.ID); got != 1 {
		t.Errorf("知らせの後のゲストの棚 = %d, want 1", got)
	}

	// 画面を読み直させない知らせでは捨てない。
	publish("key-1")
	db.publishEvents(domain.ScanChanged{})
	if got := shelfTotal(t, db, domain.AudienceGuest, folder.ID); got != 1 {
		t.Errorf("取り込みの知らせの後のゲストの棚 = %d, want 1（保持した値）", got)
	}
	clock = clock.Add(homeShelfCacheTTL)
	if got := shelfTotal(t, db, domain.AudienceGuest, folder.ID); got != 2 {
		t.Errorf("期限の後のゲストの棚 = %d, want 2", got)
	}
}

// 捨てる前に読み始めた結果は、捨てた後にしまわない。
func TestHomeShelfCacheKeepsNoResultReadBeforeADrop(t *testing.T) {
	var cache homeShelfCache
	key := homeShelfCacheKey{shelf: domain.FolderShelfKey(1), audience: domain.AudienceOwner}
	generation := cache.start()
	cache.dropFor([]domain.Event{domain.HomeChanged{}})
	cache.put(generation, key, homeShelfCacheEntry{shelf: domain.HomeShelfItems{Key: key.shelf, Total: 3}, shown: true, isShelf: true})
	if _, ok := cache.get(key); ok {
		t.Fatal("捨てる前に読んだ結果をしまった")
	}
	cache.put(cache.start(), key, homeShelfCacheEntry{shelf: domain.HomeShelfItems{Key: key.shelf, Total: 3}, shown: true, isShelf: true})
	if entry, ok := cache.get(key); !ok || entry.shelf.Total != 3 {
		t.Fatalf("get = %+v, %v", entry, ok)
	}
}

// 棚でない鍵（無いフォルダ・棚でないフォルダの棚の鍵）の結果は保持しない。見る人がいくらでも作れる
// 鍵で保持が増え続けないためである。
func TestHomeShelfCacheHoldsNoKeyThatNamesNoShelf(t *testing.T) {
	db, _ := homeFixture(t, "猫", "犬")
	folder := createRuleFolder(t, db, "All", domain.SmartRule{})
	if err := db.Home().SetShelves(context.Background(), []int64{folder.ID}); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for id := int64(1000); id < 1010; id++ {
		page, err := db.Home().ShelfItems(ctx, domain.AudienceGuest, []domain.HomeShelfKey{{FolderID: id}})
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Shelves) != 0 {
			t.Fatalf("無いフォルダ %d の棚 = %+v", id, page.Shelves)
		}
	}
	shelfTotal(t, db, domain.AudienceGuest, folder.ID)
	db.homeShelves.mu.Lock()
	held := len(db.homeShelves.entries)
	db.homeShelves.mu.Unlock()
	if held != 1 {
		t.Errorf("保持した鍵の数 = %d, want 1（棚だけ）", held)
	}
}
