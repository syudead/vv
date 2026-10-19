package app

import (
	"bytes"
	"errors"
	"image"
	_ "image/jpeg" // Decode reference images and provider output.
	"image/png"

	_ "golang.org/x/image/webp" // The standard library has no WebP decoder.

	"github.com/syudead/vv/internal/domain"
)

// Fully decode inputs before uploading and provider output before marking success.

// Bound decoded pixels before allocation to reject oversized headers without exhausting memory.
const thumbnailEditMaxPixels = 8192 * 8192

var errThumbnailEditImageUnreadable = errors.New("the image cannot be decoded")

var errImageTooManyPixels = errors.New("the image has too many pixels")

// Accept a nil mask or a fully decodable PNG matching the frame dimensions.
func checkThumbnailEditMask(mask []byte, frame domain.ThumbnailFrame) error {
	if mask == nil {
		return nil
	}
	config, err := png.DecodeConfig(bytes.NewReader(mask))
	if err != nil || config.Width != frame.Width || config.Height != frame.Height {
		return domain.ErrThumbnailEditMaskInvalid
	}
	if _, err := png.Decode(bytes.NewReader(mask)); err != nil {
		return domain.ErrThumbnailEditMaskInvalid
	}
	return nil
}

// Accept a nil reference or a fully decodable PNG, JPEG or WebP within the limits.
func checkThumbnailEditReference(reference []byte) error {
	if reference == nil {
		return nil
	}
	if len(reference) == 0 || len(reference) > domain.ThumbnailEditMaxReferenceBytes {
		return domain.ErrThumbnailEditReferenceInvalid
	}
	if err := checkThumbnailEditImage(reference); err != nil {
		return domain.ErrThumbnailEditReferenceInvalid
	}
	return nil
}

func checkThumbnailEditImage(data []byte) error {
	_, err := decodeThumbnailEditImage(data)
	return err
}

// Reject oversized images as unreadable.
func decodeThumbnailEditImage(data []byte) (image.Image, error) {
	decoded, err := decodeImage(data, thumbnailEditMaxPixels)
	if err != nil {
		return nil, errThumbnailEditImageUnreadable
	}
	return decoded, nil
}

// Shared by edit inputs and uploads: check header dimensions before fully decoding PNG, JPEG or WebP; reject truncated or corrupt data. Return errImageTooManyPixels for excess dimensions, otherwise errThumbnailEditImageUnreadable for decoding failures.
func decodeImage(data []byte, maxPixels int64) (image.Image, error) {
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "png" && format != "jpeg" && format != "webp") {
		return nil, errThumbnailEditImageUnreadable
	}
	if config.Width <= 0 || config.Height <= 0 {
		return nil, errThumbnailEditImageUnreadable
	}
	if int64(config.Width)*int64(config.Height) > maxPixels {
		return nil, errImageTooManyPixels
	}
	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, errThumbnailEditImageUnreadable
	}
	return decoded, nil
}

// Convert a saved JPEG source into a PNG frame with its original dimensions.
func thumbnailEditFrameFromImage(data []byte) (domain.ThumbnailFrame, error) {
	decoded, err := decodeThumbnailEditImage(data)
	if err != nil {
		return domain.ThumbnailFrame{}, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, decoded); err != nil {
		return domain.ThumbnailFrame{}, err
	}
	bounds := decoded.Bounds()
	return domain.ThumbnailFrame{PNG: buf.Bytes(), Width: bounds.Dx(), Height: bounds.Dy()}, nil
}
