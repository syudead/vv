package store

import (
	"context"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func favoriteSmartFolder(t *testing.T, db *DB, id, at int64) {
	t.Helper()
	if _, err := db.sql.Exec(`insert into smart_folder_favorites (smart_folder_id, favorited_at) values (?, ?)`, id, at); err != nil {
		t.Fatal(err)
	}
}

func libraryOrder(t *testing.T, db *DB, audience domain.Audience, sort domain.VideoSort) []string {
	t.Helper()
	return itemNames(libraryPages(t, db, audience, domain.VideoQuery{Sort: sort, Limit: domain.MaxLimit}))
}

// Smart folders lead the owner's library by name whatever the sort, and the items follow in their own order.
func TestListLibraryLeadsWithRootSmartFoldersByName(t *testing.T) {
	db, ids := itemsFixture(t)
	solo, x := ids[fixturePath("/media/solo.mp4")], ids[fixturePath("/media/mixed/x.mp4")]
	cat := attachNamedTag(t, db, "猫", solo, x)
	root := newSmartFolder(t, db, "q猫", domain.SmartRule{Tags: []int64{cat}}, domain.SortTitleAsc, 0)
	newSmartFolder(t, db, "子", domain.SmartRule{}, domain.SortTitleAsc, root)
	newSmartFolder(t, db, "空", domain.SmartRule{Q: "どれにも当たらない"}, domain.SortTitleAsc, 0)

	items := libraryPages(t, db, domain.AudienceOwner, domain.VideoQuery{Sort: domain.SortTitleAsc, Limit: domain.MaxLimit})
	if got, want := itemNames(items), []string{"folder:q猫", "folder:空", "group:pair", "group:show", "solo", "x", "y"}; !slices.Equal(got, want) {
		t.Fatalf("題名の順 = %v, want %v", got, want)
	}
	if got, want := *items[0].SmartFolder, (domain.SmartFolderSummary{ID: root, Name: "q猫"}); got != want {
		t.Errorf("q猫 の要約 = %+v, want %+v", got, want)
	}
	for _, sort := range []domain.VideoSort{domain.SortSizeDesc, domain.SortDurationAsc, domain.SortPlayedDesc} {
		got := libraryOrder(t, db, domain.AudienceOwner, sort)
		if !slices.Equal(got[:2], []string{"folder:q猫", "folder:空"}) || slices.ContainsFunc(got[2:], isFolderName) {
			t.Errorf("%s = %v, want both folders first by name", sort, got)
		}
	}
}

func isFolderName(name string) bool {
	return len(name) > len("folder:") && name[:len("folder:")] == "folder:"
}

func TestListLibraryShowsSmartFoldersOnlyWithoutFilters(t *testing.T) {
	db, ids := itemsFixture(t)
	solo := ids[fixturePath("/media/solo.mp4")]
	cat := attachNamedTag(t, db, "猫", solo)
	root := newSmartFolder(t, db, "猫", domain.SmartRule{Tags: []int64{cat}}, domain.SortTitleAsc, 0)
	child := newSmartFolder(t, db, "見た", domain.SmartRule{}, domain.SortTitleAsc, root)

	for _, q := range []domain.VideoQuery{
		{Query: "solo"},
		{TagIDs: []int64{cat}},
	} {
		q.Limit = domain.MaxLimit
		for _, item := range libraryPages(t, db, domain.AudienceOwner, q) {
			if item.SmartFolder != nil {
				t.Errorf("%+v にフォルダ %s が出た", q, item.SmartFolder.Name)
			}
		}
	}

	favoriteSmartFolder(t, db, child, 10)
	if _, err := db.sql.Exec(`insert into video_favorites (content_key, favorited_at)
		select content_key, 20 from videos where id = ?`, solo); err != nil {
		t.Fatal(err)
	}
	favorites := domain.VideoQuery{FavoriteOnly: true, Sort: domain.SortFavoritedAsc, Limit: domain.MaxLimit}
	items := libraryPages(t, db, domain.AudienceOwner, favorites)
	if got, want := itemNames(items), []string{"folder:見た", "solo"}; !slices.Equal(got, want) {
		t.Fatalf("お気に入りのみ = %v, want %v", got, want)
	}
	if got, want := *items[0].SmartFolder, (domain.SmartFolderSummary{ID: child, ParentID: root, Name: "見た", Favorite: true}); got != want {
		t.Errorf("見た の要約 = %+v, want %+v", got, want)
	}
}

func TestListLibraryPagesAcrossSmartFolders(t *testing.T) {
	db, ids := itemsFixture(t)
	cat := attachNamedTag(t, db, "猫", ids[fixturePath("/media/solo.mp4")], ids[fixturePath("/media/pair/p2.mp4")])
	newSmartFolder(t, db, "猫", domain.SmartRule{Tags: []int64{cat}}, domain.SortTitleAsc, 0)
	newSmartFolder(t, db, "空", domain.SmartRule{Q: "どれにも当たらない"}, domain.SortTitleAsc, 0)
	newSmartFolder(t, db, "全部", domain.SmartRule{}, domain.SortTitleAsc, 0)

	all := itemNames(libraryPages(t, db, domain.AudienceOwner, domain.VideoQuery{Limit: domain.MaxLimit}))
	slices.Sort(all)
	if want := []string{"folder:全部", "folder:猫", "folder:空", "group:pair", "group:show", "solo", "x", "y"}; !slices.Equal(all, want) {
		t.Fatalf("項目 = %v, want %v", all, want)
	}
	for _, tc := range pagingCases(allSorts, 2, 3) {
		got := itemNames(libraryPages(t, db, domain.AudienceOwner, domain.VideoQuery{Sort: tc.value, Seed: 7, Limit: tc.limit}))
		slices.Sort(got)
		if !slices.Equal(got, all) {
			t.Errorf("%s limit=%d = %v, want %v", tc.value, tc.limit, got, all)
		}
	}
}

func TestListLibraryShowsNoSmartFoldersToGuests(t *testing.T) {
	db, ids := itemsFixture(t)
	solo := ids[fixturePath("/media/solo.mp4")]
	if _, err := db.Visibility().SetVideosPublic(context.Background(), []int64{solo}, true); err != nil {
		t.Fatal(err)
	}
	newSmartFolder(t, db, "公開", domain.SmartRule{Q: "solo"}, domain.SortTitleAsc, 0)

	items := libraryPages(t, db, domain.AudienceGuest, domain.VideoQuery{Sort: domain.SortTitleAsc, Limit: 1})
	if got := itemNames(items); !slices.Equal(got, []string{"solo"}) {
		t.Errorf("ゲストの項目 = %v, want [solo]", got)
	}
}

// A folder whose rule names only a deleted tag matches nothing but still leads the library.
func TestListLibraryShowsASmartFolderWithOnlyAMissingTag(t *testing.T) {
	db, ids := itemsFixture(t)
	gone := attachNamedTag(t, db, "消す", ids[fixturePath("/media/solo.mp4")])
	newSmartFolder(t, db, "消す", domain.SmartRule{Tags: []int64{gone}}, domain.SortTitleAsc, 0)
	if err := db.Tags().DeleteTag(context.Background(), gone); err != nil {
		t.Fatal(err)
	}
	items := libraryPages(t, db, domain.AudienceOwner, domain.VideoQuery{Sort: domain.SortSizeDesc, Limit: domain.MaxLimit})
	if got := itemNames(items); len(got) != 6 || got[0] != "folder:消す" {
		t.Errorf("大きさの降順 = %v, want folder:消す first", got)
	}
}
