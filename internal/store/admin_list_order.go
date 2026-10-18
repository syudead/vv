package store

import (
	"fmt"
	"strconv"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// Admin lists share a value order, then natural name order and ID for stable ties.

// Expand keyset comparisons because the value and name can sort in opposite directions.
type adminListOrder struct {
	// The listed column preceding the name; empty for name ordering.
	value string
	// Only the value descends; name and ID always ascend.
	desc    bool
	orderBy string
}

func newAdminListOrder(value string, desc bool) adminListOrder {
	order := adminListOrder{value: value, desc: desc, orderBy: `sort_key asc, id asc`}
	if value != "" {
		direction := ` asc`
		if desc {
			direction = ` desc`
		}
		order.orderBy = value + direction + `, ` + order.orderBy
	}
	return order
}

var adminListOrders = map[domain.TagSort]adminListOrder{
	domain.TagSortName:        newAdminListOrder("", false),
	domain.TagSortCountDesc:   newAdminListOrder("video_count", true),
	domain.TagSortCountAsc:    newAdminListOrder("video_count", false),
	domain.TagSortCreatedDesc: newAdminListOrder("created_at", true),
	domain.TagSortCreatedAsc:  newAdminListOrder("created_at", false),
}

type adminListRow struct {
	id         int64
	videoCount int
	createdAt  time.Time
	sortKey    string
}

func (o adminListOrder) cursorValue(row adminListRow) string {
	switch o.value {
	case "video_count":
		return strconv.Itoa(row.videoCount)
	case "created_at":
		return strconv.FormatInt(row.createdAt.Unix(), 10)
	default:
		return ""
	}
}

// Prefix cursors with the list kind to reject cursors from another list.
func (o adminListOrder) encodeCursor(list string, sort domain.TagSort, last adminListRow) string {
	return packCursor(list, string(sort), o.cursorValue(last), strconv.FormatInt(last.id, 10), last.sortKey)
}

// Reject malformed cursors or cursors for another list or order with ErrInvalidCursor.
func (o adminListOrder) cursorCondition(list string, sort domain.TagSort, cursor string) (string, []any, error) {
	parts, err := unpackCursor(cursor, 5)
	if err != nil {
		return "", nil, err
	}
	if parts[0] != list {
		return "", nil, fmt.Errorf("%w: cursor is not for %s", domain.ErrInvalidCursor, list)
	}
	if parts[1] != string(sort) {
		return "", nil, fmt.Errorf("%w: cursor is for a different sort order", domain.ErrInvalidCursor)
	}
	id, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		return "", nil, fmt.Errorf("%w: cannot parse the id", domain.ErrInvalidCursor)
	}
	sortKey := parts[4]
	nameAfter := `(sort_key, id) > (?, ?)`
	if o.value == "" {
		if parts[2] != "" {
			return "", nil, fmt.Errorf("%w: unexpected sort value", domain.ErrInvalidCursor)
		}
		return nameAfter, []any{sortKey, id}, nil
	}
	value, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return "", nil, fmt.Errorf("%w: cannot parse the sort value", domain.ErrInvalidCursor)
	}
	op := ` > `
	if o.desc {
		op = ` < `
	}
	clause := `(` + o.value + op + `? or (` + o.value + ` = ? and ` + nameAfter + `))`
	return clause, []any{value, value, sortKey, id}, nil
}

// A window count shares the page query to avoid aggregating listed twice; fetch one extra row for pagination.
func (o adminListOrder) pageStatement(listed string, listedArgs []any, columns, cursorClause string, cursorArgs []any, limit int) (string, []any) {
	statement := listed + ` select ` + columns + `, total from (
		select ` + columns + `, count(*) over () as total from listed
	)`
	args := append([]any{}, listedArgs...)
	if cursorClause != "" {
		statement += ` where ` + cursorClause
		args = append(args, cursorArgs...)
	}
	statement += ` order by ` + o.orderBy
	if limit > 0 {
		statement += ` limit ?`
		args = append(args, limit+1)
	}
	return statement, args
}
