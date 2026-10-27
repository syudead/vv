package httpapi

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// Tag-kind operations are owner-only and complete in one TagStore transaction.

func (s *server) ListTagKinds(w http.ResponseWriter, r *http.Request) {
	if s.tags == nil {
		s.internalError(w, "Tag storage is not configured.", nil)
		return
	}
	kinds, err := s.tags.ListTagKinds(r.Context())
	if err != nil {
		s.internalError(w, "Could not load tag kinds.", err)
		return
	}
	items := make([]gen.TagKind, 0, len(kinds))
	for _, kind := range kinds {
		items = append(items, toAPITagKind(kind))
	}
	if s.mergesVocabulary(r.Context()) {
		s.listMergedTagKinds(w, r, items)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.TagKindList{Items: items}, s.logger)
}

func (s *server) CreateTagKind(w http.ResponseWriter, r *http.Request) {
	var body gen.TagKindNameRequest
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if s.tags == nil {
		s.internalError(w, "Tag storage is not configured.", nil)
		return
	}
	kind, err := s.tags.CreateTagKind(r.Context(), body.Name)
	if err != nil {
		s.writeTagKindError(w, err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusCreated, toAPITagKind(kind), s.logger)
}

// Renaming to the current name is a no-op returning 200.
func (s *server) RenameTagKind(w http.ResponseWriter, r *http.Request, id gen.TagKindId) {
	var body gen.TagKindNameRequest
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if s.tags == nil {
		s.internalError(w, "Tag storage is not configured.", nil)
		return
	}
	kind, err := s.tags.RenameTagKind(r.Context(), id, body.Name)
	if err != nil {
		s.writeTagKindError(w, err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, toAPITagKind(kind), s.logger)
}

func (s *server) SetTagKindCardFirst(w http.ResponseWriter, r *http.Request, id gen.TagKindId) {
	// A pointer tells a missing value from false.
	var body struct {
		CardFirst *bool `json:"cardFirst"`
	}
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if body.CardFirst == nil {
		s.invalidRequest(w, "cardFirst is required.")
		return
	}
	if s.tags == nil {
		s.internalError(w, "Tag storage is not configured.", nil)
		return
	}
	kind, err := s.tags.SetTagKindCardFirst(r.Context(), id, *body.CardFirst)
	if err != nil {
		s.writeTagKindError(w, err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, toAPITagKind(kind), s.logger)
}

// Deleting a kind preserves its tags with no kind.
func (s *server) DeleteTagKind(w http.ResponseWriter, r *http.Request, id gen.TagKindId) {
	if s.tags == nil {
		s.internalError(w, "Tag storage is not configured.", nil)
		return
	}
	if err := s.tags.DeleteTagKind(r.Context(), id); err != nil {
		s.writeTagKindError(w, err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	w.WriteHeader(http.StatusNoContent)
}

// Use tag-name validation reasons and include the conflicting kind name in tagName for UI messages.
func (s *server) writeTagKindError(w http.ResponseWriter, err error) {
	var conflict *domain.TagKindNameConflict
	switch {
	case errors.Is(err, domain.ErrInvalidTagName):
		s.invalidTagName(w, err, "")
	case errors.Is(err, domain.ErrTagKindNotFound):
		s.notFoundReason(w, reasonTagKindNotFound, "Tag kind not found.")
	case errors.As(err, &conflict):
		name := conflict.Kind.Name
		reason := reasonTagKindNameTaken
		s.writeErrorBody(w, http.StatusConflict, gen.Error{
			Code:    codeConflict,
			Reason:  &reason,
			TagName: &name,
			Message: fmt.Sprintf("A kind named %q already exists.", name),
		})
	default:
		s.internalError(w, "Could not update the tag kind.", err)
	}
}

func toAPITagKind(kind domain.TagKind) gen.TagKind {
	return gen.TagKind{
		Id: kind.ID, Name: kind.Name, CardFirst: kind.CardFirst, TagCount: kind.TagCount, CreatedAt: kind.CreatedAt,
	}
}

func toAPITagKindRef(kind *domain.TagKindRef) *gen.TagKindRef {
	if kind == nil {
		return nil
	}
	return &gen.TagKindRef{Id: kind.ID, Name: kind.Name, CardFirst: kind.CardFirst}
}
