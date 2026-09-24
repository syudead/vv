package media

import (
	"context"
	"fmt"
	"os"

	"github.com/syudead/vv/internal/domain"
)

// Assets は ffprobe・ffmpeg で元の動画を読み、生成物を書く入口である。
//
// internal/app はこれを自分の宣言した interface 越しに使う。書き出す先は
// 呼び出し側が渡すパスで、置き場の並べ方・公開・削除は internal/artifacts が
// 受け持つ。
type Assets struct{}

// NewAssets は Assets を返す。
func NewAssets() *Assets {
	return &Assets{}
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

// Thumbnail はライブラリ用サムネイルを1枚 output へ書く。
func (a *Assets) Thumbnail(ctx context.Context, path string, durationMs int64, output string) error {
	return Thumbnail(ctx, path, durationMs, output)
}

// SeekThumbnails はシーク用プレビューのフレームを outputPattern（連番の型）へ書く。
func (a *Assets) SeekThumbnails(ctx context.Context, path, outputPattern string) error {
	return GenerateSeekThumbnails(ctx, path, outputPattern)
}

// Preview はホバープレビューの MP4 を output へ書く。
func (a *Assets) Preview(ctx context.Context, path, output string, durationMs int64) error {
	return GeneratePreview(ctx, path, output, durationMs)
}
