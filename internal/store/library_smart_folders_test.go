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

func TestListLibraryShowsRootSmartFoldersAmongItems(t *testing.T) {
	db, ids := itemsFixture(t)
	solo, x, ep1 := ids[fixturePath("/media/solo.mp4")], ids[fixturePath("/media/mixed/x.mp4")], ids[fixturePath("/media/show/ep1.mp4")]
	cat := attachNamedTag(t, db, "猫", solo, x)
	root := newSmartFolder(t, db, "q猫", domain.SmartRule{Tags: []int64{cat}}, domain.SortTitleAsc, 0)
	newSmartFolder(t, db, "子", domain.SmartRule{}, domain.SortTitleAsc, root)

	items := libraryPages(t, db, domain.AudienceOwner, domain.VideoQuery{Sort: domain.SortTitleAsc, Limit: domain.MaxLimit})
	if got, want := itemNames(items), []string{"group:pair", "folder:q猫", "group:show", "solo", "x", "y"}; !slices.Equal(got, want) {
		t.Fatalf("題名の順 = %v, want %v", got, want)
	}
	if got, want := *items[1].SmartFolder, (domain.SmartFolderSummary{ID: root, Name: "q猫"}); got != want {
		t.Errorf("q猫 の要約 = %+v, want %+v", got, want)
	}

	for _, check := range []struct {
		sort  domain.VideoSort
		index int
	}{
		{domain.SortDurationAsc, 4},
		{domain.SortDurationDesc, 0},
		{domain.SortPlayedDesc, 2},
		{domain.SortSizeDesc, 1},
	} {
		if got := libraryOrder(t, db, domain.AudienceOwner, check.sort); got[check.index] != "folder:q猫" {
			t.Errorf("%s = %v, want folder:q猫 at %d", check.sort, got, check.index)
		}
	}

	small := attachNamedTag(t, db, "犬", ep1)
	newSmartFolder(t, db, "犬", domain.SmartRule{Tags: []int64{small}}, domain.SortTitleAsc, 0)
	if got := libraryOrder(t, db, domain.AudienceOwner, domain.SortSizeAsc); len(got) != 7 || got[2] != "folder:犬" || got[5] != "folder:q猫" || got[6] != "y" {
		t.Errorf("大きさの昇順 = %v, want folder:犬 at 2 and folder:q猫 before y", got)
	}
}

func TestListLibraryPlacesEmptySmartFoldersLast(t *testing.T) {
	db, _ := itemsFixture(t)
	newSmartFolder(t, db, "空", domain.SmartRule{Q: "どれにも当たらない"}, domain.SortTitleAsc, 0)
	valueless := map[domain.VideoSort][]string{
		domain.SortDurationAsc: {"group:pair"}, domain.SortDurationDesc: {"group:pair"},
		domain.SortPlayedAsc: {"group:pair", "x"}, domain.SortPlayedDesc: {"group:pair", "x"},
	}
	for _, sort := range allSorts {
		switch sort {
		case domain.SortTitleAsc, domain.SortTitleDesc, domain.SortFavoritedAsc, domain.SortFavoritedDesc, domain.SortRandom:
			continue
		}
		got := libraryOrder(t, db, domain.AudienceOwner, sort)
		index := slices.Index(got, "folder:空")
		for _, after := range got[index+1:] {
			if !slices.Contains(valueless[sort], after) {
				t.Errorf("%s = %v, want folder:空 after every item with a value", sort, got)
				break
			}
		}
	}
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

// Guests must not see folders whose matching videos form no eligible playback items.
func TestListLibraryHidesEmptySmartFoldersFromGuests(t *testing.T) {
	db, ids := itemsFixture(t)
	solo, x := ids[fixturePath("/media/solo.mp4")], ids[fixturePath("/media/mixed/x.mp4")]
	public := []int64{solo, ids[fixturePath("/media/show/ep10.mp4")], ids[fixturePath("/media/show/ep1.mp4")], ids[fixturePath("/media/show/ep2.mp4")]}
	if _, err := db.Visibility().SetVideosPublic(context.Background(), public, true); err != nil {
		t.Fatal(err)
	}
	publicTag := attachNamedTag(t, db, "公開", solo)
	privateTag := attachNamedTag(t, db, "非公開", x)
	newSmartFolder(t, db, "公開", domain.SmartRule{Tags: []int64{publicTag}}, domain.SortTitleAsc, 0)
	newSmartFolder(t, db, "非公開", domain.SmartRule{Tags: []int64{privateTag}}, domain.SortTitleAsc, 0)
	newSmartFolder(t, db, "空", domain.SmartRule{Q: "どれにも当たらない"}, domain.SortTitleAsc, 0)

	folders := func(audience domain.Audience) []string {
		var names []string
		for _, item := range libraryPages(t, db, audience, domain.VideoQuery{Sort: domain.SortTitleAsc, Limit: 1}) {
			if item.SmartFolder != nil {
				names = append(names, item.SmartFolder.Name)
			}
		}
		slices.Sort(names)
		return names
	}
	if got, want := folders(domain.AudienceGuest), []string{"公開"}; !slices.Equal(got, want) {
		t.Errorf("ゲストのフォルダ = %v, want %v", got, want)
	}
	if got, want := folders(domain.AudienceOwner), []string{"公開", "空", "非公開"}; !slices.Equal(got, want) {
		t.Errorf("所有者のフォルダ = %v, want %v", got, want)
	}
}

func TestListLibrarySortsTwinSmartFoldersByTheSameValues(t *testing.T) {
	db, ids := itemsFixture(t)
	cat := attachNamedTag(t, db, "猫", ids[fixturePath("/media/solo.mp4")], ids[fixturePath("/media/mixed/x.mp4")])
	newSmartFolder(t, db, "猫", domain.SmartRule{Tags: []int64{cat}}, domain.SortTitleAsc, 0)
	newSmartFolder(t, db, "同じ猫", domain.SmartRule{Tags: []int64{cat}}, domain.SortTitleAsc, 0)

	got := libraryOrder(t, db, domain.AudienceOwner, domain.SortSizeDesc)
	if len(got) != 7 || got[0] != "y" || !slices.Equal(slices.Sorted(slices.Values(got[1:3])), []string{"folder:同じ猫", "folder:猫"}) {
		t.Errorf("大きさの降順 = %v, want y then both folders", got)
	}
}

func TestListLibraryShowsASmartFolderWithOnlyAMissingTag(t *testing.T) {
	db, ids := itemsFixture(t)
	gone := attachNamedTag(t, db, "消す", ids[fixturePath("/media/solo.mp4")])
	newSmartFolder(t, db, "消す", domain.SmartRule{Tags: []int64{gone}}, domain.SortTitleAsc, 0)
	if err := db.Tags().DeleteTag(context.Background(), gone); err != nil {
		t.Fatal(err)
	}
	items := libraryPages(t, db, domain.AudienceOwner, domain.VideoQuery{Sort: domain.SortSizeDesc, Limit: domain.MaxLimit})
	if got := itemNames(items); len(got) != 6 || got[5] != "folder:消す" {
		t.Errorf("大きさの降順 = %v, want folder:消す last", got)
	}
}
