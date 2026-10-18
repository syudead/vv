package store

import (
	"context"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func TestGroupCriteriaFiltersBeforePagingAndPreservesSavedOrder(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	a, b, c, outside := looseVideo(t, db, "alpha"), looseVideo(t, db, "beta"), looseVideo(t, db, "gamma"), looseVideo(t, db, "outside")
	child := createGroup(t, db, "child", domain.GroupAdd{VideoIDs: []int64{c}})
	id := createGroup(t, db, "parent", domain.GroupAdd{VideoIDs: []int64{b, a}, GroupIDs: []int64{child}})
	if _, err := db.Playback().SaveProgress(ctx, "alpha", domain.Progress{Completed: true}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Favorites().SetFavorites(ctx, domain.FavoriteChange{VideoIDs: []int64{b, outside}, GroupIDs: []int64{child}, Favorite: true}); err != nil {
		t.Fatal(err)
	}
	before := groupEntries(t, db, id)
	q := domain.VideoQuery{Watch: domain.WatchUnwatched, FavoriteOnly: true, Sort: domain.SortTitleDesc}
	first, err := db.Library().GroupItems(ctx, domain.AudienceOwner, id, "", 1, q)
	if err != nil {
		t.Fatal(err)
	}
	if names := groupItemNames(first.Items); !slices.Equal(names, []string{"group:child"}) || first.NextCursor == "" {
		t.Fatalf("first = %v, cursor %q", names, first.NextCursor)
	}
	next, err := db.Library().GroupItems(ctx, domain.AudienceOwner, id, first.NextCursor, 1, q)
	if err != nil {
		t.Fatal(err)
	}
	if names := groupItemNames(next.Items); !slices.Equal(names, []string{"beta"}) || next.NextCursor != "" {
		t.Fatalf("next = %v", names)
	}
	if len(next.Group.Members) != 3 {
		t.Fatalf("filter changed summary: %+v", next.Group)
	}
	if _, err := db.Library().GroupItems(ctx, domain.AudienceOwner, id, first.NextCursor, 1, domain.VideoQuery{Sort: domain.SortTitleAsc}); err == nil {
		t.Fatal("cursor for another order accepted")
	}
	cleared, err := db.Library().Group(ctx, domain.AudienceOwner, id, nil, domain.VideoQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if names := groupItemNames(cleared.Items); !slices.Equal(names, []string{"beta", "alpha", "group:child"}) {
		t.Fatalf("saved order = %v", names)
	}
	if after := groupEntries(t, db, id); !slices.Equal(before, after) {
		t.Fatalf("positions changed: %v", after)
	}
}

func TestCollectionSortsPageAllItemsWithStableSeed(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	a, b := looseVideo(t, db, "alpha"), looseVideo(t, db, "beta")
	group := createGroup(t, db, "pair", domain.GroupAdd{VideoIDs: []int64{b, a}})
	for sort := range listOrders {
		q := domain.VideoQuery{Sort: sort, Seed: 37}
		first, err := db.Library().GroupItems(ctx, domain.AudienceOwner, group, "", 1, q)
		if err != nil {
			t.Fatalf("%s: %v", sort, err)
		}
		next, err := db.Library().GroupItems(ctx, domain.AudienceOwner, group, first.NextCursor, 1, q)
		if err != nil {
			t.Fatalf("%s: %v", sort, err)
		}
		if len(first.Items) != 1 || len(next.Items) != 1 || first.Items[0].ID == next.Items[0].ID || next.NextCursor != "" {
			t.Fatalf("%s: %+v / %+v", sort, first.Items, next.Items)
		}
	}
}

func TestSmartFolderTemporaryFiltersRetainRulesAndScope(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	a, b, outside := looseVideo(t, db, "alpha"), looseVideo(t, db, "beta"), looseVideo(t, db, "outside")
	tag := attachNamedTag(t, db, "scope", a, b)
	id := newSmartFolder(t, db, "scope", domain.SmartRule{Tags: []int64{tag}}, domain.SortTitleAsc, 0)
	if _, err := db.Playback().SaveProgress(ctx, "alpha", domain.Progress{Completed: true}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Favorites().SetFavorites(ctx, domain.FavoriteChange{VideoIDs: []int64{b, outside}, Favorite: true}); err != nil {
		t.Fatal(err)
	}
	q := domain.VideoQuery{Watch: domain.WatchUnwatched, FavoriteOnly: true, Sort: domain.SortTitleDesc}
	page, err := db.SmartFolders().SmartFolderContents(ctx, domain.AudienceOwner, id, "", 1, q)
	if err != nil {
		t.Fatal(err)
	}
	if names := smartItemNames(page.Items); !slices.Equal(names, []string{"beta"}) || page.NextCursor != "" {
		t.Fatalf("filtered = %v", names)
	}
	if page.Folder.Sort != domain.SortTitleAsc || len(page.SmartFolder.Members) != 2 || !slices.Equal(page.Folder.Rule.Tags, []int64{tag}) {
		t.Fatalf("settings changed: %+v", page)
	}
	cleared, err := db.SmartFolders().SmartFolderContents(ctx, domain.AudienceOwner, id, "", 10, domain.VideoQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if names := smartItemNames(cleared.Items); !slices.Equal(names, []string{"alpha", "beta"}) {
		t.Fatalf("cleared = %v", names)
	}
}

func TestGroupPlayableFilterUsesOnlyVisibleDescendants(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	playable, blocked, private := looseVideo(t, db, "playable"), looseVideo(t, db, "blocked"), looseVideo(t, db, "private")
	for _, id := range []int64{playable, private} {
		if err := db.Ingest().ApplyProbe(ctx, id, domain.Probe{VideoCodec: "h264"}, domain.Playability{Playable: true}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Visibility().SetVideosPublic(ctx, []int64{playable, blocked}, true); err != nil {
		t.Fatal(err)
	}
	child := createGroup(t, db, "child", domain.GroupAdd{VideoIDs: []int64{blocked, private}})
	parent := createGroup(t, db, "parent", domain.GroupAdd{VideoIDs: []int64{playable}, GroupIDs: []int64{child}})
	for _, tc := range []struct {
		audience domain.Audience
		want     []string
	}{
		{domain.AudienceOwner, []string{"playable", "group:child"}},
		{domain.AudienceGuest, []string{"playable"}},
	} {
		page, err := db.Library().GroupItems(ctx, tc.audience, parent, "", 0, domain.VideoQuery{PlayableOnly: true})
		if err != nil {
			t.Fatal(err)
		}
		if got := groupItemNames(page.Items); !slices.Equal(got, tc.want) || page.NextCursor != "" {
			t.Fatalf("audience %v: items = %v, cursor = %q", tc.audience, got, page.NextCursor)
		}
	}
}
