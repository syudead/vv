package mediafs

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/syudead/vv/internal/domain"
)

// ListSidecarFiles requires a video readable under OpenMediaFile rules and returns sibling regular
// files without symlinks; video and directory failures use their respective domain errors.
func (FS) ListSidecarFiles(roots []string, videoPath string) ([]domain.SidecarEntry, error) {
	folder, err := sidecarFolder(roots, videoPath)
	if err != nil {
		return nil, err
	}
	dirEntries, err := os.ReadDir(folder)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", domain.ErrDirectoryUnavailable, err)
	}
	entries := make([]domain.SidecarEntry, 0, len(dirEntries))
	for _, entry := range dirEntries {
		// Type detects symlinks without following them.
		if !entry.Type().IsRegular() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		entries = append(entries, domain.SidecarEntry{Name: entry.Name(), Size: info.Size()})
	}
	return entries, nil
}

// OpenSidecarFile accepts only an exact listed filename, never constructs paths from unchecked
// request input, and returns a caller-owned file; failures wrap ErrMediaFileUnavailable.
func (fs FS) OpenSidecarFile(roots []string, videoPath, name string) (*os.File, os.FileInfo, error) {
	entries, err := fs.ListSidecarFiles(roots, videoPath)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", domain.ErrMediaFileUnavailable, err)
	}
	found := false
	for _, entry := range entries {
		if entry.Name == name {
			found = true
			break
		}
	}
	if !found {
		return nil, nil, fmt.Errorf("%w: not a file next to the video", domain.ErrMediaFileUnavailable)
	}
	// Recheck before opening in case the listed file became a symlink.
	return fs.OpenMediaFile(roots, filepath.Join(filepath.Dir(filepath.Clean(videoPath)), name))
}

// Find sidecars beside the original playback location, before resolving video symlinks.
func sidecarFolder(roots []string, videoPath string) (string, error) {
	// Actually open the video to match serving rules; stat alone also permits unreadable locations.
	file, _, err := FS{}.OpenMediaFile(roots, videoPath)
	if err != nil {
		return "", err
	}
	_ = file.Close()
	return filepath.Dir(filepath.Clean(videoPath)), nil
}
