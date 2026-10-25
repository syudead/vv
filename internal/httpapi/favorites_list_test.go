package httpapi

import (
	"context"
	"net/http"
	"slices"
	"strconv"
	"testing"

	"github.com/syudead/vv/internal/httpapi/gen"
)

// Favorite p1 belongs to an unfavorited group and must remain a video item; manual tags span favorite and nonfavorite videos.
func newFavoriteListFixture(t *testing.T) *libraryFixture {
	t.Helper()
	f := newLibraryFixture(t)
	steps := []string{
		favoritesBody(true, nil, f.groupID(t, "show")),
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

func TestListLibraryFavoriteOnly(t *testing.T) {
	f := newFavoriteListFixture(t)

	page := f.libraryPage(t, "favorite=true")
	if names := libraryItemNames(page); !slices.Equal(names, []string{"group:show", "p1", "solo"}) || page.Total != 3 {
		t.Fatalf("お気に入りのみ = %v・total %d", names, page.Total)
	}
	if show := libraryGroupNamed(t, page, "show"); show.Favorite == nil || !*show.Favorite {
		t.Errorf("show の favorite = %v", show.Favorite)
	}

	first := f.libraryPage(t, "favorite=true&limit=2")
	if first.Total != 3 || len(first.Items) != 2 || first.NextCursor == nil {
		t.Errorf("1 ページ目 = %d 件・total %d・nextCursor %v", len(first.Items), first.Total, first.NextCursor)
	}

	// Only solo has both the manual tag and favorite state.
	tagged := f.libraryPage(t, "favorite=true&tag="+strconv.FormatInt(f.manual, 10))
	if names := libraryItemNames(tagged); !slices.Equal(names, []string{"solo"}) || tagged.Total != 1 {
		t.Errorf("お気に入りのみ・手のタグ = %v・total %d", names, tagged.Total)
	}
	folder := f.libraryPage(t, "favorite=true&tag="+strconv.FormatInt(f.folder, 10))
	if names := libraryItemNames(folder); !slices.Equal(names, []string{"group:show"}) {
		t.Errorf("お気に入りのみ・show のタグ = %v", names)
	}

	if names := libraryItemNames(f.libraryPage(t, "favorite=false")); !slices.Equal(names, []string{"group:pair", "group:show", "solo"}) {
		t.Errorf("favorite=false = %v", names)
	}

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

func TestListFolderVideosFiltersByTag(t *testing.T) {
	f := newFavoriteListFixture(t)
	folder := "/api/folders/" + strconv.FormatInt(f.rootID, 10) + "/videos?scope=subtree&sort=titleAsc&tag="
	page := f.videoPage(t, folder+strconv.FormatInt(f.manual, 10)+"&tag=999999")
	if titles := videoPageTitles(page); !slices.Equal(titles, []string{"ep1", "p2", "solo"}) || page.Total != 3 {
		t.Errorf("タグで絞ったフォルダ = %v・total %d", titles, page.Total)
	}
	if page.MissingTagIds == nil || !slices.Equal(*page.MissingTagIds, []int64{999999}) {
		t.Errorf("missingTagIds = %v, want [999999]", page.MissingTagIds)
	}
	rec := f.env.get(folder + strconv.FormatInt(f.manual, 10))
	assertErrorBody(t, "ゲストのタグ絞り込み", rec.Code, rec.Body.Bytes(),
		wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonGuestFilterNotAllowed})
}
