package domain

import (
	"fmt"
	"path/filepath"
)

// NormalizeMediaFolderPath requires an absolute path and preserves Unicode spelling because normalization can identify another filesystem entry; adapters check existence.
func NormalizeMediaFolderPath(path string) (string, error) {
	if path == "" || !filepath.IsAbs(path) {
		return "", ErrInvalidMediaFolder
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalidMediaFolder, err)
	}
	return filepath.Clean(absolute), nil
}

// Reject folder changes during scans because scans use their startup folder snapshot.
func CheckMediaFolderMutation(scanRunning bool) error {
	if scanRunning {
		return ErrScanRunning
	}
	return nil
}

// CheckMediaFolderPlacement rejects overlapping registrations using PathWithinRoot rules, avoiding ambiguous ownership; replacement excludes its own ID.
func CheckMediaFolderPlacement(scanRunning bool, folders []MediaFolder, id int64, path string) error {
	if err := CheckMediaFolderMutation(scanRunning); err != nil {
		return err
	}
	for _, folder := range folders {
		if folder.ID == id {
			continue
		}
		if PathWithinRoot(folder.Path, path) || PathWithinRoot(path, folder.Path) {
			return ErrFolderConflict
		}
	}
	return nil
}
