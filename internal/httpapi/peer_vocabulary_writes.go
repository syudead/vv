package httpapi

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"slices"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// While paired, a rename, merge or deletion of a tag, person or organization is applied on the peer by name first
// and then here, so the common failure, the peer, leaves both nodes unchanged; a failure here after the peer
// applied it is answered as is and the owner repeats the request.

// maxPeerVocabularyBody fits a merge of the most sources a merge takes.
const maxPeerVocabularyBody = 8 << 20

var vocabularyListPaths = map[domain.VocabularyKind]string{
	domain.VocabularyTag: "/api/tags", domain.VocabularyPerson: "/api/people",
	domain.VocabularyOrganization: "/api/organizations",
}

func vocabularyKindOf(kind domain.CreditKind) domain.VocabularyKind {
	if kind == domain.CreditPerson {
		return domain.VocabularyPerson
	}
	return domain.VocabularyOrganization
}

// ApplyPeerVocabulary applies the peer's change to this node's entry of the same name.
func (s *server) ApplyPeerVocabulary(w http.ResponseWriter, r *http.Request) {
	if !s.requirePeerCaller(w, r) {
		return
	}
	var body gen.PeerVocabularyRequest
	if !s.readLargeJSONBody(w, r, maxPeerVocabularyBody, &body) {
		return
	}
	change := domain.VocabularyChange{
		Kind: domain.VocabularyKind(body.Kind), Action: domain.VocabularyAction(body.Action),
		Name: valueOrEmpty(body.Name), NewName: valueOrEmpty(body.NewName), Into: valueOrEmpty(body.Into),
	}
	if body.Sources != nil {
		change.Sources = *body.Sources
	}
	if !change.Valid() {
		s.invalidRequest(w, "The change needs a kind, an action and the names that action takes.")
		return
	}
	if s.tags == nil || s.credits == nil {
		s.internalError(w, "Tag, people and organization storage is not configured.", nil)
		return
	}
	var outcome domain.VocabularyOutcome
	var err error
	if kind, ok := change.Kind.CreditKind(); ok {
		outcome, err = s.credits.ApplyByName(r.Context(), kind, change)
	} else {
		outcome, err = s.tags.ApplyTagByName(r.Context(), change)
	}
	if err != nil {
		s.writeVocabularyError(w, err)
		return
	}
	if outcome.Skipped == nil {
		outcome.Skipped = []string{}
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.PeerVocabularyResult{Changed: outcome.Changed, Skipped: outcome.Skipped}, s.logger)
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (s *server) writeVocabularyError(w http.ResponseWriter, err error) {
	var tagConflict *domain.TagNameConflict
	var creditConflict *domain.CreditNameConflict
	switch {
	case errors.As(err, &tagConflict):
		name := tagConflict.Tag.Name
		s.writeErrorBody(w, http.StatusConflict, gen.Error{
			Code: codeConflict, Reason: reasonRef(gen.ErrorReasonTagNameTaken), TagName: &name,
			Message: "Another tag already holds the name.",
		})
	case errors.As(err, &creditConflict):
		s.creditNameTaken(w, creditConflict.Kind)
	case errors.Is(err, domain.ErrInvalidTagName):
		s.invalidTagName(w, err, "")
	default:
		if failure, ok := workInfoFailureOf(err); ok {
			s.writeWorkInfoFailure(w, failure)
			return
		}
		s.internalError(w, "Could not apply the change of the other VVMDM.", err)
	}
}

func (s *server) creditNameTaken(w http.ResponseWriter, kind domain.CreditKind) {
	reason := gen.ErrorReasonPersonNameTaken
	if kind == domain.CreditOrganization {
		reason = gen.ErrorReasonOrganizationNameTaken
	}
	s.conflictReason(w, reason, "Another "+string(kind)+" on this or the other VVMDM already has the name.")
}

// vocabularyPeer reports whether a rename, merge or deletion also goes to the peer: the node is paired and the
// request is not the peer's own. While the peer is not connected it answers 409 peer_unreachable and ok is false.
func (s *server) vocabularyPeer(w http.ResponseWriter, r *http.Request) (propagate, ok bool) {
	if s.pairing == nil || isPeerRequest(r.Context()) {
		return false, true
	}
	state, err := s.pairing.State(r.Context())
	switch {
	case err != nil:
		s.internalError(w, "Could not read the pairing.", err)
		return false, false
	case state.State == domain.PeerUnpaired:
		return false, true
	case state.State == domain.PeerConnected:
		return true, true
	}
	s.peerNotConnected(w)
	return false, false
}

func (s *server) peerNotConnected(w http.ResponseWriter) {
	s.conflictReason(w, gen.ErrorReasonPeerUnreachable, "The other VVMDM is not connected, so nothing was changed.")
}

// vocabularyFailure answers an error of reading or writing the peer's vocabulary.
func (s *server) vocabularyFailure(w http.ResponseWriter, err error) {
	if errors.Is(err, domain.ErrPeerUnreachable) {
		s.peerNotConnected(w)
		return
	}
	s.internalError(w, "Could not apply the change on the other VVMDM.", err)
}

// sendVocabulary applies change on the peer; when the peer refuses it, it answers w with the peer's error and
// reports false.
func (s *server) sendVocabulary(w http.ResponseWriter, r *http.Request, change domain.VocabularyChange) (domain.VocabularyOutcome, bool) {
	body := gen.PeerVocabularyRequest{
		Kind: gen.PeerVocabularyRequestKind(change.Kind), Action: gen.PeerVocabularyRequestAction(change.Action),
		Name: optionalString(change.Name), NewName: optionalString(change.NewName), Into: optionalString(change.Into),
	}
	if len(change.Sources) > 0 {
		body.Sources = &change.Sources
	}
	var result gen.PeerVocabularyResult
	err := s.peerCall(r, http.MethodPost, "/api/peer/vocabulary", url.Values{}, body, &result)
	var refusal *peerRefusal
	switch {
	case err == nil:
		return domain.VocabularyOutcome{Changed: result.Changed, Skipped: result.Skipped}, true
	case errors.As(err, &refusal) && refusal.problem != nil:
		problem := *refusal.problem
		if problem.Reason != nil && *problem.Reason == gen.ErrorReasonTagNameTaken {
			s.writeTagNameTaken(w, change.NewName, domain.TagRef{Name: valueOrDefault(problem.TagName, change.NewName)})
			return domain.VocabularyOutcome{}, false
		}
		s.writeErrorBody(w, refusal.status, problem)
	default:
		s.vocabularyFailure(w, err)
	}
	return domain.VocabularyOutcome{}, false
}

func valueOrDefault(value *string, fallback string) string {
	if value == nil {
		return fallback
	}
	return *value
}

// vocabularyNames are the canonical names of ids: this node's from its store and the peer's from its whole list.
func (s *server) vocabularyNames(r *http.Request, kind domain.VocabularyKind, ids []int64) (map[int64]string, error) {
	local, remote := domain.SplitRemoteIDs(ids)
	names := map[int64]string{}
	if len(local) > 0 {
		var err error
		if creditKind, ok := kind.CreditKind(); ok {
			names, err = s.credits.Names(r.Context(), creditKind, local)
		} else {
			names, err = s.tags.TagNames(r.Context(), local)
		}
		if err != nil {
			return nil, err
		}
	}
	if len(remote) == 0 {
		return names, nil
	}
	items, err := s.peerEntries(r, kind)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if item.Name != "" && slices.Contains(remote, item.ID) {
			names[domain.ToRemoteID(item.ID)] = item.Name
		}
	}
	return names, nil
}

