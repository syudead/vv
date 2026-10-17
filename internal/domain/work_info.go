package domain

import (
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// 作品情報と人物・団体が共有する値と規則。動画ごとの作品情報の項目と、人物・団体の名前・別名・外部 ID は
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
	trimmed := trimWorkInfoText(input)
	if utf8.RuneCountInString(trimmed) > maxLength {
		return "", invalidWorkInfo(field, WorkInfoTooLong, maxLength)
	}
	return trimmed, nil
}

// trimWorkInfoText は文字列の前後の Unicode White_Space を除く。
func trimWorkInfoText(input string) string {
	return strings.TrimFunc(input, func(r rune) bool {
		return unicode.Is(unicode.White_Space, r)
	})
}

// normalizeExternalID は外部 ID 1 件を整える。両方が空になれば keep が偽（その件を落とす）、
// 片方だけ空になれば空の側の WorkInfoEmpty である。
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

// DateKind は動画の日付の種類である。1 つの動画は種類ごとに日付を 1 つ持つ。
type DateKind string

const (
	// DateRelease は発売日。
	DateRelease DateKind = "release"
	// DateRecorded は収録日。
	DateRecorded DateKind = "recorded"
	// DatePublished は公開日（配信の日）。
	DatePublished DateKind = "published"
)

// DateKinds は日付の種類を読み出しの順（release・recorded・published）に並べたものである。
var DateKinds = []DateKind{DateRelease, DateRecorded, DatePublished}

// Valid は k が既知の種類であることを返す。
func (k DateKind) Valid() bool { return slices.Contains(DateKinds, k) }

// PersonRole は動画に結ぶ人物の役割である。
type PersonRole string

const (
	// PersonPerformer は出演者。
	PersonPerformer PersonRole = "performer"
	// PersonDirector は監督。
	PersonDirector PersonRole = "director"
	// PersonPhotographer は撮影。
	PersonPhotographer PersonRole = "photographer"
	// PersonStreamer は配信者。
	PersonStreamer PersonRole = "streamer"
)

// PersonRoles は人物の役割を読み出しの順に並べたものである。
var PersonRoles = []PersonRole{PersonPerformer, PersonDirector, PersonPhotographer, PersonStreamer}

// Valid は r が既知の役割であることを返す。
func (r PersonRole) Valid() bool { return slices.Contains(PersonRoles, r) }

// OrganizationRole は動画に結ぶ団体の役割である。
type OrganizationRole string

const (
	// OrganizationMaker はメーカー。
	OrganizationMaker OrganizationRole = "maker"
	// OrganizationLabel はレーベル。
	OrganizationLabel OrganizationRole = "label"
)

// OrganizationRoles は団体の役割を読み出しの順に並べたものである。
var OrganizationRoles = []OrganizationRole{OrganizationMaker, OrganizationLabel}

// Valid は r が既知の役割であることを返す。
func (r OrganizationRole) Valid() bool { return slices.Contains(OrganizationRoles, r) }

// WorkDate は動画の日付 1 件である。Date は YYYY・YYYY-MM・YYYY-MM-DD の文字列で、送られた形のまま持つ。
type WorkDate struct {
	Kind DateKind
	Date string
}

// WorkPerson は動画に結ぶ人物 1 件である。書き込みでは Person.ID だけを読む。
type WorkPerson struct {
	Role   PersonRole
	Person CreditRef
}

// WorkOrganization は動画に結ぶ団体 1 件である。書き込みでは Organization.ID だけを読む。
type WorkOrganization struct {
	Role         OrganizationRole
	Organization CreditRef
}

// WorkInfo は動画 1 本の作品情報である。未設定の値は空の
// 文字列か空の一覧で、項目の並びは API の WorkInfo スキーマと同じ。誤りはこの順で最初の 1 件を返す。
type WorkInfo struct {
	Code          string
	Description   string
	Dates         []WorkDate
	People        []WorkPerson
	Organizations []WorkOrganization
	URLs          []string
	ExternalIDs   []ExternalID
}

// IsEmpty はすべての項目が未設定であることを返す。
func (w WorkInfo) IsEmpty() bool {
	return w.Code == "" && w.Description == "" && len(w.Dates) == 0 && len(w.People) == 0 &&
		len(w.Organizations) == 0 && len(w.URLs) == 0 && len(w.ExternalIDs) == 0
}

