package mediafs

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/syudead/vv/internal/domain"
)

// CheckMediaFolder は path を整えた絶対パスにし（domain.NormalizeMediaFolderPath）、
// 走査できるディレクトリかを確かめて、整えたパスを返す。
//
// 存在しない・ディレクトリでない・読めないパスは domain.ErrInvalidMediaFolder、
// パス自身かその途中の段が symlink なら domain.ErrUnsupportedMediaFolder で断る。
// 走査は symlink をたどらないので、symlink の先を登録すると、登録と走査で
// 見えるものが食い違う。
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
