package scanner

import (
	"io/fs"
	"time"

	"golang.org/x/sys/unix"
)

// Linux FileInfo lacks birth time, so request STATX_BTIME and tolerate unsupported filesystems or
// read failures.
func fileCreatedAt(path string, _ fs.FileInfo) (time.Time, bool) {
	var stx unix.Statx_t
	if err := unix.Statx(unix.AT_FDCWD, path, unix.AT_SYMLINK_NOFOLLOW|unix.AT_STATX_SYNC_AS_STAT,
		unix.STATX_BTIME, &stx); err != nil {
		return time.Time{}, false
	}
	if stx.Mask&unix.STATX_BTIME == 0 {
		return time.Time{}, false
	}
	return time.Unix(stx.Btime.Sec, int64(stx.Btime.Nsec)), true
}
