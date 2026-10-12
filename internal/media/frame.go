package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/png"
	"os/exec"
	"strconv"

	"github.com/syudead/vv/internal/domain"
)

// frameScale は、コマの長辺を動画の長辺のまま、domain.ThumbnailFrameMaxSide を越えるときだけ
// そこまで縮める。動画が枠より小さければ大きさを変えない。
var frameScale = "scale='min(" + strconv.Itoa(domain.ThumbnailFrameMaxSide) + ",iw)':'min(" +
	strconv.Itoa(domain.ThumbnailFrameMaxSide) + ",ih)':force_original_aspect_ratio=decrease,setsar=1"

// FrameAt は動画 videoPath の positionMs ミリ秒の場面を 1 枚、PNG で取り出す。サムネイルの編集で
// 持ち主が塗り、提供元へ上げるコマである（specs/045-comfyui-thumbnails/research.md R-8）。長辺は
// 動画の長辺で、上限は domain.ThumbnailFrameMaxSide である。指定の位置でコマが取れなくても先頭の
// コマで作り直さない（ThumbnailAt と同じ）。
func FrameAt(ctx context.Context, videoPath string, positionMs int64) (domain.ThumbnailFrame, error) {
	ctx, cancel := context.WithTimeout(ctx, thumbnailTimeout)
	defer cancel()

	output, err := command(ctx, thumbnailCommand, frameArgs(videoPath, positionMs)...).Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return domain.ThumbnailFrame{}, fmt.Errorf("%s failed (%s): %s", thumbnailCommand, videoPath, firstLine(exitErr.Stderr))
		}
		return domain.ThumbnailFrame{}, fmt.Errorf("cannot run %s (%s): %w", thumbnailCommand, videoPath, err)
	}
	// 指定の位置にコマが無ければ、ffmpeg は終了コード 0 のまま何も書かない（runThumbnail）。
	config, err := png.DecodeConfig(bytes.NewReader(output))
	if err != nil {
		return domain.ThumbnailFrame{}, fmt.Errorf("no frame was extracted (%s, at %d ms): %w", videoPath, positionMs, err)
	}
	return domain.ThumbnailFrame{PNG: output, Width: config.Width, Height: config.Height}, nil
}

// frameArgs は FrameAt が ffmpeg に渡す引数を組み立てる。出力は標準出力の PNG 1 枚である。
// -ss を -i の前に置くのは thumbnailArgs と同じ理由である。
func frameArgs(videoPath string, positionMs int64) []string {
	return []string{
		"-nostdin",
		"-v", "error",
		"-ss", strconv.FormatFloat(positionSeconds(positionMs), 'f', 3, 64),
		"-i", videoPath,
		"-frames:v", "1",
		"-vf", frameScale,
		"-c:v", "png",
		"-f", "image2pipe",
		"pipe:1",
	}
}

// FrameAt は FrameAt を呼ぶ。
func (a *Assets) FrameAt(ctx context.Context, path string, positionMs int64) (domain.ThumbnailFrame, error) {
	return FrameAt(ctx, path, positionMs)
}

// AutoThumbnailPositions は AutoThumbnailPositions を呼ぶ。
func (a *Assets) AutoThumbnailPositions(durationMs int64) []int64 {
	return AutoThumbnailPositions(durationMs)
}
