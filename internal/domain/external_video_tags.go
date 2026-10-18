package domain

import "fmt"

const ExternalVideoTagsMaxNames = 100

// VideoTagsAction changes only manual video_tags rows, preserving ancestor-folder tags.
type VideoTagsAction string

const (
	// VideoTagsAdd creates missing tags.
	VideoTagsAdd VideoTagsAction = "add"
	// VideoTagsRemove ignores unknown names.
	VideoTagsRemove VideoTagsAction = "remove"
	// VideoTagsReplace creates missing tags; an empty set removes all manual tags.
	VideoTagsReplace VideoTagsAction = "replace"
)

func (a VideoTagsAction) Valid() bool {
	switch a {
	case VideoTagsAdd, VideoTagsRemove, VideoTagsReplace:
		return true
	}
	return false
}

type VideoTagsOutcome struct {
	// Items preserves requested video order.
	Items []VideoTagsResult
	// SkippedNames contains normalized rejected tentative names, deduplicated in input order.
	SkippedNames []string
}

type VideoTagsResult struct {
	VideoID    int64
	ContentKey string
	// Tags includes manual and folder-derived assignments with provenance.
	Tags []VideoTag
}

// VideoRefNotFoundError carries the zero-based input index and matches ErrNotFound.
type VideoRefNotFoundError struct {
	Index int
}

func (e *VideoRefNotFoundError) Error() string {
	return fmt.Sprintf("video %d is not in the library", e.Index)
}

func (e *VideoRefNotFoundError) Unwrap() error { return ErrNotFound }

// TagNameAtError carries the zero-based input index and wraps InvalidTagNameError.
type TagNameAtError struct {
	Index int
	Err   error
}

func (e *TagNameAtError) Error() string {
	return fmt.Sprintf("tag name %d: %v", e.Index, e.Err)
}

func (e *TagNameAtError) Unwrap() error { return e.Err }
