package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
)

// ホームの棚（specs/057-home/data-model.md、research.md R-7、R-8）。棚はホームの位置
// （smart_folders.home_position）を持つスマートフォルダで、整理役（organizer）が返す棚の案
// （ProposedShelf）は保存せず、適用の取引（store の HomeStore）がフォルダにする。

// MaxHomeShelves はホームに並べるフォルダの棚の数の上限である（data-model.md「Rules」）。整理役も
// オンボードもこれを超えて棚を足さない。
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

// ProposedShelf は整理役が返す棚の案 1 つである（data-model.md「Proposed shelf」）。
type ProposedShelf struct {
	// Name は 052 のフォルダ名の規則（ValidateGroupName）に従う。
	Name string
	// Rule は Tags（1 件）・Person・Organization のちょうど 1 つを持つ条件である（research.md R-2）。
	Rule SmartRule
}

// Validate は案を確かめ、名前を整えた案を返す。名前が規則に合わなければ *InvalidGroupNameError、
// 条件が SmartRule.Validate に通らないか、Tags 1 件・Person・Organization のちょうど 1 つでなければ
// ErrInvalidSmartRule を包む誤りである。
func (p ProposedShelf) Validate() (ProposedShelf, error) {
	name, err := ValidateGroupName(p.Name)
	if err != nil {
		return ProposedShelf{}, err
	}
	if err := p.Rule.Validate(); err != nil {
		return ProposedShelf{}, err
	}
	if !isShelfRule(p.Rule) {
		return ProposedShelf{}, fmt.Errorf("%w: a proposed shelf names exactly one tag, person or organization", ErrInvalidSmartRule)
	}
	p.Name = name
	return p, nil
}

// isShelfRule は条件が Tags 1 件・Person・Organization のちょうど 1 つだけを持つかを返す。
func isShelfRule(rule SmartRule) bool {
	only := SmartRule{Tags: rule.Tags, Person: rule.Person, Organization: rule.Organization}
	if !SameSmartRule(rule, only) {
		return false
	}
	set := 0
	if len(rule.Tags) == 1 {
		set++
	} else if len(rule.Tags) > 1 {
		return false
	}
	if rule.Person != 0 {
		set++
	}
	if rule.Organization != 0 {
		set++
	}
	return set == 1
}

// Folder は案を整理役のフォルダの作成の入力にする（data-model.md「Rules」）。整理役のフォルダは
// フォルダの並び SortAddedDesc の根のフォルダである。
func (p ProposedShelf) Folder() SmartFolderCreate {
	return SmartFolderCreate{Name: p.Name, Rule: p.Rule, Sort: SortAddedDesc}
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

// HomeOrganizeRun は整理役の実行の種類である（data-model.md「Settings」）。
type HomeOrganizeRun string

const (
	// HomeRebuild は整理役のフォルダを組み直す（research.md R-8）。
	HomeRebuild HomeOrganizeRun = "rebuild"
	// HomeExtend は今の棚のあとに棚を足すだけをする。
	HomeExtend HomeOrganizeRun = "extend"
)

// Valid は既知の実行の種類かを返す。
func (r HomeOrganizeRun) Valid() bool {
	return r == HomeRebuild || r == HomeExtend
}

// HomeOrganizeOutcome は終わった実行の結果である。
type HomeOrganizeOutcome string

const (
	HomeOrganizeDone   HomeOrganizeOutcome = "done"
	HomeOrganizeFailed HomeOrganizeOutcome = "failed"
	// HomeOrganizeUnchanged は案が何も変えなかったことを表す。
	HomeOrganizeUnchanged HomeOrganizeOutcome = "unchanged"
)

// Valid は既知の結果かを返す。
func (o HomeOrganizeOutcome) Valid() bool {
	switch o {
	case HomeOrganizeDone, HomeOrganizeFailed, HomeOrganizeUnchanged:
		return true
	}
	return false
}

// HomeOrganizeLast は最後に終わった実行で、settings の home.organize.last に JSON で置く
// （data-model.md「Settings」）。
type HomeOrganizeLast struct {
	Run     HomeOrganizeRun     `json:"run"`
	Outcome HomeOrganizeOutcome `json:"outcome"`
	// FinishedAt は Unix ミリ秒である。
	FinishedAt int64 `json:"finishedAt"`
	// Added と Removed は足した棚と消した棚の数である。
	Added   int `json:"added"`
	Removed int `json:"removed"`
}

// Validate は種類と結果が既知の値かを確かめる。
func (l HomeOrganizeLast) Validate() error {
	if !l.Run.Valid() || !l.Outcome.Valid() || l.Added < 0 || l.Removed < 0 {
		return fmt.Errorf("invalid home organize run %+v", l)
	}
	return nil
}

// HomeApplied は案の適用の結果である。
type HomeApplied struct {
	// Added は挿入した棚、Removed は消したか、所有者の子孫があるので消さずにホームから外した整理役の
	// フォルダの数である。
	Added   int
	Removed int
	// Changed は棚の集合・並び・名前のどれかが変わったかである。偽なら結果は HomeOrganizeUnchanged である。
	Changed bool
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
