package media

import (
	"context"
	"fmt"
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

// GeneratePreview writes bytes; callers own layout, manifests, and publication.
func GeneratePreview(ctx context.Context, videoPath, output string, durationMs int64) error {
	processCtx, cancel := context.WithTimeout(ctx, previewTimeout)
	defer cancel()
	if combined, runErr := command(processCtx, "ffmpeg", previewArgs(videoPath, output, durationMs)...).CombinedOutput(); runErr != nil {
		if processCtx.Err() != nil {
			return fmt.Errorf("preview generation was interrupted: %w", processCtx.Err())
		}
		return exitFailure(fmt.Errorf("ffmpeg failed to generate the preview: %w: %s", runErr, firstLine(combined)), combined)
	}
	return nil
}

// Fit within 640x640 without upscaling, bounding portrait and landscape output equally.
const previewScale = "scale='min(640,iw)':'min(640,ih)':force_original_aspect_ratio=decrease:force_divisible_by=2"

// Use input-side accurate seeks and concat so work depends on selected segments rather than total
// video length. Empty segments are skipped, but -abort_on empty_output prevents publishing a
// video-less MP4 when all seeks miss, including sparse-keyframe MPEG-TS inputs.
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
