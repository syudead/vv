package store

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// continueIDs は所有者の続きから見る棚の動画の id を並びの順に読む。
func continueIDs(t *testing.T, db *DB) []int64 {
	t.Helper()
	page, err := db.Home().ShelfItems(context.Background(), domain.AudienceOwner, []domain.HomeShelfKey{{Kind: domain.HomeShelfContinue}})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Shelves) != 1 {
		t.Fatalf("shelves = %+v", page.Shelves)
	}
	ids := []int64{}
	for _, item := range page.Shelves[0].Items {
		if item.Video == nil {
			t.Fatalf("続きから見るに動画でない項目 %+v", item)
		}
		ids = append(ids, item.Video.ID)
	}
	return ids
}

// 受け入れ条件 6: 続きから見るは途中で止めた動画を最後に再生した順に出し、見終わった動画と再生して
// いない動画を出さない。再生位置を書けば、視聴状態が変わらなくても保持を捨てて並べ直す。ゲストには
// 棚が無い。
func TestContinueWatchingListsVideosInProgressLatestPlayedFirst(t *testing.T) {
	db, _ := homeFixture(t, "a", "b", "c", "d")
	ctx := context.Background()
	for _, p := range []struct {
		key      string
		progress domain.Progress
	}{
		{"key-0", domain.Progress{PositionMs: 30_000, DurationMs: 60_000}},
		{"key-1", domain.Progress{PositionMs: 60_000, DurationMs: 60_000, Completed: true}},
		{"key-3", domain.Progress{PositionMs: 10_000, DurationMs: 60_000}},
	} {
		if _, err := db.Playback().SaveProgress(ctx, p.key, p.progress, nil); err != nil {
			t.Fatal(err)
		}
	}
	// 同じ秒の保存の順は決まらないので、最後に再生した時刻を書き直す。
	if _, err := db.sql.Exec(`update playback_progress set updated_at = case content_key when 'key-0' then 100 else 200 end`); err != nil {
		t.Fatal(err)
	}
	db.homeShelves.dropKind(domain.HomeShelfContinue)
	first, last := videoIDForKey(t, db, "key-0"), videoIDForKey(t, db, "key-3")
	if got := continueIDs(t, db); !slices.Equal(got, []int64{last, first}) {
		t.Fatalf("続きから見る = %v, want [%d %d]", got, last, first)
	}

	if _, err := db.Playback().SaveProgress(ctx, "key-0", domain.Progress{PositionMs: 40_000, DurationMs: 60_000}, nil); err != nil {
		t.Fatal(err)
	}
	if got := continueIDs(t, db); !slices.Equal(got, []int64{first, last}) {
		t.Errorf("もう一度再生した後の続きから見る = %v, want [%d %d]", got, first, last)
	}

	page, err := db.Home().ShelfItems(ctx, domain.AudienceGuest, []domain.HomeShelfKey{{Kind: domain.HomeShelfContinue}})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Shelves) != 0 {
		t.Errorf("ゲストの続きから見る = %+v, want none", page.Shelves)
	}
}

// おすすめの種は 90 日以内によく再生した動画（多い順）と新しいお気に入り（新しい順）で、同じ動画は
// 1 度だけ入る。見終わった動画は別に返す。種があるときだけ所有者のホームにおすすめの棚が並ぶ。
func TestRecommendInputsReadSeedsAndWatchedVideos(t *testing.T) {
	db, _ := homeFixture(t, "a", "b", "c", "d")
	ctx := context.Background()
	ids := map[string]int64{}
	for _, key := range []string{"key-0", "key-1", "key-2", "key-3"} {
		ids[key] = videoIDForKey(t, db, key)
	}
	shelfKinds := func() []domain.HomeShelfKind {
		t.Helper()
		home, err := db.Home().Home(ctx, domain.AudienceOwner)
		if err != nil {
			t.Fatal(err)
		}
		var kinds []domain.HomeShelfKind
		for _, shelf := range home.Shelves {
			kinds = append(kinds, shelf.Key.Kind)
		}
		return kinds
	}
	if got := shelfKinds(); slices.Contains(got, domain.HomeShelfRecommended) {
		t.Fatalf("種の無いホームの棚 = %v, want no recommended", got)
	}

	now := time.Now()
	recent, old := now.Add(-time.Hour).UnixMilli(), now.Add(-100*24*time.Hour).UnixMilli()
	for _, play := range []struct {
		key   string
		at    int64
		times int
	}{{"key-1", recent, 2}, {"key-0", recent, 3}, {"key-2", old, 5}} {
		for range play.times {
			if _, err := db.sql.Exec(`insert into watch_history (content_key, title, title_key, played_at) values (?, '', '', ?)`,
				play.key, play.at); err != nil {
				t.Fatal(err)
			}
		}
	}
	for i, key := range []string{"key-0", "key-3"} {
		if _, err := db.sql.Exec(`insert into video_favorites (content_key, favorited_at) values (?, ?)`, key, 1000+i); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Playback().SaveProgress(ctx, "key-1", domain.Progress{PositionMs: 60_000, DurationMs: 60_000, Completed: true}, nil); err != nil {
		t.Fatal(err)
	}

	inputs, err := db.Home().RecommendInputs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if want := []int64{ids["key-0"], ids["key-1"], ids["key-3"]}; !slices.Equal(inputs.Seeds, want) {
		t.Errorf("種 = %v, want %v", inputs.Seeds, want)
	}
	if want := []int64{ids["key-1"]}; !slices.Equal(inputs.Watched, want) {
		t.Errorf("見終わった動画 = %v, want %v", inputs.Watched, want)
	}
	want := []domain.HomeShelfKind{domain.HomeShelfContinue, domain.HomeShelfRecent, domain.HomeShelfUnshelved, domain.HomeShelfRecommended}
	if got := shelfKinds(); !slices.Equal(got, want) {
		t.Errorf("種のあるホームの棚 = %v, want %v", got, want)
	}
}
