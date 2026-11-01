package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strconv"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// vocabularyMerge is a merge of sources into a destination, split into the change by name for the peer and the
// merge by id here.
type vocabularyMerge struct {
	change domain.VocabularyChange
	// target is this node's destination, 0 when the destination is the peer's.
	target   int64
	sources  []int64
	notFound []int64
}

// planMerge reports found false when target names nothing on either node. A local id without a name, such as a
// provisional person, merges here by id only, since the peer cannot be told about it; the local merge reports a
// local id that does not exist.
func (s *server) planMerge(r *http.Request, kind domain.VocabularyKind, target int64, sources []int64) (vocabularyMerge, bool, error) {
	unique := uniqueInt64s(sources)
	names, err := s.vocabularyNames(r, kind, append([]int64{target}, unique...))
	if err != nil {
		return vocabularyMerge{}, false, err
	}
	into, named := names[target]
	if !named && domain.IsRemoteID(target) {
		return vocabularyMerge{}, false, nil
	}
	plan := vocabularyMerge{
		change:  domain.VocabularyChange{Kind: kind, Action: domain.VocabularyMerge, Into: into},
		sources: []int64{}, notFound: []int64{},
	}
	if !domain.IsRemoteID(target) {
		plan.target = target
	}
	sent := map[string]bool{}
	for _, id := range unique {
		name, named := names[id]
		if !domain.IsRemoteID(id) {
			plan.sources = append(plan.sources, id)
		} else if !named || into == "" {
			plan.notFound = append(plan.notFound, id)
			continue
		}
		if named && into != "" && !sent[name] {
			sent[name] = true
			plan.change.Sources = append(plan.change.Sources, name)
		}
	}
	return plan, true, nil
}

// mergeSide is how one kind merges here once the peer has merged.
type mergeSide struct {
	kind     domain.VocabularyKind
	notFound func(w http.ResponseWriter)
	fail     func(w http.ResponseWriter, err error)
	// entry is this node's entry named name, created when absent.
	entry func(ctx context.Context, name string) (int64, error)
	merge func(w http.ResponseWriter, r *http.Request, target int64, sources, notFound []int64)
	// remote answers with the peer's destination when nothing merges here.
	remote func(w http.ResponseWriter, r *http.Request, name string, notFound []int64)
}

// mergeOnBothNodes merges by name on the peer, then this node's sources into its entry of the destination's
// name, which is created when only the peer held it.
func (s *server) mergeOnBothNodes(w http.ResponseWriter, r *http.Request, target int64, sources []int64, side mergeSide) {
	plan, found, err := s.planMerge(r, side.kind, target, sources)
	switch {
	case err != nil:
		s.vocabularyFailure(w, err)
		return
	case !found:
		side.notFound(w)
		return
	}
	if len(plan.change.Sources) > 0 {
		if _, ok := s.sendVocabulary(w, r, plan.change); !ok {
			return
		}
	}
	if plan.target == 0 && len(plan.sources) > 0 {
		if plan.target, err = side.entry(r.Context(), plan.change.Into); err != nil {
			side.fail(w, err)
			return
		}
	}
	if plan.target == 0 {
		side.remote(w, r, plan.change.Into, plan.notFound)
		return
	}
	side.merge(w, r, plan.target, plan.sources, plan.notFound)
}

