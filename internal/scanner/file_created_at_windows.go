package scanner

import (
	"io/fs"
	"syscall"
	"time"
)

// fileCreatedAt はファイルの作成日時を返す。Windows は走査が既に読んだ fs.FileInfo の
// CreationTime に持つ（specs/033-video-dates/research.md R-5）。
func fileCreatedAt(_ string, info fs.FileInfo) (time.Time, bool) {
	if info == nil {
		return time.Time{}, false
	}
	data, ok := info.Sys().(*syscall.Win32FileAttributeData)
	if !ok || data == nil {
		return time.Time{}, false
	}
	if data.CreationTime.HighDateTime == 0 && data.CreationTime.LowDateTime == 0 {
		return time.Time{}, false
	}
	return time.Unix(0, data.CreationTime.Nanoseconds()), true
}
