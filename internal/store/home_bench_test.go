package store

import (
	"context"
	"fmt"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// BenchmarkHomeReads は 2 万本の動画と 50 の棚で、ホームの要約（GET /api/home）と、画面に入った
// 8 つの棚（フォルダの棚 6 つ、recent、unshelved）の最初の項目（GET /api/home/shelves）を読む時間を、
// 所有者とゲストについて測る。予算はどれも 1 秒未満（specs/057-home/research.md R-12）。棚は条件が
// タグ・視聴状態・長さ・追加日・検索のものを 10 個ずつ持ち、ゲストには偶数番の動画を見せる。
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
	keys := []domain.HomeShelfKey{{Kind: domain.HomeShelfRecent}, {Kind: domain.HomeShelfUnshelved}}
	for _, id := range ids[:6] {
		keys = append(keys, domain.FolderShelfKey(id))
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
		b.Run("8 shelves for "+audience.String(), func(b *testing.B) {
			for b.Loop() {
				page, err := db.Home().ShelfItems(ctx, audience, keys)
				if err != nil {
					b.Fatal(err)
				}
				if len(page.Shelves) == 0 {
					b.Fatal("no shelf")
				}
			}
		})
	}
}
