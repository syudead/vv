package mediafs

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/syudead/vv/internal/domain"
	"golang.org/x/text/unicode/norm"
)

// ListDirectories は、設定画面のディレクトリ選択に path の子ディレクトリを返す。
//
// path は絶対パスでなければ domain.ErrInvalidDirectoryPath で断る。path が無い・
// ディレクトリでない・自身か途中の段が symlink なら domain.ErrDirectoryNotFound、
// 読めなければ domain.ErrDirectoryUnavailable を返す。メディアフォルダの登録
// （CheckMediaFolder）が symlink を断るので、選べないものは並べない。子のうち
// symlink と読めないディレクトリは除き、名前の大文字小文字を区別せずに並べる。
func (FS) ListDirectories(path string) (domain.DirectoryListing, error) {
	if path == "" || !filepath.IsAbs(path) {
		return domain.DirectoryListing{}, domain.ErrInvalidDirectoryPath
	}
	current := filepath.Clean(path)
	info, err := os.Lstat(current)
	if errors.Is(err, fs.ErrNotExist) || err == nil && !info.IsDir() {
		return domain.DirectoryListing{}, domain.ErrDirectoryNotFound
	}
	if err != nil {
		return domain.DirectoryListing{}, domain.ErrDirectoryUnavailable
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return domain.DirectoryListing{}, domain.ErrDirectoryNotFound
	}
	resolved, err := filepath.EvalSymlinks(current)
	if err != nil || !domain.SamePath(current, filepath.Clean(resolved)) {
		return domain.DirectoryListing{}, domain.ErrDirectoryNotFound
	}
	entries, err := os.ReadDir(current)
	if err != nil {
		return domain.DirectoryListing{}, domain.ErrDirectoryUnavailable
	}
	directories := make([]domain.DirectoryEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		entryInfo, err := entry.Info()
		child := filepath.Join(current, entry.Name())
		if err != nil || !entryInfo.IsDir() || !directoryReadable(child) {
			continue
		}
		directories = append(directories, domain.DirectoryEntry{
			Name: norm.NFC.String(entry.Name()), Path: child,
		})
	}
	sort.Slice(directories, func(i, j int) bool {
		left, right := strings.ToLower(directories[i].Name), strings.ToLower(directories[j].Name)
		if left == right {
			return directories[i].Name < directories[j].Name
		}
		return left < right
	})
	listing := domain.DirectoryListing{CurrentPath: current, Directories: directories}
	if parent := filepath.Dir(current); parent != current {
		listing.ParentPath = parent
	}
	return listing, nil
}

// DirectoryRoots は、ディレクトリ選択の最初に並べる根を返す。Windows では
// 存在するドライブ、それ以外では "/" だけである。
func (FS) DirectoryRoots() domain.DirectoryListing {
	directories := []domain.DirectoryEntry{}
	if runtime.GOOS != "windows" {
		directories = append(directories, domain.DirectoryEntry{Name: string(os.PathSeparator), Path: string(os.PathSeparator)})
		return domain.DirectoryListing{Directories: directories}
	}
	for drive := 'A'; drive <= 'Z'; drive++ {
		path := string(drive) + `:\`
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			directories = append(directories, domain.DirectoryEntry{Name: path, Path: path})
		}
	}
	return domain.DirectoryListing{Directories: directories}
}

func directoryReadable(path string) bool {
	dir, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = dir.Close() }()
	_, err = dir.ReadDir(1)
	return err == nil || errors.Is(err, io.EOF)
}