// mergeOnPeer merges the peer's sources into the peer's target by id. Ids in the offset form name no entry of this
// node, so nothing merges here, and a provisional person, such as the one a face split makes, has no name to send.
func (s *server) mergeOnPeer(w http.ResponseWriter, r *http.Request, api creditKindAPI, target int64, sources []int64) {
	peerSources := slices.Clone(sources)
	toPeer(&peerSources)
	collection := peopleMerge.path
	if api.kind == domain.CreditOrganization {
		collection = organizationsMerge.path
	}
	var answer struct {
		NotFoundIds  []int64           `json:"notFoundIds"`
		Person       *gen.Person       `json:"person,omitempty"`
		Organization *gen.Organization `json:"organization,omitempty"`
	}
	err := s.peerCall(r, http.MethodPost, collection+"/"+strconv.FormatInt(domain.FromRemoteID(target), 10)+"/merge",
		url.Values{}, gen.CreditMergeRequest{SourceIds: peerSources}, &answer)
	var refusal *peerRefusal
	switch {
	case errors.As(err, &refusal) && refusal.problem != nil:
		s.writeErrorBody(w, refusal.status, *refusal.problem)
		return
	case err != nil:
		s.vocabularyFailure(w, err)
		return
	}
	join, err := s.vocabularyJoin(r.Context())
	if err != nil {
		s.vocabularyFailure(w, err)
		return
	}
	for i, id := range answer.NotFoundIds {
		answer.NotFoundIds[i] = domain.ToRemoteID(id)
	}
	if answer.Person != nil {
		rewriteRemotePerson(answer.Person, join)
		s.writeCreditBody(w, http.StatusOK, gen.PersonMergeResponse{Person: *answer.Person, NotFoundIds: nonNilIDs(answer.NotFoundIds)})
		return
	}
	if answer.Organization != nil {
		rewriteRemoteOrganizationEntity(answer.Organization, join)
		s.writeCreditBody(w, http.StatusOK,
			gen.OrganizationMergeResponse{Organization: *answer.Organization, NotFoundIds: nonNilIDs(answer.NotFoundIds)})
		return
	}
	s.internalError(w, "The other VVMDM answered the merge without the "+string(api.kind)+".", nil)
}

// peerEntryNamed is the peer's entry named name from its whole list at path, in this node's id space.
func peerEntryNamed[T any](s *server, r *http.Request, path string, rewrite func(*T, domain.VocabularyJoin),
	nameOf func(T) string, name string,
) (T, bool, error) {
	var answer struct {
		Items []T `json:"items"`
	}
	var zero T
	if err := s.peerGet(r, path, url.Values{}, &answer); err != nil {
		return zero, false, err
	}
	join, err := s.vocabularyJoin(r.Context())
	if err != nil {
		return zero, false, err
	}
	for _, item := range answer.Items {
		if nameOf(item) == name {
			rewrite(&item, join)
			return item, true, nil
		}
	}
	return zero, false, nil
}

// unionIDs is a followed by the ids of b it lacks.
func unionIDs(a, b []int64) []int64 {
	return uniqueInt64s(append(append([]int64{}, a...), b...))
}

func (s *server) tagMergeSide() mergeSide {
	notFound := func(w http.ResponseWriter) { s.writeTagError(w, domain.ErrTagNotFound, "") }
	return mergeSide{
		kind:     domain.VocabularyTag,
		notFound: notFound,
		fail:     func(w http.ResponseWriter, err error) { s.writeTagError(w, err, "") },
		entry: func(ctx context.Context, name string) (int64, error) {
			owner, found, err := s.tags.LookupTagName(ctx, name)
			if err != nil || found {
				return owner.ID, err
			}
			tag, err := s.tags.CreateTag(ctx, name)
			return tag.ID, err
		},
		merge: s.mergeTagsHere,
		remote: func(w http.ResponseWriter, r *http.Request, name string, missing []int64) {
			tag, found, err := peerEntryNamed(s, r, "/api/tags", rewriteRemoteTag, func(tag gen.Tag) string { return tag.Name }, name)
			switch {
			case err != nil:
				s.vocabularyFailure(w, err)
			case !found:
				notFound(w)
			default:
				w.Header().Set("Cache-Control", cacheNoStore)
				writeJSON(w, http.StatusOK, gen.TagMergeResponse{Tag: tag, NotFoundIds: missing}, s.logger)
			}
		},
	}
}

