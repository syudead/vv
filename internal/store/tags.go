package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// Tag writes invalidate smart folders after commit; they neither delete artifacts nor wake workers.

// CreateTag rejects names already used as canonical names or synonyms.
func (s *TagStore) CreateTag(ctx context.Context, name string) (domain.Tag, error) {
	normalized, err := domain.NormalizeTagName(name)
	if err != nil {
		return domain.Tag{}, err
	}

	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.Tag{}, err
	}
	defer func() { _ = tx.Rollback() }()

	lookup, found, err := lookupTagName(ctx, tx, normalized)
	if err != nil {
		return domain.Tag{}, err
	}
	if found {
		return domain.Tag{}, &domain.TagNameConflict{Tag: lookup.ref()}
	}

	id, err := insertTag(ctx, tx, normalized, false)
	if err != nil {
		return domain.Tag{}, err
	}
	// A new tag may already match folder-derived attachments; reread it in the transaction for accurate counts and creation time.
	tag, err := tagByID(ctx, tx, id)
	if err != nil {
		return domain.Tag{}, err
	}

	if err := s.db.commitMatchChange(ctx, tx); err != nil {
		return domain.Tag{}, fmt.Errorf("cannot create the tag: %w", err)
	}
	return tag, nil
}

// RenameTag confirms changed names and clears their rejection; unchanged names are no-ops and even the tag's own synonym conflicts.
func (s *TagStore) RenameTag(ctx context.Context, id int64, name string) (domain.Tag, error) {
	normalized, err := domain.NormalizeTagName(name)
	if err != nil {
		return domain.Tag{}, err
	}

	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.Tag{}, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := renameTagInTx(ctx, tx, id, normalized); err != nil {
		return domain.Tag{}, err
	}
	tag, err := tagByID(ctx, tx, id)
	if err != nil {
		return domain.Tag{}, err
	}
	if err := s.db.commitMatchChange(ctx, tx); err != nil {
		return domain.Tag{}, fmt.Errorf("cannot rename the tag: %w", err)
	}
	return tag, nil
}

// renameTagInTx gives tag id the normalized canonical name and reports whether it changed; a tag holding the name,
// even as id's own synonym, is a *domain.TagNameConflict.
func renameTagInTx(ctx context.Context, tx *sql.Tx, id int64, normalized string) (bool, error) {
	current, err := canonicalNameByTagID(ctx, tx, id)
	if err != nil || current == normalized {
		return false, err
	}
	lookup, found, err := lookupTagName(ctx, tx, normalized)
	if err != nil {
		return false, err
	}
	if found {
		return false, &domain.TagNameConflict{Tag: lookup.ref()}
	}
	if _, err := tx.ExecContext(ctx, `
		update tag_names set name = ?, search_key = ?, sort_key = ?, search_version = ?
		 where tag_id = ? and canonical = 1`,
		normalized, domain.FoldForMatch(normalized), domain.NaturalSortKey(normalized), domain.SearchKeyVersion, id,
	); err != nil {
		return false, fmt.Errorf("cannot rename the tag (id=%d): %w", id, err)
	}
	if err := forgetRejectedName(ctx, tx, normalized); err != nil {
		return false, err
	}
	return true, confirmTagInTx(ctx, tx, id)
}

// DeleteTag cascades names and all attachments, including those for videos outside the current library.
func (s *TagStore) DeleteTag(ctx context.Context, id int64) error {
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := canonicalNameByTagID(ctx, tx, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `delete from tags where id = ?`, id); err != nil {
		return fmt.Errorf("cannot delete the tag (id=%d): %w", id, err)
	}

	if err := s.db.commitMatchChange(ctx, tx); err != nil {
		return fmt.Errorf("cannot delete the tag: %w", err)
	}
	return nil
}

// MergeTags atomically unions attachments and aliases into a confirmed target, deduplicating sources and reporting missing IDs.
func (s *TagStore) MergeTags(ctx context.Context, targetID int64, sourceIDs []int64) (domain.TagMergeOutcome, error) {
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.TagMergeOutcome{}, err
	}
	defer func() { _ = tx.Rollback() }()

	outcome, err := mergeTagsInTx(ctx, tx, targetID, sourceIDs)
	if err != nil {
		return domain.TagMergeOutcome{}, err
	}
	if err := s.db.commitMatchChange(ctx, tx); err != nil {
		return domain.TagMergeOutcome{}, fmt.Errorf("cannot merge the tags: %w", err)
	}
	return outcome, nil
}

