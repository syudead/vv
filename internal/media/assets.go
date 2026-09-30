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

// CheckSource は元の動画が読める通常ファイルかを確かめる。失敗は
// domain.ProbeErrorFileUnavailable で包む（解析の失敗理由のコード。解析以外の
// ジョブでは保存されない）。
func (a *Assets) CheckSource(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return sourceUnavailable(err)
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return sourceUnavailable(err)
	}
	if !info.Mode().IsRegular() {
		return sourceUnavailable(fmt.Errorf("not a regular file: %s", path))
	}
	return nil
}

func sourceUnavailable(err error) error {
	return domain.NewProbeFailure(domain.ProbeErrorFileUnavailable, err)
}

// Probe は ffprobe で動画の情報を読む。
func (a *Assets) Probe(ctx context.Context, path string) (domain.Probe, error) {
	return Probe(ctx, path)
}

// Thumbnail はライブラリ用サムネイルを1枚 output へ書き、先頭のコマで作ったかを返す。
func (a *Assets) Thumbnail(ctx context.Context, path string, durationMs int64, output string) (bool, error) {
	return Thumbnail(ctx, path, durationMs, output)
}

// ThumbnailAt はライブラリ用サムネイルを positionMs の場面で1枚 output へ書く。
// 先頭のコマへの代用はしない。
func (a *Assets) ThumbnailAt(ctx context.Context, path string, positionMs int64, output string) error {
	return ThumbnailAt(ctx, path, positionMs, output)
}

// SeekSprite はシーク用プレビューのシートを layout の配置で outputDir へ書き、
// 全編の復号から作ったかを返す。
func (a *Assets) SeekSprite(ctx context.Context, path, outputDir string, layout domain.SeekSpriteLayout) (bool, error) {
	return GenerateSeekSprite(ctx, path, outputDir, layout)
}

// Preview はホバープレビューの MP4 を output へ書く。
func (a *Assets) Preview(ctx context.Context, path, output string, durationMs int64) error {
	return GeneratePreview(ctx, path, output, durationMs)
}
