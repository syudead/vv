package httpapi

import (
	"net/http"
	"slices"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// The tag page reads one tag and changes one synonym at a time; a remote id answers 404 tag_not_found, since these
// act on this node's tags (docs/design-docs/tag-organizing.md).

// GetTag answers the Tag that ListTags gives for id, joined with the links' tags of the same name while this node
// reads others.
func (s *server) GetTag(w http.ResponseWriter, r *http.Request, id gen.TagId) {
	if !s.localTagID(w, id) {
		return
	}
	if s.mergesVocabulary(r.Context()) {
		s.getMergedTag(w, r, id)
		return
	}
	tag, err := s.tags.Tag(r.Context(), audienceFrom(r.Context()), id)
	s.answerTag(w, http.StatusOK, func() any { return toAPITag(tag) }, err, "")
}

func (s *server) getMergedTag(w http.ResponseWriter, r *http.Request, id int64) {
	merged, err := s.mergedTags(r)
	if err != nil {
		s.internalError(w, "Could not load the tag.", err)
		return
	}
	for _, entry := range merged {
		if entry.Value.Id == id {
			s.answerTag(w, http.StatusOK, func() any { return entry.Value }, nil, "")
			return
		}
	}
	s.writeTagError(w, domain.ErrTagNotFound, "")
}

func (s *server) PromoteTagSynonym(w http.ResponseWriter, r *http.Request, id gen.TagId) {
	var body gen.TagSynonymRequest
	if !s.readJSONBody(w, r, &body) || !s.localTagID(w, id) {
		return
	}
	tag, err := s.vocabulary.PromoteTagSynonym(r.Context(), s.links(), id, body.Name)
	if err != nil {
		s.writeTagChangeError(w, err, normalizedTagNameOrRaw(body.Name))
		return
	}
	s.answerTag(w, http.StatusOK, func() any { return toAPITag(tag) }, nil, "")
}

func (s *server) MoveTagSynonym(w http.ResponseWriter, r *http.Request, id gen.TagId) {
	var body gen.MoveTagSynonymRequest
	if !s.readJSONBody(w, r, &body) || !s.localTagID(w, id) {
		return
	}
	defer s.holdTagNames()()
	source, target, err := s.tags.MoveSynonym(r.Context(), id, body.Name, body.TargetId)
	s.answerTag(w, http.StatusOK, func() any {
		return gen.TagSynonymMoveResponse{Source: toAPITag(source), Target: toAPITag(target)}
	}, err, "")
}

func (s *server) DetachTagSynonym(w http.ResponseWriter, r *http.Request, id gen.TagId) {
	var body gen.TagSynonymRequest
	if !s.readJSONBody(w, r, &body) || !s.localTagID(w, id) {
		return
	}
	defer s.holdTagNames()()
	source, created, err := s.tags.DetachSynonym(r.Context(), id, body.Name)
	s.answerTag(w, http.StatusCreated, func() any {
		return gen.TagSynonymDetachResponse{Source: toAPITag(source), Created: toAPITag(created)}
	}, err, "")
}

// answerTag answers the body with status, or err as a tag error naming submittedName.
func (s *server) answerTag(w http.ResponseWriter, status int, body func() any, err error, submittedName string) {
	if err != nil {
		s.writeTagError(w, err, submittedName)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, status, body(), s.logger)
}

// localTagID answers w and reports false when tag storage is missing or id is a remote id, which names no tag here.
func (s *server) localTagID(w http.ResponseWriter, id int64) bool {
	switch {
	case s.tags == nil:
		s.internalError(w, "Tag storage is not configured.", nil)
		return false
	case domain.IsRemoteID(id):
		s.writeTagError(w, domain.ErrTagNotFound, "")
		return false
	}
	return true
}

// namesNewTags reports whether inputs name a tag by name, which creates the tag when no tag holds the name.
func namesNewTags(inputs []domain.TagInput) bool {
	return slices.ContainsFunc(inputs, func(input domain.TagInput) bool { return input.ID == nil })
}