func mergeTagsInTx(ctx context.Context, tx tagTx, targetID int64, sourceIDs []int64) (domain.TagMergeOutcome, error) {
	if _, err := canonicalNameByTagID(ctx, tx, targetID); err != nil {
		return domain.TagMergeOutcome{}, err
	}
	unique := uniqueTagIDs(sourceIDs)
	existing, err := tagBatchTargets(ctx, tx, unique)
	if err != nil {
		return domain.TagMergeOutcome{}, err
	}
	notFound := []int64{}
	merged := make([]int64, 0, len(unique))
	for _, id := range unique {
		switch _, found := existing[id]; {
		case !found:
			notFound = append(notFound, id)
		case id != targetID:
			merged = append(merged, id)
		}
	}
	if len(merged) > 0 {
		if err := mergeTagsInto(ctx, tx, targetID, merged); err != nil {
			return domain.TagMergeOutcome{}, err
		}
	}
	tag, err := tagByID(ctx, tx, targetID)
	if err != nil {
		return domain.TagMergeOutcome{}, err
	}
	return domain.TagMergeOutcome{Tag: tag, NotFoundIDs: notFound}, nil
}

// Use a fixed statement count without assembling intermediate tags; callers validate that all sources exist and exclude the target.
func mergeTagsInto(ctx context.Context, tx queryExecer, targetID int64, sourceIDs []int64) error {
	encoded, err := json.Marshal(sourceIDs)
	if err != nil {
		return fmt.Errorf("cannot build tag ids: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		insert or ignore into video_tags (content_key, tag_id, created_at, tentative)
		select content_key, ?, created_at, tentative from video_tags
		 where tag_id in (select value from json_each(?))`,
		targetID, string(encoded),
	); err != nil {
		return fmt.Errorf("cannot copy assignments to the merge target (target=%d): %w", targetID, err)
	}
	if _, err := tx.ExecContext(ctx, `
		update tag_names set tag_id = ?, canonical = 0
		 where tag_id in (select value from json_each(?))`,
		targetID, string(encoded),
	); err != nil {
		return fmt.Errorf("cannot move tag names to the merge target (target=%d): %w", targetID, err)
	}
	if _, err := tx.ExecContext(ctx,
		`delete from tags where id in (select value from json_each(?))`, string(encoded),
	); err != nil {
		return fmt.Errorf("cannot delete the merged tags (target=%d): %w", targetID, err)
	}
	if err := rewriteSmartRuleRefs(ctx, tx, domain.SmartRefTag, targetID, sourceIDs); err != nil {
		return err
	}
	return confirmTagInTx(ctx, tx, targetID)
}

// Resolve normalized names including synonyms, creating a confirmed tag in the same transaction if absent.
func findOrCreateTag(ctx context.Context, tx *sql.Tx, normalized string) (domain.TagRef, bool, error) {
	lookup, found, err := lookupTagName(ctx, tx, normalized)
	if err != nil {
		return domain.TagRef{}, false, err
	}
	if found {
		return lookup.ref(), false, nil
	}
	id, err := insertTag(ctx, tx, normalized, false)
	if err != nil {
		return domain.TagRef{}, false, err
	}
	return domain.TagRef{ID: id, Name: normalized}, true, nil
}

// The caller must verify name availability before insertion.
func insertTag(ctx context.Context, tx *sql.Tx, normalized string, tentative bool) (int64, error) {
	res, err := tx.ExecContext(ctx, `insert into tags (created_at, tentative) values (?, ?)`,
		time.Now().Unix(), boolToInt(tentative))
	if err != nil {
		return 0, fmt.Errorf("cannot create the tag: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("cannot create the tag: %w", err)
	}
	if err := insertTagName(ctx, tx, normalized, id, true); err != nil {
		return 0, err
	}
	return id, nil
}
