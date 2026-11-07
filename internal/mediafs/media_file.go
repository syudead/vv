package mediafs

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/syudead/vv/internal/domain"
)

// OpenMediaFile requires both cleaned and resolved paths within the same root, then opens the
// resolved regular file; the caller closes it. Failures wrap ErrMediaFileUnavailable, additionally
// wrapping ErrMediaFileOutsideRoot for escaped symlinks.
func (FS) OpenMediaFile(roots []string, path string) (*os.File, os.FileInfo, error) {
	return openMediaFile(roots, path, os.Open)
}

// openMediaFile validates path again after opening it and requires the opened file to be the one the validated path
// names, so a symlink swapped in between cannot make it open a file outside the root.
func openMediaFile(roots []string, path string, open func(string) (*os.File, error)) (*os.File, os.FileInfo, error) {
	resolved, err := resolveInside(roots, path)
	if err != nil {
		return nil, nil, err
	}
	file, err := open(resolved)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", domain.ErrMediaFileUnavailable, err)
	}
	info, err := verifyOpenedMediaFile(roots, path, file)
	if err != nil {
		_ = file.Close()
		return nil, nil, err
	}
	return file, info, nil
}

func verifyOpenedMediaFile(roots []string, path string, file *os.File) (os.FileInfo, error) {
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", domain.ErrMediaFileUnavailable, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: not a regular file", domain.ErrMediaFileUnavailable)
	}
	resolved, err := resolveInside(roots, path)
	if err != nil {
		return nil, err
	}
	current, err := os.Stat(resolved)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", domain.ErrMediaFileUnavailable, err)
	}
	if !os.SameFile(info, current) {
		return nil, fmt.Errorf("%w: the file changed while it was opened", domain.ErrMediaFileUnavailable)
	}
	return info, nil
}

// ResolveMediaFile applies the same access rules as OpenMediaFile but returns a resolved path for
// another OS process.
func (FS) ResolveMediaFile(roots []string, path string) (string, error) {
	resolved, err := resolveInside(roots, path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("%w: %w", domain.ErrMediaFileUnavailable, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%w: not a regular file", domain.ErrMediaFileUnavailable)
	}
	return resolved, nil
}

// Both cleaned and resolved paths must stay within the same registered root.
func resolveInside(roots []string, path string) (string, error) {
	cleaned := filepath.Clean(path)
	var escaped bool
	for _, root := range roots {
		if !domain.PathInsideRoot(root, cleaned) {
			continue
		}
		resolved, err := filepath.EvalSymlinks(cleaned)
		if err != nil {
			return "", fmt.Errorf("%w: %w", domain.ErrMediaFileUnavailable, err)
		}
		if domain.MediaFileInsideRoot(root, cleaned, resolved) {
			return resolved, nil
		}
		escaped = true
	}
	if escaped {
		return "", fmt.Errorf("%w: %w", domain.ErrMediaFileUnavailable, domain.ErrMediaFileOutsideRoot)
	}
	return "", fmt.Errorf("%w: outside the media folder", domain.ErrMediaFileUnavailable)
}
