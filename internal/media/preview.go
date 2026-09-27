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

// previewScale は、向きを問わず 640x640 の枠に収める（元より大きくはしない）。
// 縦長の動画でも横長と同じくらいの大きさに留まる。
const previewScale = "scale='min(640,iw)':'min(640,ih)':force_original_aspect_ratio=decrease:force_divisible_by=2"

// previewArgs は 1 回の ffmpeg の引数を作る。9 秒以下は全体を変換し、9 秒を超える
// 動画は PreviewSegments の各区間を入力側シーク（-ss/-t）の別入力として読み、concat
// フィルタで連結する。元動画を末尾までデコードしないので、読む量と保持するフレームは
// 区間の数と長さで決まり、動画の長さに比例しない（specs/019-preview-input-seek
// research.md R-1）。-ss は既定の accurate seek のままにし、区間と映る場面の対応を
// 保つ（R-3）。フレームの無い区間は concat が飛ばすので、その区間が無いだけの短い
// 出力になる（R-2）。全部が空だと ffmpeg は映像の無い MP4 を終了コード 0 で書くので、
// -abort_on empty_output で失敗にし、映像の無いプレビューを公開しない。索引の無い
// MPEG-TS ではシークがキーフレームに着かず、キーフレームの疎な入力で全区間が空になる
// ことがある（親 Issue の Edge Cases: 不完全な出力を公開せず、ジョブの失敗として扱う）。
func previewArgs(videoPath, output string, durationMs int64) []string {
	args := []string{"-nostdin", "-v", "error"}
	segments := PreviewSegments(durationMs)
	if len(segments) == 1 {
		args = append(args, "-i", videoPath, "-map", "0:V:0?", "-vf", previewScale, "-an")
	} else {
		filters := make([]string, 0, len(segments)+1)
		labels := make([]string, 0, len(segments))
		for i, segment := range segments {
			args = append(args, "-ss", previewFormatSeconds(segment[0]), "-t", previewFormatSeconds(segment[1]-segment[0]), "-i", videoPath)
			label := fmt.Sprintf("v%d", i)
			filters = append(filters, fmt.Sprintf("[%d:V:0]setpts=PTS-STARTPTS,%s,format=yuv420p[%s]", i, previewScale, label))
			labels = append(labels, "["+label+"]")
		}
		filters = append(filters, strings.Join(labels, "")+fmt.Sprintf("concat=n=%d:v=1:a=0[v]", len(labels)))
		args = append(args, "-filter_complex", strings.Join(filters, ";"), "-map", "[v]", "-abort_on", "empty_output")
	}
	return append(args, "-c:v", "libx264", "-pix_fmt", "yuv420p", "-movflags", "+faststart", "-an", "-y", output)
}

func previewFormatSeconds(value float64) string { return strconv.FormatFloat(value, 'f', 3, 64) }
