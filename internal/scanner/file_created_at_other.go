//go:build !linux && !darwin && !freebsd && !netbsd && !windows

package scanner

import (
	"io/fs"
	"time"
)

// Unsupported OSes report no creation time.
func fileCreatedAt(string, fs.FileInfo) (time.Time, bool) {
	return time.Time{}, false
}
