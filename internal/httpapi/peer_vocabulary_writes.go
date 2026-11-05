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

// While this node reads others, a rename, merge or deletion of a tag, person or organization is applied by name on
// every link in slot order and then here, once every link is connected and, for a rename, no link holds the new
// name, so the common failures, a link that is down or a name taken, leave every node unchanged; a failure after
// some links applied it is answered as is.

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
	s.writeErrorBody(w, http.StatusConflict, creditNameTakenBody(kind))
}

func creditNameTakenBody(kind domain.CreditKind) gen.Error {
	reason := gen.ErrorReasonPersonNameTaken
	if kind == domain.CreditOrganization {
		reason = gen.ErrorReasonOrganizationNameTaken
	}
	return gen.Error{
		Code: codeConflict, Reason: &reason, Message: "Another " + string(kind) + " on this or the other VVMDM already has the name.",
	}
}

// vocabularyLinks are the links a rename, merge or deletion writes by name, in slot order: none for the peer's own
// request or a node that reads none. While a link is not connected nothing is written: it answers 409
// peer_unreachable naming the first such link in slot order, and ok is false.
func (s *server) vocabularyLinks(w http.ResponseWriter, r *http.Request) ([]domain.PeerLinkState, bool) {
	links, err := s.readLinks(r.Context())
	if err != nil {
		s.internalError(w, "Could not read the pairing.", err)
		return nil, false
	}
	for _, link := range links {
		if link.State != domain.PeerConnected {
			s.peerNotConnected(w, link.Link.Name)
			return nil, false
		}
	}
	return links, true
}

// vocabularyPeer reports whether a rename, merge or deletion also goes to the links, as vocabularyLinks checks.
func (s *server) vocabularyPeer(w http.ResponseWriter, r *http.Request) (propagate, ok bool) {
	links, ok := s.vocabularyLinks(w, r)
	return len(links) > 0, ok
}

// peerNotConnected answers 409 peer_unreachable with name, the link's, in Error.peer when it is known.
func (s *server) peerNotConnected(w http.ResponseWriter, name string) {
	s.writeErrorBody(w, http.StatusConflict, gen.Error{
		Code: codeConflict, Reason: reasonRef(gen.ErrorReasonPeerUnreachable), Peer: optionalString(name),
		Message: "The other VVMDM is not connected, so nothing was changed.",
	})
}

// vocabularyFailure answers an error of reading or writing a link's vocabulary, naming the link when err does.
func (s *server) vocabularyFailure(w http.ResponseWriter, err error) {
	if errors.Is(err, domain.ErrPeerUnreachable) {
		s.peerNotConnected(w, linkErrorName(err))
		return
	}
	s.internalError(w, "Could not apply the change on the other VVMDM.", err)
}

// sendVocabulary applies change by name on every link in slot order. When a link refuses or fails, it answers w
// with that link's error, its name in Error.peer, and reports false; the links before it stay changed, since a
// deletion or merge has no inverse.
func (s *server) sendVocabulary(w http.ResponseWriter, r *http.Request, change domain.VocabularyChange) (domain.VocabularyOutcome, bool) {
	return s.sendVocabularyExcept(w, r, change, 0)
}

// sendVocabularyExcept is sendVocabulary without the link in skip, which takes the change by id instead.
func (s *server) sendVocabularyExcept(
	w http.ResponseWriter, r *http.Request, change domain.VocabularyChange, skip int64,
) (domain.VocabularyOutcome, bool) {
	links, err := s.readLinks(r.Context())
	if err != nil {
		s.internalError(w, "Could not read the pairing.", err)
		return domain.VocabularyOutcome{}, false
	}
	var out domain.VocabularyOutcome
	for _, link := range links {
		if link.Link.Slot == skip {
			continue
		}
		outcome, ok := s.sendVocabularyTo(w, r, link.Link, change)
		if !ok {
			return domain.VocabularyOutcome{}, false
		}
		out.Changed = out.Changed || outcome.Changed
		out.Skipped = append(out.Skipped, outcome.Skipped...)
	}
	return out, true
}

// sendVocabularyTo applies change on link; when link refuses or fails, it answers w naming link and reports false.
func (s *server) sendVocabularyTo(
	w http.ResponseWriter, r *http.Request, link domain.PeerLink, change domain.VocabularyChange,
) (domain.VocabularyOutcome, bool) {
	body := gen.PeerVocabularyRequest{
		Kind: gen.PeerVocabularyRequestKind(change.Kind), Action: gen.PeerVocabularyRequestAction(change.Action),
		Name: optionalString(change.Name), NewName: optionalString(change.NewName), Into: optionalString(change.Into),
	}
	if len(change.Sources) > 0 {
		body.Sources = &change.Sources
	}
	var result gen.PeerVocabularyResult
	err := s.peerCall(r, link.Slot, http.MethodPost, "/api/peer/vocabulary", url.Values{}, body, &result)
	var refusal *peerRefusal
	switch {
	case err == nil:
		return domain.VocabularyOutcome{Changed: result.Changed, Skipped: result.Skipped}, true
	case errors.As(err, &refusal) && refusal.problem != nil:
		problem := *refusal.problem
		if problem.Reason != nil && *problem.Reason == gen.ErrorReasonTagNameTaken {
			problem = tagNameTakenBody(change.NewName, domain.TagRef{Name: valueOrDefault(problem.TagName, change.NewName)})
		}
		problem.Peer = &link.Name
		s.writeErrorBody(w, refusal.status, problem)
	default:
		s.vocabularyFailure(w, &linkError{name: link.Name, err: err})
	}
	return domain.VocabularyOutcome{}, false
}

