package media

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	previewSegmentCount = 12
	previewSegmentSec   = 0.75
	previewShortSec     = 9.0
	previewTimeout      = 30 * time.Minute
)

// PreviewSegments returns the deterministic [start,end) windows used for a
// preview. Short videos are converted once so their timing is preserved.
func PreviewSegments(durationMs int64) [][2]float64 {
	duration := float64(durationMs) / 1000
	if duration <= previewShortSec {
		return [][2]float64{{0, max(0, duration)}}
	}
	segments := make([][2]float64, previewSegmentCount)
	for i := range segments {
		start := (duration - previewSegmentSec) * float64(i) / float64(previewSegmentCount-1)
		segments[i] = [2]float64{start, start + previewSegmentSec}
	}
	return segments
}

// GeneratePreview はホバープレビューの MP4 を output へ書く。置き場所、manifest、
// 公開は呼び出し側（internal/artifacts）が受け持つ。
func GeneratePreview(ctx context.Context, videoPath, output string, durationMs int64) error {
	processCtx, cancel := context.WithTimeout(ctx, previewTimeout)
	defer cancel()
	if combined, runErr := exec.CommandContext(processCtx, "ffmpeg", previewArgs(videoPath, output, durationMs)...).CombinedOutput(); runErr != nil {
		if processCtx.Err() != nil {
			return fmt.Errorf("プレビュー生成を中断しました: %w", processCtx.Err())
		}
		return fmt.Errorf("ffmpeg がプレビュー生成に失敗しました: %w: %s", runErr, firstLine(combined))
	}
	return nil
}

func previewArgs(videoPath, output string, durationMs int64) []string {
	args := []string{"-nostdin", "-v", "error", "-i", videoPath}
	segments := PreviewSegments(durationMs)
	if len(segments) == 1 {
		args = append(args, "-map", "0:V:0?", "-vf", "scale='trunc(min(640,iw)/2)*2':-2", "-an")
	} else {
		filters := make([]string, 0, len(segments)+1)
		labels := make([]string, 0, len(segments))
		for i, segment := range segments {
			label := fmt.Sprintf("v%d", i)
			filters = append(filters, fmt.Sprintf("[0:V:0]trim=start=%s:end=%s,setpts=PTS-STARTPTS,scale='trunc(min(640,iw)/2)*2':-2,format=yuv420p[%s]", previewFormatSeconds(segment[0]), previewFormatSeconds(segment[1]), label))
			labels = append(labels, "["+label+"]")
		}
		filters = append(filters, strings.Join(labels, "")+fmt.Sprintf("concat=n=%d:v=1:a=0[v]", len(labels)))
		args = append(args, "-filter_complex", strings.Join(filters, ";"), "-map", "[v]")
	}
	return append(args, "-c:v", "libx264", "-pix_fmt", "yuv420p", "-movflags", "+faststart", "-an", "-y", output)
}

func previewFormatSeconds(value float64) string { return strconv.FormatFloat(value, 'f', 3, 64) }
