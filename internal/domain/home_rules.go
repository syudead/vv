package domain

import (
	"cmp"
	"slices"
)

// 規則の整理役（specs/057-home/research.md R-1、R-2、R-10）。見せる動画に付いたタグ・結び付いた
// 人物と団体の本数（ShelfCandidate）から棚の案（ProposedShelf）を選ぶ。本数は保存層が数え、
// どれを棚にするか、どの順に並べるかはここが決める。

const (
	// RuleShelfMinVideos は規則の整理役が棚にするタグ・人物・団体の、見せる動画の本数の下限である
	// （R-2）。これより少ないものは棚にしない。
	RuleShelfMinVideos = 5
	// RuleShelfMaxShelves は規則の整理役が 1 回に出す棚の案の数の上限である（R-2）。
	RuleShelfMaxShelves = 24
	// HomeExtendMinUnshelved は extend を走らせる、所有者に見えて棚にない動画の本数の下限である（R-10）。
	HomeExtendMinUnshelved = 20
)

// ShelfSourceKind は棚の案の元の種類である。
type ShelfSourceKind string

const (
	ShelfSourceTag          ShelfSourceKind = "tag"
	ShelfSourcePerson       ShelfSourceKind = "person"
	ShelfSourceOrganization ShelfSourceKind = "organization"
)

// ShelfCandidate は棚になりうるタグ・人物・団体 1 つと、それに当たる見せる動画の本数である。
type ShelfCandidate struct {
	Kind ShelfSourceKind
	ID   int64
	// Name はタグ・人物・団体の表示の名前で、棚の名前になる。
	Name string
	// TagKindID はタグの種類の id で、種類の無いタグと人物・団体では 0 である。
	TagKindID int64
	// Videos はこの元に当たる見せる動画の本数である。
	Videos int
}

// ShelfCandidates は案を作る元の動画の集まりと、そこから数えた候補である。
type ShelfCandidates struct {
	// Videos は数えた動画（rebuild なら見せる動画すべて、extend なら棚にない動画）の本数である。
	Videos int
	// Candidates は少なくとも 1 本の動画に当たる候補である。順は決めない。
	Candidates []ShelfCandidate
}

// Rule は候補の棚の条件を返す（R-2）。
func (c ShelfCandidate) Rule() SmartRule {
	switch c.Kind {
	case ShelfSourcePerson:
		return SmartRule{Person: c.ID}
	case ShelfSourceOrganization:
		return SmartRule{Organization: c.ID}
	default:
		return SmartRule{Tags: []int64{c.ID}}
	}
}

// rank は候補の並びの段で、種類付きのタグ、種類なしのタグ、人物、団体の順に小さい。
func (c ShelfCandidate) rank() int {
	switch c.Kind {
	case ShelfSourceTag:
		if c.TagKindID != 0 {
			return 0
		}
		return 1
	case ShelfSourcePerson:
		return 2
	default:
		return 3
	}
}

// compareShelfCandidates は候補の並び（R-2）を決める。段の順、種類付きのタグは種類の id の順、
// それぞれ本数の多い順で、同じなら id の順である。
func compareShelfCandidates(a, b ShelfCandidate) int {
	return cmp.Or(
		cmp.Compare(a.rank(), b.rank()),
		cmp.Compare(a.TagKindID, b.TagKindID),
		cmp.Compare(b.Videos, a.Videos),
		cmp.Compare(a.ID, b.ID),
	)
}

// OrganizeByRules は規則の整理役の案を返す（R-2）。RuleShelfMinVideos 本以上の候補ごとにその名前と
// 条件の棚を 1 つ、compareShelfCandidates の順に、RuleShelfMaxShelves まで出す。名前か条件が
// ProposedShelf.Validate に通らない候補は数えずに飛ばす。
func OrganizeByRules(candidates []ShelfCandidate) []ProposedShelf {
	sorted := slices.Clone(candidates)
	slices.SortFunc(sorted, compareShelfCandidates)
	proposal := make([]ProposedShelf, 0, min(len(sorted), RuleShelfMaxShelves))
	for _, candidate := range sorted {
		if len(proposal) == RuleShelfMaxShelves {
			break
		}
		if candidate.Videos < RuleShelfMinVideos {
			continue
		}
		shelf, err := ProposedShelf{Name: candidate.Name, Rule: candidate.Rule()}.Validate()
		if err != nil {
			continue
		}
		proposal = append(proposal, shelf)
	}
	return proposal
}

// MergeHomeOrganizeRun は待っている実行 pending に run の依頼を重ねた待ちの実行を返す（R-9）。
// rebuild は extend を吸収する。pending が空なら run である。
func MergeHomeOrganizeRun(pending, run HomeOrganizeRun) HomeOrganizeRun {
	if pending == HomeRebuild || run == HomeRebuild {
		return HomeRebuild
	}
	if pending == HomeExtend || run == HomeExtend {
		return HomeExtend
	}
	return ""
}

// HomeOrganizerState は整理役の今の状態である（data-model.md「Organizer state」）。
type HomeOrganizerState struct {
	// Running は実行が今走っているかで、メモリにだけ持つ。
	Running bool
	// Pending は待っている実行で、無ければ空である。
	Pending HomeOrganizeRun
	// Last は最後に終わった実行で、無ければ nil である。
	Last *HomeOrganizeLast
}
