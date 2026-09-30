package domain

import "fmt"

// 外部連携 API の名前でのタグの一括操作（specs/026-external-api/data-model.md §2、
// contracts/external-api.md §4、research.md R-7）。

// ExternalVideoTagsMaxNames は 1 回のタグの一括操作が受け付けるタグ名の最大件数である。
const ExternalVideoTagsMaxNames = 100

// VideoTagsAction はタグの一括操作の種類である。どれも手で付けたタグ（video_tags の行）
// だけを書き換え、祖先のフォルダ名から付くタグは変えない。
type VideoTagsAction string

const (
	// VideoTagsAdd は名前のタグを付ける。無い名前はタグを作る。
	VideoTagsAdd VideoTagsAction = "add"
	// VideoTagsRemove は名前のタグを外す。どのタグにも当たらない名前は何もしない。
	VideoTagsRemove VideoTagsAction = "remove"
	// VideoTagsReplace は手で付けたタグをちょうど名前のタグの集合にする。無い名前は
	// タグを作り、空の集合は手で付けたタグをすべて外す。
	VideoTagsReplace VideoTagsAction = "replace"
)

// Valid は a が既知の操作であることを返す。
func (a VideoTagsAction) Valid() bool {
	switch a {
	case VideoTagsAdd, VideoTagsRemove, VideoTagsReplace:
		return true
	}
	return false
}

// VideoTagsOutcome は一括操作の結果である（specs/031-tentative-tags/data-model.md §4）。
type VideoTagsOutcome struct {
	// Items は各動画の操作後のタグで、指定した動画の順。
	Items []VideoTagsResult
	// SkippedNames は仮の作成で却下した名前に当たって飛ばした名前（整えた形、指定の順、
	// 重複なし）。無ければ空の配列。
	SkippedNames []string
}

// VideoTagsResult は一括操作の後の動画 1 本のタグである。
type VideoTagsResult struct {
	VideoID    int64
	ContentKey string
	// Tags は操作後のタグ（手で付けた分とフォルダ名から付く分、出所つき）。
	Tags []VideoTag
}

// VideoRefNotFoundError は一括操作の動画の指定のうち、今ライブラリにある動画へ引けなかった
// ものの位置（0 から）を運ぶ。errors.Is(err, ErrNotFound) は真になる。
type VideoRefNotFoundError struct {
	Index int
}

func (e *VideoRefNotFoundError) Error() string {
	return fmt.Sprintf("video %d is not in the library", e.Index)
}

func (e *VideoRefNotFoundError) Unwrap() error { return ErrNotFound }

// TagNameAtError は一括操作のタグ名のうち、規則に合わなかったものの位置（0 から）と
// 理由（*InvalidTagNameError）を運ぶ。errors.Is(err, ErrInvalidTagName) は真になり、
// errors.As で *InvalidTagNameError も取り出せる。
type TagNameAtError struct {
	Index int
	Err   error
}

func (e *TagNameAtError) Error() string {
	return fmt.Sprintf("tag name %d: %v", e.Index, e.Err)
}

func (e *TagNameAtError) Unwrap() error { return e.Err }
