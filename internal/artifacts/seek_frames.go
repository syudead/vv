package artifacts

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"

	"golang.org/x/image/draw"

	"github.com/syudead/vv/internal/domain"
)

const seekFrameQuality = 85

// SeekFrames returns n evenly spaced sprite frames as chronological JPEGs; absent stores or layouts
// wrap fs.ErrNotExist. Used by automatic tagging; see docs/design-docs/auto-tagging.md.
func (s *Store) SeekFrames(contentKey string, n int) ([][]byte, error) {
	reader, err := s.seekFrameReader(contentKey)
	if err != nil {
		return nil, err
	}
	var frames [][]byte
	for _, index := range domain.SpreadFrameIndexes(reader.sprite.FrameCount, n) {
		frame, err := reader.frame(index)
		if err != nil {
			return nil, err
		}
		data, err := encodeSeekFrame(frame, index)
		if err != nil {
			return nil, err
		}
		frames = append(frames, data)
	}
	return frames, nil
}

// SeekFramePairs places each A frame left of B, resizing B to A's dimensions, without reading media
// or invoking ffmpeg; missing stores or layouts wrap fs.ErrNotExist.
func (s *Store) SeekFramePairs(keyA, keyB string, pairs []domain.FramePair) ([][]byte, error) {
	a, err := s.seekFrameReader(keyA)
	if err != nil {
		return nil, err
	}
	b, err := s.seekFrameReader(keyB)
	if err != nil {
		return nil, err
	}
	images := make([][]byte, 0, len(pairs))
	for _, pair := range pairs {
		left, err := a.frame(pair.A)
		if err != nil {
			return nil, err
		}
		right, err := b.frame(pair.B)
		if err != nil {
			return nil, err
		}
		width, height := left.Bounds().Dx(), left.Bounds().Dy()
		canvas := image.NewRGBA(image.Rect(0, 0, 2*width, height))
		draw.Draw(canvas, image.Rect(0, 0, width, height), left, left.Bounds().Min, draw.Src)
		draw.ApproxBiLinear.Scale(canvas, image.Rect(width, 0, 2*width, height), right, right.Bounds(), draw.Src, nil)
		data, err := encodeSeekFrame(canvas, pair.A)
		if err != nil {
			return nil, err
		}
		images = append(images, data)
	}
	return images, nil
}

// Cache decoded sheets within each content read.
type seekFrames struct {
	store      *Store
	contentKey string
	sprite     domain.SeekSprite
	sheets     map[int]image.Image
}

func (s *Store) seekFrameReader(contentKey string) (*seekFrames, error) {
	sprite, err := s.SeekSprite(contentKey)
	if err != nil {
		return nil, err
	}
	return &seekFrames{store: s, contentKey: contentKey, sprite: sprite, sheets: map[int]image.Image{}}, nil
}

// Frame indexes start at zero from the beginning of the video.
func (r *seekFrames) frame(index int) (image.Image, error) {
	if index < 0 || index >= r.sprite.FrameCount {
		return nil, fmt.Errorf("seek preview frame %d is outside the %d frames", index, r.sprite.FrameCount)
	}
	perSheet := r.sprite.Columns * r.sprite.Rows
	sheet := index / perSheet
	img, ok := r.sheets[sheet]
	if !ok {
		data, err := r.store.SeekSpriteSheet(r.contentKey, sheet)
		if err != nil {
			return nil, err
		}
		if img, err = jpeg.Decode(bytes.NewReader(data)); err != nil {
			return nil, fmt.Errorf("cannot decode seek preview sheet %d: %w", sheet, err)
		}
		r.sheets[sheet] = img
	}
	return cropSeekFrame(img, r.sprite, index%perSheet)
}

// Positions are row-major from the top left.
func cropSeekFrame(sheet image.Image, sprite domain.SeekSprite, position int) (image.Image, error) {
	x := (position % sprite.Columns) * sprite.FrameWidth
	y := (position / sprite.Columns) * sprite.FrameHeight
	rect := image.Rect(x, y, x+sprite.FrameWidth, y+sprite.FrameHeight).Add(sheet.Bounds().Min)
	if !rect.In(sheet.Bounds()) {
		return nil, fmt.Errorf("seek preview frame %d is outside the sheet", position)
	}
	cropped := image.NewRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	draw.Draw(cropped, cropped.Bounds(), sheet, rect.Min, draw.Src)
	return cropped, nil
}

func encodeSeekFrame(img image.Image, index int) ([]byte, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: seekFrameQuality}); err != nil {
		return nil, fmt.Errorf("cannot encode seek preview frame %d: %w", index, err)
	}
	return buf.Bytes(), nil
}
