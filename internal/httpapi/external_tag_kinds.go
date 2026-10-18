package httpapi

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/extgen"
)

func (e *externalServer) ListTagKinds(w http.ResponseWriter, r *http.Request) {
	if !e.tagsConfigured(w) {
		return
	}
	kinds, err := e.s.tags.ListTagKinds(r.Context())
	if err != nil {
		e.internalError(w, "Could not load tag kinds.", err)
		return
	}
	items := make([]extgen.TagKind, 0, len(kinds))
	for _, kind := range kinds {
		items = append(items, toExternalTagKind(kind))
	}
	writeExternalBody(w, e.s, extgen.TagKindList{Items: items})
}

// Reject missing/null name as malformed input before it can become tag_name_empty.
func (e *externalServer) CreateTagKind(w http.ResponseWriter, r *http.Request) {
	var body extgen.TagKindCreateRequest
	fields, ok := e.readJSONBodyFields(w, r, &body)
	if !ok || !e.requireProperties(w, fields, "name") || !e.tagsConfigured(w) {
		return
	}
	kind, err := e.s.tags.CreateTagKind(r.Context(), body.Name)
	if err != nil {
		e.writeTagKindError(w, err)
		return
	}
	writeExternalBody(w, e.s, toExternalTagKind(kind))
}

// Same-name renames are no-ops; missing/null IDs or names are malformed input rather than 404/empty-name errors.
func (e *externalServer) RenameTagKind(w http.ResponseWriter, r *http.Request) {
	var body extgen.TagKindRenameRequest
	fields, ok := e.readJSONBodyFields(w, r, &body)
	if !ok || !e.requireProperties(w, fields, "id", "name") || !e.tagsConfigured(w) {
		return
	}
	kind, err := e.s.tags.RenameTagKind(r.Context(), body.Id, body.Name)
	if err != nil {
		e.writeTagKindError(w, err)
		return
	}
	writeExternalBody(w, e.s, toExternalTagKind(kind))
}

// Deleting kinds preserves tags without a kind; unexpected failure stops after earlier deletions, matching credit deletion semantics.
func (e *externalServer) DeleteTagKinds(w http.ResponseWriter, r *http.Request) {
	var body extgen.TagKindDeleteRequest
	if !e.readJSONBody(w, r, &body) {
		return
	}
	if len(body.Ids) == 0 || len(body.Ids) > domain.MaxTagBatch {
		e.invalidRequest(w, nil, fmt.Sprintf("ids must contain between 1 and %d items. Nothing was changed.", domain.MaxTagBatch))
		return
	}
	if !e.tagsConfigured(w) {
		return
	}
	out := extgen.TagKindDeleteResponse{DeletedIds: []int64{}, NotFoundIds: []int64{}}
	seen := make(map[int64]bool, len(body.Ids))
	for _, id := range body.Ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		err := e.s.tags.DeleteTagKind(r.Context(), id)
		switch {
		case err == nil:
			out.DeletedIds = append(out.DeletedIds, id)
		case errors.Is(err, domain.ErrTagKindNotFound):
			out.NotFoundIds = append(out.NotFoundIds, id)
		default:
			e.internalError(w, fmt.Sprintf("Could not delete tag kind %d. The ids before it were deleted.", id), err)
			return
		}
	}
	writeExternalBody(w, e.s, out)
}

// Kind conflicts include the existing name in tagName and share UI status/reason mappings.
func (e *externalServer) writeTagKindError(w http.ResponseWriter, err error) {
	var invalid *domain.InvalidTagNameError
	var conflict *domain.TagKindNameConflict
	switch {
	case errors.As(err, &invalid):
		e.writeInvalidTagName(w, invalid)
	case errors.Is(err, domain.ErrInvalidTagName):
		e.invalidRequest(w, nil, "The kind name cannot be used.")
	case errors.Is(err, domain.ErrTagKindNotFound):
		reason := extgen.TagKindNotFound
		writeExternalError(w, e.s, http.StatusNotFound, extgen.Error{
			Code: extgen.ErrorCodeNotFound, Reason: &reason, Message: "Tag kind not found.",
		})
	case errors.As(err, &conflict):
		reason, name := extgen.TagKindNameTaken, conflict.Kind.Name
		writeExternalError(w, e.s, http.StatusConflict, extgen.Error{
			Code: extgen.ErrorCodeConflict, Reason: &reason, TagName: &name,
			Message: fmt.Sprintf("A kind named %q already exists. Nothing was changed.", name),
		})
	default:
		e.internalError(w, "Could not update the tag kind.", err)
	}
}

func (e *externalServer) tagsConfigured(w http.ResponseWriter) bool {
	if e.s.tags == nil {
		e.internalError(w, "Tag storage is not configured.", nil)
		return false
	}
	return true
}

func toExternalTagKind(kind domain.TagKind) extgen.TagKind {
	return extgen.TagKind{Id: kind.ID, Name: kind.Name, TagCount: kind.TagCount, CreatedAt: kind.CreatedAt}
}

func toExternalTagKindRef(kind *domain.TagKindRef) *extgen.TagKindRef {
	if kind == nil {
		return nil
	}
	return &extgen.TagKindRef{Id: kind.ID, Name: kind.Name}
}