// Equal は 2 つの作品情報が同じ値かを返す。一覧は順番も比べ、nil と空の一覧は同じ。人物と団体は
// id だけを比べる（読み出しは名前を運び、書き込みは運ばない）。
func (w WorkInfo) Equal(other WorkInfo) bool {
	return w.Code == other.Code && w.Description == other.Description &&
		slices.Equal(w.Dates, other.Dates) && slices.Equal(w.URLs, other.URLs) &&
		slices.Equal(w.ExternalIDs, other.ExternalIDs) &&
		slices.EqualFunc(w.People, other.People, func(a, b WorkPerson) bool {
			return a.Role == b.Role && a.Person.ID == b.Person.ID
		}) &&
		slices.EqualFunc(w.Organizations, other.Organizations, func(a, b WorkOrganization) bool {
			return a.Role == b.Role && a.Organization.ID == b.Organization.ID
		})
}

// WorkInfoLimitSet は作品情報の各項目の上限である。長さは符号位置数、件数は落とした後の件数で数える。
type WorkInfoLimitSet struct {
	CodeMaxLength                int
	DescriptionMaxLength         int
	URLsMaxCount                 int
	URLMaxLength                 int
	ExternalIDsMaxCount          int
	ExternalIDSourceMaxLength    int
	ExternalIDValueMaxLength     int
	PeoplePerRoleMaxCount        int
	OrganizationsPerRoleMaxCount int
}

// WorkInfoLimits は作品情報の上限。API が誤りの limit に載せる。
var WorkInfoLimits = WorkInfoLimitSet{
	CodeMaxLength:                100,
	DescriptionMaxLength:         10_000,
	URLsMaxCount:                 20,
	URLMaxLength:                 2_048,
	ExternalIDsMaxCount:          ExternalIDsMaxCount,
	ExternalIDSourceMaxLength:    ExternalIDSourceMaxLength,
	ExternalIDValueMaxLength:     ExternalIDValueMaxLength,
	PeoplePerRoleMaxCount:        100,
	OrganizationsPerRoleMaxCount: 20,
}

// NormalizeWorkInfo は入力を保存する作品情報に整える。empty は整えた結果の
// すべての項目が空であることを表す。人物と団体の id があるかは保存層が確かめる。
//
// 文字列はどれも表示名と同じ手順を通り（制御文字は誤りで description の改行だけは許す、前後の空白を
// 除く、空なら解除するか一覧から落とす）、そのあと項目ごとの規則を確かめる。誤りは WorkInfo の項目の
// 順に、一覧は送られた順に確かめて最初の 1 件を *InvalidWorkInfoError で返し、Field の一覧の位置は
// 送られた一覧（落とした件も数える）の位置である。種類か役割が既知の値でなければ ErrInvalidWorkInfo を
// 包んだ誤りで、API の生成した検査が先に拒む。
//
// 返す値の一覧は読み出しの順に並べ直す。日付は DateKinds の順、人物と団体は役割の順（同じ役割の中は
// 送られた順）、外部 ID は (source, value) の順で、URL は送られた順のまま。
func NormalizeWorkInfo(input WorkInfo) (WorkInfo, bool, error) {
	var out WorkInfo
	var err error
	limits := WorkInfoLimits
	if out.Code, err = normalizeWorkInfoText("code", input.Code, limits.CodeMaxLength, false); err != nil {
		return WorkInfo{}, false, err
	}
	if out.Description, err = normalizeWorkInfoText("description", input.Description, limits.DescriptionMaxLength, true); err != nil {
		return WorkInfo{}, false, err
	}
	if out.Dates, err = normalizeWorkDates(input.Dates); err != nil {
		return WorkInfo{}, false, err
	}
	if out.People, err = normalizeWorkPeople(input.People); err != nil {
		return WorkInfo{}, false, err
	}
	if out.Organizations, err = normalizeWorkOrganizations(input.Organizations); err != nil {
		return WorkInfo{}, false, err
	}
	if out.URLs, err = normalizeWorkURLs(input.URLs); err != nil {
		return WorkInfo{}, false, err
	}
	if out.ExternalIDs, err = normalizeExternalIDs("externalIds", input.ExternalIDs); err != nil {
		return WorkInfo{}, false, err
	}
	SortExternalIDs(out.ExternalIDs)
	return out, out.IsEmpty(), nil
}

