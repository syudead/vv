package domain

import (
	"errors"
	"slices"
	"strings"
	"time"
	"unicode"
)

// 作品情報プラグインの検索・取得と、取得の結果の反映の値と規則である
// （specs/049-work-info-plugins/contracts/screen-api.md、data-model.md の Rules、research.md R-8〜R-10・R-13）。
// 実行は internal/app、反映の書き込みは internal/store（OverrideStore）が行う。

const (
	// WorkInfoSearchQueryMaxRunes は検索の語と取得の作品 ID の、前後の空白を除いたあとの長さの上限（文字数）である。
	WorkInfoSearchQueryMaxRunes = 200
	// WorkInfoFetchLifetime は取得の結果をメモリに持つ時間である（research.md R-8）。
	WorkInfoFetchLifetime = 10 * time.Minute
)

var (
	// ErrWorkInfoPluginNotApproved はプラグインが承認されていないことを表す。
	ErrWorkInfoPluginNotApproved = errors.New("the work-info plugin is not approved")
	// ErrWorkInfoOperationUnsupported はプラグインのマニフェストがその操作を宣言しないことを表す。
	ErrWorkInfoOperationUnsupported = errors.New("the work-info plugin does not support the operation")
	// ErrWorkInfoRunInvalid は検索の語か取得の作品 ID が規則に合わないことを表す。
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

// EffectiveWorkInfoPluginValue は設定 s について実行が受け取る値を返す（specs/056-plugin-tooling/research.md R-3）。
// values に空でない値が保存されていて、s に選択肢が無いか選択肢のどれかならその値、そうでなければ既定値である。
// stale は保存された値が選択肢の外にあり、使われないことを表す。値が空なら実行はこのキーを受け取らない。
func EffectiveWorkInfoPluginValue(s WorkInfoPluginSetting, values map[string]string) (value string, stale bool) {
	stored := values[s.Key]
	if stored == "" {
		return s.Default, false
	}
	if s.Options != nil && !slices.Contains(s.Options, stored) {
		return s.Default, true
	}
	return stored, false
}

// MissingWorkInfoPluginSettings は宣言 settings のうち、必須なのに実効値（EffectiveWorkInfoPluginValue）の無い
// キーを宣言の順に返す。既定値のある必須の設定は不足にならない。
func MissingWorkInfoPluginSettings(settings []WorkInfoPluginSetting, values map[string]string) []string {
	var missing []string
	for _, s := range settings {
		if v, _ := EffectiveWorkInfoPluginValue(s, values); s.Required && v == "" {
			missing = append(missing, s.Key)
		}
	}
	return missing
}

// DeclaredWorkInfoPluginValues は宣言 settings のキーごとの実効値（EffectiveWorkInfoPluginValue）のうち、
// 空でないものを返す。実行の要求にはこれを入れ、宣言に無いキーと他のプラグインの値は入れない（research.md R-13）。
func DeclaredWorkInfoPluginValues(settings []WorkInfoPluginSetting, values map[string]string) map[string]string {
	out := make(map[string]string, len(settings))
	for _, s := range settings {
		if v, _ := EffectiveWorkInfoPluginValue(s, values); v != "" {
			out[s.Key] = v
		}
	}
	return out
}

// WorkInfoRunSettings は宣言 settings と値 values から、実行の要求の settings（DeclaredWorkInfoPluginValues）を
// 返す。必須なのに実効値の無いキーがあれば、宣言の順にそれを持つ *WorkInfoSettingsMissingError を返す。
// サーバーの実行と mdm plugin run が同じ要求を組み立てるために使う（specs/056-plugin-tooling/research.md R-1）。
func WorkInfoRunSettings(settings []WorkInfoPluginSetting, values map[string]string) (map[string]string, error) {
	if missing := MissingWorkInfoPluginSettings(settings, values); len(missing) > 0 {
		return nil, &WorkInfoSettingsMissingError{Keys: missing}
	}
	return DeclaredWorkInfoPluginValues(settings, values), nil
}

// KeptWorkInfoTitle は取得した題名 raw のうち VVMDM が残すものを返す。表示名の規則（NormalizeDisplayName）で
// 整えた題名で、規則に合わなければ値が無いものとして "" を返す（specs/049-work-info-plugins/contracts/screen-api.md）。
func KeptWorkInfoTitle(raw string) string {
	title, _, err := NormalizeDisplayName(raw)
	if err != nil {
		return ""
	}
	return title
}

// NormalizeWorkInfoSearchQuery は検索の語か取得の作品 ID の前後の空白を除き、1〜WorkInfoSearchQueryMaxRunes
// 文字の正しい UTF-8 なら返す。外れれば ErrWorkInfoRunInvalid を返す。所有者が入れた文字列は URL の形でも
// 作品 ID であり、vv が URL として読むことはない（親 Issue #1232 の要件 14）。
func NormalizeWorkInfoSearchQuery(query string) (string, error) {
	trimmed := strings.TrimFunc(query, unicode.IsSpace)
	if !runeCountBetween(trimmed, 1, WorkInfoSearchQueryMaxRunes) {
		return "", ErrWorkInfoRunInvalid
	}
	return trimmed, nil
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
	fields := []WorkInfoField{
		WorkInfoFieldTitle, WorkInfoFieldTags, WorkInfoFieldCover, WorkInfoFieldCode, WorkInfoFieldDescription,
	}
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
	// Details は題名とタグ以外の作品の詳細で、OrderFetchedWorkDetails で並べてある。Title と Tags は持たない。
	Details FetchedWorkInfo
	// PersonMatches と OrganizationMatches は Details.People と Details.Organizations の各件の、取得の時点の
	// 照合の答えで、同じ順に並ぶ（specs/052-plugin-work-info-fields/research.md R-3）。
	PersonMatches       []CreditMatch
	OrganizationMatches []CreditMatch
	// Cover は取ったカバー画像で、無ければ nil である。
	Cover *WorkInfoCover
}

// OrderFetchedWorkDetails は取得した作品の詳細を、画面と反映が使う順に並べ直した複製を返す。日付は種類の順、
// 人物と団体は役割の順で、同じ種類・役割の中は取得した順を保つ
// （specs/052-plugin-work-info-fields/contracts/screen-api.md）。Title と Tags は持たない。
func OrderFetchedWorkDetails(fetched FetchedWorkInfo) FetchedWorkInfo {
	out := fetched
	out.Title, out.Tags = "", nil
	out.Dates = stableByOrder(fetched.Dates, DateKinds, func(d WorkDate) DateKind { return d.Kind })
	out.People = stableByOrder(fetched.People, PersonRoles, func(p FetchedPerson) PersonRole { return p.Role })
	out.Organizations = stableByOrder(fetched.Organizations, OrganizationRoles,
		func(o FetchedOrganization) OrganizationRole { return o.Role })
	return out
}

// stableByOrder は items の複製を、key が order の何番目かで安定に並べ替えて返す。nil は nil のまま返す。
func stableByOrder[T any, K comparable](items []T, order []K, key func(T) K) []T {
	if items == nil {
		return nil
	}
	out := slices.Clone(items)
	slices.SortStableFunc(out, func(a, b T) int {
		return slices.Index(order, key(a)) - slices.Index(order, key(b))
	})
	return out
}

// HasDetail は取得した作品の詳細に、題名とタグ以外の項目 field の値があるかを返す。知らない項目と、
// 題名とタグは偽である。
func (f FetchedWorkInfo) HasDetail(field WorkInfoField) bool {
	switch field {
	case WorkInfoFieldCode:
		return f.Code != ""
	case WorkInfoFieldDescription:
		return f.Description != ""
	case WorkInfoFieldURLs:
		return len(f.URLs) > 0
	case WorkInfoFieldExternalIDs:
		return len(f.ExternalIDs) > 0
	}
	for _, kind := range DateKinds {
		if field == DateField(kind) {
			return slices.ContainsFunc(f.Dates, func(d WorkDate) bool { return d.Kind == kind })
		}
	}
	for _, role := range PersonRoles {
		if field == PersonField(role) {
			return slices.ContainsFunc(f.People, func(p FetchedPerson) bool { return p.Role == role })
		}
	}
	for _, role := range OrganizationRoles {
		if field == OrganizationField(role) {
			return slices.ContainsFunc(f.Organizations, func(o FetchedOrganization) bool { return o.Role == role })
		}
	}
	return false
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

// WorkInfoApplied は反映の結果である。Skipped は題名とタグ以外の選んだ項目のうち飛ばした値である。
type WorkInfoApplied struct {
	Video       Video
	SkippedTags []SkippedTag
	Skipped     []SkippedWorkInfoValue
}
