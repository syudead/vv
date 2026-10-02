package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"testing"

	"github.com/syudead/vv/internal/httpapi/gen"
)

// 一覧の favorite・favorited* の並び順・GET /api/library/ids の groups
// （specs/035-favorites/contracts/screen-api.md §2・§3）を、本物の認証・保存層・経路をつないで
// 確かめる。中身は library_test.go の libraryFixture と同じ。

// newFavoriteListFixture は show のグループ、p1、solo の順にお気に入りにした
// libraryFixture を作る。お気に入りでない pair のグループに属する p1 は動画の項目になる。
// 手のタグは ep1（既定）に加えて、お気に入りの solo とお気に入りでない p2 に付ける。
func newFavoriteListFixture(t *testing.T) *libraryFixture {
	t.Helper()
	f := newLibraryFixture(t)
	steps := []string{
		favoritesBody(true, nil, gen.VideoFolder{RootId: f.rootID, Path: "show"}),
		favoritesBody(true, []int64{f.ids["p1"]}),
		favoritesBody(true, []int64{f.ids["solo"]}),
	}
	for index, body := range steps {
		if rec := f.putFavorites(body, f.owner); rec.Code != http.StatusOK {
			t.Fatalf("お気に入り %d: status = %d: %s", index, rec.Code, rec.Body)
		}
	}
	if _, _, err := f.env.db.Tags().AttachTagByID(context.Background(), []int64{f.ids["solo"], f.ids["p2"]}, f.manual); err != nil {
		t.Fatal(err)
	}
	return f
}

// orderedLibraryItemNames は項目を「group:名前」か動画の題名にして、応答の順のまま返す。
func orderedLibraryItemNames(page gen.LibraryPage) []string {
	names := make([]string, 0, len(page.Items))
	for _, item := range page.Items {
		if item.Kind == gen.LibraryItemKindGroup {
			names = append(names, "group:"+item.Group.Name)
			continue
		}
		names = append(names, item.Video.Title)
	}
	return names
}

func videoPageTitles(page gen.VideoPage) []string {
	titles := make([]string, 0, len(page.Items))
	for _, video := range page.Items {
		titles = append(titles, video.Title)
	}
	return titles
}

func (f *libraryFixture) libraryPage(t *testing.T, query string) gen.LibraryPage {
	t.Helper()
	rec := f.env.get("/api/library?"+query, f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("/api/library?%s: status = %d: %s", query, rec.Code, rec.Body)
	}
	return decode[gen.LibraryPage](t, rec)
}

func (f *libraryFixture) videoPage(t *testing.T, target string) gen.VideoPage {
	t.Helper()
	rec := f.env.get(target, f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("%s: status = %d: %s", target, rec.Code, rec.Body)
	}
	return decode[gen.VideoPage](t, rec)
}

// 所有者の GET /api/library?favorite=true はお気に入りの項目だけを返し、total が合う
// （受け入れ条件 4・5）。お気に入りでないグループに属するお気に入りの動画は動画の項目で、
// tag と同時に使うと両方に当たる項目だけになる（受け入れ条件 9）。
func TestListLibraryFavoriteOnly(t *testing.T) {
	f := newFavoriteListFixture(t)

	page := f.libraryPage(t, "favorite=true")
	if names := libraryItemNames(page); !slices.Equal(names, []string{"group:show", "p1", "solo"}) || page.Total != 3 {
		t.Fatalf("お気に入りのみ = %v・total %d", names, page.Total)
	}
	if show := libraryGroupNamed(t, page, "show"); show.Favorite == nil || !*show.Favorite {
		t.Errorf("show の favorite = %v", show.Favorite)
	}

	// 2 件ずつのページでも total は絞り込み後の項目の数である。
	first := f.libraryPage(t, "favorite=true&limit=2")
	if first.Total != 3 || len(first.Items) != 2 || first.NextCursor == nil {
		t.Errorf("1 ページ目 = %d 件・total %d・nextCursor %v", len(first.Items), first.Total, first.NextCursor)
	}

	// 手のタグは ep1（show の一部だけ）・p2・solo にある。お気に入りでもあるのは solo だけ。
	tagged := f.libraryPage(t, "favorite=true&tag="+strconv.FormatInt(f.manual, 10))
	if names := libraryItemNames(tagged); !slices.Equal(names, []string{"solo"}) || tagged.Total != 1 {
		t.Errorf("お気に入りのみ・手のタグ = %v・total %d", names, tagged.Total)
	}
	// フォルダ由来の show のタグは show の全メンバーに当たり、show はお気に入りである。
	folder := f.libraryPage(t, "favorite=true&tag="+strconv.FormatInt(f.folder, 10))
	if names := libraryItemNames(folder); !slices.Equal(names, []string{"group:show"}) {
		t.Errorf("お気に入りのみ・show のタグ = %v", names)
	}

	// favorite=false は絞り込まない。
	if names := libraryItemNames(f.libraryPage(t, "favorite=false")); !slices.Equal(names, []string{"group:pair", "group:show", "solo"}) {
		t.Errorf("favorite=false = %v", names)
	}

	// お気に入りにした日時の順。お気に入りでない項目は向きに関係なく末尾に来る。
	if names := orderedLibraryItemNames(f.libraryPage(t, "favorite=true&sort=favoritedDesc")); !slices.Equal(names, []string{"solo", "p1", "group:show"}) {
		t.Errorf("お気に入りのみ・新しい順 = %v", names)
	}
	for _, sort := range []string{"favoritedAsc", "favoritedDesc"} {
		names := orderedLibraryItemNames(f.libraryPage(t, "sort="+sort))
		if len(names) != 3 || names[2] != "group:pair" {
			t.Errorf("%s = %v, want お気に入りでない pair が末尾", sort, names)
		}
	}
	if names := orderedLibraryItemNames(f.libraryPage(t, "sort=favoritedAsc")); !slices.Equal(names, []string{"group:show", "solo", "group:pair"}) {
		t.Errorf("古い順 = %v", names)
	}
}

