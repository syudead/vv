package store

import (
	"context"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// 一覧・検索・フォルダ・グループで集まりを実効の代表の 1 件に畳む
// （specs/030-video-versions/data-model.md §4、research.md R-3・R-4）。

// libraryItemIDs は ListLibrary の全ページの項目を、動画は id、グループはメンバーの id で返す。
func libraryItemIDs(t *testing.T, db *DB, audience domain.Audience, q domain.VideoQuery) []int64 {
	t.Helper()
	var ids []int64
	for _, item := range libraryPages(t, db, audience, q) {
		if item.Group != nil {
			ids = append(ids, memberIDs(*item.Group)...)
			continue
		}
		ids = append(ids, item.Video.ID)
	}
	slices.Sort(ids)
	return ids
}

// listedPage は ListVideos の 1 ページ（全件）を返す。
func listedPage(t *testing.T, db *DB, audience domain.Audience, q domain.VideoQuery) domain.VideoPage {
	t.Helper()
	q.Limit = domain.MaxLimit
	page, err := db.Library().ListVideos(context.Background(), audience, q)
	if err != nil {
		t.Fatal(err)
	}
	return page
}

// A（タグ X）と B（タグ Y）を A を代表に束ねると、ListLibrary・ListVideos に A の 1 件だけが出て
// total が 1 になり、タグ Y で絞っても出ない（受け入れ条件 6）。CountVideos・タグの本数も
// 代表の 1 本で数え、GetVideo は代表以外の B も返す。
func TestBundledVersionsFoldToTheRepresentative(t *testing.T) {
	db, ids := versionFixture(t)
	ctx := context.Background()
	x := attachNamedTag(t, db, "X", ids["a"])
	y := attachNamedTag(t, db, "Y", ids["b"])
	bundle(t, db, ids["a"], ids["a"], ids["b"])

	want := []int64{ids["a"], ids["c"], ids["d"]}
	if got := listedIDs(t, db, domain.VideoQuery{}); !slices.Equal(got, want) {
		t.Errorf("ListVideos = %v, want %v", got, want)
	}
	if got := libraryItemIDs(t, db, domain.AudienceOwner, domain.VideoQuery{Limit: 2}); !slices.Equal(got, want) {
		t.Errorf("ListLibrary = %v, want %v", got, want)
	}
	for _, q := range []domain.VideoQuery{{TagIDs: []int64{x}}} {
		page := listedPage(t, db, domain.AudienceOwner, q)
		if page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != ids["a"] {
			t.Errorf("タグ X の ListVideos = total %d, %+v", page.Total, page.Items)
		}
		library, err := db.Library().ListLibrary(ctx, domain.AudienceOwner, q)
		if err != nil {
			t.Fatal(err)
		}
		if library.Total != 1 || len(library.Items) != 1 || library.Items[0].Video == nil || library.Items[0].Video.ID != ids["a"] {
			t.Errorf("タグ X の ListLibrary = total %d, %+v", library.Total, library.Items)
		}
	}
	if got := listedIDs(t, db, domain.VideoQuery{TagIDs: []int64{y}}); len(got) != 0 {
		t.Errorf("タグ Y で絞った ListVideos = %v, want なし", got)
	}
	if got := libraryItemIDs(t, db, domain.AudienceOwner, domain.VideoQuery{TagIDs: []int64{y}}); len(got) != 0 {
		t.Errorf("タグ Y で絞った ListLibrary = %v, want なし", got)
	}
	if count, err := db.Library().CountVideos(ctx, domain.AudienceOwner, ""); err != nil || count != 3 {
		t.Errorf("CountVideos = %d, %v, want 3", count, err)
	}
	if got := tagVideoCount(t, db, x); got != 1 {
		t.Errorf("タグ X の本数 = %d, want 1", got)
	}
	if got := tagVideoCount(t, db, y); got != 0 {
		t.Errorf("タグ Y の本数 = %d, want 0（B の content_key の行は集まりでは使わない）", got)
	}
	selection, _, err := db.Library().LibraryIDs(ctx, domain.VideoQuery{})
	if err != nil {
		t.Fatal(err)
	}
	ids2 := selection.AllVideoIDs()
	slices.Sort(ids2)
	if !slices.Equal(ids2, want) {
		t.Errorf("LibraryIDs = %v, want %v", ids2, want)
	}
	if b := ownerVideo(t, db, ids["b"]); b.ID != ids["b"] || b.Title != "b" {
		t.Errorf("GetVideo(B) = %+v", b)
	}
}

// B の題名にだけ含まれる語で検索すると、代表 A の 1 件が出る（受け入れ条件 10）。一覧の題名は
// 代表の所在のもので、式は集まりのどれか 1 つの所在で真になれば当たる。
func TestSearchMatchesAnyVersionOfTheBundle(t *testing.T) {
	db := migratedDB(t)
	paths := upsertAll(t, db,
		listingFile(fixturePath("/media/alpha.mp4"), "alpha", "key-a", 1),
		listingFile(fixturePath("/media/bravo extended.mp4"), "bravo extended", "key-b", 2),
		listingFile(fixturePath("/media/charlie.mp4"), "charlie", "key-c", 3),
	)
	a, b := paths[fixturePath("/media/alpha.mp4")], paths[fixturePath("/media/bravo extended.mp4")]
	bundle(t, db, a, a, b)

	for _, query := range []string{"bravo", "extended", "alpha"} {
		page := listedPage(t, db, domain.AudienceOwner, domain.VideoQuery{Query: query})
		if page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != a || page.Items[0].Title != "alpha" {
			t.Errorf("%q の ListVideos = total %d, %+v", query, page.Total, page.Items)
		}
		if got := libraryItemIDs(t, db, domain.AudienceOwner, domain.VideoQuery{Query: query}); !slices.Equal(got, []int64{a}) {
			t.Errorf("%q の ListLibrary = %v", query, got)
		}
	}
	// 式は所在 1 行に対して評価するので、別々のバージョンの題名の語を AND しても当たらない。
	if got := listedIDs(t, db, domain.VideoQuery{Query: "bravo alpha"}); len(got) != 0 {
		t.Errorf("bravo alpha の ListVideos = %v, want なし", got)
	}
	if got := listedIDs(t, db, domain.VideoQuery{Query: "-bravo"}); len(got) != 2 {
		t.Errorf("-bravo の ListVideos = %v, want 2 件（A と C）", got)
	}
	if count, err := db.Library().CountVideos(context.Background(), domain.AudienceOwner, "bravo"); err != nil || count != 1 {
		t.Errorf("CountVideos(bravo) = %d, %v", count, err)
	}
}

// A と B が別のフォルダにあり A が代表のとき、B のフォルダに B は出ず、A のフォルダに A が出る。
// 代表以外だけのフォルダは動画の無いフォルダで、代表以外はフォルダのグループに入らない
// （受け入れ条件 11）。束ねる・外すと同じ取引で索引を作り直す（R-4）。
func TestBundledVersionsLeaveFoldersAndGroups(t *testing.T) {
	db, ids := folderFixture(t,
		sampleFile(fixturePath("/media/A/a.mp4"), "a", "key-a", 1, 0),
		sampleFile(fixturePath("/media/A/c.mp4"), "c", "key-c", 1, 0),
		sampleFile(fixturePath("/media/B/b.mp4"), "b", "key-b", 1, 0),
		sampleFile(fixturePath("/media/B/d.mp4"), "d", "key-d", 1, 0),
		sampleFile(fixturePath("/media/E/e.mp4"), "e", "key-e", 1, 0),
	)
	ctx := context.Background()
	a, b, c, d, e := ids[fixturePath("/media/A/a.mp4")], ids[fixturePath("/media/B/b.mp4")],
		ids[fixturePath("/media/A/c.mp4")], ids[fixturePath("/media/B/d.mp4")], ids[fixturePath("/media/E/e.mp4")]
	rebuildIndexForTest(t, db)
	if !groupedVideos(t, db)[b] {
		t.Fatalf("束ねる前に B がグループに入っていない: %+v", storedGroups(t, db))
	}

	bundle(t, db, a, a, b, e)

	folderIDs := func(dir string) []int64 {
		t.Helper()
		page, err := db.Library().ListFolderVideos(ctx, domain.AudienceOwner, domain.FolderVideoQuery{Dir: fixturePath(dir), Limit: domain.MaxLimit})
		if err != nil {
			t.Fatal(err)
		}
		var out []int64
		for _, item := range page.Items {
			out = append(out, item.ID)
		}
		slices.Sort(out)
		return out
	}
	if got := folderIDs("/media/A"); !slices.Equal(got, []int64{a, c}) {
		t.Errorf("A のフォルダ = %v, want [%d %d]", got, a, c)
	}
	if got := folderIDs("/media/B"); !slices.Equal(got, []int64{d}) {
		t.Errorf("B のフォルダ = %v, want [%d]", got, d)
	}
	if got := folderIDs("/media/E"); len(got) != 0 {
		t.Errorf("E のフォルダ = %v, want なし", got)
	}
	locations, err := db.Library().FolderLocations(ctx, domain.AudienceOwner, fixturePath("/media"))
	if err != nil {
		t.Fatal(err)
	}
	for _, location := range locations {
		if location.VideoID == b || location.VideoID == e {
			t.Errorf("FolderLocations に代表以外の所在がある: %+v", location)
		}
	}
	if len(locations) != 3 {
		t.Errorf("FolderLocations = %d 件, want 3", len(locations))
	}
	if found, err := db.Library().HasFolderLocations(ctx, domain.AudienceOwner, fixturePath("/media/E")); err != nil || found {
		t.Errorf("E のフォルダの HasFolderLocations = %v, %v, want false", found, err)
	}
	grouped := groupedVideos(t, db)
	if grouped[b] || grouped[e] {
		t.Errorf("代表以外のバージョンがグループに入っている: %+v", storedGroups(t, db))
	}
	if names := storedFolderNames(t, db); len(names[b]) != 0 || len(names[e]) != 0 {
		t.Errorf("代表以外のバージョンにフォルダ名がある: %+v", names)
	}
	siblings, err := db.Library().DirectVideoPaths(ctx, domain.AudienceOwner, fixturePath("/media/B"))
	if err != nil {
		t.Fatal(err)
	}
	if len(siblings) != 1 || siblings[0].VideoID != d {
		t.Errorf("B の直下の動画 = %+v", siblings)
	}

	// B を外すと同じ取引で索引を作り直し、B は B のフォルダに戻る。
	if _, err := db.Versions().Unbundle(ctx, b); err != nil {
		t.Fatal(err)
	}
	if got := folderIDs("/media/B"); !slices.Equal(got, []int64{b, d}) {
		t.Errorf("外したあとの B のフォルダ = %v", got)
	}
	if !groupedVideos(t, db)[b] {
		t.Errorf("外したあと B がグループに戻らない: %+v", storedGroups(t, db))
	}
}

// groupedVideos は索引のグループに入っている動画の id の集合を返す。
func groupedVideos(t *testing.T, db *DB) map[int64]bool {
	t.Helper()
	out := map[int64]bool{}
	for _, group := range storedGroups(t, db) {
		for _, member := range group.Members {
			out[member] = true
		}
	}
	return out
}

// 代表を B に替えると 1 件の題名とサムネイル（内容）が B のものになり、タグは変わらない
// （受け入れ条件 8）。
func TestMakeRepresentativeChangesTheListedItem(t *testing.T) {
	db, ids := versionFixture(t)
	ctx := context.Background()
	x := attachNamedTag(t, db, "X", ids["a"])
	bundle(t, db, ids["a"], ids["a"], ids["b"])
	if _, err := db.Versions().MakeRepresentative(ctx, ids["b"]); err != nil {
		t.Fatal(err)
	}

	page := listedPage(t, db, domain.AudienceOwner, domain.VideoQuery{TagIDs: []int64{x}})
	if page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("ListVideos = total %d, %+v", page.Total, page.Items)
	}
	item := page.Items[0]
	if item.ID != ids["b"] || item.Title != "b" || item.ContentKey != "key-b" {
		t.Errorf("一覧の 1 件 = id %d, %q, %q, want B", item.ID, item.Title, item.ContentKey)
	}
	library := libraryItemIDs(t, db, domain.AudienceOwner, domain.VideoQuery{})
	if !slices.Equal(library, []int64{ids["b"], ids["c"], ids["d"]}) {
		t.Errorf("ListLibrary = %v", library)
	}
}

