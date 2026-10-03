package store

import (
	"context"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// お気に入りのみの絞り込みとお気に入りにした日時の並び順（specs/035-favorites/data-model.md §5、
// research.md R-3・R-4）。fixture のグループ show（G）のメンバーは ep1・ep2・ep10（A・B・C）で、
// 視聴状態は ep1 が見終えた、ep2 が未視聴、ep10 が途中である。

// favoriteLibrary は所有者のお気に入りのみの ListLibrary を最後まで読み、項目の名前を返す。
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

	// 受け入れ条件 4: A だけがお気に入りなら A の動画の項目 1 件で、G の項目は無い。
	favoriteVideos(t, db, true, ep1)
	page, err := db.Library().ListLibrary(ctx, domain.AudienceOwner, domain.VideoQuery{FavoriteOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].Video == nil || page.Items[0].Video.ID != ep1 {
		t.Errorf("A だけ = total %d・%v, want A の動画の項目 1 件", page.Total, itemNames(page.Items))
	}

	// 受け入れ条件 5: G がお気に入りなら G のグループの項目 1 件で、メンバーは動画の項目にならない。
	favoriteVideos(t, db, false, ep1)
	setFavorites(t, db, domain.FavoriteChange{FolderPaths: []string{show}, Favorite: true})
	if names := favoriteLibrary(t, db, domain.VideoQuery{}); !slices.Equal(names, []string{"group:show"}) {
		t.Errorf("G だけ = %v, want [group:show]", names)
	}

	// G と A の両方なら G のグループの項目だけ（A は G の項目に入る）。
	favoriteVideos(t, db, true, ep1)
	if names := favoriteLibrary(t, db, domain.VideoQuery{}); !slices.Equal(names, []string{"group:show"}) {
		t.Errorf("G と A = %v, want [group:show]", names)
	}

	// 要件 9: A・B・C の全部に付けて G に付けないと、3 本の動画の項目。
	setFavorites(t, db, domain.FavoriteChange{FolderPaths: []string{show}, Favorite: false})
	favoriteVideos(t, db, true, ep2, ep10)
	if names := favoriteLibrary(t, db, domain.VideoQuery{}); !slices.Equal(names, []string{"ep1", "ep10", "ep2"}) {
		t.Errorf("A・B・C = %v, want [ep1 ep10 ep2]", names)
	}

	// 要件 8: 視聴状態と組み合わさる。未視聴は ep2 だけ。
	if names := favoriteLibrary(t, db, domain.VideoQuery{Watch: domain.WatchUnwatched}); !slices.Equal(names, []string{"ep2"}) {
		t.Errorf("A・B・C の未視聴 = %v, want [ep2]", names)
	}
	// G がお気に入りなら G は途中なので、未視聴の項目は無い。
	setFavorites(t, db, domain.FavoriteChange{FolderPaths: []string{show}, Favorite: true})
	if names := favoriteLibrary(t, db, domain.VideoQuery{Watch: domain.WatchUnwatched}); len(names) != 0 {
		t.Errorf("G の未視聴 = %v, want 空", names)
	}
	if names := favoriteLibrary(t, db, domain.VideoQuery{Watch: domain.WatchInProgress}); !slices.Equal(names, []string{"group:show"}) {
		t.Errorf("G の途中 = %v, want [group:show]", names)
	}
}

// 受け入れ条件 9: 2 本だけに付いたタグと同時に使うと、当たってお気に入りの動画だけになる。
// 一部のメンバーだけが当たった G は、G がお気に入りでもグループの項目にならない。
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
	setFavorites(t, db, domain.FavoriteChange{VideoIDs: []int64{ep1, ep2}, FolderPaths: []string{fixturePath("/media/show")}, Favorite: true})
	if names := favoriteLibrary(t, db, domain.VideoQuery{TagIDs: []int64{two.ID}}); !slices.Equal(names, []string{"ep2"}) {
		t.Errorf("タグ two のお気に入り = %v, want [ep2]", names)
	}
}

