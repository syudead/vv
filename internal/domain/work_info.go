package domain

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// 作品情報と人物・団体が共有する値と規則（specs/047-video-work-info/data-model.md「Fields」・
// 「Values added to domain」）。動画ごとの作品情報の項目と、人物・団体の名前・別名・外部 ID は
// 同じ誤りの型 InvalidWorkInfoError で拒み、Field に失敗したプロパティの経路を載せる。

const (
	// ExternalIDsMaxCount は外部 ID の一覧の件数の上限である（動画も実体も同じ）。
	ExternalIDsMaxCount = 20
	// ExternalIDSourceMaxLength は外部 ID の source の上限（符号位置数）である。
	ExternalIDSourceMaxLength = 50
	// ExternalIDValueMaxLength は外部 ID の value の上限（符号位置数）である。
	ExternalIDValueMaxLength = 200
)

// ErrInvalidWorkInfo は作品情報か人物・団体の項目として受け付けられない入力を表す。API では
// invalid_request（400）になる。正規化の関数はこれを直接は返さず、*InvalidWorkInfoError を返す。
var ErrInvalidWorkInfo = errors.New("invalid work information")

// WorkInfoProblem は項目の規則違反の種類である。internal/httpapi が API の reason
// （work_info_*）に写す。
type WorkInfoProblem int

const (
	// WorkInfoControlCharacters は制御文字を含むことを表す（description の改行は除く）。
	WorkInfoControlCharacters WorkInfoProblem = iota + 1
	// WorkInfoTooLong は文字列が上限の符号位置数を超えることを表す。
	WorkInfoTooLong
	// WorkInfoTooMany は一覧が上限の件数を超えることを表す。
	WorkInfoTooMany
	// WorkInfoInvalidDate は日付が YYYY・YYYY-MM・暦にある YYYY-MM-DD のどれでもないことを表す。
	WorkInfoInvalidDate
	// WorkInfoInvalidURL は URL が host を持つ絶対の http・https の URL でないことを表す。
	WorkInfoInvalidURL
	// WorkInfoInvalidLanguage は別名の language が BCP 47 の構文に合わないことを表す。
	WorkInfoInvalidLanguage
	// WorkInfoDuplicate は一覧の中で同じ値が重なることを表す。
	WorkInfoDuplicate
	// WorkInfoEmpty は空にできない値が前後の空白を除くと空になることを表す（実体の名前と、
	// 片方だけ空の外部 ID）。
	WorkInfoEmpty
)

// InvalidWorkInfoError は項目の規則違反の具体的な理由を運ぶ。Field は JSON のプロパティ名を
// . でつなぎ、一覧の位置を [] で書いた経路（name・aliases[2].language・externalIds[0].source）で、
// 一覧の位置は呼び手が送った一覧の位置である。Limit は TooLong と TooMany の上限で、それ以外は 0。
type InvalidWorkInfoError struct {
	Field   string
	Problem WorkInfoProblem
	Limit   int
}

func (e *InvalidWorkInfoError) Error() string {
	switch e.Problem {
	case WorkInfoControlCharacters:
		return fmt.Sprintf("%s contains control characters", e.Field)
	case WorkInfoTooLong:
		return fmt.Sprintf("%s must be at most %d characters", e.Field, e.Limit)
	case WorkInfoTooMany:
		return fmt.Sprintf("%s must have at most %d entries", e.Field, e.Limit)
	case WorkInfoInvalidDate:
		return fmt.Sprintf("%s is not a valid date", e.Field)
	case WorkInfoInvalidURL:
		return fmt.Sprintf("%s is not a valid URL", e.Field)
	case WorkInfoInvalidLanguage:
		return fmt.Sprintf("%s is not a valid language tag", e.Field)
	case WorkInfoDuplicate:
		return fmt.Sprintf("%s is a duplicate", e.Field)
	case WorkInfoEmpty:
		return fmt.Sprintf("%s is empty", e.Field)
	default:
		return ErrInvalidWorkInfo.Error()
	}
}

func (e *InvalidWorkInfoError) Unwrap() error { return ErrInvalidWorkInfo }

func invalidWorkInfo(field string, problem WorkInfoProblem, limit int) error {
	return &InvalidWorkInfoError{Field: field, Problem: problem, Limit: limit}
}

