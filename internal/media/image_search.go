package media

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"strconv"
	"time"

	"github.com/syudead/vv/internal/domain"
)

const (
	queryProbeTimeout = 30 * time.Second
	// queryFramesTimeout bounds a full decode of a three-hour query on a slow machine; the request usually ends first.
	queryFramesTimeout = time.Hour
	queryFramesStderr  = 4 * 1024
	// refineFramesTimeout bounds one refinement, at most ten seconds of one video, so a stalled disk skips that video.
	refineFramesTimeout = time.Minute
)

// ImageFrameHash hashes an image query by the fingerprint's rule, so it compares with the search index's keyframes.
func ImageFrameHash(img image.Image) domain.FrameHash {
	plane := lumaPlane(img)
	return plane.frameHash(plane.rect)
}

// QueryVideoDuration returns the duration of a query file's first video stream other than a cover picture, or the
// file's when the stream has none, so a longer audio track does not count; a file without one wraps
// domain.ErrImageSearchUnreadable.
func QueryVideoDuration(ctx context.Context, path string) (int64, error) {
	run, cancel := context.WithTimeout(ctx, queryProbeTimeout)
	defer cancel()
	output, err := command(run, PriorityNormal, probeCommand,
		"-v", "error", "-select_streams", "V:0", "-show_entries", "stream=index,duration:format=duration", "-of", "json",
		path).Output()
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if err != nil {
		return 0, errors.Join(domain.ErrImageSearchUnreadable, fmt.Errorf("%s failed: %w", probeCommand, err))
	}
	var probed struct {
		Streams []struct {
			Duration string `json:"duration"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(output, &probed); err != nil {
		return 0, errors.Join(domain.ErrImageSearchUnreadable, err)
	}
	if len(probed.Streams) == 0 {
		return 0, fmt.Errorf("%w: no video stream", domain.ErrImageSearchUnreadable)
	}
	if ms, ok := probedMilliseconds(probed.Streams[0].Duration); ok {
		return ms, nil
	}
	if ms, ok := probedMilliseconds(probed.Format.Duration); ok {
		return ms, nil
	}
	return 0, fmt.Errorf("%w: no video stream with a duration", domain.ErrImageSearchUnreadable)
}

// probedMilliseconds reads an ffprobe duration in seconds; "N/A", an absent value and a negative one are not durations.
func probedMilliseconds(value string) (int64, bool) {
	ms, ok := secondsToMs(value)
	return ms, ok && ms >= 0
}

// QueryVideoFrames decodes the query's first video stream that is not a cover picture once a
// domain.ImageSearchQueryIntervalMs, frame i at i intervals, at most maxFrames, at the search index's size and
// gray, and hashes each frame as the index does. Cancelling ctx ends ffmpeg.
func QueryVideoFrames(ctx context.Context, path string, maxFrames int) ([]domain.FrameHash, error) {
	frames, err := decodeFrameHashes(ctx, queryFramesTimeout, queryFrameArgs(path, maxFrames))
	if err != nil && ctx.Err() == nil {
		return nil, errors.Join(domain.ErrImageSearchUnreadable, err)
	}
	return frames, err
}

// RefineFrameHashes decodes [startMs, endMs) of a library video's first video stream that is not a cover picture
// once a domain.ImageSearchRefineIntervalMs, hashed as the search index is, each with its time in the video. startMs
// is an indexed keyframe, so the seek lands on it; cancelling ctx ends ffmpeg.
func RefineFrameHashes(ctx context.Context, path string, startMs, endMs int64) ([]domain.KeyframeHash, error) {
	frames, err := decodeFrameHashes(ctx, refineFramesTimeout, refineFrameArgs(path, startMs, endMs))
	if err != nil {
		return nil, err
	}
	hashes := make([]domain.KeyframeHash, len(frames))
	for i, frame := range frames {
		hashes[i] = domain.KeyframeHash{TimeMs: startMs + int64(i)*domain.ImageSearchRefineIntervalMs, Frame: frame}
	}
	return hashes, nil
}

// decodeFrameHashes runs ffmpeg writing gray PGM frames to standard output and hashes each; ctx's error is returned
// as is.
func decodeFrameHashes(ctx context.Context, timeout time.Duration, args []string) ([]domain.FrameHash, error) {
	run, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := command(run, PriorityNormal, thumbnailCommand, args...)
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
		return nil, errors.Join(fmt.Errorf("%s failed: %s", thumbnailCommand, firstLine([]byte(stderr.String()))),
			readErr)
	}
	return frames, nil
}

// The fps filter takes the frame nearest each tick, starting from the first frame, so frame i is at i intervals.
func queryFrameArgs(path string, maxFrames int) []string {
	return grayFrameArgs(path, nil, domain.ImageSearchQueryIntervalMs, int64(maxFrames))
}

// The input seek before -i is exact when decoding, and output times restart at 0 there, so frame i is at i intervals
// after startMs.
func refineFrameArgs(path string, startMs, endMs int64) []string {
	seconds := func(ms int64) string { return strconv.FormatFloat(float64(ms)/1000, 'f', 3, 64) }
	frames := (endMs - startMs + domain.ImageSearchRefineIntervalMs - 1) / domain.ImageSearchRefineIntervalMs
	return grayFrameArgs(path, []string{"-ss", seconds(startMs), "-t", seconds(endMs - startMs)},
		domain.ImageSearchRefineIntervalMs, max(frames, 1))
}

// grayFrameArgs decodes the first video stream that is not a cover picture once an intervalMs, at most frames, at the
// search index's size and gray, as PGM frames on standard output; input options go before -i.
func grayFrameArgs(path string, input []string, intervalMs, frames int64) []string {
	args := append([]string{"-nostdin", "-hide_banner", "-nostats", "-v", "error"}, input...)
	return append(args,
		"-i", path,
		"-map", "0:V:0",
		"-an", "-sn", "-dn",
		"-vf", "fps=1000/"+strconv.FormatInt(intervalMs, 10)+","+seekSpriteFastScale+",format=gray",
		"-frames:v", strconv.FormatInt(frames, 10),
		"-c:v", "pgm",
		"-f", "image2pipe",
		"pipe:1",
	)
}
