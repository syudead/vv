package domain

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
)

type ScanOrigin string

const (
	// ScanOriginManual includes startup resumption and external clients.
	ScanOriginManual ScanOrigin = "manual"
	ScanOriginWatch  ScanOrigin = "watch"
)

// Superseded scans stop without deleting anything and close as done.
var ErrScanSuperseded = errors.New("the scan was superseded")

// Recursive scans all descendants; otherwise only direct files are read.
type DirtyDirectory struct {
	Path      string
	Recursive bool
}

func (d DirtyDirectory) Contains(path string) bool {
	if d.Recursive {
		return PathWithinRoot(d.Path, path)
	}
	return SamePath(d.Path, filepath.Dir(path))
}

func DirtyDirectoriesContain(dirs []DirtyDirectory, path string) bool {
	return slices.ContainsFunc(dirs, func(d DirtyDirectory) bool { return d.Contains(path) })
}

// NormalizeDirtyDirectories removes duplicates and descendants already covered by recursive entries.
func NormalizeDirtyDirectories(dirs []DirtyDirectory) []DirtyDirectory {
	cleaned := make([]DirtyDirectory, 0, len(dirs))
	for _, d := range dirs {
		if d.Path == "" {
			continue
		}
		cleaned = append(cleaned, DirtyDirectory{Path: filepath.Clean(d.Path), Recursive: d.Recursive})
	}
	// Recursive entries sort first so coverage checks need only preceding entries.
	slices.SortFunc(cleaned, func(a, b DirtyDirectory) int {
		if a.Recursive != b.Recursive {
			if a.Recursive {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Path, b.Path)
	})
	out := make([]DirtyDirectory, 0, len(cleaned))
	for _, d := range cleaned {
		covered := slices.ContainsFunc(out, func(kept DirtyDirectory) bool {
			if kept.Recursive {
				return PathWithinRoot(kept.Path, d.Path)
			}
			return !d.Recursive && SamePath(kept.Path, d.Path)
		})
		if !covered {
			out = append(out, d)
		}
	}
	return out
}
