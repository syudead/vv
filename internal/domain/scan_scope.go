package domain

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
)

// ScanOrigin は走査を始めた主体である。値は scans.origin に対応する
// （specs/042-folder-watch-import/data-model.md）。
type ScanOrigin string

const (
	// ScanOriginManual は利用者の操作、起動時の再開、外部クライアントが始めた走査である。
	ScanOriginManual ScanOrigin = "manual"
	// ScanOriginWatch はフォルダの監視が始めた、変わったディレクトリだけを読む走査である。
	ScanOriginWatch ScanOrigin = "watch"
)

// ErrScanSuperseded は、手動の走査やメディアフォルダの変更に取って代わられて、走査が
// 途中で止まったことを表す。止まった走査は何も消さず、done で閉じる
// （specs/042-folder-watch-import/research.md R-5）。
var ErrScanSuperseded = errors.New("the scan was superseded")

// DirtyDirectory は読み直すディレクトリである。Recursive が偽なら直下のファイルだけ、
// 真ならその下のすべてを読む（作られた・消された・名前を変えられたディレクトリ）。
type DirtyDirectory struct {
	Path      string
	Recursive bool
}

// Contains は、ファイルの path が d の読む範囲にあるかを返す。
func (d DirtyDirectory) Contains(path string) bool {
	if d.Recursive {
		return PathWithinRoot(d.Path, path)
	}
	return SamePath(d.Path, filepath.Dir(path))
}

// DirtyDirectoriesContain は、ファイルの path が dirs のどれかの読む範囲にあるかを返す。
func DirtyDirectoriesContain(dirs []DirtyDirectory, path string) bool {
	return slices.ContainsFunc(dirs, func(d DirtyDirectory) bool { return d.Contains(path) })
}

// NormalizeDirtyDirectories は dirs のパスを整え、重なりを除いて並べて返す。再帰の項目の
// 下にある項目と、同じ項目の繰り返しは落とす。
func NormalizeDirtyDirectories(dirs []DirtyDirectory) []DirtyDirectory {
	cleaned := make([]DirtyDirectory, 0, len(dirs))
	for _, d := range dirs {
		if d.Path == "" {
			continue
		}
		cleaned = append(cleaned, DirtyDirectory{Path: filepath.Clean(d.Path), Recursive: d.Recursive})
	}
	// 再帰の項目を先に並べると、あとの項目が覆われているかを前から見るだけで決まる。
	slices.SortFunc(cleaned, func(a, b DirtyDirectory) int {
		if a.Recursive != b.Recursive {
			if a.Recursive {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Path, b.Path)
	})
	out := make([]DirtyDirectory, 0, len(cleaned))
	for _, d := range cleaned {
		covered := slices.ContainsFunc(out, func(kept DirtyDirectory) bool {
			if kept.Recursive {
				return PathWithinRoot(kept.Path, d.Path)
			}
			return !d.Recursive && SamePath(kept.Path, d.Path)
		})
		if !covered {
			out = append(out, d)
		}
	}
	return out
}
