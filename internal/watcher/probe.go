package watcher

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// NewestFileChange returns the latest modification time among the files directly
// in dir and, when recursive is true, in every directory below it. It reads
// directory entries and the file information the entries carry; it never opens
// a file. Excluded directories and symbolic links are skipped, as when arming.
// A directory that does not exist, or one with no file, gives the zero time.
//
// internal/app uses it to tell whether a copy into a directory has settled
// (specs/042-folder-watch-import/research.md R-3).
func NewestFileChange(dir string, recursive bool) (time.Time, error) {
	return newestFileChange(os.ReadDir, filepath.Clean(dir), recursive)
}

func newestFileChange(
	readDir func(string) ([]fs.DirEntry, error), dir string, recursive bool,
) (time.Time, error) {
	entries, err := readDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	var newest time.Time
	for _, e := range entries {
		if e.Type()&os.ModeSymlink != 0 {
			continue
		}
		if e.IsDir() {
			if !recursive || isExcludedDir(e.Name()) {
				continue
			}
			sub, err := newestFileChange(readDir, filepath.Join(dir, e.Name()), true)
			if err != nil {
				return time.Time{}, err
			}
			if sub.After(newest) {
				newest = sub
			}
			continue
		}
		info, err := e.Info()
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return time.Time{}, err
		}
		if info.ModTime().After(newest) {
			newest = info.ModTime()
		}
	}
	return newest, nil
}
