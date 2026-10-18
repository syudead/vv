package store

import (
	"context"
	"fmt"

	"github.com/syudead/vv/internal/domain"
)

// AddSynonym confirms the target; canonical-name conflicts require an explicit matching merge ID, while other conflicts are rejected.
func (s *TagStore) AddSynonym(ctx context.Context, tagID int64, name string, mergeTagID *int64) (domain.Tag, error) {
	normalized, err := domain.NormalizeTagName(name)
	if err != nil {
		return domain.Tag{}, err
	}

	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.Tag{}, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := canonicalNameByTagID(ctx, tx, tagID); err != nil {
		return domain.Tag{}, err
	}

	lookup, found, err := lookupTagName(ctx, tx, normalized)
	if err != nil {
		return domain.Tag{}, err
	}

	switch {
	case !found:
		if err := insertTagName(ctx, tx, normalized, tagID, false); err != nil {
			return domain.Tag{}, err
		}
		if err := confirmTagInTx(ctx, tx, tagID); err != nil {
			return domain.Tag{}, err
		}
	case lookup.tagID == tagID && lookup.isCanonical:
		return domain.Tag{}, &domain.TagNameConflict{Tag: lookup.ref()}
	case lookup.tagID == tagID:
	case !lookup.isCanonical:
		return domain.Tag{}, &domain.TagNameConflict{Tag: lookup.ref()}
	case mergeTagID == nil || *mergeTagID != lookup.tagID:
		return domain.Tag{}, &domain.TagMergeRequired{Tag: lookup.ref()}
	default:
		if err := mergeTagsInto(ctx, tx, tagID, []int64{lookup.tagID}); err != nil {
			return domain.Tag{}, err
		}
	}

	tag, err := tagByID(ctx, tx, tagID)
	if err != nil {
		return domain.Tag{}, err
	}
	if err := s.db.commitMatchChange(ctx, tx); err != nil {
		return domain.Tag{}, fmt.Errorf("cannot add the synonym: %w", err)
	}
	return tag, nil
}

// RemoveSynonym returns the changed tag from the same transaction; absent synonyms are no-ops and missing tags fail.
func (s *TagStore) RemoveSynonym(ctx context.Context, tagID int64, name string) (domain.Tag, error) {
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.Tag{}, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := canonicalNameByTagID(ctx, tx, tagID); err != nil {
		return domain.Tag{}, err
	}
	if _, err := tx.ExecContext(ctx,
		`delete from tag_names where name = ? and tag_id = ? and canonical = 0`, name, tagID,
	); err != nil {
		return domain.Tag{}, fmt.Errorf("cannot remove the synonym (tag=%d): %w", tagID, err)
	}

	tag, err := tagByID(ctx, tx, tagID)
	if err != nil {
		return domain.Tag{}, err
	}
	if err := s.db.commitMatchChange(ctx, tx); err != nil {
		return domain.Tag{}, fmt.Errorf("cannot remove the synonym: %w", err)
	}
	return tag, nil
}
