package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// 人物と団体の管理 API（specs/047-video-work-info/contracts/screen-api.md「People and organizations」、
// research.md R-3・R-4・R-8・R-10）。/api/people と /api/organizations の 6 つずつの経路は、生成された
// 操作から種類（creditKindAPI）を添えて同じハンドラを呼ぶ。どの経路も所有者だけ（accessRoutes の既定）。
// internal/app は通さず、internal/store の CreditStore の 1 つの取引で済ませる。

// Credits は人物と団体の保存先である。internal/store の *CreditStore がこれを満たす。
type Credits interface {
	List(ctx context.Context, kind domain.CreditKind, query domain.CreditQuery) (domain.CreditPage, error)
	Get(ctx context.Context, kind domain.CreditKind, id int64) (domain.Credit, error)
	Create(ctx context.Context, kind domain.CreditKind, input domain.CreditInput) (domain.Credit, error)
	Update(ctx context.Context, kind domain.CreditKind, id int64, patch domain.CreditPatch) (domain.Credit, error)
	Merge(ctx context.Context, kind domain.CreditKind, targetID int64, sourceIDs []int64) (domain.CreditMergeOutcome, error)
	Delete(ctx context.Context, kind domain.CreditKind, id int64) error
}

// creditKindAPI は人物と団体で違う、応答の形と reason である。
type creditKindAPI struct {
	kind      domain.CreditKind
	notFound  gen.ErrorReason
	mergeSame gen.ErrorReason
	// one・list・merged は実体・一覧・統合の結果を応答の本文にする。
	one    func(domain.Credit) any
	list   func(domain.CreditPage) any
	merged func(domain.CreditMergeOutcome) any
}

var peopleAPI = creditKindAPI{
	kind: domain.CreditPerson, notFound: gen.ErrorReasonPersonNotFound, mergeSame: gen.ErrorReasonMergeSamePerson,
	one: func(credit domain.Credit) any { return toAPIPerson(credit) },
	list: func(page domain.CreditPage) any {
		items := make([]gen.Person, 0, len(page.Items))
		for _, credit := range page.Items {
			items = append(items, toAPIPerson(credit))
		}
		return gen.PersonList{Items: items, Total: page.Total, TotalAll: page.TotalAll, NextCursor: optionalString(page.NextCursor)}
	},
	merged: func(outcome domain.CreditMergeOutcome) any {
		return gen.PersonMergeResponse{Person: toAPIPerson(outcome.Credit), NotFoundIds: nonNilIDs(outcome.NotFoundIDs)}
	},
}

var organizationsAPI = creditKindAPI{
	kind: domain.CreditOrganization, notFound: gen.ErrorReasonOrganizationNotFound,
	mergeSame: gen.ErrorReasonMergeSameOrganization,
	one:       func(credit domain.Credit) any { return toAPIOrganization(credit) },
	list: func(page domain.CreditPage) any {
		items := make([]gen.Organization, 0, len(page.Items))
		for _, credit := range page.Items {
			items = append(items, toAPIOrganization(credit))
		}
		return gen.OrganizationList{Items: items, Total: page.Total, TotalAll: page.TotalAll, NextCursor: optionalString(page.NextCursor)}
	},
	merged: func(outcome domain.CreditMergeOutcome) any {
		return gen.OrganizationMergeResponse{Organization: toAPIOrganization(outcome.Credit), NotFoundIds: nonNilIDs(outcome.NotFoundIDs)}
	},
}

func (s *server) ListPeople(w http.ResponseWriter, r *http.Request, params gen.ListPeopleParams) {
	s.listCredits(w, r, peopleAPI, params)
}

func (s *server) ListOrganizations(w http.ResponseWriter, r *http.Request, params gen.ListOrganizationsParams) {
	s.listCredits(w, r, organizationsAPI, gen.ListPeopleParams(params))
}

func (s *server) GetPerson(w http.ResponseWriter, r *http.Request, id gen.PersonId) {
	s.getCredit(w, r, peopleAPI, id)
}

