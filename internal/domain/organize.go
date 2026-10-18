package domain

import (
	"errors"
	"strings"
	"unicode"
)

// Organize operations return before/after images in one transaction; undo restores the before image only while current rows still match the after image.

// ErrUndoConflict aborts undo without writes and maps to 409 undo_conflict.
var ErrUndoConflict = errors.New("undo conflict")

// ErrUndoExpired covers absent, used, and older-than-ten-minute tokens; it maps to 410 undo_expired.
var ErrUndoExpired = errors.New("undo expired")

type OrganizeResult struct {
	UndoToken string
	GroupID   int64
}

// Trim leading whitespace before truncating a generated group name so long whitespace prefixes cannot yield an empty name.
func GroupNameFromTitle(title string) (string, error) {
	trimmed := strings.TrimLeftFunc(title, func(r rune) bool {
		return unicode.Is(unicode.White_Space, r)
	})
	return ValidateGroupName(cutGroupName(trimmed))
}

type OrganizeRefKind string

const (
	OrganizeVideo OrganizeRefKind = "video"
	OrganizeGroup OrganizeRefKind = "group"
)

// OrganizeRef inputs follow screen order.
type OrganizeRef struct {
	Kind OrganizeRefKind
	ID   int64
}

type GroupMergeHolder struct {
	ID          int64
	HoldsTarget bool
}

// Replace preserves item positions; Remove deletes the source references.
type GroupMergePlan struct {
	ReplacePlaylists []int64
	RemovePlaylists  []int64
	ReplaceParents   []int64
	RemoveParents    []int64
}

// PlanGroupMerge replaces source references in place unless the target already exists there or would introduce a parent cycle, in which case it removes the source reference.
func PlanGroupMerge(target int64, playlists, parents []GroupMergeHolder, edges []GroupEdge) GroupMergePlan {
	var plan GroupMergePlan
	for _, playlist := range playlists {
		if playlist.HoldsTarget {
			plan.RemovePlaylists = append(plan.RemovePlaylists, playlist.ID)
		} else {
			plan.ReplacePlaylists = append(plan.ReplacePlaylists, playlist.ID)
		}
	}
	for _, parent := range parents {
		if parent.HoldsTarget || ValidateNesting(parent.ID, target, edges) != nil {
			plan.RemoveParents = append(plan.RemoveParents, parent.ID)
		} else {
			plan.ReplaceParents = append(plan.ReplaceParents, parent.ID)
		}
	}
	return plan
}

// Zero From/To means absent; a missing To removes cards from From without adding them elsewhere.
type OrganizeMove struct {
	Items []OrganizeRef
	From  int64
	To    int64
}

// MaxOrganizeBundleVideos counts all members of existing bundles.
const MaxOrganizeBundleVideos = 20

// ErrTooManyVersions aborts without writes.
var ErrTooManyVersions = errors.New("too many videos to bundle")