// ExternalID は外部の情報源での識別子 1 件である（動画にも実体にも付く）。
type ExternalID struct {
	Source string
	Value  string
}

// CreditRef は人物か団体 1 件への参照である。読み出しでは今の名前を運び、書き込みでは ID だけを使う。
type CreditRef struct {
	ID   int64
	Name string
}

// normalizeWorkInfoText は表示名と同じ手順で文字列を整える（029 R-10）。どこかに制御文字が
// あれば誤り（allowLineFeed なら改行だけは許す）、前後の Unicode White_Space を除き、
// maxLength 符号位置を超えたら誤り。空になった結果は誤りにせず空の文字列で返し、
// 解除にするか落とすか誤りにするかは呼び手が決める。
func normalizeWorkInfoText(field, input string, maxLength int, allowLineFeed bool) (string, error) {
	for _, r := range input {
		if unicode.Is(unicode.Cc, r) && (!allowLineFeed || r != '\n') {
			return "", invalidWorkInfo(field, WorkInfoControlCharacters, 0)
		}
	}
	trimmed := strings.TrimFunc(input, func(r rune) bool {
		return unicode.Is(unicode.White_Space, r)
	})
	if utf8.RuneCountInString(trimmed) > maxLength {
		return "", invalidWorkInfo(field, WorkInfoTooLong, maxLength)
	}
	return trimmed, nil
}

// normalizeExternalID は外部 ID 1 件を整える。両方が空になれば keep が偽（その件を落とす）、
// 片方だけ空になれば空の側の WorkInfoEmpty である（data-model.md「Fields」）。
func normalizeExternalID(field string, id ExternalID) (ExternalID, bool, error) {
	source, err := normalizeWorkInfoText(field+".source", id.Source, ExternalIDSourceMaxLength, false)
	if err != nil {
		return ExternalID{}, false, err
	}
	value, err := normalizeWorkInfoText(field+".value", id.Value, ExternalIDValueMaxLength, false)
	if err != nil {
		return ExternalID{}, false, err
	}
	switch {
	case source == "" && value == "":
		return ExternalID{}, false, nil
	case source == "":
		return ExternalID{}, false, invalidWorkInfo(field+".source", WorkInfoEmpty, 0)
	case value == "":
		return ExternalID{}, false, invalidWorkInfo(field+".value", WorkInfoEmpty, 0)
	}
	return ExternalID{Source: source, Value: value}, true, nil
}

// normalizeExternalIDs は外部 ID の一覧を整える。field は一覧の経路（externalIds）。各件を
// 送られた順に確かめ、同じ (source, value) の 2 件目を WorkInfoDuplicate、落とした後の件数が
// ExternalIDsMaxCount を超えれば一覧の WorkInfoTooMany にする。空の一覧は空の配列で返す。
func normalizeExternalIDs(field string, ids []ExternalID) ([]ExternalID, error) {
	normalized := make([]ExternalID, 0, len(ids))
	seen := make(map[ExternalID]bool, len(ids))
	for i, raw := range ids {
		entryField := fmt.Sprintf("%s[%d]", field, i)
		id, keep, err := normalizeExternalID(entryField, raw)
		if err != nil {
			return nil, err
		}
		if !keep {
			continue
		}
		if seen[id] {
			return nil, invalidWorkInfo(entryField, WorkInfoDuplicate, 0)
		}
		seen[id] = true
		normalized = append(normalized, id)
	}
	if len(normalized) > ExternalIDsMaxCount {
		return nil, invalidWorkInfo(field, WorkInfoTooMany, ExternalIDsMaxCount)
	}
	return normalized, nil
}

// ExternalIDIndex は、整える前の一覧 raw のうち、整えると id になる最初の件の位置を返す。無ければ -1。
// 整えた一覧の 1 件について誤り（他の実体が持つ外部 ID など）を返すとき、Field に呼び手が送った
// 一覧の位置を載せるために使う。
func ExternalIDIndex(raw []ExternalID, id ExternalID) int {
	for i, entry := range raw {
		normalized, keep, err := normalizeExternalID("", entry)
		if err == nil && keep && normalized == id {
			return i
		}
	}
	return -1
}
