package domain

import "errors"

// ドラッグの操作とその取り消し（specs/050-drag-organize/data-model.md「Operations」「Undo image」）。
// 操作は 1 つの取引で行い、変えた行の前後の像を返す。取り消しは、今の行が後の像と等しいときだけ
// 前の像を書き戻す（research.md R-1）。

// ErrUndoConflict は、操作の後で行が変わったので取り消せないことを表す。取り消しは何も書かない。
// API では 409 undo_conflict である。
var ErrUndoConflict = errors.New("undo conflict")

// ErrUndoExpired は、取り消しのトークンが無い、使い済み、または 10 分より古いことを表す
// （research.md R-2）。API では 410 undo_expired である。
var ErrUndoExpired = errors.New("undo expired")

// OrganizeResult はドラッグの操作 1 つの結果である。UndoToken はその操作を取り消すトークンで、
// GroupID は「グループにする」が作ったグループの id（それ以外の操作では 0）である。
type OrganizeResult struct {
	UndoToken string
	GroupID   int64
}

// OrganizeRefKind はドラッグしたカードの種類である。
type OrganizeRefKind string

const (
	// OrganizeVideo は動画のカードで、OrganizeRef.ID は動画の id である。
	OrganizeVideo OrganizeRefKind = "video"
	// OrganizeGroup はグループのカードで、OrganizeRef.ID はグループの id である。
	OrganizeGroup OrganizeRefKind = "group"
)

// OrganizeRef はドラッグしたカード 1 枚である（specs/050-drag-organize/research.md R-4）。操作は
// カードを画面の順の並びで受け取る。
type OrganizeRef struct {
	Kind OrganizeRefKind
	ID   int64
}

// OrganizeMove はカードの移動の入力である（data-model.md「Operations」の Move）。From はカードを
// ドラッグして出した開いているグループ、To は入れるグループで、どちらも 0 なら無い。To が無ければ
// From から外すだけ（ライブラリに落とした）である。
type OrganizeMove struct {
	Items []OrganizeRef
	From  int64
	To    int64
}
