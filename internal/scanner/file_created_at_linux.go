package scanner

import (
	"io/fs"
	"time"

	"golang.org/x/sys/unix"
)

// fileCreatedAt はファイルの作成日時を返す。Linux の lstat（fs.FileInfo）は作成日時を
// 持たないので statx に STATX_BTIME を求め、ファイルシステムが返したときだけ使う
// （specs/033-video-dates/research.md R-5）。読めなかったことは失敗にしない。
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
