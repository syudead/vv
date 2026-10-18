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

// People and organization routes share handlers and owner-only defaults; each operation completes through CreditStore without an app use case.

type Credits interface {
	List(ctx context.Context, kind domain.CreditKind, query domain.CreditQuery) (domain.CreditPage, error)
	Get(ctx context.Context, kind domain.CreditKind, id int64) (domain.Credit, error)
	Lookup(ctx context.Context, kind domain.CreditKind, source, value string) (domain.Credit, error)
	Create(ctx context.Context, kind domain.CreditKind, input domain.CreditInput) (domain.Credit, error)
	Update(ctx context.Context, kind domain.CreditKind, id int64, patch domain.CreditPatch) (domain.Credit, error)
	Merge(ctx context.Context, kind domain.CreditKind, targetID int64, sourceIDs []int64) (domain.CreditMergeOutcome, error)
	Delete(ctx context.Context, kind domain.CreditKind, id int64) error
}

type creditKindAPI struct {
	kind      domain.CreditKind
	notFound  gen.ErrorReason
	mergeSame gen.ErrorReason
	one       func(domain.Credit) any
	list      func(domain.CreditPage) any
	merged    func(domain.CreditMergeOutcome) any
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

// Omitted limit returns all matching credits under the shared tag-list query rules.
func (s *server) listCredits(w http.ResponseWriter, r *http.Request, api creditKindAPI, params gen.ListPeopleParams) {
	sort := sortParam(params.Sort)
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
	input, message, ok := creditInputOf(body, present, api.kind)
	if !ok {
		s.invalidRequest(w, message)
		return
	}
	if !s.creditsConfigured(w) {
		return
	}
	credit, err := s.credits.Create(r.Context(), api.kind, input)
	if err != nil {
		s.writeCreditError(w, api, err)
		return
	}
	s.writeCreditBody(w, http.StatusCreated, api.one(credit))
}

// creditInputOf is shared with external create requests; parentId and profile are rejected for the other kind even when null.
func creditInputOf(body gen.CreditInput, present func(name string) bool, kind domain.CreditKind) (domain.CreditInput, string, bool) {
	if present("parentId") && kind != domain.CreditOrganization {
		return domain.CreditInput{}, "parentId applies to organizations only.", false
	}
	if present("profile") && kind != domain.CreditPerson {
		return domain.CreditInput{}, "profile applies to people only.", false
	}
	input := domain.CreditInput{
		Name: body.Name, Aliases: fromAPIAliases(body.Aliases), ExternalIDs: fromAPIExternalIDs(body.ExternalIds),
		ParentID: body.ParentId,
	}
	if body.Profile != nil {
		input.Profile = fromAPIProfile(*body.Profile)
	}
	return input, "", true
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

// PATCH distinguishes omitted fields from null: null lists clear, null parentId removes the parent, and null name is invalid.
func (s *server) readCreditPatch(w http.ResponseWriter, r *http.Request, kind domain.CreditKind) (domain.CreditPatch, bool) {
	var body gen.CreditPatch
	fields, ok := s.readJSONBodyProperties(w, r, &body)
	if !ok {
		return domain.CreditPatch{}, false
	}
	patch, message, ok := creditPatchOf(body, fields, kind)
	if !ok {
		s.invalidRequest(w, message)
	}
	return patch, ok
}

// creditPatchOf is shared with external update requests and receives the raw top-level properties to see nulls.
func creditPatchOf(body gen.CreditPatch, fields bodyFields, kind domain.CreditKind) (domain.CreditPatch, string, bool) {
	present := fields.has
	patch := domain.CreditPatch{Name: body.Name}
	if present("name") && body.Name == nil {
		return domain.CreditPatch{}, "name cannot be null.", false
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
			return domain.CreditPatch{}, "parentId applies to organizations only.", false
		}
		patch.ParentSet, patch.ParentID = true, body.ParentId
	}
	if present("profile") {
		profile, message, ok := profilePatchOf(body.Profile, fields, kind)
		if !ok {
			return domain.CreditPatch{}, message, false
		}
		patch.Profile = profile
	}
	return patch, "", true
}

// profilePatchOf lists the profile properties present, including nulls, so each one replaces or clears its field.
func profilePatchOf(body *gen.PersonProfilePatch, fields bodyFields, kind domain.CreditKind) (*domain.PersonProfilePatch, string, bool) {
	if kind != domain.CreditPerson {
		return nil, "profile applies to people only.", false
	}
	raw, _ := jsonProperty(fields, "profile")
	var properties bodyFields
	if body == nil || json.Unmarshal(raw, &properties) != nil {
		return nil, "profile must be an object; send null for a property to clear it.", false
	}
	patch := &domain.PersonProfilePatch{Values: fromAPIProfilePatch(*body)}
	for name := range properties {
		for _, field := range domain.PersonProfileFields {
			if strings.EqualFold(name, string(field)) {
				patch.Fields = append(patch.Fields, field)
			}
		}
	}
	return patch, "", true
}

// Property-presence matching must use EqualFold to mirror encoding/json field matching.
func (s *server) readJSONBodyFields(w http.ResponseWriter, r *http.Request, target any) (func(name string) bool, bool) {
	fields, ok := s.readJSONBodyProperties(w, r, target)
	if !ok {
		return nil, false
	}
	return fields.has, true
}

func (s *server) readJSONBodyProperties(w http.ResponseWriter, r *http.Request, target any) (bodyFields, bool) {
	raw, ok := s.readJSONBodyRaw(w, r, target)
	if !ok {
		return nil, false
	}
	var fields bodyFields
	if err := json.Unmarshal(raw, &fields); err != nil {
		s.invalidBody(w, err, "Cannot parse the JSON body.")
		return nil, false
	}
	return fields, true
}

// Retain raw body bytes to recover field presence lost by generated types.
func (s *server) readJSONBodyRaw(w http.ResponseWriter, r *http.Request, target any) ([]byte, bool) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		s.invalidBody(w, err, "Cannot read the body.")
		return nil, false
	}
	if !s.decodeJSONBody(w, r, bytes.NewReader(raw), target) {
		return nil, false
	}
	return raw, true
}

// jsonProperty matches names case-insensitively and reports explicit null as present.
func jsonProperty(fields map[string]json.RawMessage, name string) (json.RawMessage, bool) {
	for key, value := range fields {
		if strings.EqualFold(key, name) {
			return value, true
		}
	}
	return nil, false
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

// Storage validates self-merges, parent cycles, and post-merge limits after boundary count checks.
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
		ExternalIds: toAPIExternalIDs(credit.ExternalIDs), Profile: toAPIProfile(credit.Profile),
		VideoCount: credit.VideoCount, CreatedAt: credit.CreatedAt,
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
