package mediafs

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/syudead/vv/internal/domain"
)

// ListSidecarFiles は、videoPath が OpenMediaFile と同じ規則で開けるときだけ、その
// 所在のフォルダにある通常ファイルの名前と大きさを返す
// （specs/028-sidecar-subtitles/research.md R-2）。サブフォルダと symlink は含めない。
// 名前の意味（字幕かどうか）はここでは判断しない。
//
// 動画を開けないときは domain.ErrMediaFileUnavailable を包んだ誤りを返し、フォルダを
// 読めないときは domain.ErrDirectoryUnavailable を包んだ誤りを返す。
func (FS) ListSidecarFiles(roots []string, videoPath string) ([]domain.SidecarEntry, error) {
	folder, err := sidecarFolder(roots, videoPath)
	if err != nil {
		return nil, err
	}
	dirEntries, err := os.ReadDir(folder)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", domain.ErrDirectoryUnavailable, err)
	}
	entries := make([]domain.SidecarEntry, 0, len(dirEntries))
	for _, entry := range dirEntries {
		// Type() は symlink を ModeSymlink として返すので、辿らずに除ける。
		if !entry.Type().IsRegular() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		entries = append(entries, domain.SidecarEntry{Name: entry.Name(), Size: info.Size()})
	}
	return entries, nil
}

// OpenSidecarFile は、videoPath の所在のフォルダにある name という通常ファイルを開く。
// 閉じるのは呼び出し側である。name は ListSidecarFiles が返す名前のどれかと完全に
// 一致するものだけを受け、要求の文字列からパスを組み立てない（`../x.srt` や別の
// フォルダの名前は断る）。開けないときは domain.ErrMediaFileUnavailable を包んだ誤りを返す。
func (fs FS) OpenSidecarFile(roots []string, videoPath, name string) (*os.File, os.FileInfo, error) {
	entries, err := fs.ListSidecarFiles(roots, videoPath)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", domain.ErrMediaFileUnavailable, err)
	}
	found := false
	for _, entry := range entries {
		if entry.Name == name {
			found = true
			break
		}
	}
	if !found {
		return nil, nil, fmt.Errorf("%w: not a file next to the video", domain.ErrMediaFileUnavailable)
	}
	// 一覧のあとに symlink へ差し替えられても外を開かないよう、開く前にもう一度確かめる。
	return fs.OpenMediaFile(roots, filepath.Join(filepath.Dir(filepath.Clean(videoPath)), name))
}

// sidecarFolder は、videoPath が OpenMediaFile と同じ規則で開けるときだけ、その所在
// （symlink を辿る前のパス）のフォルダを返す。再生に使う所在の隣を探すためである。
func sidecarFolder(roots []string, videoPath string) (string, error) {
	// 配信と同じ判定にするため、stat ではなく実際に開いて閉じる。stat はできても読めない
	// 所在（権限が無いなど）は配信が飛ばすので、その隣も探さない。
	file, _, err := FS{}.OpenMediaFile(roots, videoPath)
	if err != nil {
		return "", err
	}
	_ = file.Close()
	return filepath.Dir(filepath.Clean(videoPath)), nil
}
