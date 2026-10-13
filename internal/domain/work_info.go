package domain

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// 動画の作品情報（specs/047-video-work-info/data-model.md「Fields」「Values added to domain」、
// research.md R-1・R-2・R-5）。品番・発売日・メーカー・出演者・説明・出典の 6 項目を、利用者データの
// 鍵（Video.UserKey）に結ぶ利用者データとして保存層が持つ。ここには項目の規則だけを置く。

// WorkInfo は動画の作品情報である。未設定の項目は空文字、出演者が無ければ空の一覧である。
// 項目の並びは API の WorkInfo スキーマと同じで、規則に合わない項目はこの順で最初のものを返す。
type WorkInfo struct {
	Code        string
	ReleaseDate string
	Maker       string
	Performers  []string
	Description string
	SourceURL   string
}

// WorkInfoLimitSet は作品情報の各項目の上限である。長さはどれも符号位置数で数える。
type WorkInfoLimitSet struct {
	CodeMaxLength        int
	MakerMaxLength       int
	PerformerMaxLength   int
	PerformersMaxCount   int
	DescriptionMaxLength int
	SourceURLMaxLength   int
}

// WorkInfoLimits は作品情報の上限（data-model.md「Fields」）。API が誤りの limit に載せる。
var WorkInfoLimits = WorkInfoLimitSet{
	CodeMaxLength:        100,
	MakerMaxLength:       200,
	PerformerMaxLength:   200,
	PerformersMaxCount:   100,
	DescriptionMaxLength: 10_000,
	SourceURLMaxLength:   2_048,
}

// WorkInfoField は誤りが指す項目である。値は API の JSON の項目名である。
type WorkInfoField string

const (
	WorkInfoFieldCode        WorkInfoField = "code"
	WorkInfoFieldReleaseDate WorkInfoField = "releaseDate"
	WorkInfoFieldMaker       WorkInfoField = "maker"
	WorkInfoFieldPerformers  WorkInfoField = "performers"
	WorkInfoFieldDescription WorkInfoField = "description"
	WorkInfoFieldSourceURL   WorkInfoField = "sourceUrl"
)

// ErrInvalidWorkInfo は作品情報として受け付けられない入力を表す。API では invalid_request（400）に
// なる。NormalizeWorkInfo はこれを直接は返さず、項目と理由を運ぶ *InvalidWorkInfoError を返す
// （InvalidDisplayNameError と同じ形）。
var ErrInvalidWorkInfo = errors.New("invalid work information")

// WorkInfoProblem は作品情報の規則違反の種類である。
type WorkInfoProblem int

const (
	// WorkInfoControlCharacters は制御文字を含むことを表す（description の改行は除く）。
	WorkInfoControlCharacters WorkInfoProblem = iota + 1
	// WorkInfoTooLong は項目の長さの上限を超えることを表す。
	WorkInfoTooLong
	// WorkInfoInvalidDate は releaseDate が年 1〜9999 の暦日の YYYY-MM-DD でないことを表す。
	WorkInfoInvalidDate
	// WorkInfoInvalidURL は sourceUrl がホストを持つ絶対の http・https の URL でないことを表す。
	WorkInfoInvalidURL
	// WorkInfoTooMany は出演者の数が上限を超えることを表す。
	WorkInfoTooMany
)

// InvalidWorkInfoError は作品情報の規則違反の項目と理由を運ぶ。Limit は WorkInfoTooLong と
// WorkInfoTooMany のときだけ、超えた上限を持つ（それ以外は 0）。
type InvalidWorkInfoError struct {
	Field   WorkInfoField
	Problem WorkInfoProblem
	Limit   int
}

func (e *InvalidWorkInfoError) Error() string {
	switch e.Problem {
	case WorkInfoControlCharacters:
		return fmt.Sprintf("%s contains control characters", e.Field)
	case WorkInfoTooLong:
		return fmt.Sprintf("%s must be at most %d characters", e.Field, e.Limit)
	case WorkInfoInvalidDate:
		return fmt.Sprintf("%s must be a calendar date in YYYY-MM-DD", e.Field)
	case WorkInfoInvalidURL:
		return fmt.Sprintf("%s must be an absolute http or https URL", e.Field)
	case WorkInfoTooMany:
		return fmt.Sprintf("%s must have at most %d entries", e.Field, e.Limit)
	default:
		return ErrInvalidWorkInfo.Error()
	}
}