func (s *server) GetOrganization(w http.ResponseWriter, r *http.Request, id gen.OrganizationId) {
	s.getCredit(w, r, organizationsAPI, id)
}

func (s *server) CreatePerson(w http.ResponseWriter, r *http.Request) {
	s.createCredit(w, r, peopleAPI)
}

func (s *server) CreateOrganization(w http.ResponseWriter, r *http.Request) {
	s.createCredit(w, r, organizationsAPI)
}

func (s *server) UpdatePerson(w http.ResponseWriter, r *http.Request, id gen.PersonId) {
	s.updateCredit(w, r, peopleAPI, id)
}

func (s *server) UpdateOrganization(w http.ResponseWriter, r *http.Request, id gen.OrganizationId) {
	s.updateCredit(w, r, organizationsAPI, id)
}

func (s *server) DeletePerson(w http.ResponseWriter, r *http.Request, id gen.PersonId) {
	s.deleteCredit(w, r, peopleAPI, id)
}

func (s *server) DeleteOrganization(w http.ResponseWriter, r *http.Request, id gen.OrganizationId) {
	s.deleteCredit(w, r, organizationsAPI, id)
}

func (s *server) MergePeople(w http.ResponseWriter, r *http.Request, id gen.PersonId) {
	s.mergeCredits(w, r, peopleAPI, id)
}

func (s *server) MergeOrganizations(w http.ResponseWriter, r *http.Request, id gen.OrganizationId) {
	s.mergeCredits(w, r, organizationsAPI, id)
}

// listCredits は GET /api/people と GET /api/organizations である。パラメータの規則はタグの一覧と同じ
// （buildTagListQuery）で、limit を省けば全件を返す。
func (s *server) listCredits(w http.ResponseWriter, r *http.Request, api creditKindAPI, params gen.ListPeopleParams) {
	var sort *string
	if params.Sort != nil {
		value := string(*params.Sort)
		sort = &value
	}
	tagQuery, problem := buildTagListQuery(params.Q, nil, nil, sort, params.Cursor, params.Limit)
	if !s.acceptTagListQuery(w, problem) || !s.creditsConfigured(w) {
		return
	}
	query := domain.CreditQuery{Q: tagQuery.Search, Sort: tagQuery.Sort, Cursor: tagQuery.Cursor, Limit: tagQuery.Limit}
	page, err := s.credits.List(r.Context(), api.kind, query)
	switch {
	case errors.Is(err, domain.ErrInvalidCursor):
		s.invalidRequestReason(w, reasonInvalidCursor, "Cannot read the cursor. Reload the list.")
		return
	case err != nil:
		s.internalError(w, fmt.Sprintf("Could not load the %s list.", api.kind), err)
		return
	}
	s.writeCreditBody(w, http.StatusOK, api.list(page))
}

func (s *server) getCredit(w http.ResponseWriter, r *http.Request, api creditKindAPI, id int64) {
	if !s.creditsConfigured(w) {
		return
	}
	credit, err := s.credits.Get(r.Context(), api.kind, id)
	if err != nil {
		s.writeCreditError(w, api, err)
		return
	}
	s.writeCreditBody(w, http.StatusOK, api.one(credit))
}

func (s *server) createCredit(w http.ResponseWriter, r *http.Request, api creditKindAPI) {
	var body gen.CreditInput
	present, ok := s.readJSONBodyFields(w, r, &body)
	if !ok {
		return
	}
	// 人物の本文の parentId は null でも 400 である（人物に親は無い）。
	if present("parentId") && api.kind != domain.CreditOrganization {
		s.invalidRequest(w, "parentId applies to organizations only.")
		return
	}
	if !s.creditsConfigured(w) {
		return
	}
	input := domain.CreditInput{
		Name: body.Name, Aliases: fromAPIAliases(body.Aliases), ExternalIDs: fromAPIExternalIDs(body.ExternalIds),
		ParentID: body.ParentId,
	}
	credit, err := s.credits.Create(r.Context(), api.kind, input)
	if err != nil {
		s.writeCreditError(w, api, err)
		return
	}
	s.writeCreditBody(w, http.StatusCreated, api.one(credit))
}

