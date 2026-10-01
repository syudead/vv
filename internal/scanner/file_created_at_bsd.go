//go:build darwin || freebsd || netbsd

package scanner

import (
	"io/fs"
	"syscall"
	"time"
)

// fileCreatedAt はファイルの作成日時を返す。darwin・freebsd・netbsd は走査が既に読んだ
// fs.FileInfo の Birthtimespec に持つ（specs/033-video-dates/research.md R-5）。
// ファイル名を _darwin にしないのは、GOOS の暗黙の制約で freebsd・netbsd が外れるためである。
func fileCreatedAt(_ string, info fs.FileInfo) (time.Time, bool) {
	if info == nil {
		return time.Time{}, false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat == nil {
		return time.Time{}, false
	}
	sec, nsec := stat.Birthtimespec.Unix()
	// 作成日時を持たないファイルシステムは 0 や負の値を返す。
	if sec < 0 || (sec == 0 && nsec == 0) {
		return time.Time{}, false
	}
	return time.Unix(sec, nsec), true
}
