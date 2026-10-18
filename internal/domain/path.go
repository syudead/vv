package domain

import (
	"path/filepath"
	"runtime"
	"strings"
)

// PathWithinRoot applies the host OS volume, separator and path comparison rules.
func PathWithinRoot(root, path string) bool {
	cleanRoot := filepath.Clean(root)
	cleanPath := filepath.Clean(path)
	if runtime.GOOS == "windows" {
		cleanRoot = strings.ToLower(cleanRoot)
		cleanPath = strings.ToLower(cleanPath)
	}
	rel, err := filepath.Rel(cleanRoot, cleanPath)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel))
}

// SamePath uses PathWithinRoot rules to compare paths before and after symlink resolution.
func SamePath(a, b string) bool {
	return PathWithinRoot(a, b) && PathWithinRoot(b, a)
}

// PathInsideRoot excludes the root itself and prefix-only siblings such as /media-other.
func PathInsideRoot(root, path string) bool {
	return root != path && PathWithinRoot(root, path)
}

// MediaFileInsideRoot requires both cleaned and symlink-resolved paths inside the same root; the filesystem adapter checks that the target is regular.
func MediaFileInsideRoot(root, path, resolved string) bool {
	return PathInsideRoot(root, path) && PathInsideRoot(root, resolved)
}