func (s *server) updateCredit(w http.ResponseWriter, r *http.Request, api creditKindAPI, id int64) {
	patch, ok := s.readCreditPatch(w, r, api.kind)
	if !ok || !s.creditsConfigured(w) {
		return
	}
	credit, err := s.credits.Update(r.Context(), api.kind, id, patch)
	if err != nil {
		s.writeCreditError(w, api, err)
		return
	}
	s.writeCreditBody(w, http.StatusOK, api.one(credit))
}

// readCreditPatch は PATCH の本文を domain.CreditPatch にする。省いたプロパティと null を見分けるため、
// readJSONBodyFields でどのプロパティがあったかも読む。
// 一覧の null は空の一覧、parentId の null は親を外すこと、name の null は 400 である。
func (s *server) readCreditPatch(w http.ResponseWriter, r *http.Request, kind domain.CreditKind) (domain.CreditPatch, bool) {
	var body gen.CreditPatch
	present, ok := s.readJSONBodyFields(w, r, &body)
	if !ok {
		return domain.CreditPatch{}, false
	}
	patch := domain.CreditPatch{Name: body.Name}
	if present("name") && body.Name == nil {
		s.invalidRequest(w, "name cannot be null.")
		return domain.CreditPatch{}, false
	}
	if present("aliases") {
		aliases := fromAPIAliases(body.Aliases)
		patch.Aliases = &aliases
	}
	if present("externalIds") {
		ids := fromAPIExternalIDs(body.ExternalIds)
		patch.ExternalIDs = &ids
	}
	if present("parentId") {
		if kind != domain.CreditOrganization {
			s.invalidRequest(w, "parentId applies to organizations only.")
			return domain.CreditPatch{}, false
		}
		patch.ParentSet, patch.ParentID = true, body.ParentId
	}
	return patch, true
}

// readJSONBodyFields は readJSONBody と同じく本文を target に読み、加えて最上位のプロパティが本文に
// あったか（値が null でも）を答える関数を返す。encoding/json はプロパティ名を大文字小文字を区別せずに
// target へ当てるので、有無も strings.EqualFold で比べる（Aliases は aliases として読まれ、有るとも答える）。
func (s *server) readJSONBodyFields(w http.ResponseWriter, r *http.Request, target any) (func(name string) bool, bool) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		s.invalidBody(w, err, "Cannot read the body.")
		return nil, false
	}
	if !s.decodeJSONBody(w, r, bytes.NewReader(raw), target) {
		return nil, false
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		s.invalidBody(w, err, "Cannot parse the JSON body.")
		return nil, false
	}
	return func(name string) bool {
		for key := range fields {
			if strings.EqualFold(key, name) {
				return true
			}
		}
		return false
	}, true
}

