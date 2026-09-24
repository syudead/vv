package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// folderFixture は /media の下にフォルダ構成を取り込む。鍵が同じファイルは
// 同じ動画の別の所在になる。
func folderFixture(t *testing.T, files ...VideoFile) (*DB, map[string]int64) {
	t.Helper()
	db := migratedDB(t)
	ids := map[string]int64{}
	for index, file := range files {
		file.MTime = fixedTime.Add(time.Duration(index) * time.Second)
		got, err := db.UpsertVideo(context.Background(), file)
		if err != nil {
			t.Fatalf("取り込めない %s: %v", file.Path, err)
		}
		ids[file.Path] = got.ID
	}
	return db, ids
}

func TestFolderLocationsReturnsEverythingBelowTheFolder(t *testing.T) {
	db, ids := folderFixture(t,
		sampleFile("/media/A/x.mp4", "x", "key-x", 1, 0),
		sampleFile("/media/A/B/y.mp4", "y", "key-y", 1, 0),
		sampleFile("/media/A/B/C/z.mp4", "z", "key-z", 1, 0),
		sampleFile("/media/AB/other.mp4", "other", "key-o", 1, 0),
	)
	ctx := context.Background()
	if err := db.SetThumbnailState(ctx, ids["/media/A/B/y.mp4"], domain.ThumbnailStateDone); err != nil {
		t.Fatal(err)
	}

	locations, err := db.FolderLocations(ctx, "/media/A")
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, location := range locations {
		paths = append(paths, location.Path)
		if location.Path == "/media/A/B/y.mp4" && location.ThumbnailState != domain.ThumbnailStateDone {
			t.Errorf("thumbnail state = %q", location.ThumbnailState)
		}
	}
	slices.Sort(paths)
	if want := []string{"/media/A/B/C/z.mp4", "/media/A/B/y.mp4", "/media/A/x.mp4"}; !slices.Equal(paths, want) {
		t.Errorf("paths = %q, want %q (AB must not match the A prefix)", paths, want)
	}

	listing, ok := domain.SummarizeFolder(domain.MediaFolder{ID: 1, Path: "/media"}, "A", locations)
	if !ok || len(listing.Folders) != 1 || listing.Folders[0].Name != "B" ||
		listing.Folders[0].VideoCount != 1 || listing.Folders[0].FolderCount != 1 ||
		len(listing.Folders[0].Previews) != 1 || listing.Folders[0].Previews[0].VideoID != ids["/media/A/B/y.mp4"] {
		t.Errorf("listing = %+v", listing)
	}
}

func TestFolderLocationsIgnoresUnregisteredLocations(t *testing.T) {
	db, _ := folderFixture(t, sampleFile("/media/A/x.mp4", "x", "key-x", 1, 0))
	ctx := context.Background()
	if _, err := db.SQL().Exec(`update media_folders set path = '/elsewhere'`); err != nil {
		t.Fatal(err)
	}

	locations, err := db.FolderLocations(ctx, "/media")
	if err != nil {
		t.Fatal(err)
	}
	if len(locations) != 0 {
		t.Errorf("locations = %+v, want none outside registered folders", locations)
	}
	found, err := db.HasFolderLocations(ctx, "/media/A")
	if err != nil || found {
		t.Errorf("HasFolderLocations = %v, %v; want false", found, err)
	}
}

func TestHasFolderLocations(t *testing.T) {
	db, _ := folderFixture(t, sampleFile("/media/only-deeper/inner/d.mp4", "d", "key-d", 1, 0))
	ctx := context.Background()
	for dir, want := range map[string]bool{
		"/media/only-deeper":       true,
		"/media/only-deeper/inner": true,
		"/media/only":              false,
		"/media/missing":           false,
	} {
		got, err := db.HasFolderLocations(ctx, dir)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("HasFolderLocations(%q) = %v, want %v", dir, got, want)
		}
	}
}