// 代表の所在が消えると残っているバージョンが 1 件として出て、全部消えると出ない（Edge Case
// 「代表のファイルが消えても」）。値は集まりのまま。
func TestRepresentativeFallsBackWhenItsFileIsGone(t *testing.T) {
	db, ids := versionFixture(t)
	x := attachNamedTag(t, db, "X", ids["a"])
	bundle(t, db, ids["a"], ids["a"], ids["b"], ids["c"])
	removeLocations := func(id int64) {
		t.Helper()
		if _, err := db.sql.Exec(`delete from video_locations where video_id = ?`, id); err != nil {
			t.Fatal(err)
		}
	}

	removeLocations(ids["a"])
	if got := listedIDs(t, db, domain.VideoQuery{TagIDs: []int64{x}}); !slices.Equal(got, []int64{ids["b"]}) {
		t.Errorf("代表が消えたあと = %v, want [%d]（残りの id の最小）", got, ids["b"])
	}
	removeLocations(ids["b"])
	removeLocations(ids["c"])
	if got := listedIDs(t, db, domain.VideoQuery{}); !slices.Equal(got, []int64{ids["d"]}) {
		t.Errorf("全部消えたあと = %v, want [%d]", got, ids["d"])
	}
	if got := bundleCount(t, db); got != 1 {
		t.Errorf("集まりの数 = %d, want 1（関係は残る）", got)
	}
}

