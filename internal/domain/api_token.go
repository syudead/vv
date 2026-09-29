package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// APITokenNameMaxLength は API トークンの名前の上限（符号位置数）である。タグ名の上限と
// そろえる（specs/026-external-api/research.md R-10）。
const APITokenNameMaxLength = 100

// APIToken は発行した API トークン 1 件である（specs/026-external-api/data-model.md §3）。
// 平文とハッシュは持たない。平文は発行の応答にだけ現れ、どこにも保存しない。
type APIToken struct {
	ID        int64
	Name      string
	CreatedAt time.Time
	// LastUsedAt は最後に使った時刻である。未使用ならゼロ値。
	LastUsedAt time.Time
}

// ErrInvalidAPITokenName は API トークンの名前として受け付けられない入力を表す。API では
// invalid_request（400）になる。
//
// NormalizeAPITokenName はこれを直接は返さず、具体的な理由を運ぶ *InvalidAPITokenNameError を
// 返す。errors.Is(err, ErrInvalidAPITokenName) は真のままで、errors.As で取り出した Problem から
// internal/httpapi が API の reason を決める（タグ名の InvalidTagNameError と同じ形）。
var ErrInvalidAPITokenName = errors.New("invalid API token name")

// APITokenNameProblem は API トークンの名前の規則違反の種類である。
type APITokenNameProblem int

const (
	// APITokenNameEmpty は前後の空白を取り除くと空になることを表す。
	APITokenNameEmpty APITokenNameProblem = iota + 1
	// APITokenNameControlCharacters は制御文字を含むことを表す。
	APITokenNameControlCharacters
	// APITokenNameTooLong は APITokenNameMaxLength 符号位置を超えることを表す。
	APITokenNameTooLong
)

// InvalidAPITokenNameError は API トークンの名前の規則違反の具体的な理由を運ぶ。
type InvalidAPITokenNameError struct {
	Problem APITokenNameProblem
}

func (e *InvalidAPITokenNameError) Error() string {
	switch e.Problem {
	case APITokenNameEmpty:
		return "API token name is empty"
	case APITokenNameControlCharacters:
		return "API token name contains control characters"
	case APITokenNameTooLong:
		return fmt.Sprintf("API token name must be at most %d characters", APITokenNameMaxLength)
	default:
		return ErrInvalidAPITokenName.Error()
	}
}

func (e *InvalidAPITokenNameError) Unwrap() error { return ErrInvalidAPITokenName }

// NormalizeAPITokenName は受け取った入力を API トークンの名前に整える（research.md R-10）。
// 規則と手順の順番は NormalizeTagName と同じである。制御文字（Cc）を前後の空白の除去より
// 先に調べ、前後の Unicode White_Space を除き、空と APITokenNameMaxLength 符号位置の超過を
// 拒む。Unicode の正規化はしない。同じ名前は重複してよい。
func NormalizeAPITokenName(input string) (string, error) {
	for _, r := range input {
		if unicode.Is(unicode.Cc, r) {
			return "", &InvalidAPITokenNameError{Problem: APITokenNameControlCharacters}
		}
	}
	trimmed := strings.TrimFunc(input, func(r rune) bool {
		return unicode.Is(unicode.White_Space, r)
	})
	if trimmed == "" {
		return "", &InvalidAPITokenNameError{Problem: APITokenNameEmpty}
	}
	if utf8.RuneCountInString(trimmed) > APITokenNameMaxLength {
		return "", &InvalidAPITokenNameError{Problem: APITokenNameTooLong}
	}
	return trimmed, nil
}
