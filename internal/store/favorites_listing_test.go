package store

import (
	"context"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// Fixture watch states: ep1 watched, ep2 unwatched, ep10 in progress.

func favoriteLibrary(t *testing.T, db *DB, q domain.VideoQuery) []string {
	t.Helper()
	q.FavoriteOnly = true
	names := itemNames(libraryPages(t, db, domain.AudienceOwner, q))
	slices.Sort(names)
	return names
}

func TestListLibraryFavoriteOnlyBuildsItemsPerFavorite(t *testing.T) {
	db, ids := itemsFixture(t)
	ctx := context.Background()
	ep1, ep2, ep10 := ids[fixturePath("/media/show/ep1.mp4")], ids[fixturePath("/media/show/ep2.mp4")], ids[fixturePath("/media/show/ep10.mp4")]
	show := fixturePath("/media/show")

	if names := favoriteLibrary(t, db, domain.VideoQuery{}); len(names) != 0 {
		t.Errorf("お気に入りが無いとき = %v, want 空", names)
	}

	favoriteVideos(t, db, true, ep1)
	page, err := db.Library().ListLibrary(ctx, domain.AudienceOwner, domain.VideoQuery{FavoriteOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].Video == nil || page.Items[0].Video.ID != ep1 {
		t.Errorf("A だけ = total %d・%v, want A の動画の項目 1 件", page.Total, itemNames(page.Items))
	}

	favoriteVideos(t, db, false, ep1)
	setFavorites(t, db, domain.FavoriteChange{GroupIDs: []int64{groupIDOf(t, db, show)}, Favorite: true})
	if names := favoriteLibrary(t, db, domain.VideoQuery{}); !slices.Equal(names, []string{"group:show"}) {
		t.Errorf("G だけ = %v, want [group:show]", names)
	}

	favoriteVideos(t, db, true, ep1)
	if names := favoriteLibrary(t, db, domain.VideoQuery{}); !slices.Equal(names, []string{"group:show"}) {
		t.Errorf("G と A = %v, want [group:show]", names)
	}

	setFavorites(t, db, domain.FavoriteChange{GroupIDs: []int64{groupIDOf(t, db, show)}, Favorite: false})
	favoriteVideos(t, db, true, ep2, ep10)
	if names := favoriteLibrary(t, db, domain.VideoQuery{}); !slices.Equal(names, []string{"ep1", "ep10", "ep2"}) {
		t.Errorf("A・B・C = %v, want [ep1 ep10 ep2]", names)
	}

	if names := favoriteLibrary(t, db, domain.VideoQuery{Watch: domain.WatchUnwatched}); !slices.Equal(names, []string{"ep2"}) {
		t.Errorf("A・B・C の未視聴 = %v, want [ep2]", names)
	}
	setFavorites(t, db, domain.FavoriteChange{GroupIDs: []int64{groupIDOf(t, db, show)}, Favorite: true})
	if names := favoriteLibrary(t, db, domain.VideoQuery{Watch: domain.WatchUnwatched}); len(names) != 0 {
		t.Errorf("G の未視聴 = %v, want 空", names)
	}
	if names := favoriteLibrary(t, db, domain.VideoQuery{Watch: domain.WatchInProgress}); !slices.Equal(names, []string{"group:show"}) {
		t.Errorf("G の途中 = %v, want [group:show]", names)
	}
}

// A partially matched group does not become a card merely because it is favorited.
func TestListLibraryFavoriteOnlyWithTags(t *testing.T) {
	db, ids := itemsFixture(t)
	ctx := context.Background()
	ep1, ep2, ep10 := ids[fixturePath("/media/show/ep1.mp4")], ids[fixturePath("/media/show/ep2.mp4")], ids[fixturePath("/media/show/ep10.mp4")]

	two, err := db.Tags().CreateTag(ctx, "two")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.Tags().AttachTagByID(ctx, []int64{ep2, ep10}, two.ID); err != nil {
		t.Fatal(err)
	}
	setFavorites(t, db, domain.FavoriteChange{VideoIDs: []int64{ep1, ep2}, GroupIDs: []int64{groupIDOf(t, db, fixturePath("/media/show"))}, Favorite: true})
	if names := favoriteLibrary(t, db, domain.VideoQuery{TagIDs: []int64{two.ID}}); !slices.Equal(names, []string{"ep2"}) {
		t.Errorf("タグ two のお気に入り = %v, want [ep2]", names)
	}
}

// A favorite video stays its own item when its group is not a favorite.
func TestListLibraryFavoriteOnlySplitsFavoriteItems(t *testing.T) {
	db, ids := itemsFixture(t)
	setFavorites(t, db, domain.FavoriteChange{
		VideoIDs: []int64{ids[fixturePath("/media/solo.mp4")], ids[fixturePath("/media/pair/p1.mp4")]},
		GroupIDs: []int64{groupIDOf(t, db, fixturePath("/media/show"))}, Favorite: true,
	})
	if names := favoriteLibrary(t, db, domain.VideoQuery{}); !slices.Equal(names, []string{"group:show", "p1", "solo"}) {
		t.Errorf("お気に入りのみ = %v, want [group:show p1 solo]", names)
	}
}

// Group favorites do not affect per-video list filters.
func TestListVideosFavoriteOnly(t *testing.T) {
	db, ids := itemsFixture(t)
	ctx := context.Background()
	ep1, solo := ids[fixturePath("/media/show/ep1.mp4")], ids[fixturePath("/media/solo.mp4")]
	setFavorites(t, db, domain.FavoriteChange{VideoIDs: []int64{ep1, solo}, GroupIDs: []int64{groupIDOf(t, db, fixturePath("/media/pair"))}, Favorite: true})

	got := pageIDs(t, db, domain.VideoQuery{FavoriteOnly: true, Limit: 1})
	slices.Sort(got)
	want := []int64{ep1, solo}
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("ListVideos = %v, want %v", got, want)
	}
	page, err := db.Library().ListVideos(ctx, domain.AudienceOwner, domain.VideoQuery{FavoriteOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 {
		t.Errorf("ListVideos の total = %d, want 2", page.Total)
	}

	folder, err := db.Library().ListFolderVideos(ctx, domain.AudienceOwner, domain.FolderVideoQuery{
		Dir: fixturePath("/media/show"), FavoriteOnly: true, Sort: domain.SortAddedDesc,
	})
	if err != nil {
		t.Fatal(err)
	}
	if folder.Total != 1 || len(folder.Items) != 1 || folder.Items[0].ID != ep1 {
		t.Errorf("ListFolderVideos(show) = total %d・%+v, want ep1 だけ", folder.Total, folder.Items)
	}
	folder, err = db.Library().ListFolderVideos(ctx, domain.AudienceOwner, domain.FolderVideoQuery{
		Dir: fixturePath("/media/pair"), FavoriteOnly: true, Sort: domain.SortAddedDesc,
	})
	if err != nil {
		t.Fatal(err)
	}
	if folder.Total != 0 {
		t.Errorf("ListFolderVideos(pair) = total %d, want 0（グループのお気に入りは動画に効かない）", folder.Total)
	}
}

func TestListLibraryFavoritedOrder(t *testing.T) {
	db, ids := itemsFixture(t)
	solo, x := ids[fixturePath("/media/solo.mp4")], ids[fixturePath("/media/mixed/x.mp4")]
	favoriteVideos(t, db, true, solo)
	favoriteVideos(t, db, true, x)
	setFavorites(t, db, domain.FavoriteChange{GroupIDs: []int64{groupIDOf(t, db, fixturePath("/media/pair"))}, Favorite: true})

	cases := []struct {
		sort domain.VideoSort
		head []string
	}{
		{domain.SortFavoritedDesc, []string{"group:pair", "x", "solo"}},
		{domain.SortFavoritedAsc, []string{"solo", "x", "group:pair"}},
	}
	for _, pageCase := range pagingCases(cases, 2) {
		tc := pageCase.value
		names := itemNames(libraryPages(t, db, domain.AudienceOwner, domain.VideoQuery{Sort: tc.sort, Limit: pageCase.limit}))
		if len(names) != 5 || !slices.Equal(names[:3], tc.head) {
			t.Fatalf("%s limit=%d = %v, want 先頭が %v で全 5 件", tc.sort, pageCase.limit, names, tc.head)
		}
		tail := slices.Clone(names[3:])
		slices.Sort(tail)
		if !slices.Equal(tail, []string{"group:show", "y"}) {
			t.Errorf("%s limit=%d の末尾 = %v, want お気に入りでない show・y", tc.sort, pageCase.limit, tail)
		}
	}

	favoriteVideos(t, db, false, solo)
	favoriteVideos(t, db, true, solo)
	names := itemNames(libraryPages(t, db, domain.AudienceOwner, domain.VideoQuery{Sort: domain.SortFavoritedDesc, Limit: 1}))
	if len(names) == 0 || names[0] != "solo" {
		t.Errorf("付け直した後の favoritedDesc = %v, want 先頭が solo", names)
	}
}

func TestListVideosFavoritedOrderWithFrozenClock(t *testing.T) {
	db, ids := itemsFixture(t)
	freezeFavoriteClock(t, fixedTime)
	solo, x := ids[fixturePath("/media/solo.mp4")], ids[fixturePath("/media/mixed/x.mp4")]
	// Favorite larger IDs first so timestamp order opposes the ID tie breaker.
	first, second := max(solo, x), min(solo, x)
	favoriteVideos(t, db, true, first)
	favoriteVideos(t, db, true, second)

	got := pageIDs(t, db, domain.VideoQuery{Sort: domain.SortFavoritedDesc, Limit: 1})
	if len(got) != len(itemFiles) || got[0] != second || got[1] != first {
		t.Errorf("favoritedDesc = %v, want 先頭が %d・%d で全 %d 件", got, second, first, len(itemFiles))
	}
	got = pageIDs(t, db, domain.VideoQuery{Sort: domain.SortFavoritedAsc, Limit: 1})
	if len(got) != len(itemFiles) || got[0] != first || got[1] != second {
		t.Errorf("favoritedAsc = %v, want 先頭が %d・%d", got, first, second)
	}
}
