package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/extgen"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// 外部連携 API の人物と団体。/api/v1/people と /api/v1/organizations の 5 つずつの経路は、
// 種類（externalCreditKind）を添えて同じハンドラを呼ぶ。どの操作も画面の経路と同じ CreditStore の操作を
// Credits 越しに呼び、本文は画面と同じ形の gen 型に読んで画面と同じ変換（creditPatchOf・fromAPIAliases）を
// 通す。誤りは画面と同じ workInfoFailureOf で写す。実体は経路ではなく本文の id で指す（039 R-2 と同じ）。

// externalCreditKind は人物と団体で違う、応答の形と reason である。
type externalCreditKind struct {
	kind      domain.CreditKind
	notFound  extgen.ErrorReason
	mergeSame extgen.ErrorReason
	// one・list・merged は実体・一覧・統合の結果を応答の本文にする。
	one    func(domain.Credit) any
	list   func(items []domain.Credit, total, totalAll int, nextCursor string) any
	merged func(domain.CreditMergeOutcome) any
}

var externalPeople = externalCreditKind{
	kind: domain.CreditPerson, notFound: extgen.PersonNotFound, mergeSame: extgen.MergeSamePerson,
	one: func(credit domain.Credit) any { return toExternalPerson(credit) },
	list: func(credits []domain.Credit, total, totalAll int, nextCursor string) any {
		return extgen.PersonList{
			Items: mapCredits(credits, toExternalPerson), Total: total, TotalAll: totalAll, NextCursor: optionalString(nextCursor),
		}
	},
	merged: func(outcome domain.CreditMergeOutcome) any {
		return extgen.PersonMergeResponse{Person: toExternalPerson(outcome.Credit), NotFoundIds: nonNilIDs(outcome.NotFoundIDs)}
	},
}

var externalOrganizations = externalCreditKind{
	kind: domain.CreditOrganization, notFound: extgen.OrganizationNotFound, mergeSame: extgen.MergeSameOrganization,
	one: func(credit domain.Credit) any { return toExternalOrganization(credit) },
	list: func(credits []domain.Credit, total, totalAll int, nextCursor string) any {
		return extgen.OrganizationList{
			Items: mapCredits(credits, toExternalOrganization), Total: total, TotalAll: totalAll,
			NextCursor: optionalString(nextCursor),
		}
	},
	merged: func(outcome domain.CreditMergeOutcome) any {
		return extgen.OrganizationMergeResponse{
			Organization: toExternalOrganization(outcome.Credit), NotFoundIds: nonNilIDs(outcome.NotFoundIDs),
		}
	},
}

func (e *externalServer) ListPeople(w http.ResponseWriter, r *http.Request, params extgen.ListPeopleParams) {
	e.listCredits(w, r, externalPeople, params)
}

func (e *externalServer) ListOrganizations(w http.ResponseWriter, r *http.Request, params extgen.ListOrganizationsParams) {
	e.listCredits(w, r, externalOrganizations, extgen.ListPeopleParams(params))
}

func (e *externalServer) CreatePerson(w http.ResponseWriter, r *http.Request) {
	e.createCredit(w, r, externalPeople)
}

func (e *externalServer) CreateOrganization(w http.ResponseWriter, r *http.Request) {
	e.createCredit(w, r, externalOrganizations)
}

func (e *externalServer) UpdatePerson(w http.ResponseWriter, r *http.Request) {
	e.updateCredit(w, r, externalPeople)
}

func (e *externalServer) UpdateOrganization(w http.ResponseWriter, r *http.Request) {
	e.updateCredit(w, r, externalOrganizations)
}

func (e *externalServer) MergePeople(w http.ResponseWriter, r *http.Request) {
	e.mergeCredits(w, r, externalPeople)
}

func (e *externalServer) MergeOrganizations(w http.ResponseWriter, r *http.Request) {
	e.mergeCredits(w, r, externalOrganizations)
}

func (e *externalServer) DeletePeople(w http.ResponseWriter, r *http.Request) {
	e.deleteCredits(w, r, externalPeople)
}

func (e *externalServer) DeleteOrganizations(w http.ResponseWriter, r *http.Request) {
	e.deleteCredits(w, r, externalOrganizations)
}

