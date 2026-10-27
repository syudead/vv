package media

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/syudead/vv/internal/domain"
)

const (
	seekThumbnailCommand = "ffmpeg"
	// Indexed reads and per-interval extraction normally finish in seconds; a longer run means the input
	// will not finish on retry either.
	seekThumbnailTimeout = 5 * time.Minute
	seekSpriteFastScale  = "scale=min(320\\,iw):min(320\\,ih):force_original_aspect_ratio=decrease:force_divisible_by=2"
	// Use JPEG q=2 to limit blocking when cards enlarge sprite frames.
	seekSpriteQuality  = "2"
	seekSpriteParallel = 4
	// One interval normally decodes at most a GOP. Some AV1 MP4 files make ffmpeg seeking decode from
	// the start, so later intervals grow without bound; fail instead of waiting them out.
	seekFrameTimeout       = 30 * time.Second
	seekSpriteReadParallel = 8
	// Require enough distinct indexed keyframes; sparse GOPs need segment extraction to avoid
	// repeating the same scene.
	seekIndexMinDistinctNum = 3
	seekIndexMinDistinctDen = 4
)

// GenerateSeekSprite tries indexed keyframes, then segment seeking. Exceeding a time limit wraps
// domain.ErrFailureRepeats. Temporary files stay under outputDir and callers publish results; see
// docs/design-docs/seek-sprite-generation.md.
func GenerateSeekSprite(ctx context.Context, videoPath, outputDir string, layout domain.SeekSpriteLayout) error {
	processCtx, cancel := context.WithTimeout(ctx, seekThumbnailTimeout)
	defer cancel()
	err := generateSeekSprite(processCtx, videoPath, outputDir, layout)
	if ctx.Err() != nil {
		return fmt.Errorf("seek thumbnail generation was interrupted: %w", ctx.Err())
	}
	if processCtx.Err() != nil {
		return fmt.Errorf("%w: seek thumbnail generation took longer than %v", domain.ErrFailureRepeats, seekThumbnailTimeout)
	}
	return err
}

func generateSeekSprite(ctx context.Context, videoPath, outputDir string, layout domain.SeekSpriteLayout) error {
	err := generateSeekSpriteFromIndex(ctx, videoPath, outputDir, layout)
	if err == nil || ctx.Err() != nil {
		return err
	}
	if !errors.Is(err, errSeekIndexUnsupported) {
		slog.WarnContext(ctx, "cannot build seek thumbnails from the index; extracting per interval", "error", err)
	}
	return generateSeekSpriteParallel(ctx, videoPath, outputDir, layout)
}

// Read the movie index once and only selected keyframe bytes; reject unsupported or overly
// repetitive selections.
func generateSeekSpriteFromIndex(ctx context.Context, videoPath, outputDir string, layout domain.SeekSpriteLayout) error {
	file, err := os.Open(videoPath)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	movie, fragments, err := readMovieBox(ctx, file, info.Size())
	if err != nil {
		return err
	}
	track, err := parseKeyframeTrack(movie, fragments)
	if err != nil {
		return err
	}

	// Read and decode repeated keyframes once, then copy their images.
	picks := track.pick(layout)
	decodeOrder := make(map[int]int, len(picks))
	var unique []int
	for _, pick := range picks {
		if _, ok := decodeOrder[pick]; !ok {
			decodeOrder[pick] = len(unique)
			unique = append(unique, pick)
		}
	}
	if len(unique)*seekIndexMinDistinctDen < layout.FrameCount*seekIndexMinDistinctNum {
		return fmt.Errorf("%w: only %d distinct keyframes for %d frames",
			errSeekIndexUnsupported, len(unique), layout.FrameCount)
	}

	temporary, err := os.MkdirTemp(outputDir, ".seek-sprite-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(temporary) }()
	// ffmpeg runs in the keys directory, so make the output path absolute.
	if temporary, err = filepath.Abs(temporary); err != nil {
		return err
	}
	keys := filepath.Join(temporary, "keys")
	decoded := filepath.Join(temporary, "decoded")
	for _, dir := range []string{keys, decoded} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			return err
		}
	}
	allIDR, err := readKeyframes(ctx, file, track, unique, keys)
	if err != nil {
		return err
	}
	if _, err := runSeekFFmpegIn(ctx, keys, keyframeDecodeArgs(track, len(unique), allIDR, decoded)); err != nil {
		return err
	}
	// Missing decoded frames shift later scene positions; reject count mismatches.
	if _, err := os.Stat(filepath.Join(decoded, fmt.Sprintf("%03d.bmp", len(unique)))); err == nil {
		return fmt.Errorf("got more images than the %d keyframes", len(unique))
	}
	for k, pick := range picks {
		source := filepath.Join(decoded, fmt.Sprintf("%03d.bmp", decodeOrder[pick]))
		if err := copyFile(source, filepath.Join(temporary, fmt.Sprintf("%03d.bmp", k))); err != nil {
			return fmt.Errorf("could not decode the keyframe for frame %d: %w", k, err)
		}
	}
	return tileSeekSprite(ctx, temporary, outputDir, layout)
}