func (e *InvalidWorkInfoError) Unwrap() error { return ErrInvalidWorkInfo }

// NormalizeWorkInfo は入力を保存する作品情報に整える（data-model.md「Fields」）。empty は
// 整えた結果のすべての項目が空で、出演者も無いことを表す。
//
// 文字列の各項目は表示名と同じ手順を通る（specs/029-video-overrides/research.md R-10）。
// 制御文字がどこかにあれば誤り（description の改行だけは許す）、前後の空白を取り除き、
// 空になった項目は解除し、長さの上限を超えれば誤り。そのあと releaseDate は暦日、
// sourceUrl は URL として確かめる。出演者は名前ごとに同じ手順を通し、空になった名前を除き、
// 同じ綴りの名前は最初の 1 つを残し、残った数が上限を超えれば誤り。誤りは WorkInfo の
// 項目の順で最初のものを返す。
func NormalizeWorkInfo(input WorkInfo) (WorkInfo, bool, error) {
	var out WorkInfo
	var err error
	limits := WorkInfoLimits
	if out.Code, err = normalizeWorkInfoText(WorkInfoFieldCode, input.Code, limits.CodeMaxLength, false); err != nil {
		return WorkInfo{}, false, err
	}
	if out.ReleaseDate, err = normalizeReleaseDate(input.ReleaseDate); err != nil {
		return WorkInfo{}, false, err
	}
	if out.Maker, err = normalizeWorkInfoText(WorkInfoFieldMaker, input.Maker, limits.MakerMaxLength, false); err != nil {
		return WorkInfo{}, false, err
	}
	if out.Performers, err = normalizePerformers(input.Performers); err != nil {
		return WorkInfo{}, false, err
	}
	if out.Description, err = normalizeWorkInfoText(
		WorkInfoFieldDescription, input.Description, limits.DescriptionMaxLength, true,
	); err != nil {
		return WorkInfo{}, false, err
	}
	if out.SourceURL, err = normalizeSourceURL(input.SourceURL); err != nil {
		return WorkInfo{}, false, err
	}
	return out, out.IsEmpty(), nil
}

// IsEmpty はすべての項目が未設定で、出演者も無いことを返す。
func (w WorkInfo) IsEmpty() bool {
	return w.Code == "" && w.ReleaseDate == "" && w.Maker == "" && len(w.Performers) == 0 &&
		w.Description == "" && w.SourceURL == ""
}

// Equal は 2 つの作品情報が同じ値かを返す。出演者は順番も比べ、nil と空の一覧は同じである。
func (w WorkInfo) Equal(other WorkInfo) bool {
	if w.Code != other.Code || w.ReleaseDate != other.ReleaseDate || w.Maker != other.Maker ||
		w.Description != other.Description || w.SourceURL != other.SourceURL ||
		len(w.Performers) != len(other.Performers) {
		return false
	}
	for i, name := range w.Performers {
		if other.Performers[i] != name {
			return false
		}
	}
	return true
}

// normalizeWorkInfoText は文字列の項目 field を表示名と同じ手順で整える。allowLineFeed が真なら
// 改行（U+000A）を制御文字として扱わない。空になれば空文字を返す。
func normalizeWorkInfoText(field WorkInfoField, input string, maxLength int, allowLineFeed bool) (string, error) {
	for _, r := range input {
		if unicode.Is(unicode.Cc, r) && (!allowLineFeed || r != '\n') {
			return "", &InvalidWorkInfoError{Field: field, Problem: WorkInfoControlCharacters}
		}
	}
	trimmed := strings.TrimFunc(input, func(r rune) bool {
		return unicode.Is(unicode.White_Space, r)
	})
	if utf8.RuneCountInString(trimmed) > maxLength {
		return "", &InvalidWorkInfoError{Field: field, Problem: WorkInfoTooLong, Limit: maxLength}
	}
	return trimmed, nil
}