// GET /api/library/ids?favorite=true の ids と groups は同じ項目から作り、グループの folder は
// GET /api/library の group.folder と一致する（contracts/screen-api.md §3）。
func TestListLibraryIdsGroups(t *testing.T) {
	f := newFavoriteListFixture(t)

	rec := f.env.get("/api/library/ids?favorite=true", f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	got := decode[gen.VideoIdsResponse](t, rec)
	ids := slices.Clone(got.Ids)
	slices.Sort(ids)
	want := []int64{f.ids["ep1"], f.ids["ep2"], f.ids["ep10"], f.ids["p1"], f.ids["solo"]}
	slices.Sort(want)
	if !slices.Equal(ids, want) {
		t.Errorf("ids = %v, want %v", ids, want)
	}
	if got.Groups == nil || len(*got.Groups) != 1 {
		t.Fatalf("groups = %+v, want show の 1 件", got.Groups)
	}
	group := (*got.Groups)[0]
	show := libraryGroupNamed(t, f.libraryPage(t, "favorite=true"), "show")
	if group.Folder != show.Folder {
		t.Errorf("groups の folder = %+v, want %+v", group.Folder, show.Folder)
	}
	if !slices.Equal(group.VideoIds, show.VideoIds) {
		t.Errorf("groups の videoIds = %v, want %v", group.VideoIds, show.VideoIds)
	}
	for _, id := range group.VideoIds {
		if !slices.Contains(got.Ids, id) {
			t.Errorf("groups のメンバー %d が ids に無い", id)
		}
	}

	// 絞り込みが無ければ show と pair の 2 つ。一部のメンバーだけが当たったグループは入らない。
	all := decode[gen.VideoIdsResponse](t, f.env.get("/api/library/ids", f.owner))
	if all.Groups == nil || len(*all.Groups) != 2 {
		t.Errorf("絞り込みなしの groups = %+v, want 2 件", all.Groups)
	}
	partial := f.env.get("/api/library/ids?query=ep10", f.owner)
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(partial.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if value, ok := raw["groups"]; ok {
		t.Errorf("グループの項目が無いときの groups = %s, want 省略", value)
	}

	// 所有者だけの経路なので、ゲストは条件によらず 401 のまま。
	assertUnauthenticated(t, "ゲストの /api/library/ids?favorite=true", f.env.get("/api/library/ids?favorite=true"))
}

// GET /api/videos と GET /api/folders/{rootId}/videos でも favorite=true と favorited* が効く
// （要件 11、受け入れ条件 8）。動画の一覧はグループのお気に入りに関係なく動画のお気に入りで判定する。
func TestListVideosFavoriteOnly(t *testing.T) {
	f := newFavoriteListFixture(t)

	page := f.videoPage(t, "/api/videos?favorite=true&sort=favoritedDesc")
	if titles := videoPageTitles(page); !slices.Equal(titles, []string{"solo", "p1"}) || page.Total != 2 {
		t.Errorf("/api/videos のお気に入りのみ・新しい順 = %v・total %d", titles, page.Total)
	}
	if titles := videoPageTitles(f.videoPage(t, "/api/videos?sort=favoritedAsc")); len(titles) != 6 || !slices.Equal(titles[:2], []string{"p1", "solo"}) {
		t.Errorf("/api/videos の古い順 = %v, want p1・solo が先頭", titles)
	}

	folder := "/api/folders/" + strconv.FormatInt(f.rootID, 10) + "/videos?scope=subtree&favorite=true"
	if titles := videoPageTitles(f.videoPage(t, folder+"&sort=favoritedDesc")); !slices.Equal(titles, []string{"solo", "p1"}) {
		t.Errorf("フォルダのお気に入りのみ・新しい順 = %v", titles)
	}
	pair := "/api/folders/" + strconv.FormatInt(f.rootID, 10) + "/videos?path=pair&favorite=true"
	if titles := videoPageTitles(f.videoPage(t, pair)); !slices.Equal(titles, []string{"p1"}) {
		t.Errorf("pair のお気に入りのみ = %v", titles)
	}
}

// ゲストは favorite=true と favorited* を、読める 3 経路のどれでも使えない（受け入れ条件 10）。
func TestGuestRejectsFavoriteListConditions(t *testing.T) {
	f := newFavoriteListFixture(t)
	folder := "/api/folders/" + strconv.FormatInt(f.rootID, 10) + "/videos?"
	for _, base := range []string{"/api/videos?", folder, "/api/library?"} {
		for _, query := range []string{"favorite=true", "sort=favoritedAsc", "sort=favoritedDesc"} {
			target := base + query
			rec := f.env.get(target)
			assertErrorBody(t, "ゲストの "+target, rec.Code, rec.Body.Bytes(),
				wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonGuestFilterNotAllowed})
			if owner := f.env.get(target, f.owner); owner.Code != http.StatusOK {
				t.Errorf("所有者の %s: status = %d: %s", target, owner.Code, owner.Body)
			}
		}
		if rec := f.env.get(base + "favorite=false"); rec.Code != http.StatusOK {
			t.Errorf("ゲストの %sfavorite=false: status = %d: %s", base, rec.Code, rec.Body)
		}
	}
}
