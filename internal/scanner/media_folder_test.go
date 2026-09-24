package scanner

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"golang.org/x/text/unicode/norm"
)

func TestCheckMediaFolderReturnsCleanedDirectory(t *testing.T) {
	root := t.TempDir()
	got, err := NewFolderChecker().CheckMediaFolder(root + string(os.PathSeparator) + ".")
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Clean(root) {
		t.Fatalf("path = %q, want %q", got, filepath.Clean(root))
	}
}

func TestCheckMediaFolderPreservesFilesystemUnicodePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), norm.NFD.String("Café"))
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := NewFolderChecker().CheckMediaFolder(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != path {
		t.Fatalf("path = %q, want exact filesystem path %q", got, path)
	}
}

func TestCheckMediaFolderRejectsInvalidPaths(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "file.mp4")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, path := range map[string]string{
		"empty":    "",
		"relative": filepath.Join("relative", "media"),
		"missing":  filepath.Join(root, "missing"),
		"file":     file,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewFolderChecker().CheckMediaFolder(path); !errors.Is(err, domain.ErrInvalidMediaFolder) {
				t.Fatalf("error = %v, want domain.ErrInvalidMediaFolder", err)
			}
		})
	}
}

func TestCheckMediaFolderRejectsSymbolicLinks(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "real")
	child := filepath.Join(real, "child")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symbolic links are unavailable: %v", err)
	}
	for name, path := range map[string]string{
		// 末尾が symlink なら Lstat で断る。
		"last component": link,
		// 末尾は実ディレクトリなので Lstat を通り、EvalSymlinks による途中の段の
		// 検証が働く。
		"parent component": filepath.Join(link, "child"),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewFolderChecker().CheckMediaFolder(path); !errors.Is(err, domain.ErrUnsupportedMediaFolder) {
				t.Fatalf("error = %v, want domain.ErrUnsupportedMediaFolder", err)
			}
		})
	}
}
