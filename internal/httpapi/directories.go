package httpapi

import (
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
	"golang.org/x/text/unicode/norm"
)

func (s *server) ListDirectories(w http.ResponseWriter, _ *http.Request, params gen.ListDirectoriesParams) {
	listing, status, code, err := listDirectories(params.Path)
	if err != nil {
		s.writeError(w, status, code, err.Error())
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, listing, s.logger)
}

func listDirectories(requested *string) (gen.DirectoryListing, int, string, error) {
	if requested == nil {
		return directoryRoots(), 0, "", nil
	}
	if *requested == "" || !filepath.IsAbs(*requested) {
		return gen.DirectoryListing{}, http.StatusBadRequest, codeInvalidRequest, errors.New("絶対pathを指定してください")
	}
	current := norm.NFC.String(filepath.Clean(*requested))
	info, err := os.Lstat(current)
	if errors.Is(err, fs.ErrNotExist) || err == nil && !info.IsDir() {
		return gen.DirectoryListing{}, http.StatusNotFound, codeNotFound, errors.New("ディレクトリが見つかりません")
	}
	if err != nil {
		return gen.DirectoryListing{}, http.StatusBadRequest, codeDirectoryUnavailable, errors.New("ディレクトリを読み取れません")
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return gen.DirectoryListing{}, http.StatusNotFound, codeNotFound, errors.New("ディレクトリが見つかりません")
	}
	resolved, err := filepath.EvalSymlinks(current)
	resolved = norm.NFC.String(filepath.Clean(resolved))
	if err != nil || !domain.PathWithinRoot(current, resolved) || !domain.PathWithinRoot(resolved, current) {
		return gen.DirectoryListing{}, http.StatusNotFound, codeNotFound, errors.New("ディレクトリが見つかりません")
	}
	entries, err := os.ReadDir(current)
	if err != nil {
		return gen.DirectoryListing{}, http.StatusBadRequest, codeDirectoryUnavailable, errors.New("ディレクトリを読み取れません")
	}
	directories := make([]gen.DirectoryEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		entryInfo, err := entry.Info()
		path := filepath.Join(current, entry.Name())
		if err != nil || !entryInfo.IsDir() || !directoryReadable(path) {
			continue
		}
		directories = append(directories, gen.DirectoryEntry{
			Name: entry.Name(), Path: norm.NFC.String(path),
		})
	}
	sort.Slice(directories, func(i, j int) bool {
		left, right := strings.ToLower(directories[i].Name), strings.ToLower(directories[j].Name)
		if left == right {
			return directories[i].Name < directories[j].Name
		}
		return left < right
	})
	listing := gen.DirectoryListing{CurrentPath: &current, Directories: directories}
	if parent := filepath.Dir(current); parent != current {
		listing.ParentPath = &parent
	}
	return listing, 0, "", nil
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

func directoryRoots() gen.DirectoryListing {
	directories := []gen.DirectoryEntry{}
	if runtime.GOOS != "windows" {
		directories = append(directories, gen.DirectoryEntry{Name: string(os.PathSeparator), Path: string(os.PathSeparator)})
		return gen.DirectoryListing{Directories: directories}
	}
	for drive := 'A'; drive <= 'Z'; drive++ {
		path := string(drive) + `:\`
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			directories = append(directories, gen.DirectoryEntry{Name: path, Path: path})
		}
	}
	return gen.DirectoryListing{Directories: directories}
}
