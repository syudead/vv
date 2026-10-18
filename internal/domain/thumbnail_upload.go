package domain

import (
	"errors"
	"fmt"
)

const (
	ThumbnailUploadMaxBytes = 64 << 20
	// Reject oversized pixel counts from headers before decoding the full image.
	ThumbnailUploadMaxPixels = 120_000_000
)

var (
	// ThumbnailUploadTooLargeError identifies which limit was exceeded; no writes occur.
	ErrThumbnailUploadTooLarge = errors.New("the uploaded image is too large")
	// Uploads accept fully decodable PNG, JPEG, or static WebP only; invalid input performs no writes.
	ErrThumbnailUploadInvalid = errors.New("the uploaded image is not a readable PNG, JPEG or still WebP")
)

type ThumbnailUploadTooLargeError struct {
	Limit int
}

func (e *ThumbnailUploadTooLargeError) Error() string {
	return fmt.Sprintf("%s: the limit is %d", ErrThumbnailUploadTooLarge.Error(), e.Limit)
}

func (e *ThumbnailUploadTooLargeError) Unwrap() error {
	return ErrThumbnailUploadTooLarge
}
