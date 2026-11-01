package domain

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// ItemCursor is one listed item's place: the cursor that continues the list after it and its sort value as
// the cursor encodes it, nil when the value is missing and sorts last.
type ItemCursor struct {
	Cursor    string
	SortValue *string
}

// MergedCursor continues a list merged from this node and the peer with one cursor per side. A side is done
// once its items are exhausted; PeerSkipped marks a listing that left the peer out while it was unreachable,
// which keeps leaving it out so remote items are neither repeated nor counted twice.
type MergedCursor struct {
	Local       string `json:"l,omitempty"`
	Peer        string `json:"p,omitempty"`
	LocalDone   bool   `json:"ld,omitempty"`
	PeerDone    bool   `json:"pd,omitempty"`
	PeerSkipped bool   `json:"ps,omitempty"`
}

// mergedCursorPrefix tells a merged cursor from a single node's, so a cursor of the other kind is refused.
const mergedCursorPrefix = "m1."

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
	return cursor, nil
}

// MergeSide is one node's page: items in its own order, their cursors in the same order, and the cursor that
// continues after the page, empty when the side has no more items.
type MergeSide[T any] struct {
	Items   []T
	Cursors []ItemCursor
	Next    string
}

// MergePages takes up to limit items from the two sides in sort's order and returns them with the cursor that
// continues both sides; more is false once both are done. Items compare by sort value, missing values last;
// on equal values the local item comes first. A side whose page is used up but continues stops the merge,
// since its next item is unknown; each side reads limit items, so that happens only on a full page.
func MergePages[T any](sort VideoSort, cursor MergedCursor, local, peer MergeSide[T], limit int) ([]T, MergedCursor, bool) {
	out := make([]T, 0, min(limit, len(local.Items)+len(peer.Items)))
	i, j := 0, 0
	for len(out) < limit {
		localLeft, peerLeft := i < len(local.Items), j < len(peer.Items)
		if (!localLeft && local.Next != "") || (!peerLeft && peer.Next != "") || (!localLeft && !peerLeft) {
			break
		}
		takeLocal := localLeft && (!peerLeft || compareSortValues(sort, local.Cursors[i].SortValue, peer.Cursors[j].SortValue) <= 0)
		if takeLocal {
			out, i = append(out, local.Items[i]), i+1
		} else {
			out, j = append(out, peer.Items[j]), j+1
		}
	}
	next := cursor
	if i > 0 {
		next.Local = local.Cursors[i-1].Cursor
	}
	if j > 0 {
		next.Peer = peer.Cursors[j-1].Cursor
	}
	next.LocalDone = i == len(local.Items) && local.Next == ""
	next.PeerDone = j == len(peer.Items) && peer.Next == ""
	return out, next, !next.LocalDone || !next.PeerDone
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