// listCredits は GET /api/v1/people と GET /api/v1/organizations である。source と externalId の組が
// あれば外部 ID での検索で、そうでなければ画面の一覧と同じ規則（buildTagListQuery）で読む。
func (e *externalServer) listCredits(w http.ResponseWriter, r *http.Request, api externalCreditKind, params extgen.ListPeopleParams) {
	if params.Source != nil || params.ExternalId != nil {
		e.lookupCredit(w, r, api, params.Source, params.ExternalId)
		return
	}
	limit, problem := creditListLimit(params.Limit)
	if !e.acceptListQuery(w, problem) {
		return
	}
	query, problem := buildTagListQuery(params.Q, nil, nil, sortParam(params.Sort), params.Cursor, limit)
	if !e.acceptListQuery(w, problem) || !e.creditsConfigured(w) {
		return
	}
	page, err := e.s.credits.List(r.Context(), api.kind,
		domain.CreditQuery{Q: query.Search, Sort: query.Sort, Cursor: query.Cursor, Limit: query.Limit})
	if errors.Is(err, domain.ErrInvalidCursor) {
		reason := extgen.InvalidCursor
		e.invalidRequest(w, &reason, "Cannot read the cursor. Read the list again from the start with the same q and sort.")
		return
	}
	if err != nil {
		e.internalError(w, fmt.Sprintf("Could not load the %s list.", api.kind), err)
		return
	}
	writeExternalBody(w, e.s, api.list(page.Items, page.Total, page.TotalAll, page.NextCursor))
}

// creditListLimit は一覧の limit を整数に読む。外部 ID での検索は limit を読まないので、生成型では文字列の
// まま受け（api/external-v1.yaml の CreditLimit の x-go-type）、一覧のときだけここで読む。整数でなければ
// 範囲の外と同じ誤りにする。
func creditListLimit(raw *extgen.CreditLimit) (*int, tagListProblem) {
	if raw == nil {
		return nil, tagListOK
	}
	limit, err := strconv.Atoi(*raw)
	if err != nil {
		return nil, tagListLimitOutOfRange
	}
	return &limit, tagListOK
}

// lookupCredit は外部 ID (source, externalId) を持つ実体を一覧の形で返す。外部 ID は種類ごとに高々 1 つの
// 実体が持つので、items は 0 件か 1 件である。保存と同じ手順で整えてから照らし、整えられない外部 ID は
// どの実体も持ちえないので空の items にする。
func (e *externalServer) lookupCredit(w http.ResponseWriter, r *http.Request, api externalCreditKind, source, value *string) {
	if source == nil || value == nil {
		e.invalidRequest(w, nil, "Give source and externalId together.")
		return
	}
	if !e.creditsConfigured(w) {
		return
	}
	// totalAll は検索と独立に全部の実体の数を返す（一覧と同じ意味）。
	all, err := e.s.credits.List(r.Context(), api.kind, domain.CreditQuery{Limit: 1})
	if err != nil {
		e.internalError(w, fmt.Sprintf("Could not look up the %s.", api.kind), err)
		return
	}
	found := []domain.Credit{}
	if id, ok := domain.NormalizeExternalID(domain.ExternalID{Source: *source, Value: *value}); ok {
		credit, err := e.s.credits.Lookup(r.Context(), api.kind, id.Source, id.Value)
		switch {
		case err == nil:
			found = append(found, credit)
		case !errors.Is(err, domain.ErrNotFound):
			e.internalError(w, fmt.Sprintf("Could not look up the %s.", api.kind), err)
			return
		}
	}
	writeExternalBody(w, e.s, api.list(found, len(found), all.TotalAll, ""))
}

// createCredit は POST /api/v1/people と POST /api/v1/organizations である。他の外部連携の書き込みと同じく
// 200 を返す。
func (e *externalServer) createCredit(w http.ResponseWriter, r *http.Request, api externalCreditKind) {
	var body gen.CreditInput
	fields, ok := e.readJSONBodyFields(w, r, &body)
	if !ok || !e.requireProperties(w, fields, "name") {
		return
	}
	// 人物の本文の parentId は null でも 400 である（人物に親は無い。画面の経路と同じ）。
	if fields.has("parentId") && api.kind != domain.CreditOrganization {
		e.invalidRequest(w, nil, "parentId applies to organizations only.")
		return
	}
	if !e.creditsConfigured(w) {
		return
	}
	credit, err := e.s.credits.Create(r.Context(), api.kind, domain.CreditInput{
		Name: body.Name, Aliases: fromAPIAliases(body.Aliases), ExternalIDs: fromAPIExternalIDs(body.ExternalIds),
		ParentID: body.ParentId,
	})
	if err != nil {
		e.writeCreditError(w, api, err)
		return
	}
	writeExternalBody(w, e.s, api.one(credit))
}

// creditUpdateBody は POST …/update の本文で、画面の PATCH と同じ gen.CreditPatch に id を足した形である。
type creditUpdateBody struct {
	ID *int64 `json:"id"`
	gen.CreditPatch
}

