package domain

import "errors"

// ErrCodeFillQueueFull reports a code fill request larger than the room left in the fill queue; nothing is queued.
var ErrCodeFillQueueFull = errors.New("too many work-info fills are queued")

// CodedVideo is the video a code fill runs for on behalf of its user-data key, with the work code it fetches by.
type CodedVideo struct {
	VideoID    int64
	ContentKey string
	Code       string
}

// CodeFillsQueued counts a code fill request's user-data keys: those queued and those left out for lacking a code.
type CodeFillsQueued struct {
	Queued      int
	WithoutCode int
}
