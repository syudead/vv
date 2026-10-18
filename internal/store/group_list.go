package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"strconv"
	"strings"

	"modernc.org/sqlite"

	"github.com/syudead/vv/internal/domain"
)

// Reject cursors belonging to another list.
const groupListCursorKind = "groups"

const foldKeyFunction = "vv_fold_key"

func init() {
	err := sqlite.RegisterDeterministicScalarFunction(foldKeyFunction, 1,
		func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			value, ok := args[0].(string)
			if !ok {
				return nil, fmt.Errorf("%s: name is not text", foldKeyFunction)
			}
			return domain.FoldKey(value), nil
		})
	if err != nil {
		panic(err)
	}
}

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

	candidates, err := groupListCandidates(ctx, tx, audience, query, after, limit+1)
	if err != nil {
		return domain.GroupListPage{}, err
	}
	ids := make([]int64, 0, len(candidates))
	for _, candidate := range candidates[:min(limit, len(candidates))] {
		ids = append(ids, candidate.id)
	}
	groups, err := loadGroups(ctx, tx, audience, ids)
	if err != nil {
		return domain.GroupListPage{}, err
	}
	page := domain.GroupListPage{}
	for _, id := range ids {
		page.Groups = append(page.Groups, groups[id])
	}
	if len(candidates) > limit {
		last := candidates[limit-1]
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

// Filter names and audience visibility before paging; load full members only for the returned page.
func groupListCandidates(ctx context.Context, tx *sql.Tx, audience domain.Audience, query string, after *groupListCandidate, limit int) ([]groupListCandidate, error) {
	where, args := ` where `+groupHasVisibleVideosCondition("g", audience), []any{}
	if needle := domain.FoldKey(strings.TrimSpace(query)); needle != "" {
		where += ` and instr(` + foldKeyFunction + `(name), ?) > 0`
		args = append(args, needle)
	}
	if after != nil {
		where += ` and (title_key, id) > (?, ?)`
		args = append(args, after.titleKey, after.id)
	}
	args = append(args, limit)
	rows, err := tx.QueryContext(ctx, `select id, title_key from groups g`+where+`
		order by title_key, id limit ?`, args...)
	if err != nil {
		return nil, fmt.Errorf("cannot read the groups: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []groupListCandidate
	for rows.Next() {
		var candidate groupListCandidate
		if err := rows.Scan(&candidate.id, &candidate.titleKey); err != nil {
			return nil, fmt.Errorf("cannot read the groups: %w", err)
		}
		out = append(out, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the groups: %w", err)
	}
	return out, nil
}

// Visibility needs one accessible descendant, without computing every descendant's playback order.
func groupHasVisibleVideosCondition(alias string, audience domain.Audience) string {
	return `exists (with recursive visible_groups(id) as (
		select ` + alias + `.id
		union
		select gi.child_id from visible_groups vg join group_items gi on gi.group_id = vg.id
		where gi.child_id is not null)
		select 1 from visible_groups vg join group_items gi on gi.group_id = vg.id
		where gi.content_key is not null and ` + shownVideoForKeyExpr("gi.content_key", audience) + ` is not null)`
}
