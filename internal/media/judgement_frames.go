package media

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"

	"golang.org/x/image/draw"

	"github.com/syudead/vv/internal/domain"
)

// Each half of a judgement image is at most this wide, enough for the classifier and the owner to compare scenes.
const (
	judgementFrameMaxWidth = 640
	judgementFrameQuality  = 85
)

// FramePairImages cuts each frame of the first file at AMs and of the second at BMs and puts them side by side, the
// second scaled to the first's size, as JPEG.
func FramePairImages(ctx context.Context, paths [2]string, frames []domain.VersionEvidenceFrame) ([][]byte, error) {
	images := make([][]byte, 0, len(frames))
	for _, frame := range frames {
		left, err := decodedFrameAt(ctx, paths[0], frame.AMs)
		if err != nil {
			return nil, err
		}
		right, err := decodedFrameAt(ctx, paths[1], frame.BMs)
		if err != nil {
			return nil, err
		}
		data, err := sideBySide(left, right)
		if err != nil {
			return nil, err
		}
		images = append(images, data)
	}
	return images, nil
}

func decodedFrameAt(ctx context.Context, path string, positionMs int64) (image.Image, error) {
	frame, err := FrameAt(ctx, path, positionMs)
	if err != nil {
		return nil, err
	}
	decoded, err := png.Decode(bytes.NewReader(frame.PNG))
	if err != nil {
		return nil, fmt.Errorf("cannot decode the frame (%s, at %d ms): %w", path, positionMs, err)
	}
	return decoded, nil
}

func sideBySide(left, right image.Image) ([]byte, error) {
	width, height := left.Bounds().Dx(), left.Bounds().Dy()
	if width > judgementFrameMaxWidth {
		height = height * judgementFrameMaxWidth / width
		width = judgementFrameMaxWidth
	}
	canvas := image.NewRGBA(image.Rect(0, 0, 2*width, height))
	draw.ApproxBiLinear.Scale(canvas, image.Rect(0, 0, width, height), left, left.Bounds(), draw.Src, nil)
	draw.ApproxBiLinear.Scale(canvas, image.Rect(width, 0, 2*width, height), right, right.Bounds(), draw.Src, nil)
	var out bytes.Buffer
	if err := jpeg.Encode(&out, canvas, &jpeg.Options{Quality: judgementFrameQuality}); err != nil {
		return nil, fmt.Errorf("cannot encode the judgement frame: %w", err)
	}
	return out.Bytes(), nil
}

func (a *Assets) FramePairImages(ctx context.Context, paths [2]string, frames []domain.VersionEvidenceFrame) ([][]byte, error) {
	return FramePairImages(ctx, paths, frames)
}