// normalizeWorkDates は日付の一覧を整える。種類が既知でなければ誤り、日付が空になった件は落とし、
// 同じ種類の 2 件目は dates[i].kind の Duplicate。返す一覧は DateKinds の順である。
func normalizeWorkDates(dates []WorkDate) ([]WorkDate, error) {
	byKind := make(map[DateKind]string, len(DateKinds))
	for i, raw := range dates {
		field := fmt.Sprintf("dates[%d]", i)
		if !raw.Kind.Valid() {
			return nil, fmt.Errorf("%s.kind: unknown date kind %q: %w", field, raw.Kind, ErrInvalidWorkInfo)
		}
		date, err := normalizeWorkDate(field+".date", raw.Date)
		if err != nil {
			return nil, err
		}
		if date == "" {
			continue
		}
		if _, ok := byKind[raw.Kind]; ok {
			return nil, invalidWorkInfo(field+".kind", WorkInfoDuplicate, 0)
		}
		byKind[raw.Kind] = date
	}
	out := make([]WorkDate, 0, len(byKind))
	for _, kind := range DateKinds {
		if date, ok := byKind[kind]; ok {
			out = append(out, WorkDate{Kind: kind, Date: date})
		}
	}
	return out, nil
}

// workDateLayouts は R-12 の 3 つの形の長さごとの time.Parse の形である。
var workDateLayouts = map[int]string{len("2006"): "2006", len("2006-01"): "2006-01", len(time.DateOnly): time.DateOnly}

// normalizeWorkDate は日付 1 件を整え、YYYY・YYYY-MM・YYYY-MM-DD のどれかで、年が 1〜9999、月が
// 1〜12、日がその月にある日であることを確かめる（R-12）。形より長い値も形の誤りにする。
func normalizeWorkDate(field, input string) (string, error) {
	date, err := normalizeWorkInfoText(field, input, utf8.RuneCountInString(input), false)
	if err != nil || date == "" {
		return date, err
	}
	if !isWorkDate(date) {
		return "", invalidWorkInfo(field, WorkInfoInvalidDate, 0)
	}
	return date, nil
}

// isWorkDate は date が R-12 の 3 つの形のどれかの実在する日付であることを返す。数字と - の位置を
// 先に確かめるので、time.Parse が許す 1 桁の月や日は通らない。
func isWorkDate(date string) bool {
	layout, ok := workDateLayouts[len(date)]
	if !ok {
		return false
	}
	for i := 0; i < len(date); i++ {
		if i == 4 || i == 7 {
			if date[i] != '-' {
				return false
			}
		} else if date[i] < '0' || date[i] > '9' {
			return false
		}
	}
	if date[:4] == "0000" {
		return false
	}
	_, err := time.Parse(layout, date)
	return err == nil
}

// normalizeWorkPeople は人物の一覧を整える。役割が既知でなければ誤り、同じ役割に同じ人物の 2 件目は
// people[i].person.id の Duplicate、役割ごとの件数が上限を超えれば people の TooMany。返す一覧は
// PersonRoles の順（同じ役割の中は送られた順）で、名前は運ばない。
func normalizeWorkPeople(people []WorkPerson) ([]WorkPerson, error) {
	refs := make([]workRef[PersonRole], 0, len(people))
	for _, p := range people {
		refs = append(refs, workRef[PersonRole]{role: p.Role, id: p.Person.ID})
	}
	sorted, err := normalizeWorkRefs("people", "person", PersonRoles, refs, WorkInfoLimits.PeoplePerRoleMaxCount)
	if err != nil {
		return nil, err
	}
	out := make([]WorkPerson, 0, len(sorted))
	for _, ref := range sorted {
		out = append(out, WorkPerson{Role: ref.role, Person: CreditRef{ID: ref.id}})
	}
	return out, nil
}

// normalizeWorkOrganizations は団体の一覧を normalizeWorkPeople と同じ規則で整える。Field は
// organizations[i].organization.id と organizations である。
func normalizeWorkOrganizations(organizations []WorkOrganization) ([]WorkOrganization, error) {
	refs := make([]workRef[OrganizationRole], 0, len(organizations))
	for _, o := range organizations {
		refs = append(refs, workRef[OrganizationRole]{role: o.Role, id: o.Organization.ID})
	}
	sorted, err := normalizeWorkRefs("organizations", "organization", OrganizationRoles, refs,
		WorkInfoLimits.OrganizationsPerRoleMaxCount)
	if err != nil {
		return nil, err
	}
	out := make([]WorkOrganization, 0, len(sorted))
	for _, ref := range sorted {
		out = append(out, WorkOrganization{Role: ref.role, Organization: CreditRef{ID: ref.id}})
	}
	return out, nil
}

// workRef は人物か団体の結び付き 1 件の役割と id である。
type workRef[R ~string] struct {
	role R
	id   int64
}

