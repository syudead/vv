package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// vocabularyJoin holds this node's names for rewriting the peer's answers; without a library nothing joins.
func (s *server) vocabularyJoin(ctx context.Context) (domain.VocabularyJoin, error) {
	if s.videos == nil {
		return domain.VocabularyJoin{}, nil
	}
	return s.videos.VocabularyJoin(ctx)
}

// mergesVocabulary reports whether the tag, kind, person and organization lists answer with both nodes' entries.
func (s *server) mergesVocabulary(ctx context.Context) bool {
	return s.readsPeer(ctx)
}

// peerVocabulary reads the peer's whole list at path into out and rewrites it into this node's id space; a peer
// that does not answer leaves the list with the local entries.
func peerVocabulary[T any](s *server, r *http.Request, path string, out *[]T, rewrite func(*T, domain.VocabularyJoin)) error {
	var answer struct {
		Items []T `json:"items"`
	}
	if err := s.peerGet(r, path, url.Values{}, &answer); err != nil {
		s.logger.Info("the list leaves out the other VVMDM", slog.String("path", path), slog.Any("error", err))
		return nil
	}
	join, err := s.vocabularyJoin(r.Context())
	if err != nil {
		return err
	}
	for i := range answer.Items {
		rewrite(&answer.Items[i], join)
	}
	*out = answer.Items
	return nil
}

// vocabularyEntries pairs each value with what merging, searching and sorting read from it.
func vocabularyEntries[T any](values []T, entry func(T) domain.VocabularyEntry[T]) []domain.VocabularyEntry[T] {
	entries := make([]domain.VocabularyEntry[T], len(values))
	for i, value := range values {
		entries[i] = entry(value)
		entries[i].Value = value
	}
	return entries
}

// vocabularyPage keeps the merged entries keep accepts, sorts them and pages them as one node's list does.
func vocabularyPage[T any](
	merged []domain.VocabularyEntry[T], keep func(domain.VocabularyEntry[T]) bool, sort domain.TagSort, cursor string, limit int,
) (items []T, total int, next *string, err error) {
	kept := slices.DeleteFunc(merged, func(entry domain.VocabularyEntry[T]) bool { return !keep(entry) })
	domain.SortVocabulary(kept, sort)
	page, nextCursor, err := domain.PageVocabulary(kept, sort, cursor, limit)
	if err != nil {
		return nil, 0, nil, err
	}
	items = make([]T, len(page))
	for i, entry := range page {
		items[i] = entry.Value
	}
	return items, len(kept), optionalString(nextCursor), nil
}

func tagEntry(tag gen.Tag) domain.VocabularyEntry[gen.Tag] {
	return domain.VocabularyEntry[gen.Tag]{
		ID: tag.Id, Name: tag.Name, Count: tag.VideoCount, CreatedAt: tag.CreatedAt.Unix(),
		Names: append([]string{tag.Name}, tag.Synonyms...),
	}
}

// addTags sums the video counts; the rest is the local tag's, as synonyms, kinds and the tentative state stay
// with each node.
func addTags(local, peer gen.Tag) gen.Tag {
	local.VideoCount += peer.VideoCount
	return local
}

func tagKindFilterMatches(filter domain.TagKindFilter, kind *gen.TagKindRef) bool {
	switch {
	case filter.None:
		return kind == nil
	case filter.ID != 0:
		return kind != nil && kind.Id == filter.ID
	}
	return true
}

// listMergedTags lists both nodes' tags joined by name; the filters apply to the joined entries, so a tag
// unused here but used on the peer is not unused.
func (s *server) listMergedTags(w http.ResponseWriter, r *http.Request, query domain.TagListQuery) {
	all, err := s.tags.ListTags(r.Context(), domain.TagListQuery{Sort: domain.TagSortName})
	if err != nil {
		s.internalError(w, "Could not load tags.", err)
		return
	}
	local := make([]gen.Tag, len(all.Items))
	for i, tag := range all.Items {
		local[i] = toAPITag(tag)
	}
	var remote []gen.Tag
	if err := peerVocabulary(s, r, "/api/tags", &remote, rewriteRemoteTag); err != nil {
		s.internalError(w, "Could not load tags.", err)
		return
	}
	merged := domain.MergeVocabulary(vocabularyEntries(local, tagEntry), vocabularyEntries(remote, tagEntry), addTags)
	keep := func(entry domain.VocabularyEntry[gen.Tag]) bool {
		return entry.MatchesSearch(query.Search) && (!query.TentativeOnly || entry.Value.Tentative) &&
			(!query.UnusedOnly || entry.Count == 0) && tagKindFilterMatches(query.Kind, entry.Value.Kind)
	}
	body := gen.TagList{TotalAll: len(merged)}
	exactName := strings.TrimSpace(query.Search)
	if query.Limit > 0 && query.Cursor == "" && exactName != "" {
		for _, entry := range merged {
			if slices.Contains(entry.Names, exactName) && keep(entry) {
				exact := entry.Value
				body.Exact = &exact
				break
			}
		}
	}
	body.Items, body.Total, body.NextCursor, err = vocabularyPage(merged, keep, query.Sort, query.Cursor, query.Limit)
	if !s.checkListRead(w, err) {
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, body, s.logger)
}

