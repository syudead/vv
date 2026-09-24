package domain

import (
	"path/filepath"
	"runtime"
	"strings"
)

// PathWithinRoot applies the host OS volume, separator and path comparison rules.
func PathWithinRoot(root, path string) bool {
	cleanRoot := filepath.Clean(root)
	cleanPath := filepath.Clean(path)
	if runtime.GOOS == "windows" {
		cleanRoot = strings.ToLower(cleanRoot)
		cleanPath = strings.ToLower(cleanPath)
	}
	rel, err := filepath.Rel(cleanRoot, cleanPath)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel))
}

// SamePath は a と b が PathWithinRoot の規則で同じ場所を指すかを返す。
// symlink を辿る前と後のパスを比べ、途中に symlink が無いことを確かめるのに使う。
func SamePath(a, b string) bool {
	return PathWithinRoot(a, b) && PathWithinRoot(b, a)
}

// PathInsideRoot は path が root の内側にあるかを返す。root そのものは
// 「内側のファイル」ではないので false になる。
//
// 区切り文字まで含めて比べるので、"/media" と "/media-other" のように
// 接頭辞が一致するだけの別ディレクトリは内側にならない。
func PathInsideRoot(root, path string) bool {
	return root != path && PathWithinRoot(root, path)
}

// MediaFileInsideRoot は、メディアファイルとして開いてよい位置かを、
// ファイルシステムに触れずに判定する。path は所在のパスを filepath.Clean した
// もの、resolved はその symlink を辿った先である。どちらも同じ root の内側に
// あるときだけ true を返す。Clean だけでは、内側の symlink から root の外の
// ファイルへ辿り着けてしまうため、辿った先にも同じ判定を掛ける。
//
// 通常ファイルであるかは、ファイルシステムを見る側が確かめる。
func MediaFileInsideRoot(root, path, resolved string) bool {
	return PathInsideRoot(root, path) && PathInsideRoot(root, resolved)
}
