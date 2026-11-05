package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/syudead/vv/internal/domain"
)

// merges merges each still-provisional source into its person as the owner decided, after the other parts of the
// edit, and returns their events. A missing into fails the edit; a source missing or named since is skipped.
func (w *videoEditWrite) merges(ctx context.Context, merges []domain.PersonMerge, result *domain.VideoEditResult) (
	[]domain.Event, error,
) {
	t := creditTablesByKind[domain.CreditPerson]
	for i, merge := range merges {
		if _, err := creditByID(ctx, w.tx, t, merge.Into); errors.Is(err, domain.ErrNotFound) {
			return nil, &domain.CreditNotFoundError{Kind: domain.CreditPerson, ID: merge.Into, Field: fmt.Sprintf("merges[%d].into", i)}
		} else if err != nil {
			return nil, err
		}
	}
	targets := make(map[string]bool, len(w.keys))
	for _, key := range w.keys {
		targets[key] = true
	}
	var events []domain.Event
	for _, merge := range merges {
		for _, sourceID := range merge.SourceIDs {
			source, err := creditByID(ctx, w.tx, t, sourceID)
			if errors.Is(err, domain.ErrNotFound) || err == nil && !source.Provisional {
				result.SkippedPersonIDs = append(result.SkippedPersonIDs, sourceID)
				continue
			}
			if err != nil {
				return nil, err
			}
			credits, err := readFaceMergeCredits(ctx, w.tx, sourceID, merge.Into)
			if err != nil {
				return nil, err
			}
			_, merged, undoID, err := mergeByOwner(ctx, w.tx, w.c, source, merge.Into, w.now)
			if err != nil {
				return nil, err
			}
			events = append(events, merged...)
			result.Merged = append(result.Merged, domain.PersonMerged{PersonID: sourceID, Into: merge.Into, UndoActivityID: undoID})
			for _, credit := range credits {
				if targets[credit.userKey] {
					w.changed[credit.userKey] = true
				}
			}
		}
	}
	return events, nil
}
