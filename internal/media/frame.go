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

// frameScale downscales only; it never enlarges frames already within maxSide.
func frameScale(maxSide int) string {
	side := strconv.Itoa(maxSide)
	return "scale='min(" + side + ",iw)':'min(" + side + ",ih)':force_original_aspect_ratio=decrease,setsar=1"
}

// posterScale sizes the poster like a full-size frame, so the poster and FrameAt agree.
var posterScale = frameScale(domain.ThumbnailFrameMaxSide)

// FrameAt returns a PNG at positionMs whose long side is at most maxSide; like ThumbnailAt, it never
// substitutes the first frame.
func FrameAt(ctx context.Context, videoPath string, positionMs int64, maxSide int) (domain.ThumbnailFrame, error) {
	ctx, cancel := context.WithTimeout(ctx, thumbnailTimeout)
	defer cancel()

	output, err := command(ctx, PriorityNormal, thumbnailCommand, frameArgs(videoPath, positionMs, maxSide)...).Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return domain.ThumbnailFrame{}, fmt.Errorf("%s failed (%s): %s", thumbnailCommand, videoPath, firstLine(exitErr.Stderr))
		}
		return domain.ThumbnailFrame{}, fmt.Errorf("cannot run %s (%s): %w", thumbnailCommand, videoPath, err)
	}
	// ffmpeg may exit successfully without writing a frame at an unavailable position.
	config, err := png.DecodeConfig(bytes.NewReader(output))
	if err != nil {
		return domain.ThumbnailFrame{}, fmt.Errorf("no frame was extracted (%s, at %d ms): %w", videoPath, positionMs, err)
	}
	return domain.ThumbnailFrame{PNG: output, Width: config.Width, Height: config.Height}, nil
}

// Put -ss before -i for the same seeking behavior as thumbnailArgs.
func frameArgs(videoPath string, positionMs int64, maxSide int) []string {
	return []string{
		"-nostdin",
		"-v", "error",
		"-ss", strconv.FormatFloat(positionSeconds(positionMs), 'f', 3, 64),
		"-i", videoPath,
		"-frames:v", "1",
		"-vf", frameScale(maxSide),
		"-c:v", "png",
		"-f", "image2pipe",
		"pipe:1",
	}
}

func (a *Assets) FrameAt(ctx context.Context, path string, positionMs int64, maxSide int) (domain.ThumbnailFrame, error) {
	return FrameAt(ctx, path, positionMs, maxSide)
}

func (a *Assets) AutoThumbnailPositions(durationMs int64) []int64 {
	return AutoThumbnailPositions(durationMs)
}