// IDR resets decoder order/state and can share one input; non-IDR keyframes need separate decoders
// and concat to avoid POC reordering. showall allows standalone I frames to produce output.
func keyframeDecodeArgs(track *keyframeTrack, count int, allIDR bool, decoded string) []string {
	format := track.format
	filter := seekSpriteFastScale
	if track.displayFilter != "" {
		filter = track.displayFilter + "," + filter
	}
	args := []string{"-nostdin", "-v", "error"}
	output := []string{"-fps_mode", "passthrough", "-c:v", "bmp", "-start_number", "0", "-y", filepath.Join(decoded, "%03d.bmp")}
	if allIDR {
		names := make([]string, count)
		for i := range names {
			names[i] = keyframeFileName(i, format)
		}
		args = append(args, "-f", format, "-i", "concat:"+strings.Join(names, "|"), "-vf", filter)
		return append(args, output...)
	}
	var inputs strings.Builder
	for i := range count {
		args = append(args, "-threads", "1", "-flags2", "+showall", "-f", format, "-i", keyframeFileName(i, format))
		fmt.Fprintf(&inputs, "[%d:v]", i)
	}
	args = append(args, "-filter_complex", fmt.Sprintf("%sconcat=n=%d:v=1:a=0,%s", inputs.String(), count, filter))
	return append(args, output...)
}

func keyframeFileName(i int, format string) string {
	return fmt.Sprintf("%03d.%s", i, format)
}

// readKeyframes writes ffmpeg-ready frames concurrently and reports whether all are IDR.
func readKeyframes(ctx context.Context, r io.ReaderAt, track *keyframeTrack, indexes []int, dir string) (bool, error) {
	var total int64
	for _, index := range indexes {
		size := track.keyframes[index].size
		if size <= 0 || size > maxKeyframeSize {
			return false, fmt.Errorf("%w: keyframe size %d", errSeekIndexUnsupported, size)
		}
		if total += size; total > maxKeyframeTotalSize {
			return false, fmt.Errorf("%w: keyframes exceed %d bytes in total", errSeekIndexUnsupported, int64(maxKeyframeTotalSize))
		}
	}
	semaphore := make(chan struct{}, seekSpriteReadParallel)
	var workers sync.WaitGroup
	var firstErr error
	idr := make([]bool, len(indexes))
	var recordError sync.Once
	for i, index := range indexes {
		frame := track.keyframes[index]
		select {
		case semaphore <- struct{}{}:
		case <-ctx.Done():
			workers.Wait()
			return false, ctx.Err()
		}
		workers.Go(func() {
			defer func() { <-semaphore }()
			if ctx.Err() != nil {
				return
			}
			data := make([]byte, frame.size)
			_, err := readAt(ctx, r, data, frame.offset)
			if err != nil {
				err = fmt.Errorf("cannot read keyframe (offset %d): %w", frame.offset, err)
			} else if data, idr[i], err = track.annexB(data); err == nil {
				err = os.WriteFile(filepath.Join(dir, keyframeFileName(i, track.format)), data, 0600)
			}
			if err != nil {
				recordError.Do(func() { firstErr = err })
			}
		})
	}
	workers.Wait()
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return !slices.Contains(idr, false), firstErr
}

// generateSeekSpriteParallel duplicates nearby frames for empty intervals, failing on ffmpeg errors,
// an interval over seekFrameTimeout or wholly empty output.
func generateSeekSpriteParallel(ctx context.Context, videoPath, outputDir string, layout domain.SeekSpriteLayout) error {
	return generateSeekSpriteWithin(ctx, videoPath, outputDir, layout, seekFrameTimeout)
}

