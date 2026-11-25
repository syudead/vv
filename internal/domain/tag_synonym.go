package domain

import "errors"

// Synonym operations make a synonym the canonical name of its tag, move it to another tag or detach it into a new
// tag; see docs/design-docs/tag-organizing.md for what each changes.

// ErrTagSynonymNotFound is a synonym operation naming a name that is not a synonym of its tag now, the tag's
// canonical name included.
var ErrTagSynonymNotFound = errors.New("the name is not a synonym of the tag")

// ErrMoveSameTag is a synonym move whose target is its source.
var ErrMoveSameTag = errors.New("a synonym cannot move to its own tag")

// TagNameHolder is the tag that holds a name now and whether the name is that tag's canonical name.
type TagNameHolder struct {
	TagID     int64
	Canonical bool
}

// CheckSynonymOf reports ErrTagSynonymNotFound unless holder, the name's holder or nil when no tag holds it, holds
// the name as a synonym of tagID; only a synonym is promoted, moved or detached.
func CheckSynonymOf(tagID int64, holder *TagNameHolder) error {
	if holder == nil || holder.TagID != tagID || holder.Canonical {
		return ErrTagSynonymNotFound
	}
	return nil
}

// CheckSynonymMove reports ErrMoveSameTag when a synonym of sourceID would move to sourceID itself.
func CheckSynonymMove(sourceID, targetID int64) error {
	if sourceID == targetID {
		return ErrMoveSameTag
	}
	return nil
}
