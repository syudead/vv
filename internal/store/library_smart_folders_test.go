package store

import (
	"context"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// ライブラリの一番上のスマートフォルダ（specs/052-smart-folders/data-model.md「Library items」、
// research.md R-7）。

// favoriteSmartFolder はフォルダ id をお気に入りにする。お気に入りの書き込みは別の作業が足すので、
// 表に直に書く。
func favoriteSmartFolder(t *testing.T, db *DB, id, at int64) {
	t.Helper()
	if _, err := db.sql.Exec(`insert into smart_folder_favorites (smart_folder_id, favorited_at) values (?, ?)`, id, at); err != nil {
		t.Fatal(err)
	}
}

// libraryFolderIDs は LibraryIDs のスマートフォルダの id を返す。
func libraryFolderIDs(t *testing.T, db *DB, q domain.VideoQuery) []int64 {
	t.Helper()
	selection, _, err := db.Library().LibraryIDs(context.Background(), q)
	if err != nil {
		t.Fatalf("LibraryIDs(%+v) error = %v", q, err)
	}
	return selection.SmartFolderIDs
}

// 絞り込みの無いライブラリは、親の無いフォルダをグループや動画の間の並べ替えの位置に出し、そのフォルダの
// 動画も出す。子フォルダは一番上に出ない。
func TestListLibraryShowsRootSmartFoldersAmongItems(t *testing.T) {
	db, ids := itemsFixture(t)
	solo, x, ep1 := ids[fixturePath("/media/solo.mp4")], ids[fixturePath("/media/mixed/x.mp4")], ids[fixturePath("/media/show/ep1.mp4")]
	cat := attachNamedTag(t, db, "猫", solo, x)
	root := newSmartFolder(t, db, "q猫", domain.SmartRule{Tags: []int64{cat}}, domain.SortTitleAsc, 0)
	newSmartFolder(t, db, "子", domain.SmartRule{Watch: domain.WatchUnwatched}, domain.SortTitleAsc, root)

	items := libraryPages(t, db, domain.AudienceOwner, domain.VideoQuery{Sort: domain.SortTitleAsc, Limit: domain.MaxLimit})
	if got, want := itemNames(items), []string{"group:pair", "folder:q猫", "group:show", "solo", "x", "y"}; !slices.Equal(got, want) {
		t.Fatalf("題名の順 = %v, want %v", got, want)
	}
	card := items[1].SmartFolder
	if got := memberIDs(card.LibraryGroup); !slices.Equal(got, []int64{solo, x}) {
		t.Errorf("q猫 のカードの再生順 = %v, want [%d %d]", got, solo, x)
	}
	if card.ID != root || card.ParentID != 0 || card.SizeBytes != 700 {
		t.Errorf("q猫 のカード = %+v", card)
	}

	// 並べ替えの値はカードの値である。ep1（200）だけのフォルダは、大きさの昇順で 100 の 2 件の後ろ、
	// 600 の前に並ぶ。q猫（700）は同じ値の y より id（負）で前に並ぶ。
	small := attachNamedTag(t, db, "犬", ep1)
	newSmartFolder(t, db, "犬", domain.SmartRule{Tags: []int64{small}}, domain.SortTitleAsc, 0)
	items = libraryPages(t, db, domain.AudienceOwner, domain.VideoQuery{Sort: domain.SortSizeAsc, Limit: domain.MaxLimit})
	if got := itemNames(items); len(got) != 7 || got[2] != "folder:犬" || got[5] != "folder:q猫" || got[6] != "y" {
		t.Errorf("大きさの昇順 = %v, want folder:犬 at 2 and folder:q猫 before y", got)
	}

	if got := libraryFolderIDs(t, db, domain.VideoQuery{}); len(got) != 2 || got[0] != root {
		t.Errorf("LibraryIDs のフォルダ = %v, want [%d 犬]", got, root)
	}
}

// 検索やタグなどの絞り込みではフォルダが出ない。お気に入りのみでは、お気に入りにしたフォルダが深さに
// よらず出る。
func TestListLibraryShowsSmartFoldersOnlyWithoutFilters(t *testing.T) {
	db, ids := itemsFixture(t)
	solo := ids[fixturePath("/media/solo.mp4")]
	cat := attachNamedTag(t, db, "猫", solo)
	root := newSmartFolder(t, db, "猫", domain.SmartRule{Tags: []int64{cat}}, domain.SortTitleAsc, 0)
	child := newSmartFolder(t, db, "見た", domain.SmartRule{Watch: domain.WatchWatched}, domain.SortTitleAsc, root)

	one := 1
	for _, q := range []domain.VideoQuery{
		{Query: "solo"},
		{TagIDs: []int64{cat}},
		{Watch: domain.WatchWatched},
		{PlayableOnly: true},
		{AddedWithinDays: 7},
		{MinDurationSec: &one},
	} {
		q.Limit = domain.MaxLimit
		for _, item := range libraryPages(t, db, domain.AudienceOwner, q) {
			if item.SmartFolder != nil {
				t.Errorf("%+v にフォルダ %s が出た", q, item.SmartFolder.Name)
			}
		}
		if got := libraryFolderIDs(t, db, q); len(got) != 0 {
			t.Errorf("%+v の LibraryIDs のフォルダ = %v, want 空", q, got)
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
	if card := items[0].SmartFolder; card.ParentID != root || !card.Favorite || !slices.Equal(memberIDs(card.LibraryGroup), []int64{solo}) {
		t.Errorf("見た のカード = %+v", card)
	}
	if got := libraryFolderIDs(t, db, favorites); !slices.Equal(got, []int64{child}) {
		t.Errorf("お気に入りのみの LibraryIDs のフォルダ = %v, want [%d]", got, child)
	}
	if got := libraryFolderIDs(t, db, domain.VideoQuery{}); !slices.Equal(got, []int64{root}) {
		t.Errorf("絞り込みなしの LibraryIDs のフォルダ = %v, want [%d]", got, root)
	}
}

// フォルダの項目をまたぐページ送りで、どの並び順でもどの項目も 1 回だけ返る。中身の無いフォルダ
// （値の無いカード）も含める。
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
	for _, sort := range allSorts {
		for _, limit := range []int{1, 2, 3} {
			got := itemNames(libraryPages(t, db, domain.AudienceOwner, domain.VideoQuery{Sort: sort, Seed: 7, Limit: limit}))
			slices.Sort(got)
			if !slices.Equal(got, all) {
				t.Errorf("%s limit=%d = %v, want %v", sort, limit, got, all)
			}
		}
	}
}

// ゲストのライブラリに、見せる動画の無いフォルダは出ない。所有者には出る（research.md R-8）。
func TestListLibraryHidesEmptySmartFoldersFromGuests(t *testing.T) {
	db, ids := itemsFixture(t)
	solo, x := ids[fixturePath("/media/solo.mp4")], ids[fixturePath("/media/mixed/x.mp4")]
	if _, err := db.Visibility().SetVideosPublic(context.Background(), []int64{solo}, true); err != nil {
		t.Fatal(err)
	}
	public := attachNamedTag(t, db, "公開", solo)
	private := attachNamedTag(t, db, "非公開", x)
	newSmartFolder(t, db, "公開", domain.SmartRule{Tags: []int64{public}}, domain.SortTitleAsc, 0)
	newSmartFolder(t, db, "非公開", domain.SmartRule{Tags: []int64{private}}, domain.SortTitleAsc, 0)
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