func TestListFolderVideosReturnsDirectVideosOnly(t *testing.T) {
	db, _ := folderFixture(t,
		sampleFile("/media/A/x.mp4", "x", "key-x", 10, 0),
		sampleFile("/media/A/B/y.mp4", "y", "key-y", 10, 0),
		sampleFile("/media/A/B/C/z.mp4", "z", "key-z", 10, 0),
		sampleFile("/media/AB/other.mp4", "other", "key-o", 10, 0),
	)

	page, err := db.ListFolderVideos(context.Background(), domain.FolderVideoQuery{Dir: "/media/A"})
	if err != nil {
		t.Fatal(err)
	}
	if got := titlesOf(page); !slices.Equal(got, []string{"x"}) || page.Total != 1 {
		t.Errorf("titles = %q, total = %d; want only x", got, page.Total)
	}

	page, err = db.ListFolderVideos(context.Background(), domain.FolderVideoQuery{Dir: "/media"})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 0 || len(page.Items) != 0 {
		t.Errorf("root has no direct videos, got %q", titlesOf(page))
	}
}

func TestListFolderVideosUsesTheLocationInThatFolder(t *testing.T) {
	db, ids := folderFixture(t,
		sampleFile("/media/dup/same-b.mp4", "same-b", "key-same", 20, 0),
		sampleFile("/media/dup/same-a.mp4", "same-a", "key-same", 20, 0),
		sampleFile("/media/other/copy.mp4", "copy", "key-same", 20, 0),
	)
	ctx := context.Background()

	dup, err := db.ListFolderVideos(ctx, domain.FolderVideoQuery{Dir: "/media/dup"})
	if err != nil {
		t.Fatal(err)
	}
	if got := titlesOf(dup); !slices.Equal(got, []string{"same-a"}) || dup.Total != 1 {
		t.Errorf("dup = %q (total %d), want one card titled by the first path", got, dup.Total)
	}

	other, err := db.ListFolderVideos(ctx, domain.FolderVideoQuery{Dir: "/media/other"})
	if err != nil {
		t.Fatal(err)
	}
	if got := titlesOf(other); !slices.Equal(got, []string{"copy"}) {
		t.Errorf("other = %q, want the title of the location in that folder", got)
	}
	if other.Items[0].ID != dup.Items[0].ID || other.Items[0].ID != ids["/media/dup/same-b.mp4"] {
		t.Error("both folders must show the same logical video")
	}
}

func TestListFolderVideosPagesWithCursor(t *testing.T) {
	files := make([]VideoFile, 0, 61)
	for index := range 61 {
		name := fmt.Sprintf("v%02d", index)
		files = append(files, sampleFile("/media/many/"+name+".mp4", name, "key-"+name, 1, 0))
	}
	db, _ := folderFixture(t, files...)
	ctx := context.Background()

	for _, sort := range []domain.VideoSort{domain.SortAddedDesc, domain.SortTitleAsc} {
		var titles []string
		cursor := ""
		for {
			page, err := db.ListFolderVideos(ctx, domain.FolderVideoQuery{Dir: "/media/many", Sort: sort, Cursor: cursor})
			if err != nil {
				t.Fatal(err)
			}
			if page.Total != 61 {
				t.Fatalf("total = %d", page.Total)
			}
			titles = append(titles, titlesOf(page)...)
			if page.NextCursor == "" {
				break
			}
			cursor = page.NextCursor
		}
		if len(titles) != 61 {
			t.Fatalf("%s: got %d videos across pages", sort, len(titles))
		}
		if sort == domain.SortTitleAsc && (titles[0] != "v00" || titles[60] != "v60") {
			t.Errorf("titleAsc order = %s … %s", titles[0], titles[60])
		}
		if sort == domain.SortAddedDesc && titles[0] != "v60" {
			t.Errorf("addedDesc must start from the newest, got %s", titles[0])
		}
	}
}

func TestListFolderVideosRejectsBrokenCursor(t *testing.T) {
	db, _ := folderFixture(t, sampleFile("/media/A/x.mp4", "x", "key-x", 1, 0))
	_, err := db.ListFolderVideos(context.Background(), domain.FolderVideoQuery{Dir: "/media/A", Cursor: "!!"})
	if !errors.Is(err, ErrInvalidCursor) {
		t.Errorf("err = %v, want ErrInvalidCursor", err)
	}
}