// updateCredit は POST /api/v1/people/update と POST /api/v1/organizations/update である。
func (e *externalServer) updateCredit(w http.ResponseWriter, r *http.Request, api externalCreditKind) {
	var body creditUpdateBody
	fields, ok := e.readJSONBodyFields(w, r, &body)
	if !ok || !e.requireProperties(w, fields, "id") {
		return
	}
	patch, message, ok := creditPatchOf(body.CreditPatch, fields.has, api.kind)
	if !ok {
		e.invalidRequest(w, nil, message)
		return
	}
	if !e.creditsConfigured(w) {
		return
	}
	credit, err := e.s.credits.Update(r.Context(), api.kind, *body.ID, patch)
	if err != nil {
		e.writeCreditError(w, api, err)
		return
	}
	writeExternalBody(w, e.s, api.one(credit))
}

// mergeCredits は POST /api/v1/people/merge と POST /api/v1/organizations/merge である。sourceIds の件数が
// 1〜domain.MaxTagBatch の外なら work_info_too_many（field: sourceIds）にする。統合先を含む統合元・親の輪・
// 統合後の上限は保存層が断る（画面の経路と同じ）。
func (e *externalServer) mergeCredits(w http.ResponseWriter, r *http.Request, api externalCreditKind) {
	var body extgen.CreditMergeRequest
	fields, ok := e.readJSONBodyFields(w, r, &body)
	if !ok || !e.requireProperties(w, fields, "targetId") {
		return
	}
	if !e.acceptCreditIDCount(w, "sourceIds", len(body.SourceIds)) || !e.creditsConfigured(w) {
		return
	}
	outcome, err := e.s.credits.Merge(r.Context(), api.kind, body.TargetId, body.SourceIds)
	if err != nil {
		e.writeCreditError(w, api, err)
		return
	}
	writeExternalBody(w, e.s, api.merged(outcome))
}

// deleteCredits は POST /api/v1/people/delete と POST /api/v1/organizations/delete である。重複を除いた
// ids を順に、画面の DELETE と同じ Credits.Delete で 1 件ずつ消し、無い id は notFoundIds に返す。途中の
// 想定外の失敗では、それより前の id は消えたまま 500 を返す。
func (e *externalServer) deleteCredits(w http.ResponseWriter, r *http.Request, api externalCreditKind) {
	var body extgen.CreditDeleteRequest
	if !e.readJSONBody(w, r, &body) {
		return
	}
	if !e.acceptCreditIDCount(w, "ids", len(body.Ids)) || !e.creditsConfigured(w) {
		return
	}
	out := extgen.CreditDeleteResponse{DeletedIds: []int64{}, NotFoundIds: []int64{}}
	seen := make(map[int64]bool, len(body.Ids))
	for _, id := range body.Ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		err := e.s.credits.Delete(r.Context(), api.kind, id)
		switch {
		case err == nil:
			out.DeletedIds = append(out.DeletedIds, id)
		case errors.Is(err, domain.ErrNotFound):
			out.NotFoundIds = append(out.NotFoundIds, id)
		default:
			e.internalError(w, fmt.Sprintf("Could not delete %s %d. The ids before it were deleted.", api.kind, id), err)
			return
		}
	}
	writeExternalBody(w, e.s, out)
}

// acceptCreditIDCount は id の一覧 field の件数が 1〜domain.MaxTagBatch の外なら work_info_too_many を書いて
// false を返す。
func (e *externalServer) acceptCreditIDCount(w http.ResponseWriter, field string, count int) bool {
	if count >= 1 && count <= domain.MaxTagBatch {
		return true
	}
	e.writeWorkInfoFailure(w, workInfoFailure{
		status: http.StatusBadRequest, code: string(extgen.ErrorCodeInvalidRequest), reason: string(extgen.WorkInfoTooMany),
		field: field, limit: domain.MaxTagBatch,
		message: fmt.Sprintf("%s must contain between 1 and %d items. Nothing was changed.", field, domain.MaxTagBatch),
	}, nil)
	return false
}

// writeCreditError は保存層の誤りを写す。項目・参照・外部 ID・親の輪は workInfoFailureOf が、統合先そのものを
// 含む統合元と、本文の id の実体が無いことはここが種類ごとの reason で写す（画面の writeCreditError と同じ
// 振り分け）。
func (e *externalServer) writeCreditError(w http.ResponseWriter, api externalCreditKind, err error) {
	if failure, ok := workInfoFailureOf(err); ok {
		e.writeWorkInfoFailure(w, failure, nil)
		return
	}
	switch {
	case errors.Is(err, domain.ErrMergeSameCredit):
		reason := api.mergeSame
		e.invalidRequest(w, &reason, "sourceIds must not contain targetId. Nothing was changed.")
	case errors.Is(err, domain.ErrNotFound):
		writeExternalError(w, e.s, http.StatusNotFound, extgen.Error{
			Code: extgen.ErrorCodeNotFound, Reason: &api.notFound, Message: fmt.Sprintf("The %s was not found.", api.kind),
		})
	default:
		e.internalError(w, fmt.Sprintf("Could not update the %s.", api.kind), err)
	}
}

