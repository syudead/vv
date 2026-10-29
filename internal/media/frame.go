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

// Downscale only; never enlarge frames already within ThumbnailFrameMaxSide.
var frameScale = "scale='min(" + strconv.Itoa(domain.ThumbnailFrameMaxSide) + ",iw)':'min(" +
	strconv.Itoa(domain.ThumbnailFrameMaxSide) + ",ih)':force_original_aspect_ratio=decrease,setsar=1"

// FrameAt returns a bounded PNG at positionMs for editing; like ThumbnailAt, it never substitutes
// the first frame.
func FrameAt(ctx context.Context, videoPath string, positionMs int64) (domain.ThumbnailFrame, error) {
	ctx, cancel := context.WithTimeout(ctx, thumbnailTimeout)
	defer cancel()

	output, err := command(ctx, PriorityNormal, thumbnailCommand, frameArgs(videoPath, positionMs)...).Output()
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

func (a *Assets) FrameAt(ctx context.Context, path string, positionMs int64) (domain.ThumbnailFrame, error) {
	return FrameAt(ctx, path, positionMs)
}

func (a *Assets) AutoThumbnailPositions(durationMs int64) []int64 {
	return AutoThumbnailPositions(durationMs)
}