// releaseDateLength は YYYY-MM-DD の長さである。
const releaseDateLength = len("2006-01-02")

// normalizeReleaseDate は発売日を整え、年 1〜9999 の暦日の YYYY-MM-DD であることを確かめる。
// 文字列の手順の上限は形の長さとし、それを超える値も形の誤りとして返す。
func normalizeReleaseDate(input string) (string, error) {
	date, err := normalizeWorkInfoText(WorkInfoFieldReleaseDate, input, len(input), false)
	if err != nil || date == "" {
		return date, err
	}
	invalid := &InvalidWorkInfoError{Field: WorkInfoFieldReleaseDate, Problem: WorkInfoInvalidDate}
	if len(date) != releaseDateLength {
		return "", invalid
	}
	for i := 0; i < len(date); i++ {
		if i == 4 || i == 7 {
			if date[i] != '-' {
				return "", invalid
			}
		} else if date[i] < '0' || date[i] > '9' {
			return "", invalid
		}
	}
	parsed, err := time.Parse(time.DateOnly, date)
	if err != nil || parsed.Year() < 1 {
		return "", invalid
	}
	return date, nil
}

// normalizeSourceURL は出典を整え、ホストを持つ絶対の http・https の URL であることを確かめる。
func normalizeSourceURL(input string) (string, error) {
	value, err := normalizeWorkInfoText(WorkInfoFieldSourceURL, input, WorkInfoLimits.SourceURLMaxLength, false)
	if err != nil || value == "" {
		return value, err
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.Opaque != "" || parsed.Hostname() == "" {
		return "", &InvalidWorkInfoError{Field: WorkInfoFieldSourceURL, Problem: WorkInfoInvalidURL}
	}
	return value, nil
}

// normalizePerformers は出演者の名前をそれぞれ整え、空になった名前を除き、同じ綴りの名前は
// 最初の 1 つを残す。残った数が上限を超えれば誤り。名前が無ければ nil を返す。
func normalizePerformers(input []string) ([]string, error) {
	limits := WorkInfoLimits
	var out []string
	seen := make(map[string]bool, len(input))
	for _, name := range input {
		normalized, err := normalizeWorkInfoText(WorkInfoFieldPerformers, name, limits.PerformerMaxLength, false)
		if err != nil {
			return nil, err
		}
		if normalized == "" || seen[normalized] {
			continue
		}
		seen[normalized] = true
		out = append(out, normalized)
	}
	if len(out) > limits.PerformersMaxCount {
		return nil, &InvalidWorkInfoError{
			Field: WorkInfoFieldPerformers, Problem: WorkInfoTooMany, Limit: limits.PerformersMaxCount,
		}
	}
	return out, nil
}

// WorkInfoChange は外部連携の一括操作の 1 件である。WorkInfo は保存済みの値をまるごと置き換え、
// 整えて空なら解除する（research.md R-5）。
type WorkInfoChange struct {
	Video    VideoRef
	WorkInfo WorkInfo
}

// WorkInfoAtError は一括操作の作品情報のうち、規則に合わなかったものの位置（0 から）と理由
// （*InvalidWorkInfoError）を運ぶ。errors.Is(err, ErrInvalidWorkInfo) は真になり、errors.As で
// *InvalidWorkInfoError も取り出せる。
type WorkInfoAtError struct {
	Index int
	Err   error
}

func (e *WorkInfoAtError) Error() string {
	return fmt.Sprintf("work information %d: %v", e.Index, e.Err)
}

func (e *WorkInfoAtError) Unwrap() error { return e.Err }

// VideoWorkInfoChanged は動画の作品情報が変わったことを表す（research.md R-8）。VideoIDs は
// 変わった利用者データの鍵に属するすべての動画の id である。画面への知らせに写し、ワーカーの
// 起床には結ばない。
type VideoWorkInfoChanged struct {
	VideoIDs []int64
}

func (VideoWorkInfoChanged) event() {}
