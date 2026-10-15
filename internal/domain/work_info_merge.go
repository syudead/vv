package domain

import (
	"slices"
)

// 取得した作品の詳細を保存済みの作品情報に重ねる（specs/052-plugin-work-info-fields/research.md R-4）。
// 品番・説明・選んだ種類の日付は置き換え、役割ごとの人物・団体、URL、外部 ID はまだ無いものを取得の順に
// 後ろに足す。項目の規則に合わない値、上限を超える値、照合が曖昧な人物・団体は飛ばして報告し、行の残りは
// 反映する。保存層は返した作品情報を書くだけで、規則はここで決める。

// SkippedWorkInfoReason は反映で飛ばした値の理由である。
type SkippedWorkInfoReason string

const (
	// SkippedWorkInfoInvalid は項目の規則（047 data-model.md の Fields、実体の名前と外部 ID）に合わないこと。
	SkippedWorkInfoInvalid SkippedWorkInfoReason = "invalid"
	// SkippedWorkInfoTooMany は保存済みの件の後に数えて件数の上限を超えること。
	SkippedWorkInfoTooMany SkippedWorkInfoReason = "too_many"
	// SkippedWorkInfoAmbiguous は人物・団体の照合が複数の実体に当たったこと。
	SkippedWorkInfoAmbiguous SkippedWorkInfoReason = "ambiguous"
)

// SkippedDescriptionMaxRunes は飛ばした説明の Value に載せる先頭の文字数である。
const SkippedDescriptionMaxRunes = 100

// SkippedWorkInfoValue は反映で飛ばした値 1 つである。Value は比較が見せた文字列（日付、URL、名前、
// "source: value"、説明は先頭 SkippedDescriptionMaxRunes 文字）である。
type SkippedWorkInfoValue struct {
	Field  WorkInfoField
	Value  string
	Reason SkippedWorkInfoReason
}

// NewCredit は反映が作る人物か団体 1 つである。Input は NormalizeCredit を通った名前と外部 ID で、
// Links はこの実体を結ぶ、重ねた作品情報の People（団体なら Organizations）の位置である。保存層は
// 実体を作ってから、その位置の ID を埋める。
type NewCredit struct {
	Input CreditInput
	Links []int
}

// WorkInfoMergeInput は MergeFetchedWorkInfo の入力である。PersonMatches と OrganizationMatches は
// Fetched.People と Fetched.Organizations の各件の照合の答えで、同じ順に並ぶ。
type WorkInfoMergeInput struct {
	Stored              WorkInfo
	Fetched             FetchedWorkInfo
	PersonMatches       []CreditMatch
	OrganizationMatches []CreditMatch
	Fields              []WorkInfoField
}

// WorkInfoMerge は MergeFetchedWorkInfo の結果である。Info の作る人物・団体の ID は 0 で、NewPeople と
// NewOrganizations の Links が指す。Skipped は WorkInfoFields の順、項目の中は取得の順である。
type WorkInfoMerge struct {
	Info             WorkInfo
	NewPeople        []NewCredit
	NewOrganizations []NewCredit
	Skipped          []SkippedWorkInfoValue
}

// MergeFetchedWorkInfo は in.Fields が選んだ項目を保存済みの作品情報に重ねる（R-4）。結果に値の無い
// 項目と、題名とタグは何もしない。値ごとの規則は NormalizeWorkInfo で確かめる。
func MergeFetchedWorkInfo(in WorkInfoMergeInput) WorkInfoMerge {
	m := workInfoMerger{
		selected: func(field WorkInfoField) bool { return slices.Contains(in.Fields, field) },
		out:      WorkInfoMerge{Info: cloneWorkInfo(in.Stored)},
	}
	m.replaceText(WorkInfoFieldCode, in.Fetched.Code, func(w *WorkInfo) *string { return &w.Code })
	m.replaceText(WorkInfoFieldDescription, in.Fetched.Description, func(w *WorkInfo) *string { return &w.Description })
	m.replaceDates(in.Fetched.Dates)

	people := mergeCreditRoles(m.selected, creditRoleInput[PersonRole]{
		roles: PersonRoles, field: PersonField, limit: WorkInfoLimits.PeoplePerRoleMaxCount,
		stored:  workLinks(in.Stored.People, func(p WorkPerson) (PersonRole, CreditRef) { return p.Role, p.Person }),
		fetched: fetchedLinks(in.Fetched.People, func(p FetchedPerson) (PersonRole, string, []ExternalID) { return p.Role, p.Name, p.ExternalIDs }),
		matches: in.PersonMatches,
	})
	m.out.Info.People = make([]WorkPerson, len(people.links))
	for i, link := range people.links {
		m.out.Info.People[i] = WorkPerson{Role: link.role, Person: link.ref}
	}
	m.out.NewPeople = people.created
	m.out.Skipped = append(m.out.Skipped, people.skipped...)

	organizations := mergeCreditRoles(m.selected, creditRoleInput[OrganizationRole]{
		roles: OrganizationRoles, field: OrganizationField, limit: WorkInfoLimits.OrganizationsPerRoleMaxCount,
		stored: workLinks(in.Stored.Organizations, func(o WorkOrganization) (OrganizationRole, CreditRef) {
			return o.Role, o.Organization
		}),
		fetched: fetchedLinks(in.Fetched.Organizations, func(o FetchedOrganization) (OrganizationRole, string, []ExternalID) {
			return o.Role, o.Name, o.ExternalIDs
		}),
		matches: in.OrganizationMatches,
	})
	m.out.Info.Organizations = make([]WorkOrganization, len(organizations.links))
	for i, link := range organizations.links {
		m.out.Info.Organizations[i] = WorkOrganization{Role: link.role, Organization: link.ref}
	}
	m.out.NewOrganizations = organizations.created
	m.out.Skipped = append(m.out.Skipped, organizations.skipped...)

	m.appendURLs(in.Fetched.URLs)
	m.appendExternalIDs(in.Fetched.ExternalIDs)
	return m.out
}