func (s *server) creditMergeSide(api creditKindAPI) mergeSide {
	notFound := func(w http.ResponseWriter) { s.writeCreditError(w, api, domain.ErrNotFound) }
	return mergeSide{
		kind:     vocabularyKindOf(api.kind),
		notFound: notFound,
		fail:     func(w http.ResponseWriter, err error) { s.writeCreditError(w, api, err) },
		entry: func(ctx context.Context, name string) (int64, error) {
			join, err := s.vocabularyJoin(ctx)
			if err != nil {
				return 0, err
			}
			holders := join.People
			if api.kind == domain.CreditOrganization {
				holders = join.Organizations
			}
			if id, found := holders[name]; found {
				return id, nil
			}
			credit, err := s.credits.Create(ctx, api.kind, domain.CreditInput{Name: name})
			return credit.ID, err
		},
		merge: func(w http.ResponseWriter, r *http.Request, target int64, sources, missing []int64) {
			s.mergeCreditsHere(w, r, api, target, sources, missing)
		},
		remote: func(w http.ResponseWriter, r *http.Request, name string, missing []int64) {
			body, found, err := s.peerCreditMerged(r, api.kind, name, missing)
			switch {
			case err != nil:
				s.vocabularyFailure(w, err)
			case !found:
				notFound(w)
			default:
				s.writeCreditBody(w, http.StatusOK, body)
			}
		},
	}
}

// peerCreditMerged is the merge answer with the peer's person or organization named name as the destination.
func (s *server) peerCreditMerged(r *http.Request, kind domain.CreditKind, name string, missing []int64) (any, bool, error) {
	if kind == domain.CreditPerson {
		person, found, err := peerEntryNamed(s, r, peopleMerge.path, peopleMerge.rewrite,
			func(person gen.Person) string { return person.Name }, name)
		return gen.PersonMergeResponse{Person: person, NotFoundIds: missing}, found, err
	}
	organization, found, err := peerEntryNamed(s, r, organizationsMerge.path, organizationsMerge.rewrite,
		func(organization gen.Organization) string { return organization.Name }, name)
	return gen.OrganizationMergeResponse{Organization: organization, NotFoundIds: missing}, found, err
}

// batchDeleteOnBothNodes deletes the tags of ids on the peer by name, one request each, and then this node's.
// A deletion takes confirmed tags only, so a tag tentative here is not sent and stays inapplicable.
func (s *server) batchDeleteOnBothNodes(w http.ResponseWriter, r *http.Request, req domain.TagBatchRequest, ids []int64) {
	unique := uniqueInt64s(ids)
	names, err := s.vocabularyNames(r, domain.VocabularyTag, unique)
	if err != nil {
		s.vocabularyFailure(w, err)
		return
	}
	applied := map[int64]bool{}
	local := []int64{}
	for _, id := range unique {
		name, found := names[id]
		if !found {
			continue
		}
		if !domain.IsRemoteID(id) {
			local = append(local, id)
			owner, _, err := s.tags.LookupTagName(r.Context(), name)
			if err != nil {
				s.internalError(w, "Could not update the tags.", err)
				return
			}
			if owner.Tentative {
				continue
			}
		}
		outcome, ok := s.sendVocabulary(w, r, domain.VocabularyChange{Kind: domain.VocabularyTag, Action: domain.VocabularyDelete, Name: name})
		if !ok {
			return
		}
		applied[id] = outcome.Changed
	}
	var outcome domain.TagBatchOutcome
	if len(local) > 0 {
		if outcome, err = s.tags.BatchTags(r.Context(), req, local); err != nil {
			s.internalError(w, "Could not update the tags.", err)
			return
		}
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, batchDeleteResponse(unique, applied, outcome), s.logger)
}

// batchDeleteResponse sorts ids in input order: a local id as the local batch reported it, a remote id applied
// when the peer deleted it, and anything else not found.
func batchDeleteResponse(ids []int64, remoteApplied map[int64]bool, local domain.TagBatchOutcome) gen.TagBatchResponse {
	body := gen.TagBatchResponse{AppliedIds: []int64{}, NotFoundIds: []int64{}, NotApplicableIds: []int64{}}
	appliedHere := map[int64]bool{}
	for _, id := range local.AppliedIDs {
		appliedHere[id] = true
	}
	inapplicable := map[int64]bool{}
	for _, id := range local.NotApplicableIDs {
		inapplicable[id] = true
	}
	for _, id := range ids {
		switch {
		case appliedHere[id] || (domain.IsRemoteID(id) && remoteApplied[id]):
			body.AppliedIds = append(body.AppliedIds, id)
		case inapplicable[id]:
			body.NotApplicableIds = append(body.NotApplicableIds, id)
		default:
			body.NotFoundIds = append(body.NotFoundIds, id)
		}
	}
	return body
}
