package domain

import (
	"errors"
	"time"
)

// Thumbnail edit sessions live in internal/app memory and expose these values through the UI API.

const (
	ThumbnailEditIdleLimit         = 5 * time.Minute
	ThumbnailEditRunLimit          = 10 * time.Minute
	ThumbnailEditMaxInstruction    = 4000
	ThumbnailEditMaxReferenceBytes = 20 << 20
	ThumbnailFrameMaxSide          = 2048
)

var (
	ErrThumbnailEditNotFound = errors.New("the thumbnail edit has ended")
	ErrThumbnailEditNotReady = errors.New("no ComfyUI connection is saved")
	// Invalid masks cause no upload and must match frame-sized PNG dimensions.
	ErrThumbnailEditMaskInvalid = errors.New("the mask is not a PNG of the frame's size")
	// Invalid or oversized PNG/JPEG/WebP references cause no upload.
	ErrThumbnailEditReferenceInvalid = errors.New("the reference image cannot be read")
	ErrThumbnailEditNoResult         = errors.New("the thumbnail edit has no result")
)

type ThumbnailFrame struct {
	PNG    []byte
	Width  int
	Height int
}

type ThumbnailEditSource string

const (
	ThumbnailEditFromFrame     ThumbnailEditSource = "frame"
	ThumbnailEditFromThumbnail ThumbnailEditSource = "thumbnail"
)

func (s ThumbnailEditSource) Valid() bool {
	return s == ThumbnailEditFromFrame || s == ThumbnailEditFromThumbnail
}

// PositionMs applies only to ThumbnailEditFromFrame.
type ThumbnailEditStart struct {
	Source     ThumbnailEditSource
	PositionMs *int64
}

type ThumbnailEditRunInput struct {
	Instruction string
	Mask        []byte
	Reference   []byte
}

// Elapsed is nil while ready and freezes on completion; Progress is provider-optional, Error is failure-only, and PositionMs is nil for an applied source image.
type ThumbnailEdit struct {
	ID          string
	VideoID     int64
	State       ThumbnailEditState
	Source      ThumbnailEditSource
	PositionMs  *int64
	FrameWidth  int
	FrameHeight int
	Elapsed     *time.Duration
	Progress    *ImageEditProgress
	Error       *ImageEditError
	HasResult   bool
}