// ゲストには、公開した集まりが代表の 1 件として見える（受け入れ条件 12）。代表以外の B も
// GetVideo で引ける。
func TestGuestSeesThePublicBundleAsOneItem(t *testing.T) {
	db, ids := versionFixture(t)
	ctx := context.Background()
	bundle(t, db, ids["a"], ids["a"], ids["b"])
	if _, err := db.Visibility().SetVideosPublic(ctx, []int64{ids["a"]}, true); err != nil {
		t.Fatal(err)
	}

	if got := listIDs(t, db, domain.AudienceGuest, domain.VideoQuery{Limit: domain.MaxLimit}); !slices.Equal(got, []int64{ids["a"]}) {
		t.Errorf("ゲストの ListVideos = %v, want [%d]", got, ids["a"])
	}
	if got := libraryItemIDs(t, db, domain.AudienceGuest, domain.VideoQuery{}); !slices.Equal(got, []int64{ids["a"]}) {
		t.Errorf("ゲストの ListLibrary = %v, want [%d]", got, ids["a"])
	}
	if got := listIDs(t, db, domain.AudienceGuest, domain.VideoQuery{Query: "b", Limit: domain.MaxLimit}); !slices.Equal(got, []int64{ids["a"]}) {
		t.Errorf("ゲストの B の題名での検索 = %v, want [%d]", got, ids["a"])
	}
	if b, err := db.Library().GetVideo(ctx, domain.AudienceGuest, ids["b"]); err != nil || b.ID != ids["b"] {
		t.Errorf("ゲストの GetVideo(B) = %+v, %v", b, err)
	}
	related, err := db.Library().VideosByIDs(ctx, domain.AudienceGuest, []int64{ids["a"], ids["b"]})
	if err != nil {
		t.Fatal(err)
	}
	if len(related) != 1 || related[0].ID != ids["a"] {
		t.Errorf("ゲストの VideosByIDs = %+v", related)
	}
}