// listMergedTagKinds lists both nodes' kinds joined by name in id order: this node's, then the peer's others.
func (s *server) listMergedTagKinds(w http.ResponseWriter, r *http.Request, local []gen.TagKind) {
	var remote []gen.TagKind
	if err := peerVocabulary(s, r, "/api/tag-kinds", &remote, rewriteRemoteTagKind); err != nil {
		s.internalError(w, "Could not load tag kinds.", err)
		return
	}
	entry := func(kind gen.TagKind) domain.VocabularyEntry[gen.TagKind] {
		return domain.VocabularyEntry[gen.TagKind]{ID: kind.Id, Name: kind.Name, Count: kind.TagCount}
	}
	merged := domain.MergeVocabulary(vocabularyEntries(local, entry), vocabularyEntries(remote, entry),
		func(local, peer gen.TagKind) gen.TagKind {
			local.TagCount += peer.TagCount
			return local
		})
	items := make([]gen.TagKind, len(merged))
	for i, kind := range merged {
		items[i] = kind.Value
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.TagKindList{Items: items}, s.logger)
}

// creditMerge is how one credit kind's list merges: its API value, its peer path and rewrite, and its body.
type creditMerge[T any] struct {
	path     string
	toAPI    func(domain.Credit) T
	entry    func(T) domain.VocabularyEntry[T]
	rewrite  func(*T, domain.VocabularyJoin)
	add      func(local, peer T) T
	favorite func(T) bool
	list     func(items []T, total, totalAll int, next *string) any
}

var peopleMerge = creditMerge[gen.Person]{
	path:  "/api/people",
	toAPI: toAPIPerson,
	entry: func(person gen.Person) domain.VocabularyEntry[gen.Person] {
		entry := domain.VocabularyEntry[gen.Person]{
			ID: person.Id, Name: person.Name, Provisional: person.Provisional, Count: person.VideoCount,
			CreatedAt: person.CreatedAt.Unix(), Names: creditNames(person.Name, person.Aliases),
		}
		if person.Profile.BirthDate != nil {
			entry.BirthKey, _ = domain.BirthKey(*person.Profile.BirthDate)
		}
		return entry
	},
	rewrite: rewriteRemotePerson,
	add: func(local, peer gen.Person) gen.Person {
		local.VideoCount += peer.VideoCount
		return local
	},
	favorite: func(person gen.Person) bool { return person.Favorite },
	list: func(items []gen.Person, total, totalAll int, next *string) any {
		return gen.PersonList{Items: items, Total: total, TotalAll: totalAll, NextCursor: next}
	},
}

var organizationsMerge = creditMerge[gen.Organization]{
	path:  "/api/organizations",
	toAPI: toAPIOrganization,
	entry: func(organization gen.Organization) domain.VocabularyEntry[gen.Organization] {
		return domain.VocabularyEntry[gen.Organization]{
			ID: organization.Id, Name: organization.Name, Count: organization.VideoCount,
			CreatedAt: organization.CreatedAt.Unix(), Names: creditNames(organization.Name, organization.Aliases),
		}
	},
	rewrite: rewriteRemoteOrganizationEntity,
	add: func(local, peer gen.Organization) gen.Organization {
		local.VideoCount += peer.VideoCount
		return local
	},
	favorite: func(gen.Organization) bool { return false },
	list: func(items []gen.Organization, total, totalAll int, next *string) any {
		return gen.OrganizationList{Items: items, Total: total, TotalAll: totalAll, NextCursor: next}
	},
}

