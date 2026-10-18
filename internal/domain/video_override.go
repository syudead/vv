package domain

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Overrides follow the content/user-data key and are applied to Video fields on every storage read.

// DisplayNameMaxLength counts code points and accommodates at least the existing 255-byte filename presentation.
const DisplayNameMaxLength = 200

// NormalizeDisplayName wraps ErrInvalidDisplayName in InvalidDisplayNameError for API-specific reasons.
var ErrInvalidDisplayName = errors.New("invalid display name")

type DisplayNameProblem int

const (
	DisplayNameControlCharacters DisplayNameProblem = iota + 1
	DisplayNameTooLong
)

type InvalidDisplayNameError struct {
	Problem DisplayNameProblem
}

func (e *InvalidDisplayNameError) Error() string {
	switch e.Problem {
	case DisplayNameControlCharacters:
		return "display name contains control characters"
	case DisplayNameTooLong:
		return fmt.Sprintf("display name must be at most %d characters", DisplayNameMaxLength)
	default:
		return ErrInvalidDisplayName.Error()
	}
}

func (e *InvalidDisplayNameError) Unwrap() error { return ErrInvalidDisplayName }

// NormalizeDisplayName rejects controls before trimming Unicode whitespace, then treats empty as clear and rejects overlong code-point counts.
func NormalizeDisplayName(input string) (name string, clear bool, err error) {
	for _, r := range input {
		if unicode.Is(unicode.Cc, r) {
			return "", false, &InvalidDisplayNameError{Problem: DisplayNameControlCharacters}
		}
	}
	trimmed := strings.TrimFunc(input, func(r rune) bool {
		return unicode.Is(unicode.White_Space, r)
	})
	if trimmed == "" {
		return "", true, nil
	}
	if utf8.RuneCountInString(trimmed) > DisplayNameMaxLength {
		return "", false, &InvalidDisplayNameError{Problem: DisplayNameTooLong}
	}
	return trimmed, false, nil
}

type DisplayNameChange struct {
	Video       VideoRef
	DisplayName string
}

// DisplayNameAtError carries a zero-based input index and wraps InvalidDisplayNameError.
type DisplayNameAtError struct {
	Index int
	Err   error
}

func (e *DisplayNameAtError) Error() string {
	return fmt.Sprintf("display name %d: %v", e.Index, e.Err)
}

func (e *DisplayNameAtError) Unwrap() error { return e.Err }

// VideoOverrideChanged notifies the UI without waking workers.
type VideoOverrideChanged struct {
	VideoID int64
}

func (VideoOverrideChanged) event() {}

// Unknown duration is a video-state conflict (409), not invalid input.
var ErrDurationUnknown = errors.New("the video duration is not known yet")

var ErrThumbnailPositionOutOfRange = errors.New("thumbnail position is outside the video")

// Thumbnail generation failures preserve the previous image and position.
var ErrThumbnailFrameUnavailable = errors.New("cannot extract a frame for the thumbnail")

// CheckThumbnailPosition requires completed probing and positive duration, with 0 <= positionMs < duration.
func CheckThumbnailPosition(video Video, positionMs int64) error {
	if video.ProbeState != ProbeStateDone || video.DurationMs == nil || *video.DurationMs <= 0 {
		return ErrDurationUnknown
	}
	if positionMs < 0 || positionMs >= *video.DurationMs {
		return ErrThumbnailPositionOutOfRange
	}
	return nil
}
