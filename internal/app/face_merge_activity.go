package app

import (
	"context"

	"github.com/syudead/vv/internal/domain"
)

type FaceMergeUndoStore interface {
	// UndoFaceMerge re-creates the merged provisional person and records the owner's undo entry.
	UndoFaceMerge(ctx context.Context, id int64) (domain.ActivityEntry, error)
}

// FaceMergeActivity undoes face_merge entries from the activity log.
type FaceMergeActivity struct {
	undo FaceMergeUndoStore
}

func NewFaceMergeActivity(undo FaceMergeUndoStore) *FaceMergeActivity {
	return &FaceMergeActivity{undo: undo}
}

// UndoActivity ignores tagID, which only auto_tag uses.
func (a *FaceMergeActivity) UndoActivity(ctx context.Context, entry domain.ActivityEntry, _ int64) (domain.ActivityEntry, error) {
	return a.undo.UndoFaceMerge(ctx, entry.ID)
}
