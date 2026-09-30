package domain

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// 動画の表示名と代表サムネイルの位置の上書き（specs/029-video-overrides/data-model.md §2、
// research.md R-1・R-2・R-10）。上書きは content_key に結ぶ利用者データで、保存層が
// 読み出しのたびに Video.Title・FileTitle・DisplayName・ThumbnailPositionMs へ写す。

// DisplayNameMaxLength は表示名の上限（符号位置数）である。置き換える対象のファイル名
// （255 バイト）をカードと動画ページが今収めているので、それを下回らない値にする（R-10）。
const DisplayNameMaxLength = 200

// ErrInvalidDisplayName は表示名として受け付けられない入力を表す。API では
// invalid_request（400）になる。
//
// NormalizeDisplayName はこれを直接は返さず、具体的な理由を運ぶ *InvalidDisplayNameError を
// 返す。errors.Is(err, ErrInvalidDisplayName) は真のままになり、errors.As で取り出した
// Problem から internal/httpapi が API の reason を決める（InvalidTagNameError と同じ形）。
var ErrInvalidDisplayName = errors.New("invalid display name")

// DisplayNameProblem は表示名の規則違反の種類である。
type DisplayNameProblem int

const (
	// DisplayNameControlCharacters は制御文字を含むことを表す。
	DisplayNameControlCharacters DisplayNameProblem = iota + 1
	// DisplayNameTooLong は DisplayNameMaxLength 符号位置を超えることを表す。
	DisplayNameTooLong
)

// InvalidDisplayNameError は表示名の規則違反の具体的な理由を運ぶ。
type InvalidDisplayNameError struct {
	Problem DisplayNameProblem
}

func (e *InvalidDisplayNameError) Error() string {
	switch e.Problem {
	case DisplayNameControlCharacters:
		return "display name contains control characters"
	case DisplayNameTooLong:
		return fmt.Sprintf("display name must be at most %d characters", DisplayNameMaxLength)
	default:
		return ErrInvalidDisplayName.Error()
	}
}

func (e *InvalidDisplayNameError) Unwrap() error { return ErrInvalidDisplayName }

// NormalizeDisplayName は受け取った入力を表示名に整える（R-10）。手順は
// NormalizeTagName と同じ順で、空は誤りではなく解除（clear）である。
//
//  1. 入力のどこかに制御文字（一般カテゴリ Cc。改行・タブ・CR を含む）があれば誤り。
//     前後の空白を取り除く前に調べる（NormalizeTagName と同じ理由）。
//  2. 前後の Unicode White_Space を取り除く。内側の空白は残す。
//  3. 空になったら clear を返す（Edge Case「空文字や空白だけの表示名は解除」）。
//  4. DisplayNameMaxLength 符号位置を超えたら誤り。
func NormalizeDisplayName(input string) (name string, clear bool, err error) {
	for _, r := range input {
		if unicode.Is(unicode.Cc, r) {
			return "", false, &InvalidDisplayNameError{Problem: DisplayNameControlCharacters}
		}
	}
	trimmed := strings.TrimFunc(input, func(r rune) bool {
		return unicode.Is(unicode.White_Space, r)
	})
	if trimmed == "" {
		return "", true, nil
	}
	if utf8.RuneCountInString(trimmed) > DisplayNameMaxLength {
		return "", false, &InvalidDisplayNameError{Problem: DisplayNameTooLong}
	}
	return trimmed, false, nil
}

// DisplayNameChange は外部連携の一括操作の 1 件である。DisplayName が整えて空なら解除する。
type DisplayNameChange struct {
	Video       VideoRef
	DisplayName string
}

// DisplayNameAtError は一括操作の表示名のうち、規則に合わなかったものの位置（0 から）と
// 理由（*InvalidDisplayNameError）を運ぶ。errors.Is(err, ErrInvalidDisplayName) は真になり、
// errors.As で *InvalidDisplayNameError も取り出せる。
type DisplayNameAtError struct {
	Index int
	Err   error
}

func (e *DisplayNameAtError) Error() string {
	return fmt.Sprintf("display name %d: %v", e.Index, e.Err)
}

func (e *DisplayNameAtError) Unwrap() error { return e.Err }

// VideoOverrideChanged は動画の上書き（表示名・代表サムネイルの位置）が変わったことを
// 表す（R-7）。画面への知らせに写し、ワーカーの起床には結ばない。
type VideoOverrideChanged struct {
	VideoID int64
}

func (VideoOverrideChanged) event() {}
