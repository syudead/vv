package store

import (
	"context"
	"fmt"

	"github.com/syudead/vv/internal/domain"
)

// AddSynonym は名前 name をタグ tagID のシノニムにする（data-model.md §4）。
//
//   - name が無ければ canonical = 0 で1行足す。
//   - name が既に tagID のシノニムなら何も変えない。
//   - name が tagID 自身の元の名前なら *domain.TagNameConflict を返す。
//   - name が別のタグ S のシノニムなら *domain.TagNameConflict を返す（S を示す）。
//   - name が別のタグ S の元の名前なら、mergeTagID が S の id と一致すれば
//     S を tagID へ統合し、一致しなければ（無い場合を含む）
//     *domain.TagMergeRequired を返す（S を示す）。
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
	case lookup.tagID == tagID && lookup.isCanonical:
		return domain.Tag{}, &domain.TagNameConflict{Tag: domain.TagRef{ID: tagID, Name: lookup.canonicalName}}
	case lookup.tagID == tagID:
		// 既にこのタグのシノニム。何も変えない。
	case !lookup.isCanonical:
		return domain.Tag{}, &domain.TagNameConflict{Tag: domain.TagRef{ID: lookup.tagID, Name: lookup.canonicalName}}
	case mergeTagID == nil || *mergeTagID != lookup.tagID:
		return domain.Tag{}, &domain.TagMergeRequired{Tag: domain.TagRef{ID: lookup.tagID, Name: lookup.canonicalName}}
	default:
		if _, err := mergeTagInto(ctx, tx, tagID, lookup.tagID); err != nil {
			return domain.Tag{}, err
		}
	}

	tag, err := tagByID(ctx, tx, tagID)
	if err != nil {
		return domain.Tag{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Tag{}, fmt.Errorf("cannot add the synonym: %w", err)
	}
	return tag, nil
}

// RemoveSynonym はタグ tagID のシノニム name を解除する。name が tagID の
// シノニムでなければ何も変えない。tagID が無ければ domain.ErrTagNotFound を
// 返す。
func (s *TagStore) RemoveSynonym(ctx context.Context, tagID int64, name string) error {
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := canonicalNameByTagID(ctx, tx, tagID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`delete from tag_names where name = ? and tag_id = ? and canonical = 0`, name, tagID,
	); err != nil {
		return fmt.Errorf("cannot remove the synonym (tag=%d): %w", tagID, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("cannot remove the synonym: %w", err)
	}
	return nil
}
