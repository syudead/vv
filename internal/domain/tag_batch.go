package domain

// タグ管理画面のまとめての操作と、その確認に出す数（specs/036-tag-admin-scale/data-model.md §1）。

// MaxTagBatch はまとめての操作と確認の数が 1 回に受け付けるタグの id の上限である。
// 動画の一括操作の videoIds（20000 件）と同じ理由（本文の上限に収まり、json_each に 1 つの引数で渡す）で
// 20000 にする（specs/036-tag-admin-scale/research.md R-4）。
const MaxTagBatch = 20000

// TagBatchAction はまとめての操作の種類である。
type TagBatchAction string

const (
	// TagBatchConfirm は仮のタグを確定する。
	TagBatchConfirm TagBatchAction = "confirm"
	// TagBatchReject は仮のタグを却下する（消して、元の名前を却下した名前に入れる）。
	TagBatchReject TagBatchAction = "reject"
	// TagBatchDelete は確定したタグを消す。
	TagBatchDelete TagBatchAction = "delete"
)

// Valid は a が既知の操作であることを返す。
func (a TagBatchAction) Valid() bool {
	switch a {
	case TagBatchConfirm, TagBatchReject, TagBatchDelete:
		return true
	}
	return false
}

// TagBatchApplies は操作 action が、仮かどうかが tentative のタグに働くかを返す。確定・却下は
// 仮のタグ、削除は確定したタグに働く。画面の 1 行ずつの規則（仮の行に確定・却下、確定した行に
// 削除。specs/031-tentative-tags/ui-design.md「Row」）と同じである。
func TagBatchApplies(action TagBatchAction, tentative bool) bool {
	switch action {
	case TagBatchConfirm, TagBatchReject:
		return tentative
	case TagBatchDelete:
		return !tentative
	}
	return false
}

// TagBatchOutcome はまとめての操作の結果である。どの配列も重複を除いた ids に現れた順で、
// 互いに重ならない。空なら nil でなく空の配列にする。
type TagBatchOutcome struct {
	// AppliedIDs は処理した id。
	AppliedIDs []int64
	// NotFoundIDs はもう無かった id。
	NotFoundIDs []int64
	// NotApplicableIDs は操作が働かない種類だった id（何も変えない）。
	NotApplicableIDs []int64
}

// TagImpactAction は確認をとるまとめての操作の種類である（親 Issue #651 要件 10）。
type TagImpactAction string

const (
	// TagImpactReject はまとめての却下の確認。
	TagImpactReject TagImpactAction = "reject"
	// TagImpactDelete はまとめての削除の確認。
	TagImpactDelete TagImpactAction = "delete"
	// TagImpactMerge は統合の確認。
	TagImpactMerge TagImpactAction = "merge"
)

// Valid は a が既知の操作であることを返す。
func (a TagImpactAction) Valid() bool {
	switch a {
	case TagImpactReject, TagImpactDelete, TagImpactMerge:
		return true
	}
	return false
}

// TagImpactApplies は確認の数に、仮かどうかが tentative のタグを入れるかを返す。却下・削除は
// TagBatchApplies の同じ名前の操作と同じ値で、統合は種類によらず真（統合は仮のタグも確定した
// タグも統合元にとる）。確認が数えるタグと、実際に処理するタグはこれで一致する。
func TagImpactApplies(action TagImpactAction, tentative bool) bool {
	switch action {
	case TagImpactReject:
		return TagBatchApplies(TagBatchReject, tentative)
	case TagImpactDelete:
		return TagBatchApplies(TagBatchDelete, tentative)
	case TagImpactMerge:
		return true
	}
	return false
}

// TagImpact は確認に出す数である。
type TagImpact struct {
	// TagCount は ids のうち今あり、その操作が働くタグの数。
	TagCount int
	// VideoCount はそのどれかが付いた、いまライブラリにある動画の本数（重複なし）。
	VideoCount int
}

// TagMergeOutcome は統合（POST /api/tags/{id}/merge）の結果である。
type TagMergeOutcome struct {
	// Tag は統合後の統合先。統合元がすべて無かったときは変わらない統合先。
	Tag Tag
	// NotFoundIDs はもう無かった統合元の id。重複を除いた sourceIDs に現れた順で、空なら空の配列。
	NotFoundIDs []int64
}
