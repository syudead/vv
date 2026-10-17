package domain

import (
	"errors"
	"strings"
	"unicode"
)

// ドラッグの操作とその取り消し。
// 操作は 1 つの取引で行い、変えた行の前後の像を返す。取り消しは、今の行が後の像と等しいときだけ
// 前の像を書き戻す。

// ErrUndoConflict は、操作の後で行が変わったので取り消せないことを表す。取り消しは何も書かない。
// API では 409 undo_conflict である。
var ErrUndoConflict = errors.New("undo conflict")

// ErrUndoExpired は、取り消しのトークンが無い、使い済み、または 10 分より古いことを表す。API では 410 undo_expired である。
var ErrUndoExpired = errors.New("undo expired")

// OrganizeResult はドラッグの操作 1 つの結果である。UndoToken はその操作を取り消すトークンで、
// GroupID は「グループにする」が作ったグループの id（それ以外の操作では 0）である。
type OrganizeResult struct {
	UndoToken string
	GroupID   int64
}

// GroupNameFromTitle は「グループにする」が作るグループの名前を、落とした先の動画の題名 title から
// 決める。先頭の Unicode White_Space を除き、先頭 GroupNameMaxLength 符号位置に切ってから
// ValidateGroupName で整える。先に空白を除くのは、
// 長い空白で始まる題名が空白だけに切られて空の名前にならないためである。
func GroupNameFromTitle(title string) (string, error) {
	trimmed := strings.TrimLeftFunc(title, func(r rune) bool {
		return unicode.Is(unicode.White_Space, r)
	})
	return ValidateGroupName(cutGroupName(trimmed))
}

// OrganizeRefKind はドラッグしたカードの種類である。
type OrganizeRefKind string

const (
	// OrganizeVideo は動画のカードで、OrganizeRef.ID は動画の id である。
	OrganizeVideo OrganizeRefKind = "video"
	// OrganizeGroup はグループのカードで、OrganizeRef.ID はグループの id である。
	OrganizeGroup OrganizeRefKind = "group"
)

// OrganizeRef はドラッグしたカード 1 枚である。操作は
// カードを画面の順の並びで受け取る。
type OrganizeRef struct {
	Kind OrganizeRefKind
	ID   int64
}

// GroupMergeHolder は、マージで消えるグループ X を項目に持つプレイリストか親グループ 1 つである。
// HoldsTarget は、それがすでにマージ先 Y を項目に持つかである。
type GroupMergeHolder struct {
	ID          int64
	HoldsTarget bool
}

// GroupMergePlan は PlanGroupMerge の結果である。Replace の各プレイリストと親グループでは X の項目を
// その位置のまま Y の項目にし、Remove では X の項目を消す。
type GroupMergePlan struct {
	ReplacePlaylists []int64
	RemovePlaylists  []int64
	ReplaceParents   []int64
	RemoveParents    []int64
}

// PlanGroupMerge は、グループ X を target（Y）へマージするとき、X を持つプレイリスト playlists と
// 親グループ parents の項目をどうするかを決める。どれも X の位置で Y に置き換える。
// そこにすでに Y があるとき、また親グループでは Y がその親自身か祖先になる（今の入れ子 edges で
// ValidateNesting(親, Y) が通らない）ときは、X の項目を消す。
func PlanGroupMerge(target int64, playlists, parents []GroupMergeHolder, edges []GroupEdge) GroupMergePlan {
	var plan GroupMergePlan
	for _, playlist := range playlists {
		if playlist.HoldsTarget {
			plan.RemovePlaylists = append(plan.RemovePlaylists, playlist.ID)
		} else {
			plan.ReplacePlaylists = append(plan.ReplacePlaylists, playlist.ID)
		}
	}
	for _, parent := range parents {
		if parent.HoldsTarget || ValidateNesting(parent.ID, target, edges) != nil {
			plan.RemoveParents = append(plan.RemoveParents, parent.ID)
		} else {
			plan.ReplaceParents = append(plan.ReplaceParents, parent.ID)
		}
	}
	return plan
}

// OrganizeMove はカードの移動の入力である。From はカードを
// ドラッグして出した開いているグループ、To は入れるグループで、どちらも 0 なら無い。To が無ければ
// From から外すだけ（ライブラリに落とした）である。
type OrganizeMove struct {
	Items []OrganizeRef
	From  int64
	To    int64
}

// MaxOrganizeBundleVideos は「バージョンとしてまとめる」ドロップが 1 つの集まりにできる動画の
// 本数の上限である。既にまとまっている動画はその集まりの全メンバーを数える
const MaxOrganizeBundleVideos = 20

// ErrTooManyVersions は、まとまりを展開した動画が MaxOrganizeBundleVideos を超えることを表す。
// 何も書かない。API では 400 too_many_videos である。
var ErrTooManyVersions = errors.New("too many videos to bundle")