func valueOrDefault(value *string, fallback string) string {
	if value == nil {
		return fallback
	}
	return *value
}

// vocabularyNames are the canonical names of ids: this node's from its store and each link's from that link's
// whole list; an id of a slot without a link has no name here.
func (s *server) vocabularyNames(r *http.Request, kind domain.VocabularyKind, ids []int64) (map[int64]string, error) {
	local, links := domain.SlotIDs(ids)
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
	for _, slot := range sortedSlots(links) {
		items, err := s.peerEntries(r, slot, kind)
		if errors.Is(err, domain.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, s.namedLinkError(r.Context(), slot, err)
		}
		for _, item := range items {
			if item.Name != "" && slices.Contains(links[slot], item.ID) {
				names[domain.RemoteID(slot, item.ID)] = item.Name
			}
		}
	}
	return names, nil
}

// peerVocabularyItem is an entry of the peer's whole list of a kind, in the peer's id space, as peerEntries reads it;
// only a tag has synonyms.
type peerVocabularyItem struct {
	ID       int64    `json:"id"`
	Name     string   `json:"name"`
	Synonyms []string `json:"synonyms"`
}

func (s *server) peerEntries(r *http.Request, slot int64, kind domain.VocabularyKind) ([]peerVocabularyItem, error) {
	var answer struct {
		Items []peerVocabularyItem `json:"items"`
	}
	err := s.peerGet(r, slot, vocabularyListPaths[kind], url.Values{}, &answer)
	return answer.Items, err
}

// restoreBody lets the next reader of r read raw again.
func restoreBody(r *http.Request, raw []byte) {
	r.Body, r.ContentLength = io.NopCloser(bytes.NewReader(raw)), int64(len(raw))
}

