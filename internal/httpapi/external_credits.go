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

// External credit routes reuse UI conversions, validation, and storage; IDs are supplied in request bodies.

type externalCreditKind struct {
	kind      domain.CreditKind
	notFound  extgen.ErrorReason
	mergeSame extgen.ErrorReason
	one       func(domain.Credit) any
	list      func(items []domain.Credit, total, totalAll int, nextCursor string) any
	merged    func(domain.CreditMergeOutcome) any
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

// source plus externalId selects lookup; otherwise use shared list-query rules.
func (e *externalServer) listCredits(w http.ResponseWriter, r *http.Request, api externalCreditKind, params extgen.ListPeopleParams) {
	if params.Source != nil || params.ExternalId != nil {
		e.lookupCredit(w, r, api, params.Source, params.ExternalId)
		return
	}
	limit, problem := creditListLimit(params.Limit)
	if !e.acceptListQuery(w, problem) {
		return
	}
	query, problem := buildCreditListQuery(api.kind, params.Q, sortParam(params.Sort), params.Cursor, limit)
	if !e.acceptListQuery(w, problem) || !e.creditsConfigured(w) {
		return
	}
	page, err := e.s.credits.List(r.Context(), api.kind, query)
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

// Keep limit as a generated string so external-ID lookups can ignore it; parse it only for list queries.
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

// Normalize lookup IDs as storage does; invalid IDs match nothing, and uniqueness limits results to zero or one.
func (e *externalServer) lookupCredit(w http.ResponseWriter, r *http.Request, api externalCreditKind, source, value *string) {
	if source == nil || value == nil {
		e.invalidRequest(w, nil, "Give source and externalId together.")
		return
	}
	if !e.creditsConfigured(w) {
		return
	}
	// totalAll counts all credits independently of lookup.
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

func (e *externalServer) createCredit(w http.ResponseWriter, r *http.Request, api externalCreditKind) {
	var body gen.CreditInput
	fields, ok := e.readJSONBodyFields(w, r, &body)
	if !ok || !e.requireProperties(w, fields, "name") {
		return
	}
	input, message, ok := creditInputOf(body, fields.has, api.kind)
	if !ok {
		e.invalidRequest(w, nil, message)
		return
	}
	if !e.creditsConfigured(w) {
		return
	}
	credit, err := e.s.credits.Create(r.Context(), api.kind, input)
	if err != nil {
		e.writeCreditError(w, api, err)
		return
	}
	writeExternalBody(w, e.s, api.one(credit))
}

type creditUpdateBody struct {
	ID *int64 `json:"id"`
	gen.CreditPatch
}

func (e *externalServer) updateCredit(w http.ResponseWriter, r *http.Request, api externalCreditKind) {
	var body creditUpdateBody
	fields, ok := e.readJSONBodyFields(w, r, &body)
	if !ok || !e.requireProperties(w, fields, "id") {
		return
	}
	patch, message, ok := creditPatchOf(body.CreditPatch, fields, api.kind)
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

// Delete deduplicated IDs in input order; an unexpected failure returns 500 without undoing earlier deletions.
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

type bodyFields map[string]json.RawMessage

func (f bodyFields) has(name string) bool {
	_, present := jsonProperty(f, name)
	return present
}

func (f bodyFields) given(name string) bool {
	value, present := jsonProperty(f, name)
	return present && !bytes.Equal(bytes.TrimSpace(value), []byte("null"))
}

// Reject missing/null required properties before generated zero values can become semantic errors or misleading 404s.
func (e *externalServer) requireProperties(w http.ResponseWriter, fields bodyFields, names ...string) bool {
	for _, name := range names {
		if !fields.given(name) {
			e.invalidRequest(w, nil, name+" is required and cannot be null.")
			return false
		}
	}
	return true
}

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

func writeExternalBody(w http.ResponseWriter, srv *server, body any) {
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, body, srv.logger)
}

// Empty credit lists serialize as [], not null.
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
		ExternalIds: toExternalExternalIDs(credit.ExternalIDs), Profile: toExternalProfile(credit.Profile),
		VideoCount: credit.VideoCount, CreatedAt: credit.CreatedAt,
	}
}

// Root organizations expose parent as null.
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
