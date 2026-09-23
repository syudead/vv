package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// fakeFolders は所在の一覧を持ち、接頭辞で絞って返す。集計そのものは本物の
// domain.SummarizeFolder が行う。
type fakeFolders struct {
	roots     []domain.MediaFolder
	locations []domain.FolderLocation
	page      domain.VideoPage
	lastQuery domain.FolderVideoQuery
	listErr   error
}

func (f *fakeFolders) ListMediaFolders(context.Context) ([]domain.MediaFolder, error) {
	return f.roots, nil
}

func (f *fakeFolders) FolderLocations(_ context.Context, dir string) ([]domain.FolderLocation, error) {
	var out []domain.FolderLocation
	for _, location := range f.locations {
		if strings.HasPrefix(location.Path, strings.TrimRight(dir, "/")+"/") {
			out = append(out, location)
		}
	}
	return out, nil
}

func (f *fakeFolders) HasFolderLocations(ctx context.Context, dir string) (bool, error) {
	locations, _ := f.FolderLocations(ctx, dir)
	return len(locations) > 0, nil
}

func (f *fakeFolders) ListFolderVideos(_ context.Context, q domain.FolderVideoQuery) (domain.VideoPage, error) {
	f.lastQuery = q
	if f.listErr != nil {
		return domain.VideoPage{}, f.listErr
	}
	page := f.page
	if page.Items == nil {
		page.Items = []domain.Video{}
	}
	return page, nil
}

func folderFixture() *fakeFolders {
	loc := func(path string, id int64, thumbnail domain.ThumbnailState) domain.FolderLocation {
		return domain.FolderLocation{Path: path, VideoID: id, ContentKey: "abcdef0123456789:1", ThumbnailState: thumbnail}
	}
	return &fakeFolders{
		roots: []domain.MediaFolder{
			{ID: 7, Path: "/b/movies"},
			{ID: 3, Path: "/a/movies"},
			{ID: 9, Path: "/empty"},
		},
		locations: []domain.FolderLocation{
			loc("/a/movies/A/x.mp4", 1, domain.ThumbnailStateDone),
			loc("/a/movies/A/B/y.mp4", 2, domain.ThumbnailStateDone),
			loc("/a/movies/A/B/C/z.mp4", 3, domain.ThumbnailStateDone),
			loc("/a/movies/10/w10.mp4", 10, domain.ThumbnailStatePending),
			loc("/a/movies/2/w2.mp4", 11, domain.ThumbnailStatePending),
			loc("/a/movies/1/w1.mp4", 12, domain.ThumbnailStatePending),
			loc("/b/movies/copy.mp4", 1, domain.ThumbnailStateDone),
		},
	}
}

func TestListRootFoldersIncludesEveryRegisteredFolder(t *testing.T) {
	if filepath.Separator != '/' {
		t.Skip("fixture uses slash-separated absolute paths")
	}
	handler := newTestServer(t, Options{Folders: folderFixture()})

	rec := do(t, handler, http.MethodGet, "/api/folders")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("Cache-Control"); got != cacheNoStore {
		t.Errorf("Cache-Control = %q", got)
	}
	listing := decode[gen.RootFolderListing](t, rec)
	var got []string
	for _, folder := range listing.Folders {
		got = append(got, folder.Name+"@"+folder.RootPath)
		if folder.Path != "" {
			t.Errorf("root path = %q, want empty", folder.Path)
		}
	}
	if want := []string{"empty@/empty", "movies@/a/movies", "movies@/b/movies"}; !slices.Equal(got, want) {
		t.Errorf("roots = %q, want %q", got, want)
	}
	if b := listing.Folders[2]; b.RootId != 7 || b.VideoCount != 1 || b.FolderCount != 0 || len(b.Previews) != 1 {
		t.Errorf("/b/movies = %+v", b)
	}
	if empty := listing.Folders[0]; empty.VideoCount != 0 || empty.Previews == nil {
		t.Errorf("/empty = %+v, want zero counts and an empty previews array", empty)
	}
}

func TestGetFolderReturnsDirectChildrenOnly(t *testing.T) {
	if filepath.Separator != '/' {
		t.Skip("fixture uses slash-separated absolute paths")
	}
	handler := newTestServer(t, Options{Folders: folderFixture()})

	rec := do(t, handler, http.MethodGet, "/api/folders/3?path=A")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	listing := decode[gen.FolderListing](t, rec)
	if listing.Folder.Name != "A" || listing.Folder.Path != "A" || listing.Folder.VideoCount != 1 || listing.Folder.FolderCount != 1 {
		t.Errorf("folder = %+v", listing.Folder)
	}
	if len(listing.Folders) != 1 {
		t.Fatalf("children = %+v", listing.Folders)
	}
	b := listing.Folders[0]
	if b.Name != "B" || b.Path != "A/B" || b.VideoCount != 1 || b.FolderCount != 1 {
		t.Errorf("B = %+v", b)
	}
	if len(b.Previews) != 1 || b.Previews[0].VideoId != 2 || b.Previews[0].ThumbnailUrl != "/api/videos/2/thumbnail?v=abcdef012345" {
		t.Errorf("B previews = %+v, want only y", b.Previews)
	}
}

