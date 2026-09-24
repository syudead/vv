package mediafs

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// mediaTree は /media（登録フォルダ）と、接頭辞だけが一致する /media-other、
// 外側のファイルを作る。
func mediaTree(t *testing.T) (root, inside, other, outside string) {
	t.Helper()
	base := t.TempDir()
	root = filepath.Join(base, "media")
	otherDir := filepath.Join(base, "media-other")
	for _, dir := range []string{root, otherDir, filepath.Join(root, "sub")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	inside = filepath.Join(root, "sub", "movie.mp4")
	other = filepath.Join(otherDir, "movie.mp4")
	outside = filepath.Join(base, "secret.txt")
	for _, file := range []string{inside, other, outside} {
		if err := os.WriteFile(file, []byte("data"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root, inside, other, outside
}

func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symbolic links are unavailable: %v", err)
	}
}

func TestOpenMediaFileAllowsRegularFileInsideRoot(t *testing.T) {
	root, inside, _, _ := mediaTree(t)
	file, info, err := New().OpenMediaFile([]string{root}, inside)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	if !info.Mode().IsRegular() || info.Size() != 4 {
		t.Fatalf("info = %v size %d", info.Mode(), info.Size())
	}
	resolved, err := New().ResolveMediaFile([]string{root}, inside)
	if err != nil || resolved != inside {
		t.Fatalf("ResolveMediaFile = %q, %v; want %q", resolved, err, inside)
	}
}

func TestMediaFileFollowsSymlinkInsideRoot(t *testing.T) {
	root, inside, _, _ := mediaTree(t)
	link := filepath.Join(root, "link.mp4")
	symlink(t, inside, link)
	file, _, err := New().OpenMediaFile([]string{root}, link)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	// 確かめたパスと開くパスを揃えるため、辿った先で開く。
	if file.Name() != inside {
		t.Fatalf("opened %q, want resolved %q", file.Name(), inside)
	}
	if resolved, err := New().ResolveMediaFile([]string{root}, link); err != nil || resolved != inside {
		t.Fatalf("ResolveMediaFile = %q, %v; want %q", resolved, err, inside)
	}
}

func TestMediaFileRejectsWhatMustNotBeOpened(t *testing.T) {
	root, _, other, outside := mediaTree(t)
	escape := filepath.Join(root, "escape.txt")
	symlink(t, outside, escape)
	for _, tc := range []struct {
		name    string
		path    string
		escaped bool
	}{
		{name: "outside path", path: outside},
		{name: "traversal", path: filepath.Join(root, "..", "secret.txt")},
		{name: "prefix-only sibling directory", path: other},
		{name: "symlink escaping the root", path: escape, escaped: true},
		{name: "directory", path: filepath.Join(root, "sub")},
		{name: "root itself", path: root},
		{name: "missing", path: filepath.Join(root, "missing.mp4")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file, _, err := New().OpenMediaFile([]string{root}, tc.path)
			if err == nil {
				_ = file.Close()
				t.Fatalf("OpenMediaFile(%q) succeeded", tc.path)
			}
			if !errors.Is(err, domain.ErrMediaFileUnavailable) {
				t.Fatalf("OpenMediaFile error = %v, want ErrMediaFileUnavailable", err)
			}
			if got := errors.Is(err, domain.ErrMediaFileOutsideRoot); got != tc.escaped {
				t.Fatalf("OpenMediaFile error = %v, outside root = %v, want %v", err, got, tc.escaped)
			}
			if _, err := New().ResolveMediaFile([]string{root}, tc.path); !errors.Is(err, domain.ErrMediaFileUnavailable) {
				t.Fatalf("ResolveMediaFile error = %v, want ErrMediaFileUnavailable", err)
			}
		})
	}
}

func TestMediaFileRejectsDeviceFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("os.DevNull is not a path on Windows")
	}
	if _, err := os.Stat(os.DevNull); err != nil {
		t.Skip("no device file")
	}
	// デバイスファイルを登録フォルダの内側に置けないので、その親を登録フォルダに見立てる。
	root := filepath.Dir(os.DevNull)
	if _, err := New().ResolveMediaFile([]string{root}, os.DevNull); !errors.Is(err, domain.ErrMediaFileUnavailable) {
		t.Fatalf("ResolveMediaFile(%q) error = %v, want ErrMediaFileUnavailable", os.DevNull, err)
	}
	if file, _, err := New().OpenMediaFile([]string{root}, os.DevNull); err == nil {
		_ = file.Close()
		t.Fatalf("OpenMediaFile(%q) succeeded", os.DevNull)
	}
}

func TestMediaFileWithoutRoots(t *testing.T) {
	_, inside, _, _ := mediaTree(t)
	if _, _, err := New().OpenMediaFile(nil, inside); !errors.Is(err, domain.ErrMediaFileUnavailable) {
		t.Fatalf("error = %v, want ErrMediaFileUnavailable", err)
	}
}
