package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// People and organization routes share handlers and owner-only defaults; each operation completes through CreditStore without an app use case.

type Credits interface {
	List(ctx context.Context, audience domain.Audience, kind domain.CreditKind, query domain.CreditQuery) (domain.CreditPage, error)
	Get(ctx context.Context, kind domain.CreditKind, id int64) (domain.Credit, error)
	Lookup(ctx context.Context, kind domain.CreditKind, source, value string) (domain.Credit, error)
	Create(ctx context.Context, kind domain.CreditKind, input domain.CreditInput) (domain.Credit, error)
	Update(ctx context.Context, kind domain.CreditKind, id int64, patch domain.CreditPatch) (domain.Credit, error)
	Merge(ctx context.Context, kind domain.CreditKind, targetID int64, sourceIDs []int64) (domain.CreditMergeOutcome, error)
	Delete(ctx context.Context, kind domain.CreditKind, id int64) error
	FaceCandidates(ctx context.Context, personID int64) ([]domain.FacePair, error)
	ConfirmFaceCandidate(ctx context.Context, personID, otherID int64) (domain.FaceCandidateConfirmation, error)
	DismissFaceCandidate(ctx context.Context, personID, otherID int64) error
	ReopenFaceCandidate(ctx context.Context, personID, otherID int64) error
	// PersonAppearances returns domain.ErrInvalidCursor for an unreadable cursor; a nonpositive limit returns all.
	PersonAppearances(ctx context.Context, personID int64, q domain.PersonAppearanceQuery) (domain.PersonAppearancePage, error)
	FaceAppearanceCrop(ctx context.Context, id int64) ([]byte, error)
	// VideoFaceAppearances returns domain.ErrNotFound for a missing video.
	VideoFaceAppearances(ctx context.Context, videoID int64) (domain.VideoFaceAppearances, error)
	// SplitFaceAppearances returns domain.ErrFaceAppearanceNotFound when the person lacks any of the appearances.
	SplitFaceAppearances(ctx context.Context, personID int64, appearanceIDs []int64) (domain.Credit, error)
	// MoveFaceAppearances returns domain.ErrMergeSameCredit for the same person and
	// domain.ErrFaceAppearanceNotFound when the person lacks any of the appearances.
	MoveFaceAppearances(
		ctx context.Context, personID int64, appearanceIDs []int64, targetID int64, restore []domain.FaceMoveCredit,
	) (domain.FaceMove, error)
	SimilarPeople(ctx context.Context, personID int64, appearanceIDs []int64, limit int) ([]domain.SimilarPerson, error)
	// Names are the canonical names of the existing entries among ids; a provisional person has none.
	Names(ctx context.Context, kind domain.CreditKind, ids []int64) (map[int64]string, error)
	// ApplyByName applies a change of the paired node by canonical name, refusing a taken new name with
	// *domain.CreditNameConflict.
	ApplyByName(ctx context.Context, kind domain.CreditKind, change domain.VocabularyChange) (domain.VocabularyOutcome, error)
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
	query, problem := buildCreditListQuery(api.kind, params.Q, sortParam(params.Sort), params.Cursor, params.Limit)
	if !s.acceptTagListQuery(w, problem) {
		return
	}
	if params.Provisional != nil && api.kind != domain.CreditPerson {
		s.invalidRequest(w, "provisional applies to people only.")
		return
	}
	if !s.creditsConfigured(w) {
		return
	}
	query.FavoriteOnly = params.Favorite != nil && *params.Favorite
	query.Provisional = params.Provisional
	if s.mergesVocabulary(r.Context()) {
		if api.kind == domain.CreditPerson {
			listMergedCredits(s, w, r, api.kind, query, peopleMerge)
		} else {
			listMergedCredits(s, w, r, api.kind, query, organizationsMerge)
		}
		return
	}
	page, err := s.credits.List(r.Context(), audienceFrom(r.Context()), api.kind, query)
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

const tagListSortPeopleOnlyMessage = "ageAsc and ageDesc apply to people only."

// buildCreditListQuery is shared with external lists: the tag-list rules plus the age sorts, refused for organizations.
func buildCreditListQuery(kind domain.CreditKind, q, sort, cursor *string, limit *int) (domain.CreditQuery, tagListProblem) {
	shared := sort
	if sort != nil && !domain.TagSort(*sort).Valid() && domain.ValidCreditSort(domain.CreditPerson, domain.TagSort(*sort)) {
		if kind != domain.CreditPerson {
			return domain.CreditQuery{}, tagListSortPeopleOnly
		}
		shared = nil
	}
	tagQuery, problem := buildTagListQuery(q, nil, nil, shared, cursor, limit)
	if problem != tagListOK {
		return domain.CreditQuery{}, problem
	}
	query := domain.CreditQuery{Q: tagQuery.Search, Sort: tagQuery.Sort, Cursor: tagQuery.Cursor, Limit: tagQuery.Limit}
	if shared == nil && sort != nil {
		query.Sort = domain.TagSort(*sort)
	}
	return query, tagListOK
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
	fields, ok := s.readJSONBodyProperties(w, r, &body)
	if !ok {
		return
	}
	input, message, ok := creditInputOf(body, fields, api.kind)
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

// creditInputOf is shared with external create requests; names is required, and parentId, profile and
// organizationProfile are rejected for the other kind even when null.
func creditInputOf(body gen.CreditInput, fields bodyFields, kind domain.CreditKind) (domain.CreditInput, string, bool) {
	present := fields.has
	if !fields.given("names") {
		return domain.CreditInput{}, "names is required and cannot be null.", false
	}
	names, message, ok := requestNames(fields, &body.Names)
	if !ok {
		return domain.CreditInput{}, message, false
	}
	if present("parentId") && kind != domain.CreditOrganization {
		return domain.CreditInput{}, "parentId applies to organizations only.", false
	}
	if present("profile") && kind != domain.CreditPerson {
		return domain.CreditInput{}, "profile applies to people only.", false
	}
	if present("organizationProfile") && kind != domain.CreditOrganization {
		return domain.CreditInput{}, "organizationProfile applies to organizations only.", false
	}
	input := domain.CreditInput{
		Names: names, ExternalIDs: fromAPIExternalIDs(body.ExternalIds), ParentID: body.ParentId,
	}
	if body.Profile != nil {
		input.Profile = fromAPIProfile(*body.Profile)
	}
	if body.OrganizationProfile != nil {
		input.OrganizationProfile = fromAPIOrganizationProfile(*body.OrganizationProfile)
	}
	return input, "", true
}

func (s *server) updateCredit(w http.ResponseWriter, r *http.Request, api creditKindAPI, id int64) {
	patch, ok := s.readCreditPatch(w, r, api.kind)
	if !ok || !s.creditsConfigured(w) || !s.renameCreditOnPeer(w, r, api.kind, id, patchDisplayName(patch)) {
		return
	}
	credit, err := s.credits.Update(r.Context(), api.kind, id, patch)
	if err != nil {
		s.writeCreditError(w, api, err)
		return
	}
	s.writeCreditBody(w, http.StatusOK, api.one(credit))
}

// PATCH distinguishes omitted fields from null: null lists clear, null parentId removes the parent, and null name or
// name with names is invalid.
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
	if present("names") {
		if present("name") {
			return domain.CreditPatch{}, "name and names cannot be sent together.", false
		}
		names, message, ok := requestNames(fields, body.Names)
		if !ok {
			return domain.CreditPatch{}, message, false
		}
		patch.Names = &names
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
	if present("organizationProfile") {
		profile, message, ok := organizationProfilePatchOf(body.OrganizationProfile, fields, kind)
		if !ok {
			return domain.CreditPatch{}, message, false
		}
		patch.OrganizationProfile = profile
	}
	return patch, "", true
}

// profilePatchOf lists the profile properties present, including nulls, so each one replaces or clears its field.
func profilePatchOf(body *gen.PersonProfilePatch, fields bodyFields, kind domain.CreditKind) (*domain.PersonProfilePatch, string, bool) {
	if kind != domain.CreditPerson {
		return nil, "profile applies to people only.", false
	}
	present, message, ok := presentProfileFields(fields, "profile", body != nil, domain.PersonProfileFields)
	if !ok {
		return nil, message, false
	}
	return &domain.PersonProfilePatch{Fields: present, Values: fromAPIProfilePatch(*body)}, "", true
}

func organizationProfilePatchOf(
	body *gen.OrganizationProfilePatch, fields bodyFields, kind domain.CreditKind,
) (*domain.OrganizationProfilePatch, string, bool) {
	if kind != domain.CreditOrganization {
		return nil, "organizationProfile applies to organizations only.", false
	}
	present, message, ok := presentProfileFields(fields, "organizationProfile", body != nil,
		domain.OrganizationProfileFields)
	if !ok {
		return nil, message, false
	}
	return &domain.OrganizationProfilePatch{Fields: present, Values: fromAPIOrganizationProfilePatch(*body)}, "", true
}

// presentProfileFields lists the profile properties present under name, including nulls, so each one replaces or
// clears its field.
func presentProfileFields[F ~string](fields bodyFields, name string, decoded bool, known []F) ([]F, string, bool) {
	raw, _ := jsonProperty(fields, name)
	var properties bodyFields
	if !decoded || json.Unmarshal(raw, &properties) != nil {
		return nil, name + " must be an object; send null for a property to clear it.", false
	}
	var present []F
	for property := range properties {
		for _, field := range known {
			if strings.EqualFold(property, string(field)) {
				present = append(present, field)
			}
		}
	}
	return present, "", true
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
	if !s.creditsConfigured(w) || !s.deleteOnPeer(w, r, vocabularyKindOf(api.kind), id) {
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
	// The peer merges first, so a self-merge is refused before it, not by storage.
	propagate, ok := s.vocabularyPeer(w, r)
	switch {
	case !ok:
		return
	case propagate && slices.Contains(body.SourceIds, id):
		s.writeCreditError(w, api, domain.ErrMergeSameCredit)
		return
	case propagate && onlyLink(domain.RemoteSlot(id), append([]int64{id}, body.SourceIds...)):
		s.mergeOnPeer(w, r, api, id, body.SourceIds)
		return
	case propagate:
		s.mergeOnBothNodes(w, r, id, body.SourceIds, s.creditMergeSide(api))
		return
	}
	s.mergeCreditsHere(w, r, api, id, body.SourceIds, nil)
}

// mergeCreditsHere merges sources into target on this node and answers with the sources missing here or in
// missing.
func (s *server) mergeCreditsHere(w http.ResponseWriter, r *http.Request, api creditKindAPI, target int64, sources, missing []int64) {
	outcome, err := s.credits.Merge(r.Context(), api.kind, target, sources)
	if err != nil {
		s.writeCreditError(w, api, err)
		return
	}
	outcome.NotFoundIDs = unionIDs(missing, outcome.NotFoundIDs)
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
		Id: credit.ID, Name: credit.Name, Provisional: credit.Provisional, Names: toAPINames(credit.Names),
		ExternalIds: toAPIExternalIDs(credit.ExternalIDs), Profile: toAPIProfile(credit.Profile),
		Favorite: credit.Favorite, ImageRevision: optionalPositive(credit.ImageRevision),
		VideoCount: credit.VideoCount, CreatedAt: credit.CreatedAt,
	}
}

func toAPIOrganization(credit domain.Credit) gen.Organization {
	out := gen.Organization{
		Id: credit.ID, Name: credit.Name, Names: toAPINames(credit.Names),
		ExternalIds: toAPIExternalIDs(credit.ExternalIDs), Profile: toAPIOrganizationProfile(credit.OrganizationProfile),
		Favorite: credit.Favorite, ImageRevision: optionalPositive(credit.ImageRevision),
		VideoCount: credit.VideoCount, CreatedAt: credit.CreatedAt,
	}
	if credit.Parent != nil {
		out.Parent = &gen.CreditRef{Id: credit.Parent.ID, Name: credit.Parent.Name}
	}
	return out
}

func toAPINames(names []domain.CreditName) []gen.CreditName {
	out := make([]gen.CreditName, 0, len(names))
	for _, name := range names {
		out = append(out, gen.CreditName{Name: name.Name, Language: optionalString(name.Language)})
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

func fromAPINames(names *[]gen.CreditName) []domain.CreditName {
	if names == nil {
		return []domain.CreditName{}
	}
	out := make([]domain.CreditName, 0, len(*names))
	for _, name := range *names {
		entry := domain.CreditName{Name: name.Name}
		if name.Language != nil {
			entry.Language = *name.Language
		}
		out = append(out, entry)
	}
	return out
}

// requestNames converts the body's names, refusing an entry without a name, which the generated type reads as "".
func requestNames(fields bodyFields, names *[]gen.CreditName) ([]domain.CreditName, string, bool) {
	raw, _ := jsonProperty(fields, "names")
	var entries []bodyFields
	if json.Unmarshal(raw, &entries) == nil {
		for i, entry := range entries {
			if !entry.given("name") {
				return nil, fmt.Sprintf("names[%d].name is required and cannot be null.", i), false
			}
		}
	}
	return fromAPINames(names), "", true
}

// patchDisplayName is the display name a patch sets, or nil when it keeps the current one or empties the list.
func patchDisplayName(patch domain.CreditPatch) *string {
	if patch.Names != nil && len(*patch.Names) > 0 {
		return &(*patch.Names)[0].Name
	}
	return patch.Name
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
