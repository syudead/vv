package media

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/syudead/vv/internal/domain"
)

const (
	seekThumbnailCommand   = "ffmpeg"
	seekThumbnailLookahead = time.Second
	seekThumbnailWaitDelay = 5 * time.Second
)

// SeekThumbnailExtractor generates one request-scoped JPEG without persisting it.
type SeekThumbnailExtractor struct {
	serverDone     <-chan struct{}
	commandContext func(context.Context, string, ...string) *exec.Cmd
}

func NewSeekThumbnailExtractor(serverDone <-chan struct{}) *SeekThumbnailExtractor {
	if serverDone == nil {
		serverDone = make(chan struct{})
	}
	return &SeekThumbnailExtractor{serverDone: serverDone, commandContext: exec.CommandContext}
}

// Extract returns the first frame at or after positionMs within one second. If
// the media ends first, it retries the preceding one-second window and returns
// that window's final frame.
func (e *SeekThumbnailExtractor) Extract(ctx context.Context, path string, positionMs int64) ([]byte, error) {
	ctx, cancel := context.WithCancel(ctx)
	watchDone := make(chan struct{})
	go func() {
		select {
		case <-e.serverDone:
			cancel()
		case <-watchDone:
		}
	}()
	cleanup := sync.OnceFunc(func() {
		close(watchDone)
		cancel()
	})
	defer cleanup()

	image, err := e.run(ctx, seekThumbnailArgs(path, positionMs, false))
	if err == nil {
		return image, nil
	}
	if !errors.Is(err, domain.ErrSeekFrameUnavailable) || ctx.Err() != nil {
		return nil, err
	}

	image, retryErr := e.run(ctx, seekThumbnailArgs(path, positionMs, true))
	if retryErr != nil {
		return nil, retryErr
	}
	return image, nil
}

func (e *SeekThumbnailExtractor) run(ctx context.Context, args []string) ([]byte, error) {
	cmd := e.commandContext(ctx, seekThumbnailCommand, args...)
	cmd.WaitDelay = seekThumbnailWaitDelay
	output, err := cmd.Output()
	if err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return nil, fmt.Errorf("シークサムネイル生成を中断しました: %w", contextErr)
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf("%sが失敗しました: %w; stderr: %s",
				seekThumbnailCommand, err, firstLine(exitErr.Stderr))
		}
		return nil, fmt.Errorf("%sを開始できません: %w", seekThumbnailCommand, err)
	}
	if len(output) == 0 {
		return nil, domain.ErrSeekFrameUnavailable
	}
	return output, nil
}

func seekThumbnailArgs(path string, positionMs int64, tail bool) []string {
	startMs := positionMs
	duration := seekThumbnailLookahead
	// min(320, iw) keeps small sources at their original width instead of upscaling.
	filter := "scale=min(320\\,iw):-2,format=yuvj420p"
	if tail {
		startMs = max(0, positionMs-seekThumbnailLookahead.Milliseconds())
		duration = time.Duration(positionMs-startMs) * time.Millisecond
		filter = "reverse," + filter
	}

	return []string{
		"-nostdin",
		"-v", "error",
		"-ss", formatSeekMilliseconds(startMs),
		"-i", path,
		"-t", strconv.FormatFloat(duration.Seconds(), 'f', 3, 64),
		"-map", "0:V:0?",
		"-an",
		"-frames:v", "1",
		"-vf", filter,
		"-q:v", "4",
		"-f", "image2pipe",
		"-vcodec", "mjpeg",
		"pipe:1",
	}
}

func formatSeekMilliseconds(milliseconds int64) string {
	return strconv.FormatFloat(float64(milliseconds)/1000, 'f', 3, 64)
}
