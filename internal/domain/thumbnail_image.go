package domain

import "errors"

// Applied thumbnail images are persisted by content ID; representative thumbnail files can be rebuilt from them.

// ErrContentChanged aborts without writes when the video vanished or changed content.
var ErrContentChanged = errors.New("the video no longer holds the content")

// ErrThumbnailImageInvalid aborts without writes.
var ErrThumbnailImageInvalid = errors.New("the thumbnail image cannot be decoded")

// VirtualThumbnailAspect is the card frame, so a virtual video's card keeps the size of a file video's card.
const VirtualThumbnailAspect = 16.0 / 9.0

// ThumbnailImageAspect is 16:9 for a virtual video; otherwise it prefers display geometry with rotation/pixel aspect,
// then width/height, and unknown geometry returns zero to preserve the image's own aspect.
func ThumbnailImageAspect(video Video) float64 {
	if video.Virtual() {
		return VirtualThumbnailAspect
	}
	if video.DisplayAspectRatio != nil && *video.DisplayAspectRatio > 0 {
		return *video.DisplayAspectRatio
	}
	if video.Width != nil && video.Height != nil && *video.Width > 0 && *video.Height > 0 {
		return float64(*video.Width) / float64(*video.Height)
	}
	return 0
}

type ThumbnailImageOrigin string

const (
	ThumbnailImageGenerated ThumbnailImageOrigin = "generated"
	ThumbnailImageUploaded  ThumbnailImageOrigin = "uploaded"
)

func (o ThumbnailImageOrigin) Valid() bool {
	return o == ThumbnailImageGenerated || o == ThumbnailImageUploaded
}

type AppliedThumbnailImage struct {
	// Image is the same 640px-long-edge JPEG published as the representative thumbnail.
	Image []byte
	// Source preserves the same crop at up to 2048px for subsequent editing.
	Source []byte
	Origin ThumbnailImageOrigin
}

// CurrentThumbnailSelection reads the image and position from one snapshot to avoid mixing concurrent uploads, position changes, or resets.
type CurrentThumbnailSelection struct {
	// Image falls back to the representative thumbnail for old rows without a source and is nil without an applied image.
	Image      []byte
	PositionMs *int64
}
