package store

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func newSmartFolder(t *testing.T, db *DB, name string, rule domain.SmartRule, sort domain.VideoSort, parent int64) int64 {
	t.Helper()
	folder, err := db.SmartFolders().CreateSmartFolder(context.Background(),
		domain.SmartFolderCreate{Name: name, Rule: rule, Sort: sort, ParentID: parent})
	if err != nil {
		t.Fatalf("CreateSmartFolder(%q) error = %v", name, err)
	}
	return folder.ID
}

func smartFolderPages(t *testing.T, db *DB, audience domain.Audience, id int64, limit int) (domain.SmartFolderPage, []domain.LibraryItem) {
	t.Helper()
	var first domain.SmartFolderPage
	var items []domain.LibraryItem
	cursor := ""
	for i := range 50 {
		page, err := db.SmartFolders().SmartFolderContents(context.Background(), audience, id, cursor, limit, "")
		if err != nil {
			t.Fatalf("SmartFolderContents(%d, %q) error = %v", id, cursor, err)
		}
		if i == 0 {
			first = page
		}
		items = append(items, page.Items...)
		if page.NextCursor == "" {
			return first, items
		}
		cursor = page.NextCursor
	}
	t.Fatal("ページが終わらない")
	return first, items
}

func smartItemNames(items []domain.LibraryItem) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		switch {
		case item.SmartFolder != nil:
			out = append(out, "folder:"+item.SmartFolder.Name)
		case item.Group != nil:
			out = append(out, "group:"+item.Group.Name)
		default:
			out = append(out, item.Video.Title)
		}
	}
	return out
}

func smartItemKeys(items []domain.LibraryItem) []int64 {
	keys := make([]int64, 0, len(items))
	for _, item := range items {
		if item.SmartFolder == nil {
			keys = append(keys, itemKey(item))
		}
	}
	return keys
}

func smartFolderNames(t *testing.T, db *DB, audience domain.Audience, id int64) []string {
	t.Helper()
	_, items := smartFolderPages(t, db, audience, id, domain.MaxLimit)
	return smartItemNames(items)
}

func TestSmartFolderFollowsTagsWithoutAnotherWrite(t *testing.T) {
	db, ids := itemsFixture(t)
	ctx := context.Background()
	x := ids[fixturePath("/media/mixed/x.mp4")]
	cat := attachNamedTag(t, db, "猫", ids[fixturePath("/media/solo.mp4")])
	folder := newSmartFolder(t, db, "猫", domain.SmartRule{Tags: []int64{cat}}, domain.SortTitleAsc, 0)

	if got := smartFolderNames(t, db, domain.AudienceOwner, folder); !slices.Equal(got, []string{"solo"}) {
		t.Fatalf("付ける前 = %v, want [solo]", got)
	}
	if _, _, err := db.Tags().AttachTagByID(ctx, []int64{x}, cat); err != nil {
		t.Fatal(err)
	}
	if got := smartFolderNames(t, db, domain.AudienceOwner, folder); !slices.Equal(got, []string{"solo", "x"}) {
		t.Fatalf("付けた後 = %v, want [solo x]", got)
	}
	if !slices.Contains(itemNames(libraryPages(t, db, domain.AudienceOwner, domain.VideoQuery{Limit: domain.MaxLimit})), "x") {
		t.Error("付けた後のライブラリに x が無い")
	}
	if _, _, err := db.Tags().DetachTag(ctx, []int64{x}, cat); err != nil {
		t.Fatal(err)
	}
	if got := smartFolderNames(t, db, domain.AudienceOwner, folder); !slices.Equal(got, []string{"solo"}) {
		t.Fatalf("外した後 = %v, want [solo]", got)
	}
	if !slices.Contains(itemNames(libraryPages(t, db, domain.AudienceOwner, domain.VideoQuery{Limit: domain.MaxLimit})), "x") {
		t.Error("外した後のライブラリに x が無い")
	}
}