// workInfoMerger は MergeFetchedWorkInfo の途中の状態である。
type workInfoMerger struct {
	selected func(WorkInfoField) bool
	out      WorkInfoMerge
}

func (m *workInfoMerger) skip(field WorkInfoField, value string, reason SkippedWorkInfoReason) {
	m.out.Skipped = append(m.out.Skipped, SkippedWorkInfoValue{Field: field, Value: value, Reason: reason})
}

// replaceText は選んだ品番か説明 value を、規則に合えば置き換え、合わなければ invalid で飛ばす。
func (m *workInfoMerger) replaceText(field WorkInfoField, value string, at func(*WorkInfo) *string) {
	if value == "" || !m.selected(field) {
		return
	}
	var probe WorkInfo
	*at(&probe) = value
	normalized, _, err := NormalizeWorkInfo(probe)
	if err != nil {
		shown := value
		if field == WorkInfoFieldDescription {
			shown = truncateRunes(value, SkippedDescriptionMaxRunes)
		}
		m.skip(field, shown, SkippedWorkInfoInvalid)
		return
	}
	*at(&m.out.Info) = *at(&normalized)
}

// replaceDates は選んだ種類の日付を、形が正しければ置き換え、正しくなければ invalid で飛ばす。ほかの
// 種類の日付は残し、一覧は DateKinds の順に並べる。
func (m *workInfoMerger) replaceDates(dates []WorkDate) {
	for _, kind := range DateKinds {
		i := slices.IndexFunc(dates, func(d WorkDate) bool { return d.Kind == kind })
		if i < 0 || !m.selected(DateField(kind)) {
			continue
		}
		normalized, _, err := NormalizeWorkInfo(WorkInfo{Dates: []WorkDate{dates[i]}})
		if err != nil || len(normalized.Dates) != 1 {
			m.skip(DateField(kind), dates[i].Date, SkippedWorkInfoInvalid)
			continue
		}
		m.out.Info.Dates = slices.DeleteFunc(m.out.Info.Dates, func(d WorkDate) bool { return d.Kind == kind })
		m.out.Info.Dates = append(m.out.Info.Dates, normalized.Dates[0])
	}
	slices.SortStableFunc(m.out.Info.Dates, func(a, b WorkDate) int {
		return slices.Index(DateKinds, a.Kind) - slices.Index(DateKinds, b.Kind)
	})
}

// appendURLs は選んだ URL のうちまだ無いものを取得の順に足す。形の違う URL は invalid、上限を超える分は
// too_many で飛ばす。
func (m *workInfoMerger) appendURLs(urls []string) {
	if !m.selected(WorkInfoFieldURLs) {
		return
	}
	for _, u := range urls {
		normalized, _, err := NormalizeWorkInfo(WorkInfo{URLs: []string{u}})
		switch {
		case err != nil || len(normalized.URLs) != 1:
			m.skip(WorkInfoFieldURLs, u, SkippedWorkInfoInvalid)
		case slices.Contains(m.out.Info.URLs, normalized.URLs[0]):
		case len(m.out.Info.URLs) >= WorkInfoLimits.URLsMaxCount:
			m.skip(WorkInfoFieldURLs, u, SkippedWorkInfoTooMany)
		default:
			m.out.Info.URLs = append(m.out.Info.URLs, normalized.URLs[0])
		}
	}
}

