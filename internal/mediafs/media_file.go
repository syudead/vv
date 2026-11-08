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
	resolved, err := resolveInside(roots, path)
	if err != nil {
		return nil, nil, err
	}
	file, err := os.Open(resolved)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", domain.ErrMediaFileUnavailable, err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, nil, fmt.Errorf("%w: %w", domain.ErrMediaFileUnavailable, err)
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, nil, fmt.Errorf("%w: not a regular file", domain.ErrMediaFileUnavailable)
	}
	return file, info, nil
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