func TestSmartFolderShowsAGroupAsTheFilteredLibraryDoes(t *testing.T) {
	db, ids := itemsFixture(t)
	ctx := context.Background()
	p1, p2 := ids[fixturePath("/media/pair/p1.mp4")], ids[fixturePath("/media/pair/p2.mp4")]
	cat := attachNamedTag(t, db, "猫", p1, p2)
	folder := newSmartFolder(t, db, "猫", domain.SmartRule{Tags: []int64{cat}}, domain.SortAddedDesc, 0)

	first, items := smartFolderPages(t, db, domain.AudienceOwner, folder, domain.MaxLimit)
	if got := smartItemNames(items); !slices.Equal(got, []string{"group:pair"}) {
		t.Fatalf("両方に付けた中身 = %v, want [group:pair]", got)
	}
	if got := memberIDs(*items[0].Group); !slices.Equal(got, []int64{p1, p2}) {
		t.Errorf("pair のメンバー = %v", got)
	}
	card := first.SmartFolder
	if got := memberIDs(card.LibraryGroup); !slices.Equal(got, []int64{p1, p2}) {
		t.Errorf("カードの再生順 = %v, want [%d %d]", got, p1, p2)
	}
	if card.ID != folder || card.Name != "猫" || card.SizeBytes != 100 || card.WatchState != domain.WatchStateUnwatched ||
		card.OpenVideoID != p1 || card.Path != "" {
		t.Errorf("カード = %+v", card)
	}

	if _, _, err := db.Tags().DetachTag(ctx, []int64{p2}, cat); err != nil {
		t.Fatal(err)
	}
	if got := smartFolderNames(t, db, domain.AudienceOwner, folder); !slices.Equal(got, []string{"p1"}) {
		t.Fatalf("part1 だけの中身 = %v, want [p1]", got)
	}
}

func TestSmartFolderChildAndsItsParentsRule(t *testing.T) {
	db, ids := itemsFixture(t)
	solo, x, ep2 := ids[fixturePath("/media/solo.mp4")], ids[fixturePath("/media/mixed/x.mp4")], ids[fixturePath("/media/show/ep2.mp4")]
	cat := attachNamedTag(t, db, "猫", solo, x, ep2)
	parent := newSmartFolder(t, db, "猫", domain.SmartRule{Tags: []int64{cat}}, domain.SortTitleAsc, 0)
	newSmartFolder(t, db, "未視聴", domain.SmartRule{Watch: domain.WatchUnwatched}, domain.SortTitleAsc, parent)
	child := newSmartFolder(t, db, "Another", domain.SmartRule{Watch: domain.WatchWatched}, domain.SortTitleAsc, parent)

	first, items := smartFolderPages(t, db, domain.AudienceOwner, parent, 2)
	if got, want := smartItemNames(items), []string{"folder:Another", "folder:未視聴", "ep2", "solo", "x"}; !slices.Equal(got, want) {
		t.Fatalf("親の中身 = %v, want %v", got, want)
	}
	if len(first.Items) != 4 || first.NextCursor == "" {
		t.Errorf("最初のページ = %v（子フォルダ 2 つと項目 2 件）", smartItemNames(first.Items))
	}
	if got, want := *first.Items[0].SmartFolder, (domain.SmartFolderSummary{ID: child, ParentID: parent, Name: "Another"}); got != want {
		t.Errorf("Another の要約 = %+v, want %+v", got, want)
	}

	unwatched := first.Items[1].SmartFolder.ID
	page, items := smartFolderPages(t, db, domain.AudienceOwner, unwatched, domain.MaxLimit)
	if got := smartItemNames(items); !slices.Equal(got, []string{"ep2", "x"}) {
		t.Fatalf("未視聴 の中身 = %v, want [ep2 x]", got)
	}
	if len(page.Trail) != 1 || page.Trail[0] != (domain.GroupRef{ID: parent, Name: "猫"}) {
		t.Errorf("trail = %+v", page.Trail)
	}
	if got := smartFolderNames(t, db, domain.AudienceOwner, child); !slices.Equal(got, []string{"solo"}) {
		t.Errorf("Another の中身 = %v, want [solo]", got)
	}
}

func TestSmartFolderEqualsTheFilteredLibrary(t *testing.T) {
	db, ids := itemsFixture(t)
	var all []int64
	for _, id := range ids {
		all = append(all, id)
	}
	cat := attachNamedTag(t, db, "猫", all...)
	five := 5
	for _, rule := range []domain.SmartRule{
		{},
		{Tags: []int64{cat}},
		{Tags: []int64{cat}, Watch: domain.WatchUnwatched},
		{Watch: domain.WatchInProgress},
		{Q: "ep"},
		{Playable: true},
		{MaxDuration: &five},
	} {
		for _, sort := range []domain.VideoSort{domain.SortAddedDesc, domain.SortTitleAsc, domain.SortDurationAsc} {
			folder := newSmartFolder(t, db, "f", rule, sort, 0)
			q := rule.Query()
			q.Sort, q.Limit = sort, 2
			want := smartItemKeys(libraryPages(t, db, domain.AudienceOwner, q))
			_, items := smartFolderPages(t, db, domain.AudienceOwner, folder, 2)
			if got := smartItemKeys(items); !slices.Equal(got, want) {
				t.Errorf("%+v %s: 中身 = %v, ListLibrary = %v", rule, sort, got, want)
			}
		}
	}
}

