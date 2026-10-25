package domain

import (
	"errors"
	"fmt"
)

// VideoCodeEditsMaxItems caps one code table write; the table editor holds at most this many videos.
const VideoCodeEditsMaxItems = 1000

var (
	ErrVideoCodeEditCount = fmt.Errorf("code edits must have between 1 and %d items", VideoCodeEditsMaxItems)
	// ErrDuplicateVideoCodeEdit marks an item whose video or user key an earlier item already writes.
	ErrDuplicateVideoCodeEdit = errors.New("an earlier item already writes this video")
)

// VideoCodeEdit replaces the code of the user key that VideoID belongs to; ContentVersion is the
// version the editor read, so a reused video ID cannot receive another video's code.
type VideoCodeEdit struct {
	VideoID        int64
	ContentVersion string
	Code           string
}

// VideoCodeRow is one user key of a code edit selection: its shown video and current code, empty when unset.
type VideoCodeRow struct {
	Video Video
	Code  string
}

// VideoCodeEditAtError carries the zero-based index of the failing item; Err is ErrDuplicateVideoCodeEdit,
// ErrNotFound, ErrContentChanged or an InvalidWorkInfoError for the code.
type VideoCodeEditAtError struct {
	Index int
	Err   error
}

func (e *VideoCodeEditAtError) Error() string {
	return fmt.Sprintf("code edit %d: %v", e.Index, e.Err)
}

func (e *VideoCodeEditAtError) Unwrap() error { return e.Err }

// NormalizeVideoCodeEdits checks the item count, distinct video IDs and each code in input order, returning
// the items with trimmed codes. An empty code is rejected, since this write never clears a code.
func NormalizeVideoCodeEdits(items []VideoCodeEdit) ([]VideoCodeEdit, error) {
	if len(items) < 1 || len(items) > VideoCodeEditsMaxItems {
		return nil, ErrVideoCodeEditCount
	}
	out := make([]VideoCodeEdit, len(items))
	seen := make(map[int64]bool, len(items))
	for i, item := range items {
		if seen[item.VideoID] {
			return nil, &VideoCodeEditAtError{Index: i, Err: ErrDuplicateVideoCodeEdit}
		}
		seen[item.VideoID] = true
		code, err := NormalizeWorkCode(item.Code)
		if err == nil && code == "" {
			err = invalidWorkInfo("code", WorkInfoEmpty, 0)
		}
		if err != nil {
			return nil, &VideoCodeEditAtError{Index: i, Err: err}
		}
		item.Code = code
		out[i] = item
	}
	return out, nil
}
