package store

import "testing"

type pagingCase[T any] struct {
	value T
	limit int
}

// pagingCases crosses every row boundary for every ordering with one-item
// pages. Larger page sizes use the first ordering instead of repeating the
// Cartesian product; they exercise page slicing and cursor advancement.
func pagingCases[T any](values []T, largerLimits ...int) []pagingCase[T] {
	cases := make([]pagingCase[T], 0, len(values)+len(largerLimits))
	for _, value := range values {
		cases = append(cases, pagingCase[T]{value: value, limit: 1})
	}
	for _, limit := range largerLimits {
		cases = append(cases, pagingCase[T]{value: values[0], limit: limit})
	}
	return cases
}

// A full traversal alone could pass even if the query ignored its limit.
func assertPageSize(t *testing.T, count, limit int, nextCursor string) {
	t.Helper()
	if limit <= 0 {
		return
	}
	if count > limit || (nextCursor != "" && count != limit) {
		t.Fatalf("page has %d items, limit %d, next cursor %q", count, limit, nextCursor)
	}
}
