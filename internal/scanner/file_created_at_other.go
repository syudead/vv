//go:build !linux && !darwin && !freebsd && !netbsd && !windows

package scanner

import (
	"io/fs"
	"time"
)

// fileCreatedAt は作成日時の読み方を持たない OS では常に取れない
// （specs/033-video-dates/research.md R-5）。
func fileCreatedAt(string, fs.FileInfo) (time.Time, bool) {
	return time.Time{}, false
}
