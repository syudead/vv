package app

import (
	"os"
	"path/filepath"
)

// fixturePath maps a slash-form absolute fixture path to the host's volume and
// separators. No files are created there; these paths only identify test data.
func fixturePath(path string) string {
	return filepath.FromSlash(filepath.VolumeName(os.TempDir()) + path)
}
