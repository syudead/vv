package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// While this node reads others, the VocabularyChanges use case applies a rename, merge or deletion of a tag, person
// or organization by name on every link in slot order and then here; this file adapts the links to it and answers
// its errors (docs/design-docs/peer-library.md#vocabulary-by-name).

// maxPeerVocabularyBody fits a merge of the most sources a merge takes.
const maxPeerVocabularyBody = 8 << 20

var vocabularyListPaths = map[domain.VocabularyKind]string{
	domain.VocabularyTag: "/api/tags", domain.VocabularyPerson: "/api/people",
	domain.VocabularyOrganization: "/api/organizations",
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

// peerNotConnected answers 409 peer_unreachable with name, the link's, in Error.peer when it is known.
func (s *server) peerNotConnected(w http.ResponseWriter, name string) {
	s.writeErrorBody(w, http.StatusConflict, gen.Error{
		Code: codeConflict, Reason: reasonRef(gen.ErrorReasonPeerUnreachable), Peer: optionalString(name),
		Message: "The other VVMDM is not connected, so nothing was changed.",
	})
}

func valueOrDefault(value *string, fallback string) string {
	if value == nil {
		return fallback
	}
	return *value
}

// linkVocabulary is the links of this node as a change by name reaches them, for the VocabularyChanges use case.
type linkVocabulary struct{ s *server }

func (s *server) links() domain.VocabularyLinks { return linkVocabulary{s: s} }

func (l linkVocabulary) Links(ctx context.Context) ([]domain.PeerLinkState, error) {
	return l.s.readLinks(ctx)
}

func (l linkVocabulary) Entries(ctx context.Context, slot int64, kind domain.VocabularyKind) ([]domain.LinkVocabularyEntry, error) {
	return l.s.peerEntries(ctx, slot, kind)
}

func (l linkVocabulary) Apply(ctx context.Context, link domain.PeerLink, change domain.VocabularyChange) (domain.VocabularyOutcome, error) {
	body := gen.PeerVocabularyRequest{
		Kind: gen.PeerVocabularyRequestKind(change.Kind), Action: gen.PeerVocabularyRequestAction(change.Action),
		Name: optionalString(change.Name), NewName: optionalString(change.NewName), Into: optionalString(change.Into),
	}
	if len(change.Sources) > 0 {
		body.Sources = &change.Sources
	}
	var result gen.PeerVocabularyResult
	err := l.s.peerCallContext(ctx, link.Slot, http.MethodPost, "/api/peer/vocabulary", url.Values{}, body, &result)
	return domain.VocabularyOutcome{Changed: result.Changed, Skipped: result.Skipped}, err
}

// peerVocabularyItem is an entry of the peer's whole list of a kind, in the peer's id space, as peerEntries reads it;
// only a tag has synonyms.
type peerVocabularyItem struct {
	ID       int64    `json:"id"`
	Name     string   `json:"name"`
	Synonyms []string `json:"synonyms"`
}

// peerRequestEntries is peerEntries for the viewer of r.
func (s *server) peerRequestEntries(r *http.Request, slot int64, kind domain.VocabularyKind) ([]domain.LinkVocabularyEntry, error) {
	return s.peerEntries(r.Context(), slot, kind)
}

func (s *server) peerEntries(ctx context.Context, slot int64, kind domain.VocabularyKind) ([]domain.LinkVocabularyEntry, error) {
	var answer struct {
		Items []peerVocabularyItem `json:"items"`
	}
	if err := s.peerCallContext(ctx, slot, http.MethodGet, vocabularyListPaths[kind], url.Values{}, nil, &answer); err != nil {
		return nil, err
	}
	entries := make([]domain.LinkVocabularyEntry, len(answer.Items))
	for i, item := range answer.Items {
		entries[i] = domain.LinkVocabularyEntry{ID: item.ID, Name: item.Name, Synonyms: item.Synonyms}
	}
	return entries, nil
}

// vocabularyConfigured answers 500 with message and reports false when the vocabulary use case is missing.
func (s *server) vocabularyConfigured(w http.ResponseWriter, message string) bool {
	if s.vocabulary == nil {
		s.internalError(w, message, nil)
		return false
	}
	return true
}

// holdTagNames takes the tag-name lock of the vocabulary use case and returns its unlock, for a deferred call.
func (s *server) holdTagNames() (unlock func()) {
	if s.vocabulary == nil {
		return func() {}
	}
	return s.vocabulary.HoldTagNames()
}

// writeVocabularyChangeError answers an error of a change by name: a name taken here or on a link, or a link that
// is down, refused or failed; any other error is the local write's, which fallback answers.
func (s *server) writeVocabularyChangeError(w http.ResponseWriter, err error, fallback func(http.ResponseWriter, error)) {
	var taken *domain.VocabularyNameTakenError
	var link *domain.VocabularyLinkError
	switch {
	case errors.As(err, &taken):
		body := tagNameTakenBody(taken.NewName, domain.TagRef{Name: taken.Holder})
		if creditKind, ok := taken.Kind.CreditKind(); ok {
			body = creditNameTakenBody(creditKind)
		}
		body.Peer = optionalString(taken.Link)
		s.writeErrorBody(w, http.StatusConflict, body)
	case errors.As(err, &link):
		s.vocabularyLinkFailure(w, link)
	default:
		fallback(w, err)
	}
}

// vocabularyFailure answers an error of reading or writing a link's vocabulary, naming the link when err does.
func (s *server) vocabularyFailure(w http.ResponseWriter, err error) {
	s.writeVocabularyChangeError(w, err, func(w http.ResponseWriter, err error) {
		if errors.Is(err, domain.ErrPeerUnreachable) {
			s.peerNotConnected(w, linkErrorName(err))
			return
		}
		s.internalError(w, "Could not apply the change on the other VVMDM.", err)
	})
}

// vocabularyLinkFailure answers a link's refusal of a change with the link's status and error, its name in
// Error.peer; a link that is down answers 409 peer_unreachable.
func (s *server) vocabularyLinkFailure(w http.ResponseWriter, link *domain.VocabularyLinkError) {
	var refusal *peerRefusal
	switch {
	case link.Change != nil && errors.As(link.Err, &refusal) && refusal.problem != nil:
		problem := *refusal.problem
		if problem.Reason != nil && *problem.Reason == gen.ErrorReasonTagNameTaken {
			newName := link.Change.NewName
			problem = tagNameTakenBody(newName, domain.TagRef{Name: valueOrDefault(problem.TagName, newName)})
		}
		problem.Peer = &link.Link
		s.writeErrorBody(w, refusal.status, problem)
	case errors.Is(link.Err, domain.ErrPeerUnreachable):
		s.peerNotConnected(w, link.Link)
	default:
		s.internalError(w, "Could not apply the change on the other VVMDM.", link)
	}
}

// vocabularyPeer reports whether a rename, merge or deletion also goes to the links; while a link is not
// connected nothing is written: it answers 409 peer_unreachable naming the first such link, and ok is false.
func (s *server) vocabularyPeer(w http.ResponseWriter, r *http.Request) (propagate, ok bool) {
	propagate, err := s.vocabulary.Propagates(r.Context(), s.links())
	if err != nil {
		s.vocabularyFailure(w, err)
		return false, false
	}
	return propagate, true
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
	outcome, err := s.vocabulary.Send(r.Context(), s.links(), change, skip)
	if err != nil {
		s.vocabularyFailure(w, err)
		return domain.VocabularyOutcome{}, false
	}
	return outcome, true
}

// vocabularyNames are the canonical names of ids on this node and the links.
func (s *server) vocabularyNames(r *http.Request, kind domain.VocabularyKind, ids []int64) (map[int64]string, error) {
	return s.vocabulary.Names(r.Context(), s.links(), kind, ids)
}

// deleteOnPeer deletes every link's entry of id's name and reports whether the local deletion follows; a link's own
// entry has none here, so it is answered here.
func (s *server) deleteOnPeer(w http.ResponseWriter, r *http.Request, kind domain.VocabularyKind, id int64) bool {
	deleteHere, err := s.vocabulary.DeleteOnLinks(r.Context(), s.links(), kind, id)
	switch {
	case err != nil:
		s.vocabularyFailure(w, err)
		return false
	case !deleteHere:
		w.Header().Set("Cache-Control", cacheNoStore)
		w.WriteHeader(http.StatusNoContent)
	}
	return deleteHere
}

// restoreBody lets the next reader of r read raw again.
func restoreBody(r *http.Request, raw []byte) {
	r.Body, r.ContentLength = io.NopCloser(bytes.NewReader(raw)), int64(len(raw))
}

// remoteVocabularyID is the remote id in the path of a forwarded write of kind, 0 when the path names none.
func remoteVocabularyID(r *http.Request, kind domain.VocabularyKind) int64 {
	id, _, found := remotePath(r.URL.EscapedPath(), vocabularyListPaths[kind]+"/")
	if !found {
		return 0
	}
	return id
}

// checkRemoteTagRename refuses the forwarded rename of a link's tag to a name a tag here holds, and renames the
// tag of the same name on the other links first.
func (s *server) checkRemoteTagRename(w http.ResponseWriter, r *http.Request) bool {
	var body gen.RenameTagRequest
	raw, ok := s.readJSONBodyRaw(w, r, &body)
	if !ok || !s.vocabularyConfigured(w, "Tag storage is not configured.") {
		return false
	}
	restoreBody(r, raw)
	err := s.vocabulary.RenameRemoteTag(r.Context(), s.links(), remoteVocabularyID(r, domain.VocabularyTag), body.Name)
	if err != nil {
		s.vocabularyFailure(w, err)
		return false
	}
	return true
}

// checkRemoteCreditRename refuses the forwarded rename of a link's person or organization to a name an entry here
// holds, and renames the entry of the same name on the other links first.
func checkRemoteCreditRename(kind domain.CreditKind) func(*server, http.ResponseWriter, *http.Request) bool {
	return func(s *server, w http.ResponseWriter, r *http.Request) bool {
		var body gen.CreditPatch
		raw, ok := s.readJSONBodyRaw(w, r, &body)
		if !ok || !s.vocabularyConfigured(w, "People and organization storage is not configured.") {
			return false
		}
		restoreBody(r, raw)
		normalized, ok := forwardedDisplayName(kind, body, raw)
		if !ok {
			return true
		}
		id := remoteVocabularyID(r, domain.VocabularyKindOf(kind))
		if err := s.vocabulary.RenameRemoteCredit(r.Context(), s.links(), kind, id, normalized); err != nil {
			s.vocabularyFailure(w, err)
			return false
		}
		return true
	}
}

// forwardedDisplayName is the normalized display name a forwarded patch sets, "" for a person's emptied list, and
// false when the patch keeps the name or breaks the rule, which the id's own link then answers before any write.
// It reads the patch as the link does, so null names empty the list and a patch the link refuses changes no link.
func forwardedDisplayName(kind domain.CreditKind, body gen.CreditPatch, raw []byte) (string, bool) {
	var fields bodyFields
	if json.Unmarshal(raw, &fields) != nil {
		return "", false
	}
	patch, _, ok := creditPatchOf(body, fields, kind)
	switch {
	case !ok:
		return "", false
	case patch.Names != nil:
		input, err := domain.NormalizeCredit(kind, domain.CreditInput{Names: *patch.Names})
		return domain.DisplayNameOf(input.Names), err == nil
	case patch.Name == nil:
		return "", false
	}
	normalized, err := domain.NormalizeCreditName(*patch.Name)
	return normalized, err == nil && normalized != ""
}
