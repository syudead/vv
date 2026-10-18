package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// Tentative bulk edits create tentative tags; name-writing paths remove matching rejected names.

// ConfirmTag preserves names and attachments and is a no-op for confirmed tags.
func (s *TagStore) ConfirmTag(ctx context.Context, id int64) (domain.Tag, error) {
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.Tag{}, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tagRefByID(ctx, tx, id); err != nil {
		return domain.Tag{}, err
	}
	if err := confirmTagInTx(ctx, tx, id); err != nil {
		return domain.Tag{}, err
	}
	tag, err := tagByID(ctx, tx, id)
	if err != nil {
		return domain.Tag{}, err
	}
	if err := s.db.commitMatchChange(ctx, tx); err != nil {
		return domain.Tag{}, fmt.Errorf("cannot confirm the tag: %w", err)
	}
	return tag, nil
}

// RejectTag removes only tentative tags and remembers their canonical name; names and attachments cascade.
func (s *TagStore) RejectTag(ctx context.Context, id int64) (string, error) {
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()

	ref, err := tagRefByID(ctx, tx, id)
	if err != nil {
		return "", err
	}
	if !ref.Tentative {
		return "", domain.ErrTagNotTentative
	}
	if _, err := tx.ExecContext(ctx, `delete from tags where id = ?`, id); err != nil {
		return "", fmt.Errorf("cannot reject the tag (id=%d): %w", id, err)
	}
	if _, err := tx.ExecContext(ctx,
		`insert or ignore into rejected_tag_names (name, sort_key, search_version, created_at) values (?, ?, ?, ?)`,
		ref.Name, domain.NaturalSortKey(ref.Name), domain.SearchKeyVersion, time.Now().Unix(),
	); err != nil {
		return "", fmt.Errorf("cannot remember the rejected tag name (%s): %w", ref.Name, err)
	}

	if err := s.db.commitMatchChange(ctx, tx); err != nil {
		return "", fmt.Errorf("cannot reject the tag: %w", err)
	}
	return ref.Name, nil
}

const rejectedTagNameCursorSort = "rejectedTagName"

// ListRejectedTagNames uses natural order; nonpositive limits return all remaining rows and no continuation cursor.
func (s *TagStore) ListRejectedTagNames(ctx context.Context, cursor string, limit int) (domain.RejectedTagNamePage, error) {
	where, args := "", []any{}
	if cursor != "" {
		sortKey, name, err := decodeRejectedTagNameCursor(cursor)
		if err != nil {
			return domain.RejectedTagNamePage{}, err
		}
		where = ` where (sort_key, name) > (?, ?)`
		args = append(args, sortKey, name)
	}
	query := `select name, sort_key from rejected_tag_names` + where + ` order by sort_key, name`
	if limit > 0 {
		query += ` limit ?`
		args = append(args, limit+1)
	}

	// Read count and page in one deferred snapshot without taking the write slot.
	tx, err := s.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.RejectedTagNamePage{}, fmt.Errorf("cannot read rejected tag names: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	page := domain.RejectedTagNamePage{Items: []string{}}
	if err := tx.QueryRowContext(ctx, `select count(*) from rejected_tag_names`).Scan(&page.Total); err != nil {
		return domain.RejectedTagNamePage{}, fmt.Errorf("cannot count rejected tag names: %w", err)
	}
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return domain.RejectedTagNamePage{}, fmt.Errorf("cannot read rejected tag names: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var sortKeys []string
	for rows.Next() {
		var name, sortKey string
		if err := rows.Scan(&name, &sortKey); err != nil {
			return domain.RejectedTagNamePage{}, fmt.Errorf("cannot read rejected tag names: %w", err)
		}
		page.Items = append(page.Items, name)
		sortKeys = append(sortKeys, sortKey)
	}
	if err := rows.Err(); err != nil {
		return domain.RejectedTagNamePage{}, fmt.Errorf("cannot read rejected tag names: %w", err)
	}
	if limit > 0 && len(page.Items) > limit {
		page.Items = page.Items[:limit]
		page.NextCursor = encodeRejectedTagNameCursor(sortKeys[limit-1], page.Items[limit-1])
	}
	return page, nil
}

// Normalized names contain no control characters, so their sort keys cannot contain the cursor separator.
func encodeRejectedTagNameCursor(sortKey, name string) string {
	return encodeCursorFields(cursorFields{sort: rejectedTagNameCursorSort, value: sortKey + cursorSeparator + name})
}

func decodeRejectedTagNameCursor(cursor string) (sortKey, name string, err error) {
	c, err := decodeCursor(cursor)
	if err != nil {
		return "", "", err
	}
	if c.sort != rejectedTagNameCursorSort || c.seed != "" || c.isNull || c.id != 0 {
		return "", "", fmt.Errorf("%w: cursor is not for rejected tag names", domain.ErrInvalidCursor)
	}
	sortKey, name, ok := strings.Cut(c.value, cursorSeparator)
	if !ok {
		return "", "", fmt.Errorf("%w: cursor has no name", domain.ErrInvalidCursor)
	}
	return sortKey, name, nil
}

// ForgetRejectedTagName returns whether a row was removed; normalize valid input and otherwise match the raw spelling.
func (s *TagStore) ForgetRejectedTagName(ctx context.Context, name string) (bool, error) {
	if normalized, err := domain.NormalizeTagName(name); err == nil {
		name = normalized
	}
	result, err := s.sql.ExecContext(ctx, `delete from rejected_tag_names where name = ?`, name)
	if err != nil {
		return false, fmt.Errorf("cannot forget the rejected tag name: %w", err)
	}
	removed, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("cannot forget the rejected tag name: %w", err)
	}
	return removed > 0, nil
}
