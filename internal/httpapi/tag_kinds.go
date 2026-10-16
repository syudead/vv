package httpapi

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// タグの種類の API（specs/051-tag-kinds/contracts/screen-api.md）。タグの管理 API と同じく
// internal/app は通さず、どの操作も internal/store.TagStore の 1 つの取引で済む。/api/* の
// 既定で所有者だけが使える。

// ListTagKinds はすべての種類を id の順に返す（GET /api/tag-kinds）。
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
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.TagKindList{Items: items}, s.logger)
}

// CreateTagKind は種類を 1 件作る（POST /api/tag-kinds）。
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

// RenameTagKind は種類の名前を書き換える（PATCH /api/tag-kinds/{id}）。今と同じ名前なら何も
// 変えずに 200 を返す。
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

// DeleteTagKind は種類を消す（DELETE /api/tag-kinds/{id}）。その種類のタグは種類なしで残る。
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

// writeTagKindError は種類の操作の誤りを契約の表の状態へ写す。名前の規則違反はタグの名前と
// 同じ reason（tag_name_*）にする（specs/051-tag-kinds/research.md R-6）。名前の重複には、
// 画面が文に入れられるように、その名前を持つ種類の名前を tagName に載せる。
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
	return gen.TagKind{Id: kind.ID, Name: kind.Name, TagCount: kind.TagCount, CreatedAt: kind.CreatedAt}
}

// toAPITagKindRef はタグの種類を応答の形にする。種類の無いタグは nil（null）。
func toAPITagKindRef(kind *domain.TagKindRef) *gen.TagKindRef {
	if kind == nil {
		return nil
	}
	return &gen.TagKindRef{Id: kind.ID, Name: kind.Name}
}
