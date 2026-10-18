//go:build darwin || freebsd || netbsd

package scanner

import (
	"io/fs"
	"syscall"
	"time"
)

// BSD FileInfo already contains Birthtimespec; avoid a _darwin filename so the build tag can also
// include FreeBSD and NetBSD.
func fileCreatedAt(_ string, info fs.FileInfo) (time.Time, bool) {
	if info == nil {
		return time.Time{}, false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat == nil {
		return time.Time{}, false
	}
	sec, nsec := stat.Birthtimespec.Unix()
	// Filesystems without creation timestamps may return zero or negative values.
	if sec < 0 || (sec == 0 && nsec == 0) {
		return time.Time{}, false
	}
	return time.Unix(sec, nsec), true
}
