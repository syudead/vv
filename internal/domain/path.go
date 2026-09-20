package domain

import (
	"os"
	"path/filepath"
	"strings"
)

// PathWithinRoot applies the host OS volume, separator and path comparison rules.
func PathWithinRoot(root, path string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && !filepath.IsAbs(rel))
}