// Unix では `\` はファイル名の一部なので、`A\` という名前のフォルダを `A` と
// 取り違えない。
func TestFolderNamesEndingWithBackslashOnUnix(t *testing.T) {
	if os.PathSeparator != '/' {
		t.Skip("`\\` is a path separator on this OS")
	}
	db, _ := folderFixture(t,
		sampleFile("/media/A\\/movie.mp4", "movie", "key-m", 1, 0),
		sampleFile("/media/A/other.mp4", "other", "key-o", 1, 0),
	)
	page, err := db.ListFolderVideos(context.Background(), domain.FolderVideoQuery{Dir: "/media/A\\"})
	if err != nil {
		t.Fatal(err)
	}
	if got := titlesOf(page); !slices.Equal(got, []string{"movie"}) {
		t.Errorf("A\\ = %q, want only movie", got)
	}
	found, err := db.HasFolderLocations(context.Background(), "/media/A\\")
	if err != nil || !found {
		t.Errorf("HasFolderLocations(A\\) = %v, %v; want true", found, err)
	}
}

// TestDirectChildConditionOnWindows は Windows 用の条件を Linux でも実際の SQLite で
// 確かめる。以前は接頭辞の `?` が2回現れ、Windows でだけ引数が足りなくなっていた。
func TestDirectChildConditionOnWindows(t *testing.T) {
	for _, windows := range []bool{false, true} {
		condition := directChildConditionFor("l", windows)
		if got := strings.Count(condition, "?"); got != 1 {
			t.Fatalf("windows=%v: 接頭辞の ? は1つであるべき: %d (%s)", windows, got, condition)
		}
	}

	handle, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = handle.Close() })
	ctx := context.Background()
	if _, err := handle.ExecContext(ctx, `create table l (path text)`); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		`C:\Media\A\x.mp4`,   // 直下
		`C:\Media\A\B\y.mp4`, // 孫（\ 区切り）
		`C:\Media\A\B/z.mp4`, // 孫（/ 区切りが混ざる）
		`C:\Media\AB\w.mp4`,  // 接頭辞が似ているだけの別フォルダ
	} {
		if _, err := handle.ExecContext(ctx, `insert into l (path) values (?)`, path); err != nil {
			t.Fatal(err)
		}
	}

	prefix := `c:\media\a\`
	query := `select path from l where instr(` + folderPathExprFor("l", true) + `, ?) = 1 and ` +
		directChildConditionFor("l", true)
	rows, err := handle.QueryContext(ctx, query, prefix, prefix)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var got []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			t.Fatal(err)
		}
		got = append(got, path)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if want := []string{`C:\Media\A\x.mp4`}; !slices.Equal(got, want) {
		t.Fatalf("直下の所在 = %v, want %v", got, want)
	}
}

// Windows の接頭辞は、SQL の lower() と同じく ASCII の英字だけを小文字にする。
// 非 ASCII の大文字（`Ä`）を含むフォルダでも、直下と配下の所在に一致する。
func TestFolderPrefixOnWindowsFoldsASCIIOnly(t *testing.T) {
	prefix := folderPrefixFor(`C:\Media\Ä`, true)
	if want := `c:\media\Ä\`; prefix != want {
		t.Fatalf("接頭辞 = %q, want %q", prefix, want)
	}

	handle, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = handle.Close() })
	ctx := context.Background()
	if _, err := handle.ExecContext(ctx, `create table l (path text)`); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{`C:\Media\Ä\x.mp4`, `C:\Media\Ä\B\y.mp4`} {
		if _, err := handle.ExecContext(ctx, `insert into l (path) values (?)`, path); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	query := `select count(*) from l where instr(` + folderPathExprFor("l", true) + `, ?) = 1`
	if err := handle.QueryRowContext(ctx, query, prefix).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("配下の所在 = %d 件, want 2 件", count)
	}
}