func TestSmartFolderCountsABundleOnce(t *testing.T) {
	db, ids := versionFixture(t)
	cat := attachNamedTag(t, db, "猫", ids["a"], ids["b"])
	bundle(t, db, ids["a"], ids["a"], ids["b"])
	folder := newSmartFolder(t, db, "猫", domain.SmartRule{Tags: []int64{cat}}, domain.SortAddedDesc, 0)

	first, items := smartFolderPages(t, db, domain.AudienceOwner, folder, domain.MaxLimit)
	if got := smartItemNames(items); !slices.Equal(got, []string{"a"}) {
		t.Fatalf("中身 = %v, want [a]", got)
	}
	if got := memberIDs(first.SmartFolder.LibraryGroup); !slices.Equal(got, []int64{ids["a"]}) {
		t.Errorf("再生順 = %v, want [%d]", got, ids["a"])
	}
}

func TestSmartFolderVisibility(t *testing.T) {
	db, ids := versionFixture(t)
	ctx := context.Background()
	setPublic(t, db, true, ids["a"], ids["c"])
	cat := attachNamedTag(t, db, "猫", ids["a"], ids["b"])
	dog := attachNamedTag(t, db, "犬", ids["d"])
	cats := newSmartFolder(t, db, "猫", domain.SmartRule{Tags: []int64{cat}}, domain.SortTitleAsc, 0)
	dogs := newSmartFolder(t, db, "犬", domain.SmartRule{Tags: []int64{dog}}, domain.SortTitleAsc, cats)
	byTagName := newSmartFolder(t, db, "名前", domain.SmartRule{Q: "猫"}, domain.SortTitleAsc, 0)
	empty := newSmartFolder(t, db, "空", domain.SmartRule{Q: "no such video"}, domain.SortTitleAsc, 0)

	if got := smartFolderNames(t, db, domain.AudienceGuest, cats); !slices.Equal(got, []string{"a"}) {
		t.Errorf("ゲストの 猫 = %v, want [a]（犬 の子フォルダは見せない）", got)
	}
	if got := smartFolderNames(t, db, domain.AudienceOwner, cats); !slices.Equal(got, []string{"folder:犬", "a", "b"}) {
		t.Errorf("所有者の 猫 = %v", got)
	}
	if got := smartFolderNames(t, db, domain.AudienceGuest, byTagName); !slices.Equal(got, []string{"a"}) {
		t.Errorf("ゲストのタグ名の検索 = %v, want [a]（所有者の意味で照合する）", got)
	}
	for _, id := range []int64{dogs, empty} {
		if _, err := db.SmartFolders().SmartFolderContents(ctx, domain.AudienceGuest, id, "", 0, ""); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("ゲストの %d error = %v, want ErrNotFound", id, err)
		}
	}
	page, err := db.SmartFolders().SmartFolderContents(ctx, domain.AudienceOwner, empty, "", 0, "")
	if err != nil || len(page.Items) != 0 || len(page.SmartFolder.Members) != 0 || page.SmartFolder.OpenVideoID != 0 {
		t.Errorf("所有者の空のフォルダ = %+v, %v", page, err)
	}
	if _, err := db.SmartFolders().SmartFolderContents(ctx, domain.AudienceOwner, 9999, "", 0, ""); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("無いフォルダ error = %v, want ErrNotFound", err)
	}
}

func TestSmartFolderWithAMissingTagListsOnlyItsChildren(t *testing.T) {
	db, ids := versionFixture(t)
	ctx := context.Background()
	cat := attachNamedTag(t, db, "猫", ids["a"], ids["b"])
	parent := newSmartFolder(t, db, "猫", domain.SmartRule{Tags: []int64{cat}}, domain.SortTitleAsc, 0)
	child := newSmartFolder(t, db, "全部", domain.SmartRule{}, domain.SortTitleAsc, parent)
	if err := db.Tags().DeleteTag(ctx, cat); err != nil {
		t.Fatal(err)
	}

	first, items := smartFolderPages(t, db, domain.AudienceOwner, parent, domain.MaxLimit)
	if got := smartItemNames(items); !slices.Equal(got, []string{"folder:全部"}) {
		t.Fatalf("親の中身 = %v, want [folder:全部]", got)
	}
	want := []domain.SmartRuleRef{{Kind: domain.SmartRefTag, ID: cat}}
	if !slices.Equal(first.Missing, want) || len(first.SmartFolder.Members) != 0 {
		t.Errorf("親 missing = %v, 再生順 = %v", first.Missing, memberIDs(first.SmartFolder.LibraryGroup))
	}
	page, items := smartFolderPages(t, db, domain.AudienceOwner, child, domain.MaxLimit)
	if len(items) != 0 || !slices.Equal(page.Missing, want) {
		t.Errorf("子の中身 = %v, missing = %v", smartItemNames(items), page.Missing)
	}
	if _, err := db.SmartFolders().SmartFolderContents(ctx, domain.AudienceGuest, child, "", 0, ""); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("ゲストの子 error = %v, want ErrNotFound", err)
	}
}

