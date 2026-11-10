package domain

import (
	"errors"
	"fmt"
)

// An image search takes one uploaded image or video and keeps nothing of it.

const (
	// ImageSearchImageMaxBytes and ImageSearchImageMaxPixels are the thumbnail upload's limits, whose decoder image
	// queries share.
	ImageSearchImageMaxBytes  = ThumbnailUploadMaxBytes
	ImageSearchImageMaxPixels = ThumbnailUploadMaxPixels
	// ImageSearchVideoMaxBytes admits a whole re-encoded video as a query.
	ImageSearchVideoMaxBytes int64 = 4 << 30
	// ImageSearchVideoMaxMs bounds a query to ImageSearchQueryMaxFrames sampled frames.
	ImageSearchVideoMaxMs int64 = 3 * 60 * 60 * 1000
	// ImageSearchVideoMinMs is the shortest stream read as a video rather than refused as unreadable.
	ImageSearchVideoMinMs int64 = 1000
	// ImageSearchQueryIntervalMs samples a query video densely enough that every indexed keyframe has a query frame
	// within half of it.
	ImageSearchQueryIntervalMs int64 = 1000
	ImageSearchQueryMaxFrames        = int(ImageSearchVideoMaxMs / ImageSearchQueryIntervalMs)
)

type ImageSearchQueryKind string

const (
	ImageSearchQueryImage ImageSearchQueryKind = "image"
	ImageSearchQueryVideo ImageSearchQueryKind = "video"
)

var (
	// ErrImageSearchUnreadable: neither a PNG, JPEG or still WebP nor a file with a video stream of at least
	// ImageSearchVideoMinMs, or its decoding failed.
	ErrImageSearchUnreadable = errors.New("the file is not a readable image or video")
	// ErrImageSearchUninformative: a flat image, or a video with fewer than FingerprintMinComparableFrames
	// non-flat sampled frames.
	ErrImageSearchUninformative = errors.New("the file has too little detail to search by")
	ErrImageSearchTooLarge      = errors.New("the file is too large to search by")
	ErrImageSearchTooLong       = errors.New("the video is too long to search by")
)

// ImageSearchTooLargeError names the exceeded limit: bytes of an image or a video, or pixels of an image.
type ImageSearchTooLargeError struct {
	Limit int64
}

func (e *ImageSearchTooLargeError) Error() string {
	return fmt.Sprintf("%s: the limit is %d", ErrImageSearchTooLarge.Error(), e.Limit)
}

func (e *ImageSearchTooLargeError) Unwrap() error { return ErrImageSearchTooLarge }

type ImageSearchTooLongError struct {
	LimitMs int64
}

func (e *ImageSearchTooLongError) Error() string {
	return fmt.Sprintf("%s: the limit is %d ms", ErrImageSearchTooLong.Error(), e.LimitMs)
}

func (e *ImageSearchTooLongError) Unwrap() error { return ErrImageSearchTooLong }

// ImageSearchCandidate is one searchable video with the id the library lists it under: its bundle's representative,
// or itself.
type ImageSearchCandidate struct {
	ListedID int64
	Content  ImageSearchContent
}

// ImageSearchSnapshot is the index one search reads; UnindexedCount counts the listed file videos, bundle members
// included, without a readable index of the current version.
type ImageSearchSnapshot struct {
	Candidates     []ImageSearchCandidate
	UnindexedCount int
}

// ImageSearchItem is a listed video and its best match, whose VideoID may be one of the video's versions.
type ImageSearchItem struct {
	Video Video
	Match ImageSearchMatch
}

type ImageSearchResult struct {
	Kind ImageSearchQueryKind
	// DurationMs is set for a video query.
	DurationMs     int64
	Items          []ImageSearchItem
	UnindexedCount int
}
