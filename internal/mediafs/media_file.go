package mediafs

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/syudead/vv/internal/domain"
)

// OpenMediaFile は、path が roots のどれかの内側にある通常ファイルを指すときだけ
// それを開く。閉じるのは呼び出し側である。
//
// DB の値をそのまま os.Open に渡すと、将来 DB へ書き込む経路が増えたときに
// 任意ファイル読み出しになりうる。次の3つをすべて満たすときだけ開く。
//
//  1. filepath.Clean 後のパスが登録フォルダの内側にある
//  2. filepath.EvalSymlinks で辿った先も同じ登録フォルダの内側にある
//  3. 開いたものが通常ファイルである（ディレクトリ・デバイスファイルを開かない）
//
// 確かめたパスと開くパスを揃えるため、辿った先のパスで開く。開けないときは
// domain.ErrMediaFileUnavailable を包んだ誤りを返し、辿った先が外にあったときは
// domain.ErrMediaFileOutsideRoot も包む。
func (FS) OpenMediaFile(roots []string, path string) (*os.File, os.FileInfo, error) {
	resolved, err := resolveInside(roots, path)
	if err != nil {
		return nil, nil, err
	}
	file, err := os.Open(resolved)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", domain.ErrMediaFileUnavailable, err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, nil, fmt.Errorf("%w: %w", domain.ErrMediaFileUnavailable, err)
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, nil, fmt.Errorf("%w: 通常ファイルではありません", domain.ErrMediaFileUnavailable)
	}
	return file, info, nil
}

// ResolveMediaFile は、OpenMediaFile と同じ規則で開いてよい path について、
// symlink を辿った先のパスを返す。開かずに OS の別のプロセスへ渡すときに使う。
func (FS) ResolveMediaFile(roots []string, path string) (string, error) {
	resolved, err := resolveInside(roots, path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("%w: %w", domain.ErrMediaFileUnavailable, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%w: 通常ファイルではありません", domain.ErrMediaFileUnavailable)
	}
	return resolved, nil
}

// resolveInside は path の symlink を辿り、Clean 後のパスと辿った先の両方が
// 同じ登録フォルダの内側にあるときだけ、辿った先を返す。
func resolveInside(roots []string, path string) (string, error) {
	cleaned := filepath.Clean(path)
	var escaped bool
	for _, root := range roots {
		if !domain.PathInsideRoot(root, cleaned) {
			continue
		}
		resolved, err := filepath.EvalSymlinks(cleaned)
		if err != nil {
			return "", fmt.Errorf("%w: %w", domain.ErrMediaFileUnavailable, err)
		}
		if domain.MediaFileInsideRoot(root, cleaned, resolved) {
			return resolved, nil
		}
		escaped = true
	}
	if escaped {
		return "", fmt.Errorf("%w: %w", domain.ErrMediaFileUnavailable, domain.ErrMediaFileOutsideRoot)
	}
	return "", fmt.Errorf("%w: 登録フォルダの外です", domain.ErrMediaFileUnavailable)
}