// normalizeWorkRefs は人物と団体の一覧に共通の規則である。list は一覧の経路（people）、entity は
// 参照のプロパティ名（person）、roles は既知の役割の読み出しの順、limit は役割ごとの件数の上限。
func normalizeWorkRefs[R ~string](list, entity string, roles []R, refs []workRef[R], limit int) ([]workRef[R], error) {
	byRole := make(map[R][]workRef[R], len(roles))
	seen := make(map[workRef[R]]bool, len(refs))
	for i, ref := range refs {
		field := fmt.Sprintf("%s[%d]", list, i)
		if !slices.Contains(roles, ref.role) {
			return nil, fmt.Errorf("%s.role: unknown role %q: %w", field, ref.role, ErrInvalidWorkInfo)
		}
		if seen[ref] {
			return nil, invalidWorkInfo(field+"."+entity+".id", WorkInfoDuplicate, 0)
		}
		seen[ref] = true
		byRole[ref.role] = append(byRole[ref.role], ref)
		if len(byRole[ref.role]) > limit {
			return nil, invalidWorkInfo(list, WorkInfoTooMany, limit)
		}
	}
	out := make([]workRef[R], 0, len(refs))
	for _, role := range roles {
		out = append(out, byRole[role]...)
	}
	return out, nil
}

// normalizeWorkURLs は URL の一覧を整える。空になった件は落とし、host を持つ絶対の http・https の URL で
// なければ urls[i] の InvalidURL、同じ URL の 2 件目は urls[i] の Duplicate、落とした後の件数が上限を
// 超えれば urls の TooMany。送られた順のまま返す。
func normalizeWorkURLs(urls []string) ([]string, error) {
	limits := WorkInfoLimits
	out := make([]string, 0, len(urls))
	seen := make(map[string]bool, len(urls))
	for i, raw := range urls {
		field := fmt.Sprintf("urls[%d]", i)
		value, err := normalizeWorkInfoText(field, raw, limits.URLMaxLength, false)
		if err != nil {
			return nil, err
		}
		if value == "" {
			continue
		}
		if !isWorkURL(value) {
			return nil, invalidWorkInfo(field, WorkInfoInvalidURL, 0)
		}
		if seen[value] {
			return nil, invalidWorkInfo(field, WorkInfoDuplicate, 0)
		}
		seen[value] = true
		out = append(out, value)
	}
	if len(out) > limits.URLsMaxCount {
		return nil, invalidWorkInfo("urls", WorkInfoTooMany, limits.URLsMaxCount)
	}
	return out, nil
}

// isWorkURL は value が host を持つ絶対の http・https の URL であることを返す。
func isWorkURL(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Opaque == "" &&
		parsed.Hostname() != ""
}

// WorkInfoChange は外部連携の一括操作の 1 件である。WorkInfo は保存済みの値をまるごと置き換え、
// 整えて空なら解除する。
type WorkInfoChange struct {
	Video    VideoRef
	WorkInfo WorkInfo
}

// WorkInfoAtError は一括操作の作品情報のうち、規則に合わなかったか無い実体を指したものの位置（0 から）と
// 理由（*InvalidWorkInfoError か *CreditNotFoundError）を運ぶ。DisplayNameAtError と同じ形である。
type WorkInfoAtError struct {
	Index int
	Err   error
}

func (e *WorkInfoAtError) Error() string {
	return fmt.Sprintf("work information %d: %v", e.Index, e.Err)
}

func (e *WorkInfoAtError) Unwrap() error { return e.Err }

// VideoWorkInfoChanged は動画の作品情報が変わったことを表す。VideoIDs は
// 変わった利用者データの鍵に属するすべての動画の id である。画面への知らせに写し、ワーカーの
// 起床には結ばない。
type VideoWorkInfoChanged struct {
	VideoIDs []int64
}

func (VideoWorkInfoChanged) event() {}

// ExternalIDIndex は、整える前の一覧 raw のうち、整えると id になる最初の件の位置を返す。無ければ -1。
// 整えた一覧の 1 件について誤り（他の実体が持つ外部 ID など）を返すとき、Field に呼び手が送った
// 一覧の位置を載せるために使う。
func ExternalIDIndex(raw []ExternalID, id ExternalID) int {
	for i, entry := range raw {
		if normalized, ok := NormalizeExternalID(entry); ok && normalized == id {
			return i
		}
	}
	return -1
}

// NormalizeExternalID は外部 ID 1 件を保存と同じ手順で整え、両方の半分が空でなく規則に合えば true を
// 返す。外部 ID での検索（外部連携 API の GET /api/v1/people?source=…&externalId=…）が保存済みの綴りと
// 同じ形で照らすのにも使う。規則に合わない外部 ID はどの実体も持ちえないので、呼び手は何にも当たらない
// ものとして扱う。
func NormalizeExternalID(id ExternalID) (ExternalID, bool) {
	normalized, keep, err := normalizeExternalID("", id)
	return normalized, keep && err == nil
}
