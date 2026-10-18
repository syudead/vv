package media

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"strconv"
	"time"
)

// Prefer ten percent to avoid opening logos/black frames, capped to keep long-video seeks short.
const (
	thumbnailFraction = 0.10
	// Seconds.
	thumbnailMinOffset = 1.0
	// Seconds.
	thumbnailMaxOffset = 60.0
)

// Bound each extraction so one input cannot stall ingestion.
const thumbnailTimeout = 60 * time.Second

const thumbnailCommand = "ffmpeg"

// Thumbnail uses universally available mjpeg rather than requiring libwebp; the caller supplies its
// temporary output path. It reports first-frame substitution as a value for the caller to persist.
func Thumbnail(ctx context.Context, videoPath string, durationMs int64, output string) (firstFrame bool, err error) {
	ctx, cancel := context.WithTimeout(ctx, thumbnailTimeout)
	defer cancel()

	positions := AutoThumbnailPositions(durationMs)
	err = runThumbnail(ctx, videoPath, positionSeconds(positions[0]), output)
	if err == nil {
		return false, nil
	}
	// If the selected position has no frame, retry once from the beginning rather than leave the
	// thumbnail absent.
	if len(positions) < 2 {
		return false, err
	}
	if retryErr := runThumbnail(ctx, videoPath, positionSeconds(positions[1]), output); retryErr != nil {
		return false, err
	}
	return true, nil
}

// AutoThumbnailPositions shares extraction order between jobs and editing: automatic position, then
// zero, with duplicates removed; nonpositive duration means unknown.
func AutoThumbnailPositions(durationMs int64) []int64 {
	auto := int64(math.Round(thumbnailOffset(durationMs) * 1000))
	if auto <= 0 {
		return []int64{0}
	}
	return []int64{auto, 0}
}

// Negative positions map to zero.
func positionSeconds(positionMs int64) float64 {
	return float64(max(positionMs, 0)) / 1000
}

// ThumbnailAt fails instead of silently substituting a different scene; callers validate duration
// bounds with domain.CheckThumbnailPosition.
func ThumbnailAt(ctx context.Context, videoPath string, positionMs int64, output string) error {
	ctx, cancel := context.WithTimeout(ctx, thumbnailTimeout)
	defer cancel()

	return runThumbnail(ctx, videoPath, positionSeconds(positionMs), output)
}

// Check output existence because ffmpeg can exit zero without producing a frame at the requested
// position.
func runThumbnail(ctx context.Context, videoPath string, offsetSec float64, output string) error {
	args := thumbnailArgs(videoPath, offsetSec, output)
	if _, err := command(ctx, thumbnailCommand, args...).Output(); err != nil {
		_ = os.Remove(output)

		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return fmt.Errorf(
				"%s failed (%s): %s", thumbnailCommand, videoPath, firstLine(exitErr.Stderr))
		}
		return fmt.Errorf("cannot run %s (%s): %w", thumbnailCommand, videoPath, err)
	}

	if info, err := os.Stat(output); err != nil || info.Size() == 0 {
		// Remove partial output so readers cannot mistake a truncated JPEG for a finished thumbnail.
		_ = os.Remove(output)
		return fmt.Errorf("thumbnail was not generated (%s, at %.3f s)", videoPath, offsetSec)
	}
	return nil
}

// For videos shorter than the minimum offset, use ten percent instead of clamping beyond the final
// frame; unknown duration uses the minimum.
func thumbnailOffset(durationMs int64) float64 {
	if durationMs <= 0 {
		return thumbnailMinOffset
	}

	seconds := float64(durationMs) / 1000
	offset := min(max(seconds*thumbnailFraction, thumbnailMinOffset), thumbnailMaxOffset)

	if offset >= seconds {
		// Ten percent is always inside a positive duration.
		return seconds * thumbnailFraction
	}
	return offset
}

// Bound the longest side to 640px in either orientation.
const thumbnailScale = "scale=640:640:force_original_aspect_ratio=decrease:force_divisible_by=2"

// Place -ss before -i for fast input seeking instead of decoding from the beginning.
func thumbnailArgs(videoPath string, offsetSec float64, output string) []string {
	return []string{
		"-nostdin",
		"-v", "error",
		"-ss", strconv.FormatFloat(offsetSec, 'f', 3, 64),
		"-i", videoPath,
		"-frames:v", "1",
		"-vf", thumbnailScale,
		"-q:v", "4",
		"-y",
		output,
	}
}