// peerVocabularyItem is an entry of the peer's whole list of a kind, in the peer's id space, as peerEntries reads it.
type peerVocabularyItem struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func (s *server) peerEntries(r *http.Request, kind domain.VocabularyKind) ([]peerVocabularyItem, error) {
	var answer struct {
		Items []peerVocabularyItem `json:"items"`
	}
	err := s.peerGet(r, vocabularyListPaths[kind], url.Values{}, &answer)
	return answer.Items, err
}

// restoreBody lets the next reader of r read raw again.
func restoreBody(r *http.Request, raw []byte) {
	r.Body, r.ContentLength = io.NopCloser(bytes.NewReader(raw)), int64(len(raw))
}

// renameTagOnPeer renames the peer's tag of id's name before RenameTag renames id; it answers w and reports false
// when the rename stops here. A name the rule refuses, or a tag that is not here, is left to the local rename.
func (s *server) renameTagOnPeer(w http.ResponseWriter, r *http.Request, id int64, name string) bool {
	normalized, err := domain.NormalizeTagName(name)
	if err != nil {
		return true
	}
	propagate, ok := s.vocabularyPeer(w, r)
	if !ok || !propagate {
		return ok
	}
	names, err := s.tags.TagNames(r.Context(), []int64{id})
	if err != nil {
		s.internalError(w, "Could not load the tag.", err)
		return false
	}
	current, found := names[id]
	if !found || current == normalized {
		return true
	}
	if !s.tagNameFree(w, r.Context(), normalized) {
		return false
	}
	_, ok = s.sendVocabulary(w, r, domain.VocabularyChange{
		Kind: domain.VocabularyTag, Action: domain.VocabularyRename, Name: current, NewName: normalized,
	})
	return ok
}

// tagNameFree answers 409 tag_name_taken and reports false when a tag here holds the normalized name.
func (s *server) tagNameFree(w http.ResponseWriter, ctx context.Context, normalized string) bool {
	owner, taken, err := s.tags.LookupTagName(ctx, normalized)
	switch {
	case err != nil:
		s.internalError(w, "Could not look up the tag name.", err)
		return false
	case taken:
		s.writeTagNameTaken(w, normalized, owner)
		return false
	}
	return true
}

