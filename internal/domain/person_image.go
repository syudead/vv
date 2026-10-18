package domain

import "errors"

// PersonImageMaxSide bounds the long side of a stored face; uploads share the thumbnail upload's limits.
const PersonImageMaxSide = 800

// PersonImage is a stored face: a JPEG that fits PersonImageMaxSide.
type PersonImage struct {
	JPEG          []byte
	Width, Height int
	// Revision is the person's image revision, which changes with every change of the face.
	Revision int64
}

var (
	// ErrPersonImageNotFound means the person exists but has no face.
	ErrPersonImageNotFound = errors.New("the person has no image")
	// ErrPersonImageInvalid covers a body that is not a readable PNG, JPEG or still WebP, or is over the pixel limit.
	ErrPersonImageInvalid = errors.New("the image is not a readable PNG, JPEG or still WebP within the pixel limit")
)
