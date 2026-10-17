package store

import (
	"context"
	"fmt"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// BenchmarkHomeReads は 2 万本の動画と 50 の棚で、ホームの要約（GET /api/home）と、画面に入った
// 8 つの棚（フォルダの棚 6 つ、recent、unshelved）の最初の項目（GET /api/home/shelves）を読む時間を、
// 所有者とゲストについて測る。棚の項目は保持の無いときと保持したときを測る。予算はどれも 1 秒未満
// （docs/design-docs/home.md）。棚は条件がタグ・視聴状態・長さ・追加日・検索のものを 10 個ずつ
// 持ち、ゲストには偶数番の動画を見せる。所有者の続きから見る棚（6,667 本が視聴途中）と、おすすめの
// 並び（10 本の再生と 10 本のお気に入りを種に、どの動画も 81 コマの指紋を持つ 2 万本を internal/app の
// Home と同じ手順で並べる）も保持の無いときを測る（R-13）。
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
	b.Run("continue for owner", func(b *testing.B) {
		continueKeys := []domain.HomeShelfKey{{Kind: domain.HomeShelfContinue}}
		for b.Loop() {
			db.homeShelves.dropKind(domain.HomeShelfContinue)
			if page, err := db.Home().ShelfItems(ctx, domain.AudienceOwner, continueKeys); err != nil || len(page.Shelves) != 1 {
				b.Fatalf("continue = %+v, %v", page.Shelves, err)
			}
		}
	})
	benchRecommended(b, db)
	// 準備は表へ直接書いたので、書き込みの確定のあとに走る維持（根の値、棚への一致）をここで 1 度行う。
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
		// 動画の書き込み（再生位置の保存）の直後の読み出し: 引き金が消した根の値と棚への一致の印を、確定のあとの
		// 維持が埋め直してから読む。
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

// benchRecommended はどの動画にも 81 コマの指紋を、10 本に再生を、別の 10 本にお気に入りを持たせ、
// おすすめの元の読み出しと、internal/app の Home の並べ方と同じ手順の全体（保持の無いとき）を測る。
func benchRecommended(b *testing.B, db *DB) {
	b.Helper()
	tx, err := db.sql.Begin()
	if err != nil {
		b.Fatal(err)
	}
	random := rand.New(rand.NewPCG(1, 2))
	for i := range 20000 {
		frames := make([]domain.FrameHash, domain.SeekSpriteMaxFrames)
		for j := range frames {
			frames[j].Hash = random.Uint64()
		}
		fingerprint := domain.Fingerprint{Version: domain.FingerprintVersion, IntervalMs: 1000, Frames: frames}
		if _, err := tx.Exec(`insert into video_fingerprints (content_key, version, interval_ms, hashes, updated_at)
			values (?, ?, 1000, ?, 1)`, fmt.Sprintf("key-%d", i), domain.FingerprintVersion, fingerprint.Encode()); err != nil {
			b.Fatal(err)
		}
	}
	playedAt := time.Now().Add(-time.Hour).UnixMilli()
	for i := range domain.RecommendSeedPlays {
		if _, err := tx.Exec(`insert into watch_history (content_key, title, title_key, played_at) values (?, '', '', ?)`,
			fmt.Sprintf("key-%d", i*7+1), playedAt); err != nil {
			b.Fatal(err)
		}
		if _, err := tx.Exec(`insert into video_favorites (content_key, favorited_at) values (?, ?)`,
			fmt.Sprintf("key-%d", i*11+5), i); err != nil {
			b.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		b.Fatal(err)
	}
	ctx := context.Background()
	b.Run("recommended inputs for owner", func(b *testing.B) {
		for b.Loop() {
			if inputs, err := db.Home().RecommendInputs(ctx); err != nil || len(inputs.Seeds) != 20 {
				b.Fatalf("inputs = %+v, %v", inputs, err)
			}
		}
	})
	b.Run("recommended clues for owner", func(b *testing.B) {
		for b.Loop() {
			if _, err := db.Library().RelatedCandidates(ctx, domain.AudienceOwner, 0); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("recommended for owner", func(b *testing.B) {
		for b.Loop() {
			inputs, err := db.Home().RecommendInputs(ctx)
			if err != nil {
				b.Fatal(err)
			}
			candidates, err := db.Library().RelatedCandidates(ctx, domain.AudienceOwner, 0)
			if err != nil {
				b.Fatal(err)
			}
			ids := domain.RankRecommended(domain.DefaultRelatedScorer, inputs, candidates, domain.HomeShelfItemLimit)
			if videos, err := db.Library().VideosByIDs(ctx, domain.AudienceOwner, ids); err != nil || len(videos) != domain.HomeShelfItemLimit {
				b.Fatalf("recommended = %d, %v", len(videos), err)
			}
		}
	})
}
