package store

import (
	"context"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func TestListVideosFiltersByTagName(t *testing.T) {
	db, ids := sortFixture(t)
	ctx := context.Background()
	cat, err := db.Tags().CreateTag(ctx, "猫")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Tags().AddSynonym(ctx, cat.ID, "ねこ", nil); err != nil {
		t.Fatal(err)
	}
	dog, err := db.Tags().CreateTag(ctx, "犬")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.Tags().AttachTagByID(ctx, []int64{ids[0], ids[1]}, cat.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.Tags().AttachTagByID(ctx, []int64{ids[1], ids[2]}, dog.ID); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		label string
		q     domain.VideoQuery
		want  []int64
	}{
		{"名前", domain.VideoQuery{TagNames: []string{"猫"}}, []int64{ids[0], ids[1]}},
		{"同義語", domain.VideoQuery{TagNames: []string{"ねこ"}}, []int64{ids[0], ids[1]}},
		{"ID との AND", domain.VideoQuery{TagIDs: []int64{dog.ID}, TagNames: []string{"猫"}}, []int64{ids[1]}},
		{"無い名前", domain.VideoQuery{TagNames: []string{"猫", "居ない"}}, nil},
	} {
		page, err := db.Library().ListVideos(ctx, domain.AudienceOwner, tc.q)
		if err != nil {
			t.Fatalf("%s: %v", tc.label, err)
		}
		var got []int64
		for _, video := range page.Items {
			got = append(got, video.ID)
		}
		slices.Sort(got)
		if !slices.Equal(got, tc.want) || page.Total != len(tc.want) {
			t.Errorf("%s: ids = %v, total = %d, want %v", tc.label, got, page.Total, tc.want)
		}
	}
}

func TestNameFiltersNamesTheFiltersForThePeer(t *testing.T) {
	db, _, hanako, makerA, _ := workInfoListingFixture(t)
	ctx := context.Background()
	cat, err := db.Tags().CreateTag(ctx, "猫")
	if err != nil {
		t.Fatal(err)
	}

	named, found, err := db.Library().NameFilters(ctx, domain.VideoQuery{
		TagIDs: []int64{cat.ID, cat.ID + 1000}, Person: hanako.ID, PersonRole: domain.PersonDirector,
		Organization: makerA.ID, Sort: domain.SortTitleAsc,
	})
	if err != nil || !found {
		t.Fatalf("NameFilters: found = %v, err = %v", found, err)
	}
	if !slices.Equal(named.TagNames, []string{"猫"}) || named.TagIDs != nil || named.Person != 0 || named.Organization != 0 ||
		named.PersonName != "山田花子" || named.PersonRole != domain.PersonDirector ||
		named.OrganizationName != "メーカーA" || named.Sort != domain.SortTitleAsc {
		t.Errorf("named = %+v", named)
	}
	if _, found, err := db.Library().NameFilters(ctx, domain.VideoQuery{Person: hanako.ID + 1000}); err != nil || found {
		t.Errorf("unknown person: found = %v, err = %v, want not found", found, err)
	}
}

// Each item's cursor continues the list right after that item, as the node's own next cursor does.
func TestListItemCursorsContinueAfterTheirItem(t *testing.T) {
	db, ids := sortFixture(t)
	ctx := context.Background()
	for _, sort := range allSorts {
		want := expectedOrder(sort, 42, ids)
		page, err := db.Library().ListVideos(ctx, domain.AudienceOwner, domain.VideoQuery{Sort: sort, Seed: 42, Limit: 3})
		if err != nil {
			t.Fatal(err)
		}
		if len(page.ItemCursors) != len(page.Items) {
			t.Fatalf("%s: %d cursors for %d items", sort, len(page.ItemCursors), len(page.Items))
		}
		for i, item := range page.ItemCursors {
			rest := pageIDs(t, db, domain.VideoQuery{Sort: sort, Seed: 42, Limit: 2, Cursor: item.Cursor})
			if !slices.Equal(rest, want[i+1:]) {
				t.Errorf("%s: after item %d = %v, want %v", sort, i, rest, want[i+1:])
			}
		}
		if last := page.ItemCursors[len(page.ItemCursors)-1].Cursor; last != page.NextCursor {
			t.Errorf("%s: the last item's cursor %q differs from the next cursor %q", sort, last, page.NextCursor)
		}
	}

	// A missing value is null, and a title is its sort key.
	page, err := db.Library().ListVideos(ctx, domain.AudienceOwner, domain.VideoQuery{Sort: domain.SortDurationAsc, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	for i, video := range page.Items {
		if (video.DurationMs == nil) != (page.ItemCursors[i].SortValue == nil) {
			t.Errorf("duration %v has sort value %v", video.DurationMs, page.ItemCursors[i].SortValue)
		}
	}
	page, err = db.Library().ListVideos(ctx, domain.AudienceOwner, domain.VideoQuery{Sort: domain.SortTitleAsc, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if value := page.ItemCursors[0].SortValue; value == nil || *value != domain.NaturalSortKey(page.Items[0].Title) {
		t.Errorf("title %q has sort value %v", page.Items[0].Title, value)
	}
}

func TestLibraryItemCursorsContinueAfterTheirItem(t *testing.T) {
	db, _ := itemsFixture(t)
	ctx := context.Background()
	for _, sort := range []domain.VideoSort{domain.SortTitleAsc, domain.SortAddedDesc, domain.SortRandom} {
		q := domain.VideoQuery{Sort: sort, Seed: 7, Limit: 50}
		want := itemNames(libraryPages(t, db, domain.AudienceOwner, q))
		page, err := db.Library().ListLibrary(ctx, domain.AudienceOwner, q)
		if err != nil {
			t.Fatal(err)
		}
		for i, item := range page.ItemCursors {
			q.Cursor = item.Cursor
			after, err := db.Library().ListLibrary(ctx, domain.AudienceOwner, q)
			if err != nil {
				t.Fatal(err)
			}
			if rest := itemNames(after.Items); !slices.Equal(rest, want[i+1:]) {
				t.Errorf("%s: after item %d = %v, want %v", sort, i, rest, want[i+1:])
			}
		}
	}
}
