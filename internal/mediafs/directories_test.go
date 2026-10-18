package mediafs

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"golang.org/x/text/unicode/norm"
)

func TestListDirectoriesListsChildDirectoriesSortedIgnoringCase(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"B", "a", "C"} {
		if err := os.Mkdir(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "file.mp4"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := New().ListDirectories(root + string(os.PathSeparator) + ".")
	if err != nil {
		t.Fatal(err)
	}
	if got.CurrentPath != filepath.Clean(root) {
		t.Fatalf("CurrentPath = %q, want %q", got.CurrentPath, filepath.Clean(root))
	}
	if got.ParentPath != filepath.Dir(filepath.Clean(root)) {
		t.Fatalf("ParentPath = %q, want %q", got.ParentPath, filepath.Dir(filepath.Clean(root)))
	}
	var names []string
	for _, entry := range got.Directories {
		names = append(names, entry.Name)
		if want := filepath.Join(filepath.Clean(root), entry.Name); entry.Path != want {
			t.Fatalf("Path of %q = %q, want %q", entry.Name, entry.Path, want)
		}
	}
	if want := []string{"a", "B", "C"}; !slices.Equal(names, want) {
		t.Fatalf("names = %v, want %v", names, want)
	}
}

func TestListDirectoriesReturnsEmptyListForDirectoryWithoutChildren(t *testing.T) {
	got, err := New().ListDirectories(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got.Directories == nil || len(got.Directories) != 0 {
		t.Fatalf("Directories = %#v, want an empty non-nil list", got.Directories)
	}
}

func TestListDirectoriesNormalizesNameButKeepsFilesystemPath(t *testing.T) {
	root := t.TempDir()
	name := norm.NFD.String("Café")
	if err := os.Mkdir(filepath.Join(root, name), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := New().ListDirectories(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Directories) != 1 {
		t.Fatalf("Directories = %v, want one entry", got.Directories)
	}
	entry := got.Directories[0]
	if entry.Name != norm.NFC.String("Café") {
		t.Fatalf("Name = %q, want NFC %q", entry.Name, norm.NFC.String("Café"))
	}
	// Preserve filesystem spelling so returned paths remain directly usable.
	if _, err := os.Stat(entry.Path); err != nil {
		t.Fatalf("Path %q cannot be opened: %v", entry.Path, err)
	}
}

func TestListDirectoriesHasNoParentAtFilesystemRoot(t *testing.T) {
	root := filepath.VolumeName(t.TempDir()) + string(os.PathSeparator)
	got, err := New().ListDirectories(root)
	if err != nil {
		t.Fatal(err)
	}
	if got.CurrentPath != root || got.ParentPath != "" {
		t.Fatalf("listing = %q (parent %q), want %q without parent", got.CurrentPath, got.ParentPath, root)
	}
}

func TestListDirectoriesRejectsInvalidPaths(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "file.mp4")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		path string
		want error
	}{
		"空":         {path: "", want: domain.ErrInvalidDirectoryPath},
		"相対パス":      {path: filepath.Join("relative", "media"), want: domain.ErrInvalidDirectoryPath},
		"存在しない":     {path: filepath.Join(root, "missing"), want: domain.ErrDirectoryNotFound},
		"ディレクトリでない": {path: file, want: domain.ErrDirectoryNotFound},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := New().ListDirectories(tc.path); !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestListDirectoriesRejectsSymbolicLinks(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "real")
	if err := os.MkdirAll(filepath.Join(real, "child"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symbolic links are unavailable: %v", err)
	}
	for name, path := range map[string]string{
		// Lstat rejects a final symlink.
		"末尾の段": link,
		// A real final directory passes Lstat; EvalSymlinks detects a symlink ancestor.
		"途中の段": filepath.Join(link, "child"),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := New().ListDirectories(path); !errors.Is(err, domain.ErrDirectoryNotFound) {
				t.Fatalf("error = %v, want domain.ErrDirectoryNotFound", err)
			}
		})
	}
}

func TestListDirectoriesOmitsSymbolicLinkChildren(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "real"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "real"), filepath.Join(root, "link")); err != nil {
		t.Skipf("symbolic links are unavailable: %v", err)
	}
	got, err := New().ListDirectories(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Directories) != 1 || got.Directories[0].Name != "real" {
		t.Fatalf("Directories = %v, want only the real directory", got.Directories)
	}
}

func TestListDirectoriesReportsUnreadableDirectories(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("permission bits do not deny reads on this platform or user")
	}
	root := t.TempDir()
	unreadable := filepath.Join(root, "unreadable")
	if err := os.MkdirAll(filepath.Join(unreadable, "child"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(unreadable, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(unreadable, 0o755) })

	for name, path := range map[string]string{
		// Lstat succeeds but reading contents fails.
		"中身を読めない": unreadable,
		// An inaccessible parent makes Lstat fail with permission denied.
		"親を辿れない": filepath.Join(unreadable, "child"),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := New().ListDirectories(path); !errors.Is(err, domain.ErrDirectoryUnavailable) {
				t.Fatalf("error = %v, want domain.ErrDirectoryUnavailable", err)
			}
		})
	}

	if err := os.Mkdir(filepath.Join(root, "readable"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := New().ListDirectories(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Directories) != 1 || got.Directories[0].Name != "readable" {
		t.Fatalf("Directories = %v, want only the readable directory", got.Directories)
	}
}

func TestDirectoryRootsListsFilesystemRoots(t *testing.T) {
	got := New().DirectoryRoots()
	if got.CurrentPath != "" || got.ParentPath != "" {
		t.Fatalf("roots listing has a current or parent path: %+v", got)
	}
	if runtime.GOOS != "windows" {
		want := []domain.DirectoryEntry{{Name: "/", Path: "/"}}
		if len(got.Directories) != 1 || got.Directories[0] != want[0] {
			t.Fatalf("Directories = %v, want %v", got.Directories, want)
		}
		return
	}
	// Windows must include the temporary directory's drive.
	drive := filepath.VolumeName(t.TempDir()) + `\`
	for _, entry := range got.Directories {
		if entry.Name != entry.Path {
			t.Fatalf("drive entry %+v has differing name and path", entry)
		}
		if _, err := os.Stat(entry.Path); err != nil {
			t.Fatalf("listed drive %q does not exist: %v", entry.Path, err)
		}
	}
	for _, entry := range got.Directories {
		if entry.Path == drive {
			return
		}
	}
	t.Fatalf("Directories = %v, want to include %q", got.Directories, drive)
}