func (s *server) deleteCredit(w http.ResponseWriter, r *http.Request, api creditKindAPI, id int64) {
	if !s.creditsConfigured(w) {
		return
	}
	if err := s.credits.Delete(r.Context(), api.kind, id); err != nil {
		s.writeCreditError(w, api, err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	w.WriteHeader(http.StatusNoContent)
}

// mergeCredits は POST …/{id}/merge である。sourceIds の件数が 1〜domain.MaxTagBatch の外なら
// work_info_too_many（field: sourceIds）にする。統合先を含む統合元・親の輪・統合後の上限は保存層が断る。
func (s *server) mergeCredits(w http.ResponseWriter, r *http.Request, api creditKindAPI, id int64) {
	var body gen.CreditMergeRequest
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if len(body.SourceIds) == 0 || len(body.SourceIds) > domain.MaxTagBatch {
		s.writeWorkInfoFailure(w, workInfoFailure{
			status: http.StatusBadRequest, code: string(codeInvalidRequest), reason: string(gen.ErrorReasonWorkInfoTooMany),
			field: "sourceIds", limit: domain.MaxTagBatch,
			message: fmt.Sprintf("sourceIds must contain between 1 and %d items.", domain.MaxTagBatch),
		})
		return
	}
	if !s.creditsConfigured(w) {
		return
	}
	outcome, err := s.credits.Merge(r.Context(), api.kind, id, body.SourceIds)
	if err != nil {
		s.writeCreditError(w, api, err)
		return
	}
	s.writeCreditBody(w, http.StatusOK, api.merged(outcome))
}

// writeCreditError は保存層の誤りを写す。項目・参照・外部 ID・親の輪は workInfoFailureOf が、
// 統合先そのものを含む統合元と、経路の id の実体が無いことはここが種類ごとの reason で写す。
func (s *server) writeCreditError(w http.ResponseWriter, api creditKindAPI, err error) {
	if failure, ok := workInfoFailureOf(err); ok {
		s.writeWorkInfoFailure(w, failure)
		return
	}
	switch {
	case errors.Is(err, domain.ErrMergeSameCredit):
		s.invalidRequestReason(w, api.mergeSame, fmt.Sprintf("The source and target of a merge must be different %s entries.", api.kind))
	case errors.Is(err, domain.ErrNotFound):
		s.notFoundReason(w, api.notFound, fmt.Sprintf("The %s was not found.", api.kind))
	default:
		s.internalError(w, fmt.Sprintf("Could not update the %s.", api.kind), err)
	}
}

func (s *server) creditsConfigured(w http.ResponseWriter) bool {
	if s.credits == nil {
		s.internalError(w, "People and organization storage is not configured.", nil)
		return false
	}
	return true
}

func (s *server) writeCreditBody(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, status, body, s.logger)
}

func toAPIPerson(credit domain.Credit) gen.Person {
	return gen.Person{
		Id: credit.ID, Name: credit.Name, Aliases: toAPIAliases(credit.Aliases),
		ExternalIds: toAPIExternalIDs(credit.ExternalIDs), VideoCount: credit.VideoCount, CreatedAt: credit.CreatedAt,
	}
}

func toAPIOrganization(credit domain.Credit) gen.Organization {
	out := gen.Organization{
		Id: credit.ID, Name: credit.Name, Aliases: toAPIAliases(credit.Aliases),
		ExternalIds: toAPIExternalIDs(credit.ExternalIDs), VideoCount: credit.VideoCount, CreatedAt: credit.CreatedAt,
	}
	if credit.Parent != nil {
		out.Parent = &gen.CreditRef{Id: credit.Parent.ID, Name: credit.Parent.Name}
	}
	return out
}

func toAPIAliases(aliases []domain.Alias) []gen.Alias {
	out := make([]gen.Alias, 0, len(aliases))
	for _, alias := range aliases {
		out = append(out, gen.Alias{Name: alias.Name, Language: optionalString(alias.Language)})
	}
	return out
}

func toAPIExternalIDs(ids []domain.ExternalID) []gen.ExternalId {
	out := make([]gen.ExternalId, 0, len(ids))
	for _, id := range ids {
		out = append(out, gen.ExternalId{Source: id.Source, Value: id.Value})
	}
	return out
}

// fromAPIAliases は要求の別名を domain の形にする。無い一覧と null は空の一覧である。
func fromAPIAliases(aliases *[]gen.Alias) []domain.Alias {
	if aliases == nil {
		return []domain.Alias{}
	}
	out := make([]domain.Alias, 0, len(*aliases))
	for _, alias := range *aliases {
		entry := domain.Alias{Name: alias.Name}
		if alias.Language != nil {
			entry.Language = *alias.Language
		}
		out = append(out, entry)
	}
	return out
}

// fromAPIExternalIDs は要求の外部 ID を domain の形にする。無い一覧と null は空の一覧である。
func fromAPIExternalIDs(ids *[]gen.ExternalId) []domain.ExternalID {
	if ids == nil {
		return []domain.ExternalID{}
	}
	out := make([]domain.ExternalID, 0, len(*ids))
	for _, id := range *ids {
		out = append(out, domain.ExternalID{Source: id.Source, Value: id.Value})
	}
	return out
}
