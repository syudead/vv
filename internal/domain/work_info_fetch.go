package domain

import (
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// 作品情報プラグインの検索・取得と、取得の結果の反映の値と規則である
// （specs/049-work-info-plugins/contracts/screen-api.md、data-model.md の Rules、research.md R-8〜R-10・R-13）。
// 実行は internal/app、反映の書き込みは internal/store（OverrideStore）が行う。

const (
	// WorkInfoSearchQueryMaxRunes は検索の語の、前後の空白を除いたあとの長さの上限（文字数）である。
	WorkInfoSearchQueryMaxRunes = 200
	// WorkInfoFetchURLMaxBytes は取得する作品の URL の長さの上限である。
	WorkInfoFetchURLMaxBytes = 2048
	// WorkInfoFetchLifetime は取得の結果をメモリに持つ時間である（research.md R-8）。
	WorkInfoFetchLifetime = 10 * time.Minute
)

var (
	// ErrWorkInfoPluginNotApproved はプラグインが承認されていないことを表す。
	ErrWorkInfoPluginNotApproved = errors.New("the work-info plugin is not approved")
	// ErrWorkInfoOperationUnsupported はプラグインのマニフェストがその操作を宣言しないことを表す。
	ErrWorkInfoOperationUnsupported = errors.New("the work-info plugin does not support the operation")
	// ErrWorkInfoRunInvalid は検索の語か取得の URL が規則に合わないことを表す。
	ErrWorkInfoRunInvalid = errors.New("the work-info plugin request is invalid")
	// ErrWorkInfoFetchStale は取得の結果が期限切れか不明か、取得のあとにプラグインが差し替えられたか、
	// 承認を取り消されたか、消されたことを表す（research.md R-8）。
	ErrWorkInfoFetchStale = errors.New("the fetched work information is out of date")
	// ErrWorkInfoApplyInvalid は反映する項目が空か、結果に値の無い項目を選んだことを表す。
	ErrWorkInfoApplyInvalid = errors.New("the selected fields cannot be applied")
)

// WorkInfoSettingsMissingError は必須の設定の値が無いので実行しなかったことを表す。Keys は値の無い
// 必須のキーで、宣言の順である。
type WorkInfoSettingsMissingError struct {
	Keys []string
}

func (e *WorkInfoSettingsMissingError) Error() string {
	return "the work-info plugin needs settings: " + strings.Join(e.Keys, ", ")
}

// MissingWorkInfoPluginSettings は宣言 settings のうち、必須なのに values に値の無いキーを宣言の順に返す。
func MissingWorkInfoPluginSettings(settings []WorkInfoPluginSetting, values map[string]string) []string {
	var missing []string
	for _, s := range settings {
		if s.Required && values[s.Key] == "" {
			missing = append(missing, s.Key)
		}
	}
	return missing
}

// DeclaredWorkInfoPluginValues は values のうち、宣言 settings にあって値のあるものだけを返す。実行の要求には
// これを入れ、宣言に無いキーと他のプラグインの値は入れない（research.md R-13）。
func DeclaredWorkInfoPluginValues(settings []WorkInfoPluginSetting, values map[string]string) map[string]string {
	out := make(map[string]string, len(settings))
	for _, s := range settings {
		if v := values[s.Key]; v != "" {
			out[s.Key] = v
		}
	}
	return out
}

// NormalizeWorkInfoSearchQuery は検索の語の前後の空白を除き、1〜WorkInfoSearchQueryMaxRunes 文字の
// 正しい UTF-8 なら返す。外れれば ErrWorkInfoRunInvalid を返す。
func NormalizeWorkInfoSearchQuery(query string) (string, error) {
	trimmed := strings.TrimFunc(query, unicode.IsSpace)
	if !runeCountBetween(trimmed, 1, WorkInfoSearchQueryMaxRunes) {
		return "", ErrWorkInfoRunInvalid
	}
	return trimmed, nil
}

// CheckWorkInfoFetchURL は取得の URL が host を持つ http・https の絶対 URL で、WorkInfoFetchURLMaxBytes
// 以下かを確かめる。外れれば ErrWorkInfoRunInvalid を返す。
func CheckWorkInfoFetchURL(raw string) error {
	if len(raw) > WorkInfoFetchURLMaxBytes || !utf8.ValidString(raw) || !isWorkURL(raw) {
		return ErrWorkInfoRunInvalid
	}
	return nil
}

// WorkInfoField は取得の結果のうち、動画に反映できる項目である。
type WorkInfoField string

const (
	// WorkInfoFieldTitle は題名で、動画の表示名に書く。
	WorkInfoFieldTitle WorkInfoField = "title"
	// WorkInfoFieldTags はタグで、仮のタグとして付ける。
	WorkInfoFieldTags WorkInfoField = "tags"
)

// WorkInfoFetchResult はメモリに持つ取得の結果を画面に見せる形である。Title は表示名の規則を通った
// 題名で、空は値が無いことを表す。Tags はプラグインが返したタグの名前である。
type WorkInfoFetchResult struct {
	ID        string
	ExpiresAt time.Time
	Title     string
	Tags      []string
}

// SkippedTagReason は反映で付けなかったタグの名前の理由である。
type SkippedTagReason string

const (
	// SkippedTagRejected は所有者が却下した名前であることを表す。
	SkippedTagRejected SkippedTagReason = "rejected"
	// SkippedTagInvalid はタグ名の規則に合わないことを表す。
	SkippedTagInvalid SkippedTagReason = "invalid"
)

// SkippedTag は反映で付けなかったタグの名前とその理由である。
type SkippedTag struct {
	Name   string
	Reason SkippedTagReason
}

// FetchedWorkInfoApply は取得の結果のうち選んだ項目を動画に書く要求である。保存先は 1 つの取引の中で、
// プラグイン PluginID の行がまだあり、承認済みで、ダイジェストが Digest のままであることを読み直してから
// 書く（data-model.md の Rules）。
type FetchedWorkInfoApply struct {
	VideoID  int64
	PluginID int64
	Digest   string
	// Title は表示名に書く題名である。nil なら表示名は変えない。
	Title *string
	// Tags は仮のタグとして付ける、タグ名の規則で整えた名前である。空ならタグは変えない。
	Tags []string
}

// WorkInfoApplied は反映の結果である。
type WorkInfoApplied struct {
	Video       Video
	SkippedTags []SkippedTag
}