func generateSeekSpriteWithin(
	ctx context.Context, videoPath, outputDir string, layout domain.SeekSpriteLayout, frameTimeout time.Duration,
) error {
	temporary, err := os.MkdirTemp(outputDir, ".seek-sprite-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(temporary) }()

	workCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	semaphore := make(chan struct{}, seekSpriteParallel)
	var workers sync.WaitGroup
	var firstErr error
	var recordError sync.Once
	missing := make([]bool, layout.FrameCount)
frames:
	for frame := range layout.FrameCount {
		select {
		case semaphore <- struct{}{}:
		case <-workCtx.Done():
			break frames
		}
		if workCtx.Err() != nil {
			<-semaphore
			break frames
		}
		workers.Add(1)
		go func(frame int) {
			defer workers.Done()
			defer func() { <-semaphore }()
			frameCtx, cancelFrame := context.WithTimeout(workCtx, frameTimeout)
			data, err := runSeekFFmpeg(frameCtx, seekFrameArgs(videoPath, frame, layout.IntervalMs))
			cancelFrame()
			if errors.Is(err, context.DeadlineExceeded) && workCtx.Err() == nil {
				err = fmt.Errorf("%w: extraction took longer than %v", domain.ErrFailureRepeats, frameTimeout)
			}
			if err == nil && len(data) == 0 {
				missing[frame] = true
				return
			}
			if err == nil && (len(data) < 54 || string(data[:2]) != "BM") {
				err = errors.New("could not extract an image at the seek position")
			}
			if err == nil {
				err = os.WriteFile(filepath.Join(temporary, fmt.Sprintf("%03d.bmp", frame)), data, 0600)
			}
			if err != nil {
				recordError.Do(func() { firstErr = fmt.Errorf("frame %d: %w", frame, err); cancel() })
			}
		}(frame)
	}
	workers.Wait()
	if firstErr != nil {
		return firstErr
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := fillMissingFrames(temporary, missing); err != nil {
		return err
	}
	return tileSeekSprite(ctx, temporary, outputDir, layout)
}

// seekFrameArgs extracts the first frame of one interval as a BMP on stdout. A damaged interval, such as zeroed bytes
// in a partly downloaded file, yields no image and reuses a nearby frame: -max_error_rate 1 keeps ffmpeg 7 and later
// from exiting with status 69 when most packets fail to decode.
func seekFrameArgs(videoPath string, frame int, intervalMs int64) []string {
	at := strconv.FormatFloat(float64(int64(frame)*intervalMs)/1000, 'f', 3, 64)
	interval := strconv.FormatFloat(float64(intervalMs)/1000, 'f', 3, 64)
	return []string{
		"-nostdin", "-v", "error", "-max_error_rate", "1", "-ss", at, "-t", interval, "-i", videoPath,
		"-map", "0:V:0?", "-frames:v", "1", "-vf", "trim=end=" + interval + "," + seekSpriteFastScale,
		"-c:v", "bmp", "-f", "rawvideo", "pipe:1",
	}
}

// Prefer the preceding frame, falling back to the following frame if needed.
func fillMissingFrames(dir string, missing []bool) error {
	for frame, isMissing := range missing {
		if !isMissing {
			continue
		}
		source := -1
		for earlier := frame - 1; earlier >= 0 && source < 0; earlier-- {
			if !missing[earlier] {
				source = earlier
			}
		}
		for later := frame + 1; later < len(missing) && source < 0; later++ {
			if !missing[later] {
				source = later
			}
		}
		if source < 0 {
			return errors.New("could not extract an image from any interval")
		}
		if err := copyFile(filepath.Join(dir, fmt.Sprintf("%03d.bmp", source)), filepath.Join(dir, fmt.Sprintf("%03d.bmp", frame))); err != nil {
			return err
		}
	}
	return nil
}

func tileSeekSprite(ctx context.Context, frameDir, outputDir string, layout domain.SeekSpriteLayout) error {
	_, err := runSeekFFmpeg(ctx, []string{
		"-nostdin", "-v", "error", "-framerate", "1", "-i", filepath.Join(frameDir, "%03d.bmp"),
		"-vf", fmt.Sprintf("tile=%dx%d,format=yuvj420p", layout.Columns, layout.Rows),
		"-frames:v", "1", "-q:v", seekSpriteQuality, "-y", filepath.Join(outputDir, "000.jpg"),
	})
	return err
}

func copyFile(source, destination string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	return os.WriteFile(destination, data, 0600)
}

func runSeekFFmpeg(ctx context.Context, args []string) ([]byte, error) {
	return runSeekFFmpegIn(ctx, "", args)
}

// Use relative inputs from dir to keep multi-input command lines short.
func runSeekFFmpegIn(ctx context.Context, dir string, args []string) ([]byte, error) {
	cmd := command(ctx, seekThumbnailCommand, args...)
	cmd.Dir = dir
	data, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf("ffmpeg failed to generate seek thumbnails: %w: %s", err, firstLine(exitErr.Stderr))
		}
		return nil, fmt.Errorf("cannot run ffmpeg: %w", err)
	}
	return data, nil
}

// GenerateSeekThumbnailSet is previewbench's generation-only boundary; it requires an existing
// outputDir.
func GenerateSeekThumbnailSet(ctx context.Context, videoPath, outputDir string, durationMs int64) error {
	return GenerateSeekSprite(ctx, videoPath, outputDir, domain.NewSeekSpriteLayout(durationMs))
}