func TestGetFolderOrdersChildrenNaturally(t *testing.T) {
	if filepath.Separator != '/' {
		t.Skip("fixture uses slash-separated absolute paths")
	}
	handler := newTestServer(t, Options{Folders: folderFixture()})

	listing := decode[gen.FolderListing](t, do(t, handler, http.MethodGet, "/api/folders/3"))
	var names []string
	for _, folder := range listing.Folders {
		names = append(names, folder.Name)
	}
	if want := []string{"1", "2", "10", "A"}; !slices.Equal(names, want) {
		t.Errorf("children = %q, want %q", names, want)
	}
	if listing.Folder.Name != "movies" || listing.Folder.RootPath != "/a/movies" {
		t.Errorf("root = %+v", listing.Folder)
	}
}

func TestFolderRoutesRejectInvalidPaths(t *testing.T) {
	handler := newTestServer(t, Options{Folders: folderFixture()})
	for _, path := range []string{"..", "A/../B", "/A", "A/", "A//B", "."} {
		for _, route := range []string{"/api/folders/3", "/api/folders/3/videos"} {
			rec := do(t, handler, http.MethodGet, route+"?path="+url.QueryEscape(path))
			if rec.Code != http.StatusBadRequest {
				t.Errorf("%s path=%q: status = %d, want 400", route, path, rec.Code)
				continue
			}
			if got := decode[gen.Error](t, rec); got.Code != gen.ErrorCodeInvalidRequest {
				t.Errorf("%s path=%q: code = %q", route, path, got.Code)
			}
		}
	}
}

func TestFolderRoutesReturnNotFound(t *testing.T) {
	handler := newTestServer(t, Options{Folders: folderFixture()})
	for _, target := range []string{
		"/api/folders/99",
		"/api/folders/99/videos",
		"/api/folders/3?path=missing",
		"/api/folders/3/videos?path=missing",
		"/api/folders/3?path=" + url.QueryEscape("A/B/C/z.mp4"),
	} {
		rec := do(t, handler, http.MethodGet, target)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, want 404", target, rec.Code)
			continue
		}
		if got := decode[gen.Error](t, rec); got.Code != gen.ErrorCodeNotFound {
			t.Errorf("%s: code = %q", target, got.Code)
		}
	}

	// 登録フォルダ自身は動画が無くても存在する。
	if rec := do(t, handler, http.MethodGet, "/api/folders/9"); rec.Code != http.StatusOK {
		t.Errorf("empty root: status = %d, want 200", rec.Code)
	}
	if rec := do(t, handler, http.MethodGet, "/api/folders/9/videos"); rec.Code != http.StatusOK {
		t.Errorf("empty root videos: status = %d, want 200", rec.Code)
	}
}

func TestListFolderVideosPassesQueryAndReturnsPage(t *testing.T) {
	if filepath.Separator != '/' {
		t.Skip("fixture uses slash-separated absolute paths")
	}
	folders := folderFixture()
	folders.page = domain.VideoPage{
		Items:      []domain.Video{sampleVideo(1, "x")},
		Total:      61,
		NextCursor: "next",
	}
	handler := newTestServer(t, Options{Folders: folders})

	rec := do(t, handler, http.MethodGet, "/api/folders/3/videos?path=A&sort=titleAsc&limit=500&cursor=abc")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if want := (domain.FolderVideoQuery{Dir: "/a/movies/A", Sort: domain.SortTitleAsc, Cursor: "abc", Limit: domain.MaxLimit}); folders.lastQuery != want {
		t.Errorf("query = %+v, want %+v", folders.lastQuery, want)
	}
	page := decode[gen.VideoPage](t, rec)
	if page.Total != 61 || len(page.Items) != 1 || page.Items[0].Title != "x" || page.NextCursor == nil || *page.NextCursor != "next" {
		t.Errorf("page = %+v", page)
	}

	do(t, handler, http.MethodGet, "/api/folders/3/videos")
	if folders.lastQuery.Dir != "/a/movies" || folders.lastQuery.Sort != domain.SortAddedDesc || folders.lastQuery.Limit != domain.DefaultLimit {
		t.Errorf("default query = %+v", folders.lastQuery)
	}
}

func TestListFolderVideosRejectsBadParameters(t *testing.T) {
	folders := folderFixture()
	handler := newTestServer(t, Options{Folders: folders})

	for _, target := range []string{
		"/api/folders/3/videos?sort=sideways",
		"/api/folders/3/videos?limit=0",
	} {
		if rec := do(t, handler, http.MethodGet, target); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", target, rec.Code)
		}
	}

	folders.listErr = domain.ErrInvalidCursor
	if rec := do(t, handler, http.MethodGet, "/api/folders/3/videos?cursor=broken"); rec.Code != http.StatusBadRequest {
		t.Errorf("broken cursor: status = %d, want 400", rec.Code)
	}

	folders.listErr = errors.New("disk on fire")
	if rec := do(t, handler, http.MethodGet, "/api/folders/3/videos"); rec.Code != http.StatusInternalServerError {
		t.Errorf("store failure: status = %d, want 500", rec.Code)
	}
}

func TestFolderRoutesWithoutStoreFailClosed(t *testing.T) {
	handler := newTestServer(t, Options{})
	for _, target := range []string{"/api/folders", "/api/folders/1", "/api/folders/1/videos"} {
		if rec := do(t, handler, http.MethodGet, target); rec.Code != http.StatusInternalServerError {
			t.Errorf("%s: status = %d, want 500", target, rec.Code)
		}
	}
}
