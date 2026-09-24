package media

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
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
	interval := strconv.FormatInt(int64(domain.SeekThumbnailInterval/time.Second), 10)
	return []string{
		"-nostdin",
		"-v", "error",
		"-i", videoPath,
		"-map", "0:V:0?",
		"-vf", "select='isnan(prev_selected_t)+gt(floor(t/" + interval + ")\\,floor(prev_selected_t/" + interval + "))',scale=min(320\\,iw):-2,format=yuvj420p",
		"-fps_mode", "vfr",
		"-q:v", "4",
		"-start_number", "0",
		"-y",
		outputPattern,
	}
}
