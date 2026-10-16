package store

import (
	"context"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// recentTotal は見る人 audience の最近追加の棚の項目の数を読む。
func recentTotal(t *testing.T, db *DB, audience domain.Audience) int {
	t.Helper()
	page, err := db.Home().ShelfItems(context.Background(), audience, []domain.HomeShelfKey{{Kind: domain.HomeShelfRecent}})
	if err != nil {
		t.Fatal(err)
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
	clock := time.Unix(1_000_000, 0)
	homeShelfCacheNow = func() time.Time { return clock }
	t.Cleanup(func() { homeShelfCacheNow = time.Now })
	publish := func(key string) {
		t.Helper()
		if _, err := db.sql.Exec(`insert into public_videos (content_key, published_at) values (?, 1)`, key); err != nil {
			t.Fatal(err)
		}
	}

	if got := recentTotal(t, db, domain.AudienceGuest); got != 0 {
		t.Fatalf("ゲストの最近追加 = %d, want 0", got)
	}
	publish("key-0")
	if got := recentTotal(t, db, domain.AudienceGuest); got != 0 {
		t.Errorf("知らせの前のゲストの最近追加 = %d, want 0（保持した値）", got)
	}
	if got := recentTotal(t, db, domain.AudienceOwner); got != 2 {
		t.Errorf("所有者の最近追加 = %d, want 2（見る人ごとに保持する）", got)
	}

	// スマートフォルダの知らせ（ホームの画面を読み直させる）で捨てる。
	db.publishEvents(domain.SmartFoldersChanged{IDs: []int64{1}})
	if got := recentTotal(t, db, domain.AudienceGuest); got != 1 {
		t.Errorf("知らせの後のゲストの最近追加 = %d, want 1", got)
	}

	// 画面を読み直させない知らせでは捨てない。
	publish("key-1")
	db.publishEvents(domain.ScanChanged{})
	if got := recentTotal(t, db, domain.AudienceGuest); got != 1 {
		t.Errorf("取り込みの知らせの後のゲストの最近追加 = %d, want 1（保持した値）", got)
	}
	clock = clock.Add(homeShelfCacheTTL)
	if got := recentTotal(t, db, domain.AudienceGuest); got != 2 {
		t.Errorf("期限の後のゲストの最近追加 = %d, want 2", got)
	}
}

// 捨てる前に読み始めた結果は、捨てた後にしまわない。
func TestHomeShelfCacheKeepsNoResultReadBeforeADrop(t *testing.T) {
	var cache homeShelfCache
	key := homeShelfCacheKey{shelf: domain.HomeShelfKey{Kind: domain.HomeShelfRecent}, audience: domain.AudienceOwner}
	generation := cache.start()
	cache.dropFor([]domain.Event{domain.HomeChanged{}})
	cache.put(generation, key, domain.HomeShelfItems{Key: key.shelf, Total: 3}, true)
	if _, ok := cache.get(key); ok {
		t.Fatal("捨てる前に読んだ結果をしまった")
	}
	cache.put(cache.start(), key, domain.HomeShelfItems{Key: key.shelf, Total: 3}, true)
	if entry, ok := cache.get(key); !ok || entry.shelf.Total != 3 {
		t.Fatalf("get = %+v, %v", entry, ok)
	}
}