// checkRemoteTagRename refuses the forwarded rename of the peer's tag to a name a tag here holds.
func (s *server) checkRemoteTagRename(w http.ResponseWriter, r *http.Request) bool {
	var body gen.RenameTagRequest
	raw, ok := s.readJSONBodyRaw(w, r, &body)
	if !ok {
		return false
	}
	restoreBody(r, raw)
	normalized, err := domain.NormalizeTagName(body.Name)
	return err != nil || s.tagNameFree(w, r.Context(), normalized)
}

// checkRemoteCreditRename refuses the forwarded rename of the peer's person or organization to a name an entry
// here holds.
func checkRemoteCreditRename(kind domain.CreditKind) func(*server, http.ResponseWriter, *http.Request) bool {
	return func(s *server, w http.ResponseWriter, r *http.Request) bool {
		var body gen.CreditPatch
		raw, ok := s.readJSONBodyRaw(w, r, &body)
		if !ok {
			return false
		}
		restoreBody(r, raw)
		if body.Name == nil {
			return true
		}
		normalized, err := domain.NormalizeCredit(domain.CreditInput{Name: *body.Name})
		return err != nil || s.creditNameFree(w, r.Context(), kind, 0, normalized.Name)
	}
}

// creditNameFree answers 409 with the kind's name_taken reason and reports false when an entry here other than id
// holds the name.
func (s *server) creditNameFree(w http.ResponseWriter, ctx context.Context, kind domain.CreditKind, id int64, name string) bool {
	join, err := s.vocabularyJoin(ctx)
	if err != nil {
		s.internalError(w, "Could not read the names of people and organizations.", err)
		return false
	}
	holders := join.People
	if kind == domain.CreditOrganization {
		holders = join.Organizations
	}
	if holder, taken := holders[name]; taken && holder != id {
		s.creditNameTaken(w, kind)
		return false
	}
	return true
}

// peerCreditNameFree answers 409 with the kind's name_taken reason and reports false when the peer holds name; a
// provisional person has no name to rename on the peer, so the peer's list is read instead.
func (s *server) peerCreditNameFree(w http.ResponseWriter, r *http.Request, kind domain.CreditKind, name string) bool {
	items, err := s.peerEntries(r, vocabularyKindOf(kind))
	if err != nil {
		s.vocabularyFailure(w, err)
		return false
	}
	if slices.ContainsFunc(items, func(item peerVocabularyItem) bool { return item.Name == name }) {
		s.creditNameTaken(w, kind)
		return false
	}
	return true
}

// renameCreditOnPeer renames the peer's entry of id's name before the local update; a provisional person has no
// name the peer could hold, so only the new name is checked on both nodes.
func (s *server) renameCreditOnPeer(w http.ResponseWriter, r *http.Request, kind domain.CreditKind, id int64, name *string) bool {
	if name == nil {
		return true
	}
	normalized, err := domain.NormalizeCredit(domain.CreditInput{Name: *name})
	if err != nil {
		return true
	}
	propagate, ok := s.vocabularyPeer(w, r)
	if !ok || !propagate {
		return ok
	}
	current, err := s.credits.Get(r.Context(), kind, id)
	if err != nil || current.Name == normalized.Name {
		return true
	}
	if !s.creditNameFree(w, r.Context(), kind, id, normalized.Name) {
		return false
	}
	if current.Name == "" {
		return s.peerCreditNameFree(w, r, kind, normalized.Name)
	}
	_, ok = s.sendVocabulary(w, r, domain.VocabularyChange{
		Kind: vocabularyKindOf(kind), Action: domain.VocabularyRename, Name: current.Name, NewName: normalized.Name,
	})
	return ok
}

// deleteOnPeer deletes the peer's entry of id's name and reports whether the local deletion follows; the peer's
// own entry has none, so it is answered here. An id that names nothing is left to the local deletion's 404.
func (s *server) deleteOnPeer(w http.ResponseWriter, r *http.Request, kind domain.VocabularyKind, id int64) bool {
	propagate, ok := s.vocabularyPeer(w, r)
	if !ok || !propagate {
		return ok
	}
	names, err := s.vocabularyNames(r, kind, []int64{id})
	if err != nil {
		s.vocabularyFailure(w, err)
		return false
	}
	name, found := names[id]
	if !found {
		return true
	}
	if _, ok := s.sendVocabulary(w, r, domain.VocabularyChange{Kind: kind, Action: domain.VocabularyDelete, Name: name}); !ok {
		return false
	}
	if domain.IsRemoteID(id) {
		w.Header().Set("Cache-Control", cacheNoStore)
		w.WriteHeader(http.StatusNoContent)
		return false
	}
	return true
}
