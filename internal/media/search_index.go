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
	"slices"
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
// the start of the video; a keyframe without a time gets -1. Each keyframe goes to keep as soon as its time is known,
// so the read holds no list of every keyframe; keep may already have some when an error is returned.
func KeyframeHashes(ctx context.Context, videoPath string, keep func(domain.KeyframeHash)) error {
	run, cancel := context.WithTimeout(ctx, keyframeHashesTimeout)
	defer cancel()
	cmd := command(run, PriorityLow, thumbnailCommand, keyframeHashArgs(videoPath)...)
	stderr := &keyframeTimes{tail: tailWriter{limit: keyframeHashesStderr}}
	cmd.Stderr = stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("cannot run %s (%s): %w", thumbnailCommand, videoPath, err)
	}
	pairs := keyframePairs{times: stderr, keep: keep}
	readErr := readGrayFrameHashes(bufio.NewReaderSize(out, 1<<20), pairs.add)
	if readErr != nil {
		// Stop ffmpeg when reading ended early, so Wait does not block on a full pipe.
		cancel()
	}
	waitErr := cmd.Wait()
	if err := ctx.Err(); err != nil {
		return err
	}
	if errors.Is(run.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("%w: reading keyframes took longer than %v (%s)", domain.ErrFailureRepeats,
			keyframeHashesTimeout, videoPath)
	}
	if waitErr != nil {
		output := []byte(stderr.tail.String())
		return exitFailure(fmt.Errorf("%s failed (%s): %s", thumbnailCommand, videoPath, firstLine(output)), output)
	}
	if readErr != nil {
		return readErr
	}
	if times, frames := pairs.finish(); times != frames {
		return fmt.Errorf("%s reported %d keyframe times for %d frames (%s)", thumbnailCommand, times, frames,
			videoPath)
	}
	return nil
}

// keyframePairs gives each frame, in order, the next time showinfo reported. showinfo logs a frame before ffmpeg
// writes it, so a frame waits only until standard error is copied and few are ever held.
type keyframePairs struct {
	times   *keyframeTimes
	keep    func(domain.KeyframeHash)
	pending []domain.FrameHash
	frames  int
}

func (p *keyframePairs) add(frame domain.FrameHash) {
	p.frames++
	p.pending = append(p.pending, frame)
	p.pair()
}

func (p *keyframePairs) pair() {
	times := p.times.take(len(p.pending))
	for i, timeMs := range times {
		p.keep(domain.KeyframeHash{TimeMs: timeMs, Frame: p.pending[i]})
	}
	p.pending = p.pending[len(times):]
}

// finish pairs the frames still waiting once standard error is complete and returns how many times and frames ffmpeg
// reported in all.
func (p *keyframePairs) finish() (times, frames int) {
	p.pair()
	return p.times.reported(), p.frames
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

// readGrayFrameHashes hashes every binary PGM frame of r in order; a stream that ends inside a frame is an error.
func readGrayFrameHashes(r *bufio.Reader, add func(domain.FrameHash)) error {
	var pix []byte
	for {
		width, height, err := readPGMHeader(r)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		size := width * height
		if cap(pix) < size {
			pix = make([]byte, size)
		}
		pix = pix[:size]
		if _, err := io.ReadFull(r, pix); err != nil {
			return fmt.Errorf("cannot read a keyframe: %w", err)
		}
		gray := luma{rect: image.Rect(0, 0, width, height), stride: width, pix: pix}
		add(gray.frameHash(gray.rect))
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
	// times are reported but not yet taken; total counts every reported time.
	times []int64
	total int
	tail  tailWriter
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
	timeMs := int64(-1)
	if err == nil && !math.IsNaN(seconds) && !math.IsInf(seconds, 0) {
		timeMs = int64(math.Round(seconds * 1000))
	}
	k.times = append(k.times, timeMs)
	k.total++
}

// take removes and returns up to n of the earliest times not yet taken.
func (k *keyframeTimes) take(n int) []int64 {
	k.mu.Lock()
	defer k.mu.Unlock()
	n = min(n, len(k.times))
	taken := slices.Clone(k.times[:n])
	k.times = k.times[n:]
	return taken
}

func (k *keyframeTimes) reported() int {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.total
}
