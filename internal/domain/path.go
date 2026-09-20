package domain

import (
	"os"
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
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && !filepath.IsAbs(rel))
}
