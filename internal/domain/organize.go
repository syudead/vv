package domain

import (
	"errors"
	"strings"
	"unicode"
)

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

// GroupNameFromTitle は「グループにする」が作るグループの名前を、落とした先の動画の題名 title から
// 決める。先頭の Unicode White_Space を除き、先頭 GroupNameMaxLength 符号位置に切ってから
// ValidateGroupName で整える（specs/050-drag-organize/data-model.md「Rules」）。先に空白を除くのは、
// 長い空白で始まる題名が空白だけに切られて空の名前にならないためである。
func GroupNameFromTitle(title string) (string, error) {
	trimmed := strings.TrimLeftFunc(title, func(r rune) bool {
		return unicode.Is(unicode.White_Space, r)
	})
	return ValidateGroupName(cutGroupName(trimmed))
}
