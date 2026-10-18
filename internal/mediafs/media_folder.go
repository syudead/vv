package mediafs

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/syudead/vv/internal/domain"
)

// CheckMediaFolder rejects unreadable/non-directory paths and any symlink component; allowing
// symlink roots would disagree with the scanner, which does not follow links.
func (FS) CheckMediaFolder(path string) (string, error) {
	cleaned, err := domain.NormalizeMediaFolderPath(path)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(cleaned)
	if err != nil {
		return "", fmt.Errorf("%w: %w", domain.ErrInvalidMediaFolder, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", domain.ErrUnsupportedMediaFolder
	}
	if !info.IsDir() {
		return "", domain.ErrInvalidMediaFolder
	}
	resolved, err := filepath.EvalSymlinks(cleaned)
	if err != nil {
		return "", fmt.Errorf("%w: %w", domain.ErrInvalidMediaFolder, err)
	}
	resolved, err = domain.NormalizeMediaFolderPath(resolved)
	if err != nil || !domain.SamePath(cleaned, resolved) {
		return "", fmt.Errorf("%w: symbolic link", domain.ErrUnsupportedMediaFolder)
	}
	dir, err := os.Open(cleaned)
	if err != nil {
		return "", fmt.Errorf("%w: %w", domain.ErrInvalidMediaFolder, err)
	}
	defer func() { _ = dir.Close() }()
	if _, err := dir.ReadDir(1); err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("%w: %w", domain.ErrInvalidMediaFolder, err)
	}
	return cleaned, nil
}
