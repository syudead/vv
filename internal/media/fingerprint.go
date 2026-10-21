package media

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"

	"github.com/syudead/vv/internal/domain"
)

// Fingerprint completed sprites without rerunning ffmpeg or reading source video.

const fingerprintBlackLevel = 16

var ErrSpriteMismatch = errors.New("the seek sprite sheets do not match its layout")

// SpriteFingerprint crops each frame, removes black borders, and hashes 32x32 luma; sheets are
// ordered from zero.
func SpriteFingerprint(sprite domain.SeekSprite, sheets [][]byte) (domain.Fingerprint, error) {
	perSheet := sprite.Columns * sprite.Rows
	if sprite.IntervalMs <= 0 || sprite.FrameCount <= 0 || perSheet <= 0 ||
		sprite.FrameWidth <= 0 || sprite.FrameHeight <= 0 {
		return domain.Fingerprint{}, fmt.Errorf("%w: %+v", ErrSpriteMismatch, sprite)
	}
	needed := (sprite.FrameCount + perSheet - 1) / perSheet
	if len(sheets) < needed {
		return domain.Fingerprint{}, fmt.Errorf("%w: %d sheets for %d frames", ErrSpriteMismatch, len(sheets), sprite.FrameCount)
	}
	frames := make([]domain.FrameHash, 0, sprite.FrameCount)
	for sheetIndex := range needed {
		sheet, err := jpeg.Decode(bytes.NewReader(sheets[sheetIndex]))
		if err != nil {
			return domain.Fingerprint{}, fmt.Errorf("cannot decode seek sprite sheet %d: %w", sheetIndex, err)
		}
		luma := lumaPlane(sheet)
		for slot := range perSheet {
			frame := sheetIndex*perSheet + slot
			if frame >= sprite.FrameCount {
				break
			}
			origin := luma.rect.Min.Add(image.Pt(
				(slot%sprite.Columns)*sprite.FrameWidth, (slot/sprite.Columns)*sprite.FrameHeight))
			rect := image.Rectangle{Min: origin, Max: origin.Add(image.Pt(sprite.FrameWidth, sprite.FrameHeight))}
			if !rect.In(luma.rect) {
				return domain.Fingerprint{}, fmt.Errorf("%w: frame %d is outside sheet %d", ErrSpriteMismatch, frame, sheetIndex)
			}
			frames = append(frames, domain.HashFrame(luma.shrink(luma.trimBlackBands(rect))))
		}
	}
	return domain.Fingerprint{Version: domain.FingerprintVersion, IntervalMs: sprite.IntervalMs, Frames: frames}, nil
}

// Luma values range from 0 to 255.
type luma struct {
	rect   image.Rectangle
	stride int
	pix    []uint8
}

// Use JPEG's Y plane directly when available.
func lumaPlane(img image.Image) luma {
	bounds := img.Bounds()
	switch typed := img.(type) {
	case *image.YCbCr:
		return luma{rect: bounds, stride: typed.YStride, pix: typed.Y[typed.YOffset(bounds.Min.X, bounds.Min.Y):]}
	case *image.Gray:
		return luma{rect: bounds, stride: typed.Stride, pix: typed.Pix[typed.PixOffset(bounds.Min.X, bounds.Min.Y):]}
	}
	out := luma{rect: bounds, stride: bounds.Dx(), pix: make([]uint8, bounds.Dx()*bounds.Dy())}
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			gray, _ := color.GrayModel.Convert(img.At(x, y)).(color.Gray)
			out.pix[(y-bounds.Min.Y)*out.stride+(x-bounds.Min.X)] = gray.Y
		}
	}
	return out
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
