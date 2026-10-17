package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
)

// MaxHomeShelves limits the smart-folder shelves on Home.
const MaxHomeShelves = 50

// ErrInvalidHomeShelves は棚の並びの書き込みが規則に合わないこと（MaxHomeShelves を超える、同じ
// フォルダを 2 度名指す）を表す。API では 400 invalid_request である（contracts/screen-api.md）。
var ErrInvalidHomeShelves = errors.New("invalid home shelves")

// ValidateHomeShelfOrder は棚の並び ids（ホームに置くフォルダを上から順に）を確かめる。
// MaxHomeShelves を超えるか、同じ id か正でない id があれば ErrInvalidHomeShelves を包む誤りである。
func ValidateHomeShelfOrder(ids []int64) error {
	if len(ids) > MaxHomeShelves {
		return fmt.Errorf("%w: %d shelves, more than %d", ErrInvalidHomeShelves, len(ids), MaxHomeShelves)
	}
	seen := make(map[int64]bool, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return fmt.Errorf("%w: a non-positive folder id", ErrInvalidHomeShelves)
		}
		if seen[id] {
			return fmt.Errorf("%w: folder %d twice", ErrInvalidHomeShelves, id)
		}
		seen[id] = true
	}
	return nil
}

// SameSmartRule は 2 つの条件が同じ絞り込みかを返す（research.md R-8 の「同じ規則」）。タグは
// 順によらず同じ集合なら同じである。
func SameSmartRule(a, b SmartRule) bool {
	a.Tags, b.Tags = sortedTags(a.Tags), sortedTags(b.Tags)
	left, errA := json.Marshal(a)
	right, errB := json.Marshal(b)
	return errA == nil && errB == nil && string(left) == string(right)
}

// sortedTags は tags を小さい順にした写しを返す。nil は nil のままにする。
func sortedTags(tags []int64) []int64 {
	if tags == nil {
		return nil
	}
	out := slices.Clone(tags)
	slices.Sort(out)
	return out
}

// HomeChanged はホームの棚の集合か並び（名前を含む）が変わったことを表す（research.md R-16）。
// 棚の中身の変化は SmartFoldersChanged が知らせるので、id は持たない。保存層は取引の確定後に
// だけ発行する。
type HomeChanged struct{}

func (HomeChanged) event() {}

// MovedShelves は、棚の並びを before から after へ変えたとき、所有者が足したか動かした棚の id を
// after の順に返す（research.md R-7）。before にない棚は足した棚で、残った棚のうち並びの順を保つ
// 最も長い列（最長増加部分列）に入らない棚が動かした棚である。ほかの棚が動いて位置がずれただけの棚は
// 返さない。
func MovedShelves(before, after []int64) []int64 {
	index := make(map[int64]int, len(before))
	for i, id := range before {
		index[id] = i
	}
	var stayed []int64
	for _, id := range after {
		if _, ok := index[id]; ok {
			stayed = append(stayed, id)
		}
	}
	kept := longestOrdered(stayed, index)
	var moved []int64
	for _, id := range after {
		if _, ok := index[id]; !ok || !kept[id] {
			moved = append(moved, id)
		}
	}
	return moved
}

// longestOrdered は ids のうち index の順が増える最も長い部分列を集合で返す。
func longestOrdered(ids []int64, index map[int64]int) map[int64]bool {
	length := make([]int, len(ids))
	prev := make([]int, len(ids))
	best := -1
	for i := range ids {
		length[i], prev[i] = 1, -1
		for j := 0; j < i; j++ {
			if index[ids[j]] < index[ids[i]] && length[j]+1 > length[i] {
				length[i], prev[i] = length[j]+1, j
			}
		}
		if best < 0 || length[i] > length[best] {
			best = i
		}
	}
	kept := map[int64]bool{}
	for at := best; at >= 0; at = prev[at] {
		kept[ids[at]] = true
	}
	return kept
}
