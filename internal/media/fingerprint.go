package media

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"time"

	"github.com/syudead/vv/internal/domain"
)

const fingerprintBlackLevel = 16

const (
	// Frames are sampled once a second so a start shifted by a second or two still lines up with the other file.
	fingerprintSampleMs = 1000
	// Gray frames are this size before black bands are trimmed and they are shrunk to the hash size.
	fingerprintSampleWidth  = 96
	fingerprintSampleHeight = 54
	fingerprintTimeout      = 30 * time.Minute
)

// VideoFingerprint decodes the source once and hashes one gray frame per second; letterboxed and pillarboxed
// versions compare equal because black bands are trimmed before hashing.
func VideoFingerprint(ctx context.Context, videoPath string) (domain.Fingerprint, error) {
	ctx, cancel := context.WithTimeout(ctx, fingerprintTimeout)
	defer cancel()
	cmd := command(ctx, seekThumbnailCommand, fingerprintArgs(videoPath)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return domain.Fingerprint{}, fmt.Errorf("cannot read ffmpeg for the fingerprint: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return domain.Fingerprint{}, fmt.Errorf("cannot run ffmpeg for the fingerprint: %w", err)
	}
	frames, readErr := readFingerprintFrames(bufio.NewReader(stdout))
	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return domain.Fingerprint{}, ctx.Err()
		}
		return domain.Fingerprint{}, fmt.Errorf("ffmpeg failed to sample the fingerprint frames (%s): %w: %s",
			videoPath, err, firstLine(stderr.Bytes()))
	}
	if readErr != nil {
		return domain.Fingerprint{}, readErr
	}
	if len(frames) == 0 {
		return domain.Fingerprint{}, fmt.Errorf("ffmpeg returned no fingerprint frames (%s)", videoPath)
	}
	return domain.Fingerprint{Version: domain.FingerprintVersion, IntervalMs: fingerprintSampleMs, Frames: frames}, nil
}

func fingerprintArgs(videoPath string) []string {
	return []string{
		"-nostdin",
		"-v", "error",
		"-i", videoPath,
		"-map", "0:V:0",
		"-an", "-sn", "-dn",
		"-vf", fmt.Sprintf("fps=1000/%d:round=down,scale=%d:%d:flags=area,format=gray",
			fingerprintSampleMs, fingerprintSampleWidth, fingerprintSampleHeight),
		"-f", "rawvideo",
		"pipe:1",
	}
}

func readFingerprintFrames(r io.Reader) ([]domain.FrameHash, error) {
	size := fingerprintSampleWidth * fingerprintSampleHeight
	buffer := make([]byte, size)
	frame := luma{rect: image.Rect(0, 0, fingerprintSampleWidth, fingerprintSampleHeight),
		stride: fingerprintSampleWidth, pix: buffer}
	var frames []domain.FrameHash
	for {
		if _, err := io.ReadFull(r, buffer); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return frames, nil
			}
			return nil, fmt.Errorf("cannot read the fingerprint frames: %w", err)
		}
		frames = append(frames, domain.HashFrame(frame.shrink(frame.trimBlackBands(frame.rect))))
	}
}

func (a *Assets) VideoFingerprint(ctx context.Context, path string) (domain.Fingerprint, error) {
	return VideoFingerprint(ctx, path)
}

// Luma values range from 0 to 255.
type luma struct {
	rect   image.Rectangle
	stride int
	pix    []uint8
}

func (l luma) at(x, y int) uint8 {
	return l.pix[(y-l.rect.Min.Y)*l.stride+(x-l.rect.Min.X)]
}

func (l luma) rowMean(rect image.Rectangle, y int) int {
	total := 0
	for x := rect.Min.X; x < rect.Max.X; x++ {
		total += int(l.at(x, y))
	}
	return total / rect.Dx()
}

func (l luma) columnMean(rect image.Rectangle, x int) int {
	total := 0
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		total += int(l.at(x, y))
	}
	return total / rect.Dy()
}

// Remove black borders to compare letterboxed and pillarboxed versions; leave wholly dark frames
// intact so solid-frame detection excludes them.
func (l luma) trimBlackBands(rect image.Rectangle) image.Rectangle {
	trimmed := rect
	for trimmed.Dy() > 0 && l.rowMean(trimmed, trimmed.Min.Y) < fingerprintBlackLevel {
		trimmed.Min.Y++
	}
	for trimmed.Dy() > 0 && l.rowMean(trimmed, trimmed.Max.Y-1) < fingerprintBlackLevel {
		trimmed.Max.Y--
	}
	if trimmed.Dy() == 0 {
		return rect
	}
	for trimmed.Dx() > 0 && l.columnMean(trimmed, trimmed.Min.X) < fingerprintBlackLevel {
		trimmed.Min.X++
	}
	for trimmed.Dx() > 0 && l.columnMean(trimmed, trimmed.Max.X-1) < fingerprintBlackLevel {
		trimmed.Max.X--
	}
	if trimmed.Dx() == 0 {
		return rect
	}
	return trimmed
}

// Area-average into 32x32 luma, using a single pixel where a cell is smaller than one pixel.
func (l luma) shrink(rect image.Rectangle) [domain.FingerprintFrameSize][domain.FingerprintFrameSize]uint8 {
	const size = domain.FingerprintFrameSize
	var out [size][size]uint8
	for cy := range size {
		y0, y1 := span(rect.Min.Y, rect.Dy(), cy)
		for cx := range size {
			x0, x1 := span(rect.Min.X, rect.Dx(), cx)
			total, count := 0, 0
			for y := y0; y < y1; y++ {
				for x := x0; x < x1; x++ {
					total += int(l.at(x, y))
					count++
				}
			}
			out[cy][cx] = uint8((total + count/2) / count)
		}
	}
	return out
}

func span(origin, length, index int) (int, int) {
	start := origin + index*length/domain.FingerprintFrameSize
	end := origin + (index+1)*length/domain.FingerprintFrameSize
	if end <= start {
		end = start + 1
	}
	return start, end
}
