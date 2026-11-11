package media

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"math"
	"strconv"
	"time"

	"github.com/syudead/vv/internal/domain"
)

const (
	queryProbeTimeout = 30 * time.Second
	// queryFramesTimeout bounds a full decode of a three-hour query on a slow machine; the request usually ends first.
	queryFramesTimeout = time.Hour
	queryFramesStderr  = 4 * 1024
)

// ImageFrameHash hashes an image query by the fingerprint's rule, so it compares with the search index's keyframes.
func ImageFrameHash(img image.Image) domain.FrameHash {
	plane := lumaPlane(img)
	return plane.frameHash(plane.rect)
}

// QueryVideoDuration returns the duration of a query file that has a video stream other than a cover picture; a file
// without one wraps domain.ErrImageSearchUnreadable.
func QueryVideoDuration(ctx context.Context, path string) (int64, error) {
	run, cancel := context.WithTimeout(ctx, queryProbeTimeout)
	defer cancel()
	output, err := command(run, PriorityNormal, probeCommand,
		"-v", "error", "-select_streams", "V:0", "-show_entries", "stream=index:format=duration", "-of", "json",
		path).Output()
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if err != nil {
		return 0, errors.Join(domain.ErrImageSearchUnreadable, fmt.Errorf("%s failed: %w", probeCommand, err))
	}
	var probed struct {
		Streams []struct{} `json:"streams"`
		Format  struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(output, &probed); err != nil {
		return 0, errors.Join(domain.ErrImageSearchUnreadable, err)
	}
	seconds, err := strconv.ParseFloat(probed.Format.Duration, 64)
	if len(probed.Streams) == 0 || err != nil || math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 {
		return 0, fmt.Errorf("%w: no video stream with a duration", domain.ErrImageSearchUnreadable)
	}
	return int64(math.Round(seconds * 1000)), nil
}

// QueryVideoFrames decodes the query's first video stream that is not a cover picture once a
// domain.ImageSearchQueryIntervalMs, frame i at i intervals, at most maxFrames, at the search index's size and
// gray, and hashes each frame as the index does. Cancelling ctx ends ffmpeg.
func QueryVideoFrames(ctx context.Context, path string, maxFrames int) ([]domain.FrameHash, error) {
	run, cancel := context.WithTimeout(ctx, queryFramesTimeout)
	defer cancel()
	cmd := command(run, PriorityNormal, thumbnailCommand, queryFrameArgs(path, maxFrames)...)
	stderr := &tailWriter{limit: queryFramesStderr}
	cmd.Stderr = stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("cannot run %s: %w", thumbnailCommand, err)
	}
	var frames []domain.FrameHash
	readErr := readGrayFrameHashes(bufio.NewReaderSize(out, 1<<20), func(frame domain.FrameHash) {
		frames = append(frames, frame)
	})
	if readErr != nil {
		// Stop ffmpeg when reading ended early, so Wait does not block on a full pipe.
		cancel()
	}
	waitErr := cmd.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if waitErr != nil || readErr != nil {
		return nil, errors.Join(domain.ErrImageSearchUnreadable,
			fmt.Errorf("%s failed: %s", thumbnailCommand, firstLine([]byte(stderr.String()))), readErr)
	}
	return frames, nil
}

// The fps filter takes the frame nearest each tick, starting from the first frame, so frame i is at i intervals.
func queryFrameArgs(path string, maxFrames int) []string {
	return []string{
		"-nostdin", "-hide_banner", "-nostats",
		"-v", "error",
		"-i", path,
		"-map", "0:V:0",
		"-an", "-sn", "-dn",
		"-vf", "fps=1000/" + strconv.FormatInt(domain.ImageSearchQueryIntervalMs, 10) + "," + seekSpriteFastScale +
			",format=gray",
		"-frames:v", strconv.Itoa(maxFrames),
		"-c:v", "pgm",
		"-f", "image2pipe",
		"pipe:1",
	}
}
