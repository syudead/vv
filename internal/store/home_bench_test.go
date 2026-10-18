package store

import (
	"context"
	"fmt"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// BenchmarkHomeReads measures summaries and eight saved shelves with cold and warm caches.
func BenchmarkHomeReads(b *testing.B) {
	db, cat := smartFolderBenchDB(b)
	ctx := context.Background()
	if _, err := db.sql.Exec(`insert into public_videos (content_key, published_at)
		select content_key, 1 from videos where id % 2 = 0`); err != nil {
		b.Fatal(err)
	}
	ids := make([]int64, 0, domain.MaxHomeShelves)
	for i := range domain.MaxHomeShelves {
		bound := (i%10 + 1) * 6 * 60
		days := i + 1
		rules := []domain.SmartRule{
			{Tags: []int64{cat}},
			{Watch: domain.WatchUnwatched},
			{MaxDuration: &bound},
			{AddedWithin: &days},
			{Q: fmt.Sprintf("series-%02d", i%40)},
		}
		ids = append(ids, benchSmartFolder(b, db, fmt.Sprintf("shelf-%02d", i), rules[i%len(rules)], 0))
	}
	if err := db.Home().SetShelves(ctx, ids); err != nil {
		b.Fatal(err)
	}
	var keys []domain.HomeShelfKey
	for _, id := range ids[:8] {
		keys = append(keys, domain.FolderShelfKey(id))
	}
	// Direct fixture writes need one maintenance pass before reading.
	if err := db.maintainHomeIndexes(ctx); err != nil {
		b.Fatal(err)
	}
	for _, audience := range []domain.Audience{domain.AudienceOwner, domain.AudienceGuest} {
		b.Run("home for "+audience.String(), func(b *testing.B) {
			for b.Loop() {
				home, err := db.Home().Home(ctx, audience)
				if err != nil {
					b.Fatal(err)
				}
				if len(home.Shelves) < 2 {
					b.Fatalf("shelves = %d", len(home.Shelves))
				}
			}
		})
		readShelves := func(b *testing.B) {
			b.Helper()
			page, err := db.Home().ShelfItems(ctx, audience, keys)
			if err != nil {
				b.Fatal(err)
			}
			if len(page.Shelves) == 0 {
				b.Fatal("no shelf")
			}
		}
		// 保持の無い読み出し（知らせのあとの最初の読み出し）と、保持した棚の読み出しを測る。
		b.Run("8 shelves for "+audience.String(), func(b *testing.B) {
			for b.Loop() {
				db.homeShelves.dropFor([]domain.Event{domain.HomeChanged{}})
				readShelves(b)
			}
		})
		b.Run("8 held shelves for "+audience.String(), func(b *testing.B) {
			readShelves(b)
			for b.Loop() {
				readShelves(b)
			}
		})
		// Measure reads after progress writes refresh the shared indexes.
		b.Run("8 shelves after a progress write for "+audience.String(), func(b *testing.B) {
			n := 0
			for b.Loop() {
				n++
				if _, err := db.sql.Exec(`update playback_progress set position_ms = ?, updated_at = ? where content_key = 'key-3'`, 1000+n, 100000+n); err != nil {
					b.Fatal(err)
				}
				db.publishEvents(domain.HomeChanged{})
				readShelves(b)
			}
		})
	}
}
