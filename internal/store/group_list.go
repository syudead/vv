package store

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"github.com/syudead/vv/internal/domain"
)

// Reject cursors belonging to another list.
const groupListCursorKind = "groups"

type groupListCandidate struct {
	id       int64
	titleKey string
}

// ListGroups returns visible nonempty groups at any depth in natural-name order, with case-insensitive name filtering.
func (s *LibraryStore) ListGroups(ctx context.Context, audience domain.Audience, query, cursor string, limit int) (domain.GroupListPage, error) {
	after, err := decodeGroupListCursor(cursor)
	if err != nil {
		return domain.GroupListPage{}, err
	}
	if limit <= 0 {
		limit = domain.GroupListDefaultLimit
	}
	limit = min(limit, domain.GroupListMaxLimit)

	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.GroupListPage{}, fmt.Errorf("cannot start reading the groups: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	candidates, err := groupListCandidates(ctx, tx, query, after)
	if err != nil {
		return domain.GroupListPage{}, err
	}
	// Fetch candidates in batches until enough audience-visible groups remain for a page and continuation.
	var shown []groupListCandidate
	page := domain.GroupListPage{}
	for start := 0; start < len(candidates) && len(shown) <= limit; start += limit + 1 {
		chunk := candidates[start:min(start+limit+1, len(candidates))]
		ids := make([]int64, 0, len(chunk))
		for _, candidate := range chunk {
			ids = append(ids, candidate.id)
		}
		groups, err := loadGroups(ctx, tx, audience, ids)
		if err != nil {
			return domain.GroupListPage{}, err
		}
		for _, candidate := range chunk {
			group, ok := groups[candidate.id]
			if !ok || len(shown) > limit {
				continue
			}
			shown = append(shown, candidate)
			page.Groups = append(page.Groups, group)
		}
	}
	if len(shown) > limit {
		last := shown[limit-1]
		page.Groups = page.Groups[:limit]
		page.NextCursor = packCursor(groupListCursorKind, strconv.FormatInt(last.id, 10), last.titleKey)
	}
	if page.Roots, err = listMediaFolders(ctx, tx); err != nil {
		return domain.GroupListPage{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.GroupListPage{}, fmt.Errorf("cannot finish reading the groups: %w", err)
	}
	return page, nil
}

func decodeGroupListCursor(cursor string) (*groupListCandidate, error) {
	if cursor == "" {
		return nil, nil
	}
	parts, err := unpackCursor(cursor, 3)
	if err != nil {
		return nil, err
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if parts[0] != groupListCursorKind || err != nil || id < 1 {
		return nil, fmt.Errorf("%w: not a group list cursor", domain.ErrInvalidCursor)
	}
	return &groupListCandidate{id: id, titleKey: parts[2]}, nil
}

// Use domain.FoldKey after reading names: SQLite lower() is ASCII-only and strings.ToLower does not unify sigma forms.
func groupListCandidates(ctx context.Context, tx *sql.Tx, query string, after *groupListCandidate) ([]groupListCandidate, error) {
	where, args := "", []any{}
	if after != nil {
		where = ` where title_key > ?1 or (title_key = ?1 and id > ?2)`
		args = append(args, after.titleKey, after.id)
	}
	rows, err := tx.QueryContext(ctx, `select id, name, title_key from groups`+where+` order by title_key, id`, args...)
	if err != nil {
		return nil, fmt.Errorf("cannot read the groups: %w", err)
	}
	defer func() { _ = rows.Close() }()
	needle := domain.FoldKey(strings.TrimSpace(query))
	var out []groupListCandidate
	for rows.Next() {
		var candidate groupListCandidate
		var name string
		if err := rows.Scan(&candidate.id, &name, &candidate.titleKey); err != nil {
			return nil, fmt.Errorf("cannot read the groups: %w", err)
		}
		if needle == "" || strings.Contains(domain.FoldKey(name), needle) {
			out = append(out, candidate)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the groups: %w", err)
	}
	return out, nil
}