func creditNames(name string, aliases []gen.Alias) []string {
	names := []string{name}
	for _, alias := range aliases {
		names = append(names, alias.Name)
	}
	return names
}

// listMergedCredits lists both nodes' people or organizations joined by name. Favorites stay with each node, so
// a favorite filter keeps this node's favorites only.
func listMergedCredits[T any](s *server, w http.ResponseWriter, r *http.Request, kind domain.CreditKind, query domain.CreditQuery,
	merge creditMerge[T],
) {
	all, err := s.credits.List(r.Context(), kind, domain.CreditQuery{Sort: domain.TagSortName})
	if err != nil {
		s.internalError(w, "Could not load the "+string(kind)+" list.", err)
		return
	}
	local := make([]T, len(all.Items))
	for i, credit := range all.Items {
		local[i] = merge.toAPI(credit)
	}
	var remote []T
	if err := peerVocabulary(s, r, merge.path, &remote, merge.rewrite); err != nil {
		s.internalError(w, "Could not load the "+string(kind)+" list.", err)
		return
	}
	merged := domain.MergeVocabulary(vocabularyEntries(local, merge.entry), vocabularyEntries(remote, merge.entry), merge.add)
	keep := func(entry domain.VocabularyEntry[T]) bool {
		return entry.MatchesSearch(query.Q) && (!query.FavoriteOnly || merge.favorite(entry.Value)) &&
			(query.Provisional == nil || *query.Provisional == entry.Provisional)
	}
	totalAll := len(merged)
	items, total, next, err := vocabularyPage(merged, keep, query.Sort, query.Cursor, query.Limit)
	if !s.checkListRead(w, err) {
		return
	}
	s.writeCreditBody(w, http.StatusOK, merge.list(items, total, totalAll, next))
}

// remoteOrganizationPatch sends the parent of a forwarded organization edit as the peer's id: a remote parent
// loses the offset, and a local one goes as the peer's organization of the same name, which must exist there.
func (s *server) remoteOrganizationPatch(w http.ResponseWriter, r *http.Request) bool {
	var patch gen.CreditPatch
	raw, ok := s.readJSONBodyRaw(w, r, &patch)
	if !ok {
		return false
	}
	if patch.ParentId != nil {
		parentID, ok := s.peerOrganizationID(w, r, *patch.ParentId)
		if !ok {
			return false
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			s.invalidBody(w, err, "Cannot parse the JSON body.")
			return false
		}
		for key := range fields {
			if strings.EqualFold(key, "parentId") {
				delete(fields, key)
			}
		}
		fields["parentId"] = json.RawMessage(strconv.FormatInt(parentID, 10))
		var err error
		if raw, err = json.Marshal(fields); err != nil {
			s.internalError(w, "Could not forward the request to the other VVMDM.", err)
			return false
		}
	}
	r.Body, r.ContentLength = io.NopCloser(bytes.NewReader(raw)), int64(len(raw))
	return true
}

// peerOrganizationID is the peer's id of the organization id names on the screens; among the peer's
// organizations of a local organization's name it takes the lowest id, as the join does.
func (s *server) peerOrganizationID(w http.ResponseWriter, r *http.Request, id int64) (int64, bool) {
	if domain.IsRemoteID(id) {
		return domain.FromRemoteID(id), true
	}
	local, err := s.credits.Get(r.Context(), domain.CreditOrganization, id)
	if errors.Is(err, domain.ErrNotFound) {
		s.notFoundReason(w, gen.ErrorReasonOrganizationNotFound, "The parent organization was not found.")
		return 0, false
	}
	if err != nil {
		s.internalError(w, "Could not load the parent organization.", err)
		return 0, false
	}
	var answer gen.OrganizationList
	if err := s.peerGet(r, "/api/organizations", url.Values{"q": {local.Name}}, &answer); err != nil {
		if errors.Is(err, domain.ErrPeerUnreachable) {
			s.peerUnreachable(w)
		} else {
			s.internalError(w, "Could not read the organizations of the other VVMDM.", err)
		}
		return 0, false
	}
	var peerID int64
	for _, organization := range answer.Items {
		if organization.Name == local.Name && (peerID == 0 || organization.Id < peerID) {
			peerID = organization.Id
		}
	}
	if peerID == 0 {
		s.notFoundReason(w, gen.ErrorReasonOrganizationNotFound,
			"The parent organization does not exist on the other VVMDM.")
		return 0, false
	}
	return peerID, true
}
