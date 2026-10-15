package domain

import (
	"errors"
	"strings"
	"time"
	"unicode"
)

// 作品情報プラグインの検索・取得と、取得の結果の反映の値と規則である
// （specs/049-work-info-plugins/contracts/screen-api.md、data-model.md の Rules、research.md R-8〜R-10・R-13）。
// 実行は internal/app、反映の書き込みは internal/store（OverrideStore）が行う。

const (
	// WorkInfoSearchQueryMaxRunes は検索の語の、前後の空白を除いたあとの長さの上限（文字数）である。
	WorkInfoSearchQueryMaxRunes = 200
	// WorkInfoFetchURLMaxRunes は取得する作品の URL の長さの上限（文字数）である（contracts/screen-api.md）。
	WorkInfoFetchURLMaxRunes = 2048
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

// CheckWorkInfoFetchURL は取得の URL が正しい UTF-8 で WorkInfoFetchURLMaxRunes 文字以下の、host を持つ
// http・https の絶対 URL かを確かめる。上限はバイト数ではなく文字数で数える。外れれば ErrWorkInfoRunInvalid を返す。
func CheckWorkInfoFetchURL(raw string) error {
	if !runeCountBetween(raw, 1, WorkInfoFetchURLMaxRunes) || !isWorkURL(raw) {
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
	// WorkInfoFieldCode は品番で、保存済みの品番を置き換える。
	WorkInfoFieldCode WorkInfoField = "code"
	// WorkInfoFieldDescription は説明で、保存済みの説明を置き換える。
	WorkInfoFieldDescription WorkInfoField = "description"
	// WorkInfoFieldURLs はリンクで、まだ無い URL を後ろに足す。
	WorkInfoFieldURLs WorkInfoField = "urls"
	// WorkInfoFieldExternalIDs は動画の外部 ID で、まだ無い (source, value) を足す。
	WorkInfoFieldExternalIDs WorkInfoField = "externalIds"
)

// DateField は日付の種類 kind の項目（dates.release など）を返す。保存済みのその種類の日付を置き換える。
func DateField(kind DateKind) WorkInfoField { return WorkInfoField("dates." + string(kind)) }

// PersonField は人物の役割 role の項目（people.performer など）を返す。その役割にまだ結んでいない人物を足す。
func PersonField(role PersonRole) WorkInfoField { return WorkInfoField("people." + string(role)) }

// OrganizationField は団体の役割 role の項目（organizations.maker など）を返す。
func OrganizationField(role OrganizationRole) WorkInfoField {
	return WorkInfoField("organizations." + string(role))
}

// WorkInfoFields は反映できる項目を比較の行の順に並べたものである
// （specs/052-plugin-work-info-fields/contracts/screen-api.md の WorkInfoField）。
var WorkInfoFields = func() []WorkInfoField {
	fields := []WorkInfoField{WorkInfoFieldTitle, WorkInfoFieldTags, WorkInfoFieldCode, WorkInfoFieldDescription}
	for _, kind := range DateKinds {
		fields = append(fields, DateField(kind))
	}
	for _, role := range PersonRoles {
		fields = append(fields, PersonField(role))
	}
	for _, role := range OrganizationRoles {
		fields = append(fields, OrganizationField(role))
	}
	return append(fields, WorkInfoFieldURLs, WorkInfoFieldExternalIDs)
}()

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
// プラグイン PluginID の行がまだあり、承認済みで、ダイジェストが Digest、世代が Generation のままであることを
// 読み直してから書く（data-model.md の Rules、research.md R-8）。
type FetchedWorkInfoApply struct {
	VideoID  int64
	PluginID int64
	Digest   string
	// Generation は取得した時のプラグインの行の世代である。取得のあとの差し替えと承認の取り消し（同じ
	// ダイジェストの再承認を含む）で変わる。
	Generation int64
	// Title は表示名に書く題名である。nil なら表示名は変えない。
	Title *string
	// Tags は仮のタグとして付ける、タグ名の規則で整えた名前である。空ならタグは変えない。
	Tags []string
	// Details は取得した作品の詳細で、DetailFields が選んだ項目だけを作品情報に反映する。
	Details FetchedWorkInfo
	// DetailFields は作品情報に反映する項目（題名とタグ以外の WorkInfoField）である。空なら作品情報は変えない。
	DetailFields []WorkInfoField
}

// FetchedWorkInfoApplied は保存先の反映の結果である。RejectedTags は却下した名前なので付けなかったタグの
// 名前（Tags の順、重複なし）、Skipped は作品の詳細のうち反映で飛ばした値である。
type FetchedWorkInfoApplied struct {
	Video        Video
	RejectedTags []string
	Skipped      []SkippedWorkInfoValue
}

// WorkInfoApplied は反映の結果である。
type WorkInfoApplied struct {
	Video       Video
	SkippedTags []SkippedTag
}
