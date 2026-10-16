package store

import (
	"context"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// ライブラリの棚に無い動画の絞り込み（specs/057-home/research.md R-11）。

// setHomeShelves はホームの棚をフォルダ ids にする。
func setHomeShelves(t *testing.T, db *DB, ids ...int64) {
	t.Helper()
	if err := db.Home().SetShelves(context.Background(), ids); err != nil {
		t.Fatalf("SetShelves(%v) error = %v", ids, err)
	}
}

// unshelvedNames は見る人 audience の、棚に無い動画だけのライブラリの項目の名前を題名の順に返す。
func unshelvedNames(t *testing.T, db *DB, audience domain.Audience) []string {
	t.Helper()
	return itemNames(libraryPages(t, db, audience,
		domain.VideoQuery{Unshelved: true, Sort: domain.SortTitleAsc, Limit: 2}))
}

// unshelved は、どの棚の経路の条件にも一致しない動画だけを出す。子の棚は祖先の条件も満たす動画だけを
// 棚に入れ、子の条件だけに一致する動画は棚に無い。ホームに無いフォルダは動画を棚に入れず、フォルダを
// ホームから外すとその動画がまた出る。スマートフォルダの項目は出さない。
func TestListLibraryUnshelvedExcludesVideosOnHomeShelves(t *testing.T) {
	db, ids := itemsFixture(t)
	solo, x, y := ids[fixturePath("/media/solo.mp4")], ids[fixturePath("/media/mixed/x.mp4")], ids[fixturePath("/media/mixed/sub/y.mp4")]
	cat := attachNamedTag(t, db, "猫", solo, x)
	dog := attachNamedTag(t, db, "犬", x, y)
	parent := newSmartFolder(t, db, "猫", domain.SmartRule{Tags: []int64{cat}}, domain.SortTitleAsc, 0)
	child := newSmartFolder(t, db, "猫と犬", domain.SmartRule{Tags: []int64{dog}}, domain.SortTitleAsc, parent)

	if got, want := unshelvedNames(t, db, domain.AudienceOwner), []string{"group:pair", "group:show", "solo", "x", "y"}; !slices.Equal(got, want) {
		t.Errorf("棚の無いホームの棚に無い動画 = %v, want %v", got, want)
	}

	// 子の棚の経路（猫かつ犬）に合うのは x だけで、犬だけの y と猫だけの solo は棚に無い。
	setHomeShelves(t, db, child)
	if got, want := unshelvedNames(t, db, domain.AudienceOwner), []string{"group:pair", "group:show", "solo", "y"}; !slices.Equal(got, want) {
		t.Errorf("子の棚だけのときの棚に無い動画 = %v, want %v", got, want)
	}

	bird := attachNamedTag(t, db, "鳥", ids[fixturePath("/media/pair/p1.mp4")], y)
	shelf := newSmartFolder(t, db, "鳥", domain.SmartRule{Tags: []int64{bird}}, domain.SortTitleAsc, 0)
	setHomeShelves(t, db, child, shelf)
	// pair は p1 だけが棚にあるので、グループの項目にならず p2 が 1 本で出る。
	if got, want := unshelvedNames(t, db, domain.AudienceOwner), []string{"p2", "group:show", "solo"}; !slices.Equal(got, want) {
		t.Errorf("2 つの棚のときの棚に無い動画 = %v, want %v", got, want)
	}

	setHomeShelves(t, db, shelf)
	if got, want := unshelvedNames(t, db, domain.AudienceOwner), []string{"p2", "group:show", "solo", "x"}; !slices.Equal(got, want) {
		t.Errorf("子の棚を外したあとの棚に無い動画 = %v, want %v", got, want)
	}
}

// 名指すタグの無い棚は何にも一致しないので、どの動画も棚に入れない。
func TestListLibraryUnshelvedIgnoresShelvesWithMissingRefs(t *testing.T) {
	db, ids := itemsFixture(t)
	cat := attachNamedTag(t, db, "猫", ids[fixturePath("/media/solo.mp4")])
	shelf := newSmartFolder(t, db, "猫", domain.SmartRule{Tags: []int64{cat}}, domain.SortTitleAsc, 0)
	setHomeShelves(t, db, shelf)
	if err := db.Tags().DeleteTag(context.Background(), cat); err != nil {
		t.Fatal(err)
	}
	if got, want := unshelvedNames(t, db, domain.AudienceOwner), []string{"group:pair", "group:show", "solo", "x", "y"}; !slices.Equal(got, want) {
		t.Errorf("棚に無い動画 = %v, want %v", got, want)
	}
}

// ゲストの棚に無い動画は公開の動画だけで、非公開の動画は棚に無くても出ない（親 Issue の受け入れ条件 8）。
func TestListLibraryUnshelvedShowsGuestsOnlyPublicVideos(t *testing.T) {
	db, ids := itemsFixture(t)
	solo, x, y := ids[fixturePath("/media/solo.mp4")], ids[fixturePath("/media/mixed/x.mp4")], ids[fixturePath("/media/mixed/sub/y.mp4")]
	if _, err := db.Visibility().SetVideosPublic(context.Background(), []int64{solo, y}, true); err != nil {
		t.Fatal(err)
	}
	bird := attachNamedTag(t, db, "鳥", solo, x)
	setHomeShelves(t, db, newSmartFolder(t, db, "鳥", domain.SmartRule{Tags: []int64{bird}}, domain.SortTitleAsc, 0))

	if got, want := unshelvedNames(t, db, domain.AudienceGuest), []string{"y"}; !slices.Equal(got, want) {
		t.Errorf("ゲストの棚に無い動画 = %v, want %v", got, want)
	}
	if got, want := unshelvedNames(t, db, domain.AudienceOwner), []string{"group:pair", "group:show", "y"}; !slices.Equal(got, want) {
		t.Errorf("所有者の棚に無い動画 = %v, want %v", got, want)
	}
}