func (e *externalServer) creditsConfigured(w http.ResponseWriter) bool {
	if e.s.credits == nil {
		e.internalError(w, "People and organization storage is not configured.", nil)
		return false
	}
	return true
}

// bodyFields は本文の最上位のプロパティである。生成型に読むと失われる、プロパティの有無と null を答える。
type bodyFields map[string]json.RawMessage

// has はプロパティ name が本文にあったか（値が null でも）を答える。
func (f bodyFields) has(name string) bool {
	_, present := jsonProperty(f, name)
	return present
}

// given はプロパティ name が本文にあり、null でないかを答える。
func (f bodyFields) given(name string) bool {
	value, present := jsonProperty(f, name)
	return present && !bytes.Equal(bytes.TrimSpace(value), []byte("null"))
}

// requireProperties は必須のプロパティが無いか null なら、形の誤りとして 400 invalid_request を書いて
// false を返す。生成型では省略と null がゼロ値に変わり、項目の規則の誤り（work_info_empty）や無い実体
// （404）に化けるためである。
func (e *externalServer) requireProperties(w http.ResponseWriter, fields bodyFields, names ...string) bool {
	for _, name := range names {
		if !fields.given(name) {
			e.invalidRequest(w, nil, name+" is required and cannot be null.")
			return false
		}
	}
	return true
}

// readJSONBodyFields は readJSONBody と同じく本文を target に読み、加えて最上位のプロパティを返す。
// 省いたプロパティと null を見分ける書き込みに使う。
func (e *externalServer) readJSONBodyFields(w http.ResponseWriter, r *http.Request, target any) (bodyFields, bool) {
	var raw json.RawMessage
	if !e.readJSONBody(w, r, &raw) {
		return nil, false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		e.invalidBody(w, err, "Cannot parse the JSON body.")
		return nil, false
	}
	var fields bodyFields
	if err := json.Unmarshal(raw, &fields); err != nil {
		e.invalidBody(w, err, "The body must be a JSON object.")
		return nil, false
	}
	return fields, true
}

// writeExternalBody は 200 と本文を、キャッシュさせずに書く。
func writeExternalBody(w http.ResponseWriter, srv *server, body any) {
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, body, srv.logger)
}

// mapCredits は実体の一覧を応答の要素へ写す。空でも [] にする。
func mapCredits[T any](credits []domain.Credit, convert func(domain.Credit) T) []T {
	out := make([]T, 0, len(credits))
	for _, credit := range credits {
		out = append(out, convert(credit))
	}
	return out
}

func toExternalPerson(credit domain.Credit) extgen.Person {
	return extgen.Person{
		Id: credit.ID, Name: credit.Name, Aliases: toExternalAliases(credit.Aliases),
		ExternalIds: toExternalExternalIDs(credit.ExternalIDs), VideoCount: credit.VideoCount, CreatedAt: credit.CreatedAt,
	}
}

// toExternalOrganization は団体を外部連携 API の形へ写す。根の団体の parent は null である。
func toExternalOrganization(credit domain.Credit) extgen.Organization {
	person := toExternalPerson(credit)
	out := extgen.Organization{
		Id: person.Id, Name: person.Name, Aliases: person.Aliases, ExternalIds: person.ExternalIds,
		VideoCount: person.VideoCount, CreatedAt: person.CreatedAt,
	}
	if credit.Parent != nil {
		out.Parent = &struct {
			Id   int64  `json:"id"`
			Name string `json:"name"`
		}{Id: credit.Parent.ID, Name: credit.Parent.Name}
	}
	return out
}

func toExternalAliases(aliases []domain.Alias) []extgen.Alias {
	out := make([]extgen.Alias, 0, len(aliases))
	for _, alias := range aliases {
		out = append(out, extgen.Alias{Name: alias.Name, Language: optionalString(alias.Language)})
	}
	return out
}

func toExternalExternalIDs(ids []domain.ExternalID) []extgen.ExternalId {
	out := make([]extgen.ExternalId, 0, len(ids))
	for _, id := range ids {
		out = append(out, extgen.ExternalId{Source: id.Source, Value: id.Value})
	}
	return out
}