// appendExternalIDs は選んだ外部 ID のうちまだ無い (source, value) を足す。規則に合わない件は invalid、
// 上限を超える分は too_many で飛ばす。
func (m *workInfoMerger) appendExternalIDs(ids []ExternalID) {
	if !m.selected(WorkInfoFieldExternalIDs) {
		return
	}
	for _, id := range ids {
		shown := id.Source + ": " + id.Value
		normalized, _, err := NormalizeWorkInfo(WorkInfo{ExternalIDs: []ExternalID{id}})
		switch {
		case err != nil || len(normalized.ExternalIDs) != 1:
			m.skip(WorkInfoFieldExternalIDs, shown, SkippedWorkInfoInvalid)
		case slices.Contains(m.out.Info.ExternalIDs, normalized.ExternalIDs[0]):
		case len(m.out.Info.ExternalIDs) >= WorkInfoLimits.ExternalIDsMaxCount:
			m.skip(WorkInfoFieldExternalIDs, shown, SkippedWorkInfoTooMany)
		default:
			m.out.Info.ExternalIDs = append(m.out.Info.ExternalIDs, normalized.ExternalIDs[0])
		}
	}
}

// cloneWorkInfo は一覧を写した作品情報を返す。重ねる側が保存済みの値の配列を書き換えないためである。
func cloneWorkInfo(w WorkInfo) WorkInfo {
	w.Dates = slices.Clone(w.Dates)
	w.People = slices.Clone(w.People)
	w.Organizations = slices.Clone(w.Organizations)
	w.URLs = slices.Clone(w.URLs)
	w.ExternalIDs = slices.Clone(w.ExternalIDs)
	return w
}

// creditLink は重ねる途中の人物か団体の結び付き 1 件である。created は作る実体の NewCredit の位置で、
// 既存の実体なら -1 である。pending は足すときに新しく作る実体の入力で、それ以外は nil である。absorb は
// もう作ると決めた実体にまとめた件の外部 ID で、その実体の入力に足す。
type creditLink[R ~string] struct {
	role    R
	ref     CreditRef
	created int
	pending *CreditInput
	absorb  []ExternalID
}

// fetchedCredit は取得した人物か団体 1 件である。
type fetchedCredit[R ~string] struct {
	role R
	name string
	ids  []ExternalID
}

func workLinks[R ~string, W any](stored []W, split func(W) (R, CreditRef)) []creditLink[R] {
	out := make([]creditLink[R], len(stored))
	for i, w := range stored {
		role, ref := split(w)
		out[i] = creditLink[R]{role: role, ref: ref, created: -1}
	}
	return out
}

func fetchedLinks[R ~string, F any](fetched []F, split func(F) (R, string, []ExternalID)) []fetchedCredit[R] {
	out := make([]fetchedCredit[R], len(fetched))
	for i, f := range fetched {
		role, name, ids := split(f)
		out[i] = fetchedCredit[R]{role: role, name: name, ids: ids}
	}
	return out
}

// creditRoleInput は人物か団体の一覧を重ねる入力である。matches は fetched と同じ順の照合の答えである。
type creditRoleInput[R ~string] struct {
	roles   []R
	field   func(R) WorkInfoField
	limit   int
	stored  []creditLink[R]
	fetched []fetchedCredit[R]
	matches []CreditMatch
}

// creditRoleMerge は人物か団体の一覧を重ねた結果である。links は保存済みの結び付きの後ろに足したもので、
// 位置がそのまま作品情報の一覧の位置になる。
type creditRoleMerge[R ~string] struct {
	links   []creditLink[R]
	created []NewCredit
	skipped []SkippedWorkInfoValue
}

// mergeCreditRoles は選んだ役割ごとに、取得の順に件を足す。既存の実体はその役割にまだ結んでいなければ
// 足し、曖昧な件は ambiguous、作る件は NormalizeCredit に合わなければ invalid で飛ばす。件数は保存済みの
// 件の後に数え、上限を超える分は too_many。同じ結果の中で外部 ID か名前の照合形が重なる作る件は 1 つの
// 実体にまとめ、後の件の外部 ID もその実体に足す（mergeIDs）。
func mergeCreditRoles[R ~string](selected func(WorkInfoField) bool, in creditRoleInput[R]) creditRoleMerge[R] {
	out := creditRoleMerge[R]{links: slices.Clone(in.stored)}
	for _, role := range in.roles {
		field := in.field(role)
		if !selected(field) {
			continue
		}
		count := 0
		for _, link := range out.links {
			if link.role == role {
				count++
			}
		}
		for i, entry := range in.fetched {
			if entry.role != role {
				continue
			}
			match := CreditMatch{State: CreditMatchNew}
			if i < len(in.matches) {
				match = in.matches[i]
			}
			link, reason, keep := out.resolve(entry, match)
			switch {
			case reason != "":
				out.skipped = append(out.skipped, SkippedWorkInfoValue{Field: field, Value: entry.name, Reason: reason})
			case keep && count >= in.limit:
				out.skipped = append(out.skipped, SkippedWorkInfoValue{Field: field, Value: entry.name, Reason: SkippedWorkInfoTooMany})
			default:
				out.mergeIDs(field, link)
				if keep {
					out.add(link)
					count++
				}
			}
		}
	}
	return out
}

