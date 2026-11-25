package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/syudead/vv/internal/domain"
)

// Synonym operations rewrite tag_names rows of one synonym in one transaction, re-reading the source, the name and
// the target there; explicit attachments stay with the tag id, and folder-derived ones follow the name.

// Tag reads tag id as ListTags lists it for audience.
func (s *TagStore) Tag(ctx context.Context, audience domain.Audience, id int64) (domain.Tag, error) {
	tx, err := s.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.Tag{}, fmt.Errorf("cannot start reading the tag: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	return listedTagByID(ctx, tx, audience, id)
}

// PromoteSynonym makes the synonym name of tag id its canonical name, keeps the old canonical name as a synonym and
// confirms the tag.
func (s *TagStore) PromoteSynonym(ctx context.Context, id int64, name string) (domain.Tag, error) {
	tag, _, err := s.changeSynonym(ctx, "promote", id, name, func(tx *sql.Tx, normalized string) (int64, error) {
		return id, promoteSynonymInTx(ctx, tx, id, normalized)
	})
	return tag, err
}

// MoveSynonym makes the synonym name of tag sourceID a synonym of tag targetID and confirms the target; the source
// keeps its explicit attachments and tentative state.
func (s *TagStore) MoveSynonym(ctx context.Context, sourceID int64, name string, targetID int64) (source, target domain.Tag, err error) {
	if err := domain.CheckSynonymMove(sourceID, targetID); err != nil {
		return domain.Tag{}, domain.Tag{}, err
	}
	return s.changeSynonym(ctx, "move", sourceID, name, func(tx *sql.Tx, normalized string) (int64, error) {
		if _, err := canonicalNameByTagID(ctx, tx, targetID); err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx,
			`update tag_names set tag_id = ? where tag_id = ? and name = ? and canonical = 0`, targetID, sourceID, normalized,
		); err != nil {
			return 0, fmt.Errorf("cannot move the synonym (tag=%d): %w", sourceID, err)
		}
		return targetID, confirmTagInTx(ctx, tx, targetID)
	})
}

// DetachSynonym removes the synonym name from tag sourceID and creates a confirmed tag named name, with no kind and
// no explicit attachments.
func (s *TagStore) DetachSynonym(ctx context.Context, sourceID int64, name string) (source, created domain.Tag, err error) {
	return s.changeSynonym(ctx, "detach", sourceID, name, func(tx *sql.Tx, normalized string) (int64, error) {
		if _, err := tx.ExecContext(ctx,
			`delete from tag_names where tag_id = ? and name = ? and canonical = 0`, sourceID, normalized,
		); err != nil {
			return 0, fmt.Errorf("cannot detach the synonym (tag=%d): %w", sourceID, err)
		}
		return insertTag(ctx, tx, normalized, false)
	})
}

// changeSynonym checks that name is a synonym of sourceID, applies change, which answers the other tag it changed,
// and commits as a match change with both tags read in the transaction.
func (s *TagStore) changeSynonym(
	ctx context.Context, verb string, sourceID int64, name string, change func(tx *sql.Tx, normalized string) (int64, error),
) (source, other domain.Tag, err error) {
	normalized, err := domain.NormalizeTagName(name)
	if err != nil {
		return domain.Tag{}, domain.Tag{}, err
	}
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.Tag{}, domain.Tag{}, err
	}
	defer func() { _ = tx.Rollback() }()

	if err := checkHeldSynonym(ctx, tx, sourceID, normalized); err != nil {
		return domain.Tag{}, domain.Tag{}, err
	}
	otherID, err := change(tx, normalized)
	if err != nil {
		return domain.Tag{}, domain.Tag{}, err
	}
	if source, err = tagByID(ctx, tx, sourceID); err != nil {
		return domain.Tag{}, domain.Tag{}, err
	}
	if other, err = tagByID(ctx, tx, otherID); err != nil {
		return domain.Tag{}, domain.Tag{}, err
	}
	if err := s.db.commitMatchChange(ctx, tx); err != nil {
		return domain.Tag{}, domain.Tag{}, fmt.Errorf("cannot %s the synonym: %w", verb, err)
	}
	return source, other, nil
}

// checkHeldSynonym answers domain.ErrTagNotFound for a missing tag id and domain.ErrTagSynonymNotFound unless the
// normalized name is a synonym of id.
func checkHeldSynonym(ctx context.Context, tx tagTx, id int64, normalized string) error {
	if _, err := canonicalNameByTagID(ctx, tx, id); err != nil {
		return err
	}
	lookup, found, err := lookupTagName(ctx, tx, normalized)
	if err != nil {
		return err
	}
	var holder *domain.TagNameHolder
	if found {
		holder = &domain.TagNameHolder{TagID: lookup.tagID, Canonical: lookup.isCanonical}
	}
	return domain.CheckSynonymOf(id, holder)
}

// promoteSynonymInTx swaps the canonical name of tag id with its synonym name and confirms the tag; the caller has
// checked that name is id's synonym.
func promoteSynonymInTx(ctx context.Context, tx queryExecer, id int64, name string) error {
	// The canonical row is demoted first, as one tag may hold one canonical name.
	if _, err := tx.ExecContext(ctx, `update tag_names set canonical = 0 where tag_id = ? and canonical = 1`, id); err != nil {
		return fmt.Errorf("cannot demote the canonical name (tag=%d): %w", id, err)
	}
	if _, err := tx.ExecContext(ctx,
		`update tag_names set canonical = 1 where tag_id = ? and name = ?`, id, name,
	); err != nil {
		return fmt.Errorf("cannot promote the synonym (tag=%d): %w", id, err)
	}
	return confirmTagInTx(ctx, tx, id)
}
