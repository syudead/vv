package app

import (
	"context"

	"github.com/syudead/vv/internal/domain"
)

type FaceLinkUndoStore interface {
	// UndoFaceLinks undoes one link of a face_analysis entry, or every link not yet undone when appearanceID is
	// zero, and returns the owner's undo entry.
	UndoFaceLinks(ctx context.Context, entryID, appearanceID int64) (domain.ActivityEntry, error)
}

// FaceLinkUndoer dispatches the undo of face_analysis entries; it wakes no worker, since nothing is queued again.
type FaceLinkUndoer struct {
	store FaceLinkUndoStore
}

func NewFaceLinkUndoer(store FaceLinkUndoStore) *FaceLinkUndoer {
	return &FaceLinkUndoer{store: store}
}

func (u *FaceLinkUndoer) UndoActivity(
	ctx context.Context, entry domain.ActivityEntry, appearanceID int64,
) (domain.ActivityEntry, error) {
	return u.store.UndoFaceLinks(ctx, entry.ID, appearanceID)
}
