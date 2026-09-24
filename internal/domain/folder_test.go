package domain

import (
	"errors"
	"path/filepath"
	"slices"
	"testing"
)

func TestValidateFolderPath(t *testing.T) {
	valid := []string{"", "A", "A/B/C", "100% #1 日本語", "a b", "..a", "a.."}
	invalid := []string{"/", "/A", "A/", "A//B", ".", "..", "A/../B", "A/./B", "A\x00B", "\xff"}
	// 区切りが `\` の OS では、段の中の `\` を許すと OS のパスで `..` が解決されて
	// 登録フォルダの外を指す。区切りが `/` の OS では `\` はファイル名の一部である。
	if filepath.Separator == '\\' {
		invalid = append(invalid, `safe\..\outside`, `a\b`)
	} else {
		valid = append(valid, `safe\..\outside`)
	}
	for _, rel := range valid {
		if err := ValidateFolderPath(rel); err != nil {
			t.Errorf("ValidateFolderPath(%q) = %v, want nil", rel, err)
		}
	}
	for _, rel := range invalid {
		if err := ValidateFolderPath(rel); !errors.Is(err, ErrInvalidFolderPath) {
			t.Errorf("ValidateFolderPath(%q) = %v, want ErrInvalidFolderPath", rel, err)
		}
	}
}

func TestCompareNatural(t *testing.T) {
	names := []string{"10", "b", "2", "A", "1", "100% #1 日本語", "a2", "a10", "file01", "file1"}
	slices.SortStableFunc(names, CompareNatural)
	want := []string{"1", "2", "10", "100% #1 日本語", "A", "a2", "a10", "b", "file01", "file1"}
	if !slices.Equal(names, want) {
		t.Fatalf("sorted = %q, want %q", names, want)
	}
	if CompareNatural("99999999999999999999999", "100000000000000000000000") >= 0 {
		t.Error("long digit runs must compare numerically without overflow")
	}
}

func TestFolderName(t *testing.T) {
	root := filepath.FromSlash("/media/movies")
	if got := FolderName(root, ""); got != "movies" {
		t.Errorf("root name = %q", got)
	}
	if got := FolderName(root, "A/B"); got != "B" {
		t.Errorf("child name = %q", got)
	}
	if got := FolderName(string(filepath.Separator), ""); got != string(filepath.Separator) {
		t.Errorf("filesystem root name = %q", got)
	}
}

func location(root string, rel string, videoID int64, thumbnail ThumbnailState) FolderLocation {
	return FolderLocation{
		Path:           filepath.Join(root, filepath.FromSlash(rel)),
		VideoID:        videoID,
		ContentKey:     "key",
		ThumbnailState: thumbnail,
	}
}

func TestSummarizeFolderCountsOnlyDirectChildren(t *testing.T) {
	rootPath := filepath.FromSlash("/media/movies")
	root := MediaFolder{ID: 3, Path: rootPath}
	locations := []FolderLocation{
		location(rootPath, "A/B/C/z.mp4", 3, ThumbnailStateDone),
		location(rootPath, "A/x.mp4", 1, ThumbnailStateDone),
		location(rootPath, "A/B/y.mp4", 2, ThumbnailStateDone),
		location(rootPath, "AB/other.mp4", 4, ThumbnailStateDone),
	}

	listing, ok := SummarizeFolder(root, "A", locations)
	if !ok {
		t.Fatal("A must exist")
	}
	if listing.Folder.Name != "A" || listing.Folder.VideoCount != 1 || listing.Folder.FolderCount != 1 {
		t.Fatalf("folder = %+v", listing.Folder)
	}
	if len(listing.Folders) != 1 {
		t.Fatalf("children = %+v", listing.Folders)
	}
	child := listing.Folders[0]
	if child.Name != "B" || child.Path != "A/B" || child.RootID != 3 || child.RootPath != rootPath {
		t.Errorf("child identity = %+v", child)
	}
	if child.VideoCount != 1 || child.FolderCount != 1 {
		t.Errorf("child counts = %d videos, %d folders", child.VideoCount, child.FolderCount)
	}
	if len(child.Previews) != 1 || child.Previews[0].VideoID != 2 {
		t.Errorf("child previews = %+v, want only y (video 2)", child.Previews)
	}
}