// renameTagOnPeer renames every link's tag of id's name before RenameTag renames id; it answers w and reports false
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
	if !s.tagNameFree(w, r.Context(), normalized) || !s.renameFreeOnLinks(w, r, domain.VocabularyTag, current, normalized, 0) {
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

// checkRemoteTagRename refuses the forwarded rename of a link's tag to a name a tag here holds, and renames the
// tag of the same name on the other links first.
func (s *server) checkRemoteTagRename(w http.ResponseWriter, r *http.Request) bool {
	var body gen.RenameTagRequest
	raw, ok := s.readJSONBodyRaw(w, r, &body)
	if !ok {
		return false
	}
	restoreBody(r, raw)
	normalized, err := domain.NormalizeTagName(body.Name)
	return err != nil ||
		(s.tagNameFree(w, r.Context(), normalized) && s.renameOnOtherLinks(w, r, domain.VocabularyTag, normalized))
}

// renameOnOtherLinks renames, by name, the entry of the path's remote id on every other link before the rename is
// forwarded to the id's own link, so one name stays one entry on every node; it answers w and reports false when
// the rename stops. With one link it does nothing, and the forward answers as before. An entry without a name, a
// provisional person, has nothing to rename elsewhere, so only the new name is checked there.
func (s *server) renameOnOtherLinks(w http.ResponseWriter, r *http.Request, kind domain.VocabularyKind, newName string) bool {
	id, _, found := remotePath(r.URL.EscapedPath(), vocabularyListPaths[kind]+"/")
	links, err := s.readLinks(r.Context())
	switch {
	case !found || err != nil || len(links) < 2:
		return true
	case !s.linksAllConnected(w, r):
		return false
	}
	names, err := s.vocabularyNames(r, kind, []int64{id})
	if err != nil {
		s.vocabularyFailure(w, err)
		return false
	}
	current, named := names[id]
	_, credit := kind.CreditKind()
	switch {
	case !named && credit:
		return s.renameFreeOnLinks(w, r, kind, "", newName, domain.RemoteSlot(id))
	case !named || current == newName:
		return true
	case !s.renameFreeOnLinks(w, r, kind, current, newName, 0):
		return false
	}
	_, ok := s.sendVocabularyExcept(w, r, domain.VocabularyChange{
		Kind: kind, Action: domain.VocabularyRename, Name: current, NewName: newName,
	}, domain.RemoteSlot(id))
	return ok
}

// linksAllConnected is vocabularyLinks as a check alone.
func (s *server) linksAllConnected(w http.ResponseWriter, r *http.Request) bool {
	_, ok := s.vocabularyLinks(w, r)
	return ok
}

// checkRemoteCreditRename refuses the forwarded rename of a link's person or organization to a name an entry here
// holds, and renames the entry of the same name on the other links first.
func checkRemoteCreditRename(kind domain.CreditKind) func(*server, http.ResponseWriter, *http.Request) bool {
	return func(s *server, w http.ResponseWriter, r *http.Request) bool {
		var body gen.CreditPatch
		raw, ok := s.readJSONBodyRaw(w, r, &body)
		if !ok {
			return false
		}
		restoreBody(r, raw)
		name := body.Name
		if body.Names != nil && len(*body.Names) > 0 {
			name = &(*body.Names)[0].Name
		}
		if name == nil {
			return true
		}
		normalized, err := domain.NormalizeCreditName(*name)
		return err != nil || (s.creditNameFree(w, r.Context(), kind, 0, normalized) &&
			s.renameOnOtherLinks(w, r, vocabularyKindOf(kind), normalized))
	}
}

// creditNameFree answers 409 with the kind's name_taken reason and reports false when an entry here other than id
// holds the name.
func (s *server) creditNameFree(w http.ResponseWriter, ctx context.Context, kind domain.CreditKind, id int64, name string) bool {
	join, err := s.vocabularyJoin(ctx, 0)
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

// renameFreeOnLinks answers 409, naming the link, and reports false when a link other than the one in skip would
// refuse renaming its entry of name to newName, because another of its entries holds newName, as its own rule
// does. It runs before any link is written, so a taken name leaves every node unchanged and the owner can repeat
// the rename once the name is free. An empty name, a provisional person's, names no entry, so any holder refuses.
func (s *server) renameFreeOnLinks(
	w http.ResponseWriter, r *http.Request, kind domain.VocabularyKind, name, newName string, skip int64,
) bool {
	links, err := s.readLinks(r.Context())
	if err != nil {
		s.internalError(w, "Could not read the pairing.", err)
		return false
	}
	for _, link := range links {
		if link.Link.Slot == skip {
			continue
		}
		items, err := s.peerEntries(r, link.Link.Slot, kind)
		if err != nil {
			s.vocabularyFailure(w, &linkError{name: link.Link.Name, err: err})
			return false
		}
		holder, taken := renameHolder(items, kind, name, newName)
		if !taken {
			continue
		}
		body := tagNameTakenBody(newName, domain.TagRef{Name: holder})
		if creditKind, ok := kind.CreditKind(); ok {
			body = creditNameTakenBody(creditKind)
		}
		body.Peer = &link.Link.Name
		s.writeErrorBody(w, http.StatusConflict, body)
		return false
	}
	return true
}

// renameHolder is the canonical name of the entry among items, other than name's, that holds newName as its
// canonical name or a tag's synonym; a name several entries hold names the lowest id, as the store's rule does.
func renameHolder(items []peerVocabularyItem, kind domain.VocabularyKind, name, newName string) (string, bool) {
	var renamed, holder *peerVocabularyItem
	for i := range items {
		item := &items[i]
		if name != "" && item.Name == name && (renamed == nil || item.ID < renamed.ID) {
			renamed = item
		}
		holds := item.Name == newName || (kind == domain.VocabularyTag && slices.Contains(item.Synonyms, newName))
		if holds && (holder == nil || item.ID < holder.ID) {
			holder = item
		}
	}
	if holder == nil || (renamed != nil && renamed.ID == holder.ID) {
		return "", false
	}
	return holder.Name, true
}

// renameCreditOnPeer renames every link's entry of id's name before the local update; a provisional person has no
// name a link could hold, so only the new name is checked on every node.
func (s *server) renameCreditOnPeer(w http.ResponseWriter, r *http.Request, kind domain.CreditKind, id int64, name *string) bool {
	if name == nil {
		return true
	}
	normalized, err := domain.NormalizeCreditName(*name)
	if err != nil {
		return true
	}
	propagate, ok := s.vocabularyPeer(w, r)
	if !ok || !propagate {
		return ok
	}
	current, err := s.credits.Get(r.Context(), kind, id)
	if err != nil || current.Name == normalized {
		return true
	}
	if !s.creditNameFree(w, r.Context(), kind, id, normalized) ||
		!s.renameFreeOnLinks(w, r, vocabularyKindOf(kind), current.Name, normalized, 0) {
		return false
	}
	if current.Name == "" {
		return true
	}
	_, ok = s.sendVocabulary(w, r, domain.VocabularyChange{
		Kind: vocabularyKindOf(kind), Action: domain.VocabularyRename, Name: current.Name, NewName: normalized,
	})
	return ok
}

// deleteOnPeer deletes every link's entry of id's name and reports whether the local deletion follows; a link's own
// entry has none here, so it is answered here. An id that names nothing is left to the local deletion's 404.
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
