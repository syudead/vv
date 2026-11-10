package media

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/syudead/vv/internal/domain"
)

const (
	// A whole-file read of a long video on a spinning disk takes minutes; a run past this never finishes.
	keyframeHashesTimeout = time.Hour
	keyframeHashesStderr  = 4 * 1024
	// keyframeFrameMaxBytes bounds one PGM frame, at most 320 pixels on either side after the sprite scale.
	keyframeFrameMaxBytes = 320 * 320
)

// KeyframeHashes reads the video's first video stream that is not a cover picture once, in order, decoding only its
// keyframes at the seek sprite's size in full-range gray, and hashes each by the fingerprint's rule. Times are from
// the start of the video; a keyframe without a time gets -1.
func KeyframeHashes(ctx context.Context, videoPath string) ([]domain.KeyframeHash, error) {
	run, cancel := context.WithTimeout(ctx, keyframeHashesTimeout)
	defer cancel()
	cmd := command(run, PriorityLow, thumbnailCommand, keyframeHashArgs(videoPath)...)
	stderr := &keyframeTimes{tail: tailWriter{limit: keyframeHashesStderr}}
	cmd.Stderr = stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("cannot run %s (%s): %w", thumbnailCommand, videoPath, err)
	}
	frames, readErr := readGrayFrameHashes(bufio.NewReaderSize(out, 1<<20))
	if readErr != nil {
		// Stop ffmpeg when reading ended early, so Wait does not block on a full pipe.
		cancel()
	}
	waitErr := cmd.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if errors.Is(run.Err(), context.DeadlineExceeded) {
		return nil, fmt.Errorf("%w: reading keyframes took longer than %v (%s)", domain.ErrFailureRepeats,
			keyframeHashesTimeout, videoPath)
	}
	if waitErr != nil {
		output := []byte(stderr.tail.String())
		return nil, exitFailure(fmt.Errorf("%s failed (%s): %s", thumbnailCommand, videoPath, firstLine(output)), output)
	}
	if readErr != nil {
		return nil, readErr
	}
	times := stderr.timesMs()
	if len(times) != len(frames) {
		return nil, fmt.Errorf("%s reported %d keyframe times for %d frames (%s)", thumbnailCommand, len(times),
			len(frames), videoPath)
	}
	hashes := make([]domain.KeyframeHash, len(frames))
	for i, frame := range frames {
		hashes[i] = domain.KeyframeHash{TimeMs: times[i], Frame: frame}
	}
	return hashes, nil
}

// showinfo reports each frame's time on standard error, in the order the frames reach standard output.
func keyframeHashArgs(videoPath string) []string {
	return []string{
		"-nostdin", "-hide_banner", "-nostats",
		"-v", "level+info",
		"-skip_frame", "nokey",
		"-i", videoPath,
		"-map", "0:V:0",
		"-an", "-sn", "-dn",
		"-vf", seekSpriteFastScale + ",format=gray,showinfo",
		"-fps_mode", "passthrough",
		"-c:v", "pgm",
		"-f", "image2pipe",
		"pipe:1",
	}
}

// readGrayFrameHashes hashes every binary PGM frame of r; a stream that ends inside a frame is an error.
func readGrayFrameHashes(r *bufio.Reader) ([]domain.FrameHash, error) {
	var frames []domain.FrameHash
	var pix []byte
	for {
		width, height, err := readPGMHeader(r)
		if errors.Is(err, io.EOF) {
			return frames, nil
		}
		if err != nil {
			return nil, err
		}
		size := width * height
		if cap(pix) < size {
			pix = make([]byte, size)
		}
		pix = pix[:size]
		if _, err := io.ReadFull(r, pix); err != nil {
			return nil, fmt.Errorf("cannot read a keyframe: %w", err)
		}
		gray := luma{rect: image.Rect(0, 0, width, height), stride: width, pix: pix}
		frames = append(frames, gray.frameHash(gray.rect))
	}
}

// readPGMHeader reads the "P5 width height 255" header ffmpeg's pgm encoder writes, one field per whitespace run.
// io.EOF means the stream ended cleanly before a header.
func readPGMHeader(r *bufio.Reader) (width, height int, err error) {
	var fields [4]string
	for i := range fields {
		if fields[i], err = readPGMField(r); err != nil {
			if i == 0 && errors.Is(err, io.EOF) {
				return 0, 0, io.EOF
			}
			return 0, 0, fmt.Errorf("cannot read a keyframe header: %w", err)
		}
	}
	width, errW := strconv.Atoi(fields[1])
	height, errH := strconv.Atoi(fields[2])
	if fields[0] != "P5" || fields[3] != "255" || errW != nil || errH != nil || width <= 0 || height <= 0 ||
		width*height > keyframeFrameMaxBytes {
		return 0, 0, fmt.Errorf("unexpected keyframe header %q", strings.Join(fields[:], " "))
	}
	return width, height, nil
}

// readPGMField reads one header field and the single whitespace byte after it.
func readPGMField(r *bufio.Reader) (string, error) {
	var field []byte
	for {
		b, err := r.ReadByte()
		if err != nil {
			if len(field) > 0 && errors.Is(err, io.EOF) {
				err = io.ErrUnexpectedEOF
			}
			return "", err
		}
		if b == ' ' || b == '\n' || b == '\t' || b == '\r' {
			if len(field) > 0 {
				return string(field), nil
			}
			continue
		}
		if len(field) == 16 {
			return "", errors.New("the keyframe header field is too long")
		}
		field = append(field, b)
	}
}

// keyframeTimes takes ffmpeg's standard error: showinfo's frame lines give the keyframe times, and every line that
// is not informational goes to the tail kept for a failure message.
type keyframeTimes struct {
	mu      sync.Mutex
	partial []byte
	times   []int64
	tail    tailWriter
}

func (k *keyframeTimes) Write(p []byte) (int, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.partial = append(k.partial, p...)
	for {
		end := bytes.IndexByte(k.partial, '\n')
		if end < 0 {
			break
		}
		k.line(string(k.partial[:end]))
		k.partial = k.partial[end+1:]
	}
	return len(p), nil
}

func (k *keyframeTimes) line(line string) {
	_, info, isInfo := strings.Cut(line, "[info] ")
	if !isInfo {
		_, _ = k.tail.Write([]byte(line + "\n"))
		return
	}
	if !strings.HasPrefix(info, "n:") {
		return
	}
	_, after, ok := strings.Cut(info, " pts_time:")
	if !ok {
		return
	}
	value, _, _ := strings.Cut(after, " ")
	seconds, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
		k.times = append(k.times, -1)
		return
	}
	k.times = append(k.times, int64(math.Round(seconds*1000)))
}

func (k *keyframeTimes) timesMs() []int64 {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.times
}