func TestSummarizeFolderPreviewsAndDuplicates(t *testing.T) {
	rootPath := filepath.FromSlash("/media")
	root := MediaFolder{ID: 1, Path: rootPath}
	locations := []FolderLocation{
		location(rootPath, "five/e.mp4", 15, ThumbnailStateDone),
		location(rootPath, "five/a.mp4", 11, ThumbnailStatePending),
		location(rootPath, "five/b.mp4", 12, ThumbnailStateDone),
		location(rootPath, "five/c.mp4", 13, ThumbnailStateDone),
		location(rootPath, "five/d.mp4", 14, ThumbnailStateDone),
		location(rootPath, "five/f.mp4", 16, ThumbnailStateDone),
		location(rootPath, "dup/same-b.mp4", 20, ThumbnailStateDone),
		location(rootPath, "dup/same-a.mp4", 20, ThumbnailStateDone),
		location(rootPath, "only-deeper/inner/d.mp4", 30, ThumbnailStateDone),
	}

	listing, ok := SummarizeFolder(root, "", locations)
	if !ok {
		t.Fatal("root must exist")
	}
	byName := map[string]FolderSummary{}
	for _, folder := range listing.Folders {
		byName[folder.Name] = folder
	}

	five := byName["five"]
	if five.VideoCount != 6 {
		t.Errorf("five videos = %d", five.VideoCount)
	}
	var ids []int64
	for _, preview := range five.Previews {
		ids = append(ids, preview.VideoID)
	}
	if !slices.Equal(ids, []int64{12, 13, 14, 15}) {
		t.Errorf("five previews = %v, want thumbnails only, by path, at most 4", ids)
	}

	if dup := byName["dup"]; dup.VideoCount != 1 || len(dup.Previews) != 1 {
		t.Errorf("dup = %+v, want one video", dup)
	}

	deeper := byName["only-deeper"]
	if deeper.VideoCount != 0 || deeper.FolderCount != 1 || deeper.Previews == nil || len(deeper.Previews) != 0 {
		t.Errorf("only-deeper = %+v, want 0 videos, 1 folder, empty previews", deeper)
	}

	var names []string
	for _, folder := range listing.Folders {
		names = append(names, folder.Name)
	}
	if !slices.Equal(names, []string{"dup", "five", "only-deeper"}) {
		t.Errorf("order = %q", names)
	}
}

func TestSummarizeFolderExistence(t *testing.T) {
	rootPath := filepath.FromSlash("/media")
	root := MediaFolder{ID: 1, Path: rootPath}

	listing, ok := SummarizeFolder(root, "", nil)
	if !ok || listing.Folder.VideoCount != 0 || len(listing.Folders) != 0 || listing.Folders == nil {
		t.Errorf("empty root = %+v, %v; want existing empty folder", listing, ok)
	}
	if _, ok := SummarizeFolder(root, "missing", []FolderLocation{location(rootPath, "other/x.mp4", 1, ThumbnailStateDone)}); ok {
		t.Error("a folder with no locations below it must not exist")
	}
}

func TestSortFoldersNaturalThenRootPath(t *testing.T) {
	folders := []FolderSummary{
		{Name: "movies", RootPath: "/b/movies"},
		{Name: "10"},
		{Name: "movies", RootPath: "/a/movies"},
		{Name: "2"},
	}
	SortFolders(folders)
	got := []string{}
	for _, folder := range folders {
		got = append(got, folder.Name+folder.RootPath)
	}
	want := []string{"2", "10", "movies/a/movies", "movies/b/movies"}
	if !slices.Equal(got, want) {
		t.Errorf("order = %q, want %q", got, want)
	}
}

func TestLocateVideoFolderUnix(t *testing.T) {
	roots := []MediaFolder{{ID: 1, Path: "/media/a"}, {ID: 2, Path: "/media/b/"}, {ID: 3, Path: "/"}}
	cases := []struct {
		roots []MediaFolder
		path  string
		want  VideoFolder
		ok    bool
	}{
		{roots[:2], "/media/a/x.mp4", VideoFolder{RootID: 1, Path: ""}, true},
		{roots[:2], "/media/a/A/B/y.mp4", VideoFolder{RootID: 1, Path: "A/B"}, true},
		{roots[:2], "/media/b/C/z.mp4", VideoFolder{RootID: 2, Path: "C"}, true},
		// 名前の前方が一致するだけの兄弟は含まない。
		{roots[:2], "/media/ab/x.mp4", VideoFolder{}, false},
		// Unix の `\` はファイル名の一部で、大文字小文字も区別する。
		{roots[:2], `/media/a/A\B.mp4`, VideoFolder{RootID: 1, Path: ""}, true},
		{roots[:2], "/MEDIA/a/x.mp4", VideoFolder{}, false},
		{roots[2:], "/srv/x.mp4", VideoFolder{RootID: 3, Path: "srv"}, true},
	}
	for _, tc := range cases {
		got, ok := locateVideoFolderFor(tc.roots, tc.path, false, '/')
		if ok != tc.ok || got != tc.want {
			t.Errorf("locate(%q) = %+v, %v; want %+v, %v", tc.path, got, ok, tc.want, tc.ok)
		}
	}
}

func TestLocateVideoFolderWindows(t *testing.T) {
	roots := []MediaFolder{{ID: 1, Path: `C:\Media`}, {ID: 2, Path: `D:\`}}
	cases := []struct {
		path string
		want VideoFolder
		ok   bool
	}{
		{`C:\Media\A\B\y.mp4`, VideoFolder{RootID: 1, Path: "A/B"}, true},
		{`c:\media/A/x.mp4`, VideoFolder{RootID: 1, Path: "A"}, true},
		{`C:\MediaX\x.mp4`, VideoFolder{}, false},
		{`D:\x.mp4`, VideoFolder{RootID: 2, Path: ""}, true},
	}
	for _, tc := range cases {
		got, ok := locateVideoFolderFor(roots, tc.path, true, '\\')
		if ok != tc.ok || got != tc.want {
			t.Errorf("locate(%q) = %+v, %v; want %+v, %v", tc.path, got, ok, tc.want, tc.ok)
		}
	}
}
