package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif" // Register supported image decoders.
	_ "image/jpeg"
	"image/png" // ComfyUI outputs PNG.
	"math"
	"os/exec"
	"strconv"

	_ "golang.org/x/image/webp" // Accept provider WebP output like reference images.

	"github.com/syudead/vv/internal/domain"
)

// Match thumbnailScale's extracted-thumbnail size.
const thumbnailLongSide = 640

const thumbnailSourceLongSide = 2048

// FitThumbnailImage center-crops to the positive display aspect at 640px, avoiding nested
// letterboxing; zero keeps the image aspect, and quality matches thumbnail extraction. Invalid
// images return domain.ErrThumbnailImageInvalid; WebP is decoded to PNG locally so ffmpeg need not
// support it.
func FitThumbnailImage(ctx context.Context, data []byte, aspect float64) ([]byte, error) {
	_, data, err := decodeThumbnailImage(data)
	if err != nil {
		return nil, err
	}
	filter := thumbnailScale
	if aspect > 0 {
		filter = fitThumbnailFilter(fitThumbnailSize(aspect))
	}
	return runFitThumbnail(ctx, data, filter, "4")
}

// FitThumbnailSource center-crops to the display aspect and limits the longest side to 2048px
// without upscaling, using JPEG q=2; invalid input returns domain.ErrThumbnailImageInvalid.
func FitThumbnailSource(ctx context.Context, data []byte, aspect float64) ([]byte, error) {
	img, data, err := decodeThumbnailImage(data)
	if err != nil {
		return nil, err
	}
	bounds := img.Bounds()
	width, height := fitThumbnailSourceSize(bounds.Dx(), bounds.Dy(), aspect)
	return runFitThumbnail(ctx, data, fitThumbnailFilter(width, height), "2")
}

// Convert WebP to PNG for ffmpeg; unreadable input returns domain.ErrThumbnailImageInvalid.
func decodeThumbnailImage(data []byte) (image.Image, []byte, error) {
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, nil, errors.Join(domain.ErrThumbnailImageInvalid, err)
	}
	if format == "webp" {
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			return nil, nil, fmt.Errorf("cannot convert the WebP thumbnail image to PNG: %w", err)
		}
		data = buf.Bytes()
	}
	return img, data, nil
}

func runFitThumbnail(ctx context.Context, data []byte, filter, quality string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, thumbnailTimeout)
	defer cancel()

	cmd := command(ctx, thumbnailCommand, fitThumbnailArgs(filter, quality)...)
	cmd.Stdin = bytes.NewReader(data)
	output, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf("%s failed to fit the thumbnail image: %s", thumbnailCommand, firstLine(exitErr.Stderr))
		}
		return nil, fmt.Errorf("cannot run %s: %w", thumbnailCommand, err)
	}
	if len(output) == 0 {
		return nil, errors.New("the fitted thumbnail image is empty")
	}
	return output, nil
}

func fitThumbnailArgs(filter, quality string) []string {
	return []string{
		"-v", "error",
		"-i", "pipe:0",
		"-frames:v", "1",
		"-vf", filter,
		"-q:v", quality,
		"-c:v", "mjpeg",
		"-f", "image2pipe",
		"pipe:1",
	}
}

func fitThumbnailFilter(width, height int) string {
	w, h := strconv.Itoa(width), strconv.Itoa(height)
	return "scale=" + w + ":" + h + ":force_original_aspect_ratio=increase,crop=" + w + ":" + h + ",setsar=1"
}

// Round the shorter side to even pixels, matching thumbnailScale.
func fitThumbnailSize(aspect float64) (width, height int) {
	short := func(value float64) int {
		even := int(math.Round(value/2)) * 2
		return max(even, 2)
	}
	if aspect >= 1 {
		return thumbnailLongSide, short(thumbnailLongSide / aspect)
	}
	return short(thumbnailLongSide * aspect), thumbnailLongSide
}

// Crop without upscaling or even-pixel rounding, which distorts small images such as 16x9; JPEG
// supports odd dimensions. Clamp subpixel sides to one pixel, accepting aspect mismatch rather than
// enlarging source pixels.
func fitThumbnailSourceSize(width, height int, aspect float64) (int, int) {
	w, h := float64(width), float64(height)
	if aspect > 0 && w/h > aspect {
		w = h * aspect
	} else if aspect > 0 {
		h = w / aspect
	}
	if long := math.Max(w, h); long > thumbnailSourceLongSide {
		w, h = w*thumbnailSourceLongSide/long, h*thumbnailSourceLongSide/long
	}
	return sideAtMost(w), sideAtMost(h)
}

// Floor to at least one pixel, tolerating floating-point error just below an integer.
func sideAtMost(value float64) int {
	return max(int(math.Floor(value+1e-9)), 1)
}

func (a *Assets) FitThumbnailImage(ctx context.Context, data []byte, aspect float64) ([]byte, error) {
	return FitThumbnailImage(ctx, data, aspect)
}

func (a *Assets) FitThumbnailSource(ctx context.Context, data []byte, aspect float64) ([]byte, error) {
	return FitThumbnailSource(ctx, data, aspect)
}
