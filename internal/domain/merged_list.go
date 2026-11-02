package domain

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
)

// ItemCursor is one listed item's place: the cursor that continues the list after it and its sort value as
// the cursor encodes it, nil when the value is missing and sorts last.
type ItemCursor struct {
	Cursor    string
	SortValue *string
}

// SideCursor is how far a merged listing has read one node's list. Done means that node has no more items;
// Skipped marks a link the listing left out while it was unreachable, which stays out so its items are neither
// repeated nor counted twice.
type SideCursor struct {
	Cursor  string `json:"c,omitempty"`
	Done    bool   `json:"d,omitempty"`
	Skipped bool   `json:"s,omitempty"`
}

// MergedCursor continues a list merged from this node, in slot 0, and each link, in its slot, with one side per
// node. Once a listing has started, a link its cursor does not hold is left out for the rest of it, so a link
// paired or connected mid-listing never adds items before ones already shown.
type MergedCursor struct {
	Sides map[int64]SideCursor `json:"s,omitempty"`
}

// mergedCursorPrefix tells a merged cursor from a single node's, so a cursor of the other kind is refused; an m1.
// cursor names one peer without its slot and is refused the same way.
const mergedCursorPrefix = "m2."

// LinkReach is one link as a merged read sees it: its slot and whether it is connected now.
type LinkReach struct {
	Slot      int64
	Connected bool
}

// Begin starts a listing over links, reading the connected ones and leaving out the others for the whole
// listing; a cursor that already continues a listing is returned unchanged.
func (c MergedCursor) Begin(links []LinkReach) MergedCursor {
	if c.Started() {
		return c
	}
	sides := map[int64]SideCursor{0: {}}
	for _, link := range links {
		sides[link.Slot] = SideCursor{Done: !link.Connected, Skipped: !link.Connected}
	}
	return MergedCursor{Sides: sides}
}

// Started reports a cursor that continues a listing, as opposed to the first page's.
func (c MergedCursor) Started() bool {
	return len(c.Sides) > 0
}

// Side is the place of the node in slot; a link a started listing does not hold is skipped.
func (c MergedCursor) Side(slot int64) SideCursor {
	if side, ok := c.Sides[slot]; ok || slot == 0 || !c.Started() {
		return side
	}
	return SideCursor{Done: true, Skipped: true}
}

// Links are the slots of the links the listing holds, in slot order.
func (c MergedCursor) Links() []int64 {
	slots := make([]int64, 0, len(c.Sides))
	for slot := range c.Sides {
		if slot != 0 {
			slots = append(slots, slot)
		}
	}
	slices.Sort(slots)
	return slots
}

// Skip is c with the link in slot left out for the rest of the listing.
func (c MergedCursor) Skip(slot int64) MergedCursor {
	sides := maps.Clone(c.Sides)
	if sides == nil {
		sides = map[int64]SideCursor{}
	}
	sides[slot] = SideCursor{Cursor: sides[slot].Cursor, Done: true, Skipped: true}
	return MergedCursor{Sides: sides}
}

// Encode is the opaque text of c.
func (c MergedCursor) Encode() string {
	raw, _ := json.Marshal(c)
	return mergedCursorPrefix + base64.RawURLEncoding.EncodeToString(raw)
}

// ParseMergedCursor reads a cursor Encode wrote; empty text starts the list.
func ParseMergedCursor(text string) (MergedCursor, error) {
	if text == "" {
		return MergedCursor{}, nil
	}
	encoded, ok := strings.CutPrefix(text, mergedCursorPrefix)
	if !ok {
		return MergedCursor{}, fmt.Errorf("%w: not a cursor of a merged list", ErrInvalidCursor)
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return MergedCursor{}, fmt.Errorf("%w: %w", ErrInvalidCursor, err)
	}
	var cursor MergedCursor
	if err := json.Unmarshal(raw, &cursor); err != nil {
		return MergedCursor{}, fmt.Errorf("%w: %w", ErrInvalidCursor, err)
	}
	if _, ok := cursor.Sides[0]; !ok {
		return MergedCursor{}, fmt.Errorf("%w: the cursor holds no local side", ErrInvalidCursor)
	}
	for slot := range cursor.Sides {
		if slot < 0 {
			return MergedCursor{}, fmt.Errorf("%w: slot %d", ErrInvalidCursor, slot)
		}
	}
	return cursor, nil
}

// MergeSide is one node's page: the node's slot, 0 for this node, its items in its own order, their cursors in the
// same order, and the cursor that continues after the page, empty when the side has no more items.
type MergeSide[T any] struct {
	Slot    int64
	Items   []T
	Cursors []ItemCursor
	Next    string
}

// MergePages takes up to limit items from the sides in sort's order and returns them with the cursor that
// continues every side; more is false once all are done. Every side of the listing is passed, a done or skipped
// one empty. Items compare by sort value, missing values last; on equal values the local item comes first, then
// the links in slot order. A side whose page is used up but continues stops the merge, since its next item is
// unknown; each side reads limit items, so that happens only on a full page.
func MergePages[T any](sort VideoSort, cursor MergedCursor, sides []MergeSide[T], limit int) ([]T, MergedCursor, bool) {
	sides = slices.Clone(sides)
	slices.SortStableFunc(sides, func(a, b MergeSide[T]) int { return compareInt64(a.Slot, b.Slot) })
	taken := make([]int, len(sides))
	count := 0
	for _, side := range sides {
		count += len(side.Items)
	}
	out := make([]T, 0, min(limit, count))
	for len(out) < limit {
		k := nextSide(sort, sides, taken)
		if k < 0 {
			break
		}
		out, taken[k] = append(out, sides[k].Items[taken[k]]), taken[k]+1
	}
	next := MergedCursor{Sides: maps.Clone(cursor.Sides)}
	if next.Sides == nil {
		next.Sides = map[int64]SideCursor{}
	}
	more := false
	for k, side := range sides {
		place := cursor.Side(side.Slot)
		if taken[k] > 0 {
			place.Cursor = side.Cursors[taken[k]-1].Cursor
		}
		place.Done = taken[k] == len(side.Items) && side.Next == ""
		next.Sides[side.Slot] = place
		more = more || !place.Done
	}
	return out, next, more
}

// nextSide is the index of the side whose next item comes first, the earliest side on equal values, or -1 when
// the merge stops: every side is used up, or one is used up on a page that continues.
func nextSide[T any](sort VideoSort, sides []MergeSide[T], taken []int) int {
	best := -1
	for k, side := range sides {
		if taken[k] == len(side.Items) {
			if side.Next != "" {
				return -1
			}
			continue
		}
		if best < 0 || compareSortValues(sort, side.Cursors[taken[k]].SortValue, sides[best].Cursors[taken[best]].SortValue) < 0 {
			best = k
		}
	}
	return best
}

// compareSortValues orders two sort values as both nodes' cursors encode them: numbers numerically, titles
// by their key bytes, missing values last in either direction.
func compareSortValues(sort VideoSort, a, b *string) int {
	switch {
	case a == nil && b == nil:
		return 0
	case a == nil:
		return 1
	case b == nil:
		return -1
	}
	order := 0
	if sort == SortTitleAsc || sort == SortTitleDesc {
		order = strings.Compare(*a, *b)
	} else {
		x, errA := strconv.ParseInt(*a, 10, 64)
		y, errB := strconv.ParseInt(*b, 10, 64)
		if errA != nil || errB != nil {
			order = strings.Compare(*a, *b)
		} else {
			order = compareInt64(x, y)
		}
	}
	if strings.HasSuffix(string(sort), "Desc") {
		return -order
	}
	return order
}

func compareInt64(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
