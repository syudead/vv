package domain

import (
	"errors"
	"fmt"
)

// Storage reads group-edit inputs inside the write transaction before applying these rules.

var ErrGroupNotFound = fmt.Errorf("group %w", ErrNotFound)

// ErrGroupTargetNotFound aborts the entire edit and intentionally does not wrap ErrNotFound, preserving the distinct group_target_not_found API error.
var ErrGroupTargetNotFound = errors.New("group target not found")

var ErrGroupCycle = errors.New("group cycle")

// GroupAdd appends videos before groups, preserving request order within each list.
type GroupAdd struct {
	VideoIDs []int64
	GroupIDs []int64
}

type GroupTag struct {
	Tag     TagRef
	Created bool
}

type GroupEdge struct {
	Parent int64
	Child  int64
}

func ValidateNesting(parent, child int64, edges []GroupEdge) error {
	if parent == child {
		return ErrGroupCycle
	}
	children := make(map[int64][]int64, len(edges))
	for _, edge := range edges {
		children[edge.Parent] = append(children[edge.Parent], edge.Child)
	}
	seen := map[int64]bool{child: true}
	stack := []int64{child}
	for len(stack) > 0 {
		id := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, next := range children[id] {
			if next == parent {
				return ErrGroupCycle
			}
			if !seen[next] {
				seen[next] = true
				stack = append(stack, next)
			}
		}
	}
	return nil
}

type GroupPlayItem struct {
	VideoID int64
	ChildID int64
}

// GroupPlayOrder expands children depth-first, retains each video only on first encounter, and skips inaccessible videos and missing children; visited groups prevent cycles.
func GroupPlayOrder(items []GroupPlayItem, children map[int64][]GroupPlayItem) []int64 {
	order := []int64{}
	seenVideos := map[int64]bool{}
	seenGroups := map[int64]bool{}
	var walk func([]GroupPlayItem)
	walk = func(items []GroupPlayItem) {
		for _, item := range items {
			switch {
			case item.ChildID != 0:
				if seenGroups[item.ChildID] {
					continue
				}
				seenGroups[item.ChildID] = true
				walk(children[item.ChildID])
			case item.VideoID != 0 && !seenVideos[item.VideoID]:
				seenVideos[item.VideoID] = true
				order = append(order, item.VideoID)
			}
		}
	}
	walk(items)
	return order
}
