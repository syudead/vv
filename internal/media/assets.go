package media

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/syudead/vv/internal/domain"
)

// Assets は1つのサムネイル置き場に対する生成・確認・削除の入口である。
//
// internal/app はこれを自分の宣言した interface 越しに使う。置き場の場所と
// ファイルの並べ方を知っているのは internal/media だけにする。
type Assets struct {
	thumbnailsDir string
}

// NewAssets は thumbnailsDir を置き場とする Assets を返す。
func NewAssets(thumbnailsDir string) *Assets {
	return &Assets{thumbnailsDir: thumbnailsDir}
}

// CheckSource は元の動画が読める通常ファイルかを確かめる。
func (a *Assets) CheckSource(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("通常ファイルではありません: %s", path)
	}
	return nil
}

// Probe は ffprobe で動画の情報を読む。
func (a *Assets) Probe(ctx context.Context, path string) (domain.Probe, error) {
	return Probe(ctx, path)
}

// Thumbnail は代表サムネイルを1枚作る。
func (a *Assets) Thumbnail(ctx context.Context, path string, durationMs int64, contentKey string) error {
	_, err := Thumbnail(ctx, path, durationMs, a.thumbnailsDir, contentKey)
	return err
}

// SeekThumbnails はシーク用プレビューを作る。
func (a *Assets) SeekThumbnails(ctx context.Context, path, contentKey string) error {
	return GenerateSeekThumbnails(ctx, path, a.thumbnailsDir, contentKey)
}

// Preview は一覧用プレビューを作る。validate が false を返したら公開せずに
// domain.ErrPreviewStale を返す。
func (a *Assets) Preview(
	ctx context.Context, path, contentKey string, durationMs int64,
	validate func(context.Context) (bool, error),
) error {
	return GeneratePreview(ctx, path, a.thumbnailsDir, contentKey, durationMs, validate)
}

// RemoveContent は内容1つ分の生成物をすべて消す。
func (a *Assets) RemoveContent(contentKey string) error {
	return RemoveContentArtifacts(a.thumbnailsDir, contentKey)
}

// PreviewAvailable は一覧用プレビューのファイルが空でない通常ファイルとして
// あるかを返す。
func (a *Assets) PreviewAvailable(contentKey string) bool {
	if a.thumbnailsDir == "" || contentKey == "" {
		return false
	}
	info, err := os.Stat(PreviewPath(a.thumbnailsDir, contentKey))
	return err == nil && info.Mode().IsRegular() && info.Size() > 0
}

// SeekThumbnailsAvailable はシーク用プレビューの置き場があるかを返す。置き場は
// 生成の完了時に一時ディレクトリから改名して作られるので、あれば完成している。
func (a *Assets) SeekThumbnailsAvailable(contentKey string) bool {
	if a.thumbnailsDir == "" || contentKey == "" {
		return false
	}
	info, err := os.Stat(SeekThumbnailDir(filepath.Join(a.thumbnailsDir, "seek"), contentKey))
	return err == nil && info.IsDir()
}
