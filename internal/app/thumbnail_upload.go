package app

import (
	"bytes"
	"context"
	"errors"
	"image"

	"github.com/syudead/vv/internal/domain"
)

// Validate uploaded images in memory, then apply them through the same Ingest path as generated images.

// Serialize all upload decoding and both fitting passes: a 120-megapixel PNG expands to 480 MB.
type uploadGate chan struct{}

func newUploadGate() uploadGate {
	return make(uploadGate, 1)
}

// acquire returns a release function, or ctx.Err if cancelled while waiting.
func (g uploadGate) acquire(ctx context.Context) (release func(), err error) {
	select {
	case g <- struct{}{}:
		return func() { <-g }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// UploadThumbnailImage validates under the upload gate, then releases it before waiting for the generation lock; fitting reacquires the gate only while decoding. Size/pixel limits return ThumbnailUploadTooLargeError, invalid PNG/JPEG/static WebP returns ErrThumbnailUploadInvalid without writes, and application uses uploaded origin with ApplyThumbnailImage errors.
func (i *Ingest) UploadThumbnailImage(
	ctx context.Context, videoID int64, contentKey string, image []byte,
) (domain.Video, error) {
	return i.uploadThumbnailImage(ctx, videoID, contentKey, image, false)
}

// FillThumbnailImage uploads like UploadThumbnailImage only while the video has no applied image, checked under the
// generation lock that every image write takes; otherwise it returns an error and writes nothing.
func (i *Ingest) FillThumbnailImage(
	ctx context.Context, videoID int64, contentKey string, image []byte,
) (domain.Video, error) {
	return i.uploadThumbnailImage(ctx, videoID, contentKey, image, true)
}

func (i *Ingest) uploadThumbnailImage(
	ctx context.Context, videoID int64, contentKey string, image []byte, onlyEmpty bool,
) (domain.Video, error) {
	release, err := i.uploads.acquire(ctx)
	if err != nil {
		return domain.Video{}, err
	}
	err = i.checkUpload(image)
	release()
	if err != nil {
		return domain.Video{}, err
	}
	return i.applyThumbnailImage(ctx, videoID, contentKey, image, domain.ThumbnailImageUploaded, onlyEmpty)
}

func checkThumbnailUpload(data []byte) error {
	_, err := decodeUpload(data)
	return err
}

// decodeUpload applies the upload limits, decode gate checks and animated-WebP refusal that thumbnail and person
// image uploads share; the caller holds the upload gate.
func decodeUpload(data []byte) (image.Image, error) {
	if err := checkThumbnailUploadSize(len(data)); err != nil {
		return nil, err
	}
	if animatedWebP(data) {
		return nil, domain.ErrThumbnailUploadInvalid
	}
	decoded, err := decodeImage(data, domain.ThumbnailUploadMaxPixels)
	switch {
	case errors.Is(err, errImageTooManyPixels):
		return nil, &domain.ThumbnailUploadTooLargeError{Limit: domain.ThumbnailUploadMaxPixels}
	case err != nil:
		return nil, domain.ErrThumbnailUploadInvalid
	}
	return decoded, nil
}

// checkThumbnailUploadSize checks the byte count before image decoding.
func checkThumbnailUploadSize(size int) error {
	if size > domain.ThumbnailUploadMaxBytes {
		return &domain.ThumbnailUploadTooLargeError{Limit: domain.ThumbnailUploadMaxBytes}
	}
	return nil
}

// Reject WebP animation from the VP8X flag independently of decoder support.
func animatedWebP(image []byte) bool {
	const animationBit = 1 << 1
	if len(image) < 21 || !bytes.Equal(image[0:4], []byte("RIFF")) || !bytes.Equal(image[8:12], []byte("WEBP")) {
		return false
	}
	return bytes.Equal(image[12:16], []byte("VP8X")) && image[20]&animationBit != 0
}