// resolve は取得した件 entry を照合の答え match で結び付きにする。飛ばすなら理由を、その役割にもう
// 結んでいる実体なら keep を偽で返す。
func (m *creditRoleMerge[R]) resolve(entry fetchedCredit[R], match CreditMatch) (creditLink[R], SkippedWorkInfoReason, bool) {
	link := creditLink[R]{role: entry.role, created: -1}
	switch match.State {
	case CreditMatchAmbiguous:
		return link, SkippedWorkInfoAmbiguous, false
	case CreditMatchExisting:
		if match.Credit == nil {
			return link, SkippedWorkInfoAmbiguous, false
		}
		link.ref = *match.Credit
	default:
		input, err := NormalizeCredit(CreditInput{Name: entry.name, ExternalIDs: entry.ids})
		if err != nil {
			return link, SkippedWorkInfoInvalid, false
		}
		link.ref = CreditRef{Name: input.Name}
		link.created = m.createdIndex(input)
		if link.created >= 0 {
			link.ref.Name = m.created[link.created].Input.Name
			link.absorb = input.ExternalIDs
		} else {
			// 作るのは足すときで、飛ばした件は実体を残さない。
			link.created, link.pending = len(m.created), &input
		}
	}
	return link, "", !m.linked(link)
}

// createdIndex は作る実体のうち、input と外部 ID を共有するか、なければ名前の照合形が等しいものの位置を返す。
// 無ければ -1。
func (m *creditRoleMerge[R]) createdIndex(input CreditInput) int {
	for i, created := range m.created {
		for _, id := range input.ExternalIDs {
			if slices.Contains(created.Input.ExternalIDs, id) {
				return i
			}
		}
	}
	key := FoldForMatch(input.Name)
	return slices.IndexFunc(m.created, func(c NewCredit) bool { return FoldForMatch(c.Input.Name) == key })
}

// mergeIDs は、もう作ると決めた実体にまとめた件 link の外部 ID のうち、その実体にまだ無いものを足す。
// ほかの作る実体が持つ ID は ambiguous、実体の外部 ID の上限を超える分は too_many で飛ばす。
func (m *creditRoleMerge[R]) mergeIDs(field WorkInfoField, link creditLink[R]) {
	if link.pending != nil || link.created < 0 {
		return
	}
	target := &m.created[link.created].Input
	for _, id := range link.absorb {
		shown := id.Source + ": " + id.Value
		switch {
		case slices.Contains(target.ExternalIDs, id):
		case slices.ContainsFunc(m.created, func(c NewCredit) bool { return slices.Contains(c.Input.ExternalIDs, id) }):
			m.skipped = append(m.skipped, SkippedWorkInfoValue{Field: field, Value: shown, Reason: SkippedWorkInfoAmbiguous})
		case len(target.ExternalIDs) >= ExternalIDsMaxCount:
			m.skipped = append(m.skipped, SkippedWorkInfoValue{Field: field, Value: shown, Reason: SkippedWorkInfoTooMany})
		default:
			target.ExternalIDs = append(target.ExternalIDs, id)
		}
	}
}

// linked は link と同じ役割に同じ実体がもう結ばれているかを返す。
func (m *creditRoleMerge[R]) linked(link creditLink[R]) bool {
	return slices.ContainsFunc(m.links, func(l creditLink[R]) bool {
		if l.role != link.role {
			return false
		}
		if link.created >= 0 {
			return l.created == link.created
		}
		return l.created < 0 && l.ref.ID == link.ref.ID
	})
}

// add は link を足し、作る実体ならその Links に位置を記す。
func (m *creditRoleMerge[R]) add(link creditLink[R]) {
	if link.pending != nil {
		m.created = append(m.created, NewCredit{Input: *link.pending})
		link.pending = nil
	}
	if link.created >= 0 {
		m.created[link.created].Links = append(m.created[link.created].Links, len(m.links))
	}
	m.links = append(m.links, link)
}
