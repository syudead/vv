package artifacts

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// SearchUploads holds the video files an image search receives, each only for its request. It lives in the data
// directory rather than the OS temporary directory, which in a container is often a small tmpfs.
type SearchUploads struct {
	dir string
}

// NewSearchUploads keeps uploads in dir, which nothing else may use: Empty removes all of it.
func NewSearchUploads(dir string) *SearchUploads {
	return &SearchUploads{dir: dir}
}

// Create opens a new empty file for one upload; the caller removes it with Remove.
func (u *SearchUploads) Create() (*os.File, error) {
	if err := os.MkdirAll(u.dir, dirPerm); err != nil {
		return nil, fmt.Errorf("cannot create the search upload directory: %w", err)
	}
	file, err := os.CreateTemp(u.dir, "query-*")
	if err != nil {
		return nil, fmt.Errorf("cannot create a search upload: %w", err)
	}
	return file, nil
}

// Remove deletes one upload Create made; a path outside the area is refused and a missing file is not an error.
func (u *SearchUploads) Remove(path string) error {
	if filepath.Dir(path) != filepath.Clean(u.dir) {
		return fmt.Errorf("%s is not a search upload", path)
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("cannot delete a search upload: %w", err)
	}
	return nil
}

// Empty removes every upload; startup calls it before the listener opens, so a search a crash or kill interrupted
// leaves nothing behind.
func (u *SearchUploads) Empty() error {
	if err := os.RemoveAll(u.dir); err != nil {
		return fmt.Errorf("cannot empty the search upload directory: %w", err)
	}
	return nil
}