// LibraryIDs は ListLibrary と同じ項目を、動画の項目の id とグループのフォルダ・メンバーに分けて返す。
func TestLibraryIDsSplitsFavoriteItems(t *testing.T) {
	db, ids := itemsFixture(t)
	ctx := context.Background()
	ep1, ep2, ep10 := ids[fixturePath("/media/show/ep1.mp4")], ids[fixturePath("/media/show/ep2.mp4")], ids[fixturePath("/media/show/ep10.mp4")]
	solo, p1 := ids[fixturePath("/media/solo.mp4")], ids[fixturePath("/media/pair/p1.mp4")]
	show := fixturePath("/media/show")

	setFavorites(t, db, domain.FavoriteChange{VideoIDs: []int64{solo, p1}, FolderPaths: []string{show}, Favorite: true})
	selection, _, err := db.Library().LibraryIDs(ctx, domain.VideoQuery{FavoriteOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	videoIDs := slices.Clone(selection.VideoIDs)
	slices.Sort(videoIDs)
	want := []int64{solo, p1}
	slices.Sort(want)
	if !slices.Equal(videoIDs, want) {
		t.Errorf("動画の項目の id = %v, want %v", videoIDs, want)
	}
	if len(selection.Groups) != 1 || selection.Groups[0].Path != show ||
		!slices.Equal(selection.Groups[0].VideoIDs, []int64{ep1, ep2, ep10}) {
		t.Errorf("グループ = %+v, want [{%s [%d %d %d]}]", selection.Groups, show, ep1, ep2, ep10)
	}

	// 絞り込みが無ければ、グループの項目は show と pair、動画の項目はグループに属さない 3 本。
	selection, _, err = db.Library().LibraryIDs(ctx, domain.VideoQuery{})
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, group := range selection.Groups {
		paths = append(paths, group.Path)
	}
	slices.Sort(paths)
	if wantPaths := []string{fixturePath("/media/pair"), show}; !slices.Equal(paths, wantPaths) {
		t.Errorf("グループのフォルダ = %v, want %v", paths, wantPaths)
	}
	if len(selection.VideoIDs) != 3 {
		t.Errorf("動画の項目の id = %v, want 3 本", selection.VideoIDs)
	}

	// 登録フォルダは選択と同じスナップショットから返り、どのグループのフォルダもその下にある。
	roots, err := db.Library().ListMediaFolders(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(selection.Roots, roots) {
		t.Errorf("登録フォルダ = %+v, want %+v", selection.Roots, roots)
	}
	for _, group := range selection.Groups {
		if _, ok := domain.LocateFolder(selection.Roots, group.Path); !ok {
			t.Errorf("グループ %s が登録フォルダの下に無い", group.Path)
		}
	}
}

// 要件 11: ListVideos・ListFolderVideos の FavoriteOnly はお気に入りの動画だけを返す。
// グループのお気に入りは動画ごとの一覧に効かない。
func TestListVideosFavoriteOnly(t *testing.T) {
	db, ids := itemsFixture(t)
	ctx := context.Background()
	ep1, solo := ids[fixturePath("/media/show/ep1.mp4")], ids[fixturePath("/media/solo.mp4")]
	setFavorites(t, db, domain.FavoriteChange{VideoIDs: []int64{ep1, solo}, FolderPaths: []string{fixturePath("/media/pair")}, Favorite: true})

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

// 受け入れ条件 8・Edge Case: favoritedDesc は最後に付けたものが先頭で、お気に入りでない項目は
// 昇順・降順のどちらでも末尾にまとまり、カーソルをまたいでも並びが保たれる。外して付け直すと
// 先頭に来る。
func TestListLibraryFavoritedOrder(t *testing.T) {
	db, ids := itemsFixture(t)
	solo, x := ids[fixturePath("/media/solo.mp4")], ids[fixturePath("/media/mixed/x.mp4")]
	favoriteVideos(t, db, true, solo)
	favoriteVideos(t, db, true, x)
	setFavorites(t, db, domain.FavoriteChange{FolderPaths: []string{fixturePath("/media/pair")}, Favorite: true})

	for _, tc := range []struct {
		sort domain.VideoSort
		head []string
	}{
		{domain.SortFavoritedDesc, []string{"group:pair", "x", "solo"}},
		{domain.SortFavoritedAsc, []string{"solo", "x", "group:pair"}},
	} {
		for _, limit := range []int{1, 2} {
			names := itemNames(libraryPages(t, db, domain.AudienceOwner, domain.VideoQuery{Sort: tc.sort, Limit: limit}))
			if len(names) != 5 || !slices.Equal(names[:3], tc.head) {
				t.Fatalf("%s limit=%d = %v, want 先頭が %v で全 5 件", tc.sort, limit, names, tc.head)
			}
			tail := slices.Clone(names[3:])
			slices.Sort(tail)
			if !slices.Equal(tail, []string{"group:show", "y"}) {
				t.Errorf("%s limit=%d の末尾 = %v, want お気に入りでない show・y", tc.sort, limit, tail)
			}
		}
	}

	// 外して付け直すと先頭に来る。
	favoriteVideos(t, db, false, solo)
	favoriteVideos(t, db, true, solo)
	names := itemNames(libraryPages(t, db, domain.AudienceOwner, domain.VideoQuery{Sort: domain.SortFavoritedDesc, Limit: 1}))
	if len(names) == 0 || names[0] != "solo" {
		t.Errorf("付け直した後の favoritedDesc = %v, want 先頭が solo", names)
	}
}

// 時計を止めて別々の取引で付けても、後に付けたほうが favoritedDesc の先頭に来る。
func TestListVideosFavoritedOrderWithFrozenClock(t *testing.T) {
	db, ids := itemsFixture(t)
	freezeFavoriteClock(t, fixedTime)
	solo, x := ids[fixturePath("/media/solo.mp4")], ids[fixturePath("/media/mixed/x.mp4")]
	// id の決着と逆になるよう、id の大きいほうから付ける。
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