// Ancestor and child search expressions must match one location, not different versions in a bundle.
func TestSmartFolderSearchesOfEveryLevelMatchOneLocation(t *testing.T) {
	db := migratedDB(t)
	paths := upsertAll(t, db,
		listingFile(fixturePath("/media/1.mp4"), "犬", "key-1", 1),
		listingFile(fixturePath("/media/2.mp4"), "鳥", "key-2", 2),
		listingFile(fixturePath("/media/3.mp4"), "犬と鳥", "key-3", 3),
	)
	one, two := paths[fixturePath("/media/1.mp4")], paths[fixturePath("/media/2.mp4")]
	bundle(t, db, one, one, two)
	parent := newSmartFolder(t, db, "犬", domain.SmartRule{Q: "犬"}, domain.SortTitleAsc, 0)
	child := newSmartFolder(t, db, "鳥", domain.SmartRule{Q: "鳥"}, domain.SortTitleAsc, parent)

	if got := smartFolderNames(t, db, domain.AudienceOwner, child); !slices.Equal(got, []string{"犬と鳥"}) {
		t.Errorf("子の中身 = %v, want [犬と鳥]", got)
	}
	if got := smartFolderNames(t, db, domain.AudienceOwner, parent); !slices.Equal(got, []string{"folder:鳥", "犬", "犬と鳥"}) {
		t.Errorf("親の中身 = %v, want [folder:鳥 犬 犬と鳥]", got)
	}
}

func TestSmartFolderWithAMissingTagRejectsAnInvalidCursor(t *testing.T) {
	db, ids := versionFixture(t)
	ctx := context.Background()
	cat := attachNamedTag(t, db, "猫", ids["a"])
	folder := newSmartFolder(t, db, "猫", domain.SmartRule{Tags: []int64{cat}}, domain.SortTitleAsc, 0)
	if err := db.Tags().DeleteTag(ctx, cat); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SmartFolders().SmartFolderContents(ctx, domain.AudienceOwner, folder, "not-a-cursor", 10, ""); !errors.Is(err, domain.ErrInvalidCursor) {
		t.Errorf("error = %v, want ErrInvalidCursor", err)
	}
}

func TestSmartFolderPlayOrderExpandsGroups(t *testing.T) {
	db, ids := itemsFixture(t)
	path := func(name string) int64 { return ids[fixturePath(name)] }
	folder := newSmartFolder(t, db, "全部", domain.SmartRule{}, domain.SortTitleAsc, 0)
	page, err := db.SmartFolders().SmartFolderContents(context.Background(), domain.AudienceOwner, folder, "", 2, "")
	if err != nil {
		t.Fatal(err)
	}
	want := []int64{
		path("/media/pair/p1.mp4"), path("/media/pair/p2.mp4"),
		path("/media/show/ep1.mp4"), path("/media/show/ep2.mp4"), path("/media/show/ep10.mp4"),
		path("/media/solo.mp4"), path("/media/mixed/x.mp4"), path("/media/mixed/sub/y.mp4"),
	}
	card := page.SmartFolder
	if got := memberIDs(card.LibraryGroup); !slices.Equal(got, want) {
		t.Fatalf("再生順 = %v, want %v", got, want)
	}
	if card.SizeBytes != 2100 || card.DurationMs == nil || *card.DurationMs != 9100 ||
		card.WatchState != domain.WatchStateInProgress || card.WatchedCount != 2 || card.LastPlayedAt == nil {
		t.Errorf("カード = %+v", card)
	}
	guest, err := db.SmartFolders().SmartFolderContents(context.Background(), domain.AudienceGuest, folder, "", 2, "")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("公開の動画が無いときのゲスト = %+v, %v", guest.SmartFolder, err)
	}
}
