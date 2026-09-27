package media

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/syudead/vv/internal/domain"
)

const (
	seekThumbnailCommand = "ffmpeg"
	seekThumbnailTimeout = 30 * time.Minute
)

// GenerateSeekThumbnails は動画を1度だけ復号し、domain.SeekThumbnailInterval
// ごとに1枚のフレームを outputPattern（ffmpeg の連番の型）へ書く。置き場所と
// 公開は呼び出し側（internal/artifacts）が受け持つ。
func GenerateSeekThumbnails(ctx context.Context, videoPath, outputPattern string) error {
	processCtx, cancel := context.WithTimeout(ctx, seekThumbnailTimeout)
	defer cancel()
	args := seekThumbnailArgs(videoPath, outputPattern)
	if combined, runErr := exec.CommandContext(processCtx, seekThumbnailCommand, args...).CombinedOutput(); runErr != nil {
		if processCtx.Err() != nil {
			return fmt.Errorf("シークサムネイル生成を中断しました: %w", processCtx.Err())
		}
		return fmt.Errorf("%s が失敗しました (%s): %w: %s",
			seekThumbnailCommand, videoPath, runErr, firstLine(combined))
	}
	return nil
}

func seekThumbnailArgs(videoPath, outputPattern string) []string {
	interval := strconv.FormatFloat(domain.SeekThumbnailInterval.Seconds(), 'f', -1, 64)
	return []string{
		"-nostdin",
		"-v", "error",
		"-i", videoPath,
		"-map", "0:V:0?",
		"-vf", "select='isnan(prev_selected_t)+gt(floor(t/" + interval + ")\\,floor(prev_selected_t/" + interval + "))',scale=min(320\\,iw):min(320\\,ih):force_original_aspect_ratio=decrease:force_divisible_by=2,format=yuvj420p",
		"-fps_mode", "vfr",
		"-q:v", "4",
		"-start_number", "0",
		"-y",
		outputPattern,
	}
}

// GenerateSeekSprite は動画を1度だけ復号し、layout の間隔ごとに1コマを取って
// layout.Columns × layout.Rows の格子のシートを outputDir（既にあるディレクトリ）へ
// 000.jpg から順に書く（specs/021-seek-thumbnail-sprite/research.md R-2）。コマの数は
// layout.FrameCount に揃え、映像が途中で尽きたら最後のコマを複製する。最後の
// シートの空きは黒で埋まる。置き場所と公開は呼び出し側が受け持つ。
func GenerateSeekSprite(ctx context.Context, videoPath, outputDir string, layout domain.SeekSpriteLayout) error {
	processCtx, cancel := context.WithTimeout(ctx, seekThumbnailTimeout)
	defer cancel()
	args := seekSpriteArgs(videoPath, filepath.Join(outputDir, "%03d.jpg"), layout)
	if combined, runErr := exec.CommandContext(processCtx, seekThumbnailCommand, args...).CombinedOutput(); runErr != nil {
		if processCtx.Err() != nil {
			return fmt.Errorf("シークサムネイル生成を中断しました: %w", processCtx.Err())
		}
		return fmt.Errorf("%s が失敗しました (%s): %w: %s",
			seekThumbnailCommand, videoPath, runErr, firstLine(combined))
	}
	return nil
}

func seekSpriteArgs(videoPath, outputPattern string, layout domain.SeekSpriteLayout) []string {
	filters := []string{
		"fps=1000/" + strconv.FormatInt(layout.IntervalMs, 10) + ":eof_action=pass",
		"tpad=stop_mode=clone:stop=-1",
		"trim=end_frame=" + strconv.Itoa(layout.FrameCount),
		"scale=min(320\\,iw):min(320\\,ih):force_original_aspect_ratio=decrease:force_divisible_by=2",
		"tile=" + strconv.Itoa(layout.Columns) + "x" + strconv.Itoa(layout.Rows),
		"format=yuvj420p",
	}
	return []string{
		"-nostdin",
		"-v", "error",
		"-i", videoPath,
		"-map", "0:V:0?",
		"-vf", strings.Join(filters, ","),
		"-fps_mode", "passthrough",
		"-q:v", "4",
		"-start_number", "0",
		"-y",
		outputPattern,
	}
}

// GenerateSeekThumbnailSet はシーク用サムネイルの一式を outputDir（既にある
// ディレクトリ）へ書く。scripts/previewbench がシーク用の生成を測る境界で、
// 生成方式を変えるときはこの関数の中身を変える。今は durationMs から
// domain.NewSeekSpriteLayout で配置を決め、GenerateSeekSprite でシートを書く。
func GenerateSeekThumbnailSet(ctx context.Context, videoPath, outputDir string, durationMs int64) error {
	return GenerateSeekSprite(ctx, videoPath, outputDir, domain.NewSeekSpriteLayout(durationMs))
}
