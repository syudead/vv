package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/syudead/vv/internal/domain"
)

type tagTx interface {
	queryExecer
	rowQueryer
}

type nameLookup struct {
	tagID int64
	// The canonical display name differs from the queried spelling when it is a synonym.
	canonicalName string
	isCanonical   bool
	tentative     bool
	kind          *domain.TagKindRef
}

// Canonical names and synonyms resolve to the same tag.
func lookupTagName(ctx context.Context, q tagTx, name string) (nameLookup, bool, error) {
	var lookup nameLookup
	var canonicalInt int
	var kind scannedTagKind
	err := q.QueryRowContext(ctx, `
		select tn.tag_id, tn.canonical, canon.name, tg.tentative, `+tagKindColumns+`
		  from tag_names tn
		  join tag_names canon on canon.tag_id = tn.tag_id and canon.canonical = 1
		  join tags tg on tg.id = tn.tag_id`+tagKindJoin+`
		 where tn.name = ?`, name,
	).Scan(append([]any{&lookup.tagID, &canonicalInt, &lookup.canonicalName, &lookup.tentative}, kind.targets()...)...)
	if errors.Is(err, sql.ErrNoRows) {
		return nameLookup{}, false, nil
	}
	if err != nil {
		return nameLookup{}, false, fmt.Errorf("cannot look up the tag by name (%s): %w", name, err)
	}
	lookup.isCanonical = canonicalInt != 0
	lookup.kind = kind.ref()
	return lookup, true, nil
}

func (l nameLookup) ref() domain.TagRef {
	return domain.TagRef{ID: l.tagID, Name: l.canonicalName, Tentative: l.tentative, Kind: l.kind}
}

func tagRefByID(ctx context.Context, q rowQueryer, id int64) (domain.TagRef, error) {
	ref := domain.TagRef{ID: id}
	var kind scannedTagKind
	err := q.QueryRowContext(ctx, `
		select tn.name, tg.tentative, `+tagKindColumns+`
		  from tags tg
		  join tag_names tn on tn.tag_id = tg.id and tn.canonical = 1`+tagKindJoin+`
		 where tg.id = ?`, id).Scan(append([]any{&ref.Name, &ref.Tentative}, kind.targets()...)...)
	ref.Kind = kind.ref()
	if errors.Is(err, sql.ErrNoRows) {
		return domain.TagRef{}, domain.ErrTagNotFound
	}
	if err != nil {
		return domain.TagRef{}, fmt.Errorf("cannot read the tag (id=%d): %w", id, err)
	}
	return ref, nil
}

func canonicalNameByTagID(ctx context.Context, q rowQueryer, id int64) (string, error) {
	var name string
	err := q.QueryRowContext(ctx, `select name from tag_names where tag_id = ? and canonical = 1`, id).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return "", domain.ErrTagNotFound
	}
	if err != nil {
		return "", fmt.Errorf("cannot read the tag names (id=%d): %w", id, err)
	}
	return name, nil
}

func tagByID(ctx context.Context, q tagTx, id int64) (domain.Tag, error) {
	ref, err := tagRefByID(ctx, q, id)
	if err != nil {
		return domain.Tag{}, err
	}
	var createdAt int64
	if err := q.QueryRowContext(ctx, `select created_at from tags where id = ?`, id).Scan(&createdAt); err != nil {
		return domain.Tag{}, fmt.Errorf("cannot read the tag (id=%d): %w", id, err)
	}
	count, err := videoCountByTagID(ctx, q, id)
	if err != nil {
		return domain.Tag{}, err
	}
	synonyms, err := synonymsByTagID(ctx, q, id)
	if err != nil {
		return domain.Tag{}, err
	}
	return domain.Tag{
		ID: id, Name: ref.Name, Synonyms: synonyms, VideoCount: count, Tentative: ref.Tentative, Kind: ref.Kind,
		CreatedAt: time.Unix(createdAt, 0),
	}, nil
}

// Count both manual and folder-derived attachments on current library videos.
func videoCountByTagID(ctx context.Context, q rowQueryer, id int64) (int, error) {
	query := `select count(*) from (` + taggedVideosSQL(` and tag_id = ?`) + `)`
	var count int
	if err := q.QueryRowContext(ctx, query, id, id).Scan(&count); err != nil {
		return 0, fmt.Errorf("cannot count tag uses (id=%d): %w", id, err)
	}
	return count, nil
}

func synonymsByTagID(ctx context.Context, q queryExecer, id int64) ([]string, error) {
	rows, err := q.QueryContext(ctx, `select name from tag_names where tag_id = ? and canonical = 0`, id)
	if err != nil {
		return nil, fmt.Errorf("cannot read synonyms (id=%d): %w", id, err)
	}
	defer func() { _ = rows.Close() }()
	names := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("cannot read synonyms (id=%d): %w", id, err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read synonyms (id=%d): %w", id, err)
	}
	domain.SortTagNames(names)
	return names, nil
}

// Write search and sort keys with the name, and clear its rejection in the same transaction.
func insertTagName(ctx context.Context, tx *sql.Tx, name string, tagID int64, canonical bool) error {
	if _, err := tx.ExecContext(ctx, `
		insert into tag_names (name, tag_id, canonical, search_key, sort_key, search_version)
		values (?, ?, ?, ?, ?, ?)`,
		name, tagID, boolToInt(canonical), domain.FoldForMatch(name), domain.NaturalSortKey(name), domain.SearchKeyVersion,
	); err != nil {
		return fmt.Errorf("cannot save the tag name (%s): %w", name, err)
	}
	return forgetRejectedName(ctx, tx, name)
}

func forgetRejectedName(ctx context.Context, tx *sql.Tx, name string) error {
	if _, err := tx.ExecContext(ctx, `delete from rejected_tag_names where name = ?`, name); err != nil {
		return fmt.Errorf("cannot forget the rejected tag name (%s): %w", name, err)
	}
	return nil
}

func confirmTagInTx(ctx context.Context, tx queryExecer, id int64) error {
	if _, err := tx.ExecContext(ctx, `update tags set tentative = 0 where id = ? and tentative = 1`, id); err != nil {
		return fmt.Errorf("cannot confirm the tag (id=%d): %w", id, err)
	}
	return nil
}

// Deduplicate IDs in input order and split existing/missing IDs; an empty selection needs no query.
func existingTagIDs(ctx context.Context, q queryExecer, ids []int64) (existing, missing []int64, err error) {
	if len(ids) == 0 {
		return nil, nil, nil
	}
	seen := make(map[int64]bool, len(ids))
	deduped := make([]int64, 0, len(ids))
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		deduped = append(deduped, id)
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(deduped)), ",")
	args := make([]any, 0, len(deduped))
	for _, id := range deduped {
		args = append(args, id)
	}

	rows, err := q.QueryContext(ctx, `select id from tags where id in (`+placeholders+`)`, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot check whether the tag exists: %w", err)
	}
	defer func() { _ = rows.Close() }()

	found := make(map[int64]bool, len(deduped))
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, nil, fmt.Errorf("cannot check whether the tag exists: %w", err)
		}
		found[id] = true
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("cannot check whether the tag exists: %w", err)
	}

	for _, id := range deduped {
		if found[id] {
			existing = append(existing, id)
		} else {
			missing = append(missing, id)
		}
	}
	return existing, missing, nil
}
