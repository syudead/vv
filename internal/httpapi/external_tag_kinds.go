package httpapi

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/extgen"
)

// 外部連携 API のタグの種類（specs/051-tag-kinds/contracts/external-api.md）。どの操作も画面の経路
// （tag_kinds.go）と同じ TagStore の操作を呼び、新しい書き込みの経路は持たない
// （specs/039-external-tag-admin/research.md R-6）。形は人物の操作（external_credits.go）に倣い、一覧と
// 作成は集まりの経路、名前の変更と削除は POST の下位の経路で、種類は本文の id で指す。

// ListTagKinds はすべての種類を id の順に返す（GET /api/v1/tag-kinds）。
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

// CreateTagKind は種類を 1 つ作る（POST /api/v1/tag-kinds）。他の外部連携の書き込みと同じく 200 を返す。
func (e *externalServer) CreateTagKind(w http.ResponseWriter, r *http.Request) {
	var body extgen.TagKindCreateRequest
	if !e.readJSONBody(w, r, &body) || !e.tagsConfigured(w) {
		return
	}
	kind, err := e.s.tags.CreateTagKind(r.Context(), body.Name)
	if err != nil {
		e.writeTagKindError(w, err)
		return
	}
	writeExternalBody(w, e.s, toExternalTagKind(kind))
}

// RenameTagKind は id の種類の名前を変える（POST /api/v1/tag-kinds/update）。今と同じ名前なら何も
// 変えずに今の種類を返す。id の省略と null は、無い種類（404）に化けないよう形の誤りとして断る。
func (e *externalServer) RenameTagKind(w http.ResponseWriter, r *http.Request) {
	var body extgen.TagKindRenameRequest
	fields, ok := e.readJSONBodyFields(w, r, &body)
	if !ok || !e.requireProperties(w, fields, "id") || !e.tagsConfigured(w) {
		return
	}
	kind, err := e.s.tags.RenameTagKind(r.Context(), body.Id, body.Name)
	if err != nil {
		e.writeTagKindError(w, err)
		return
	}
	writeExternalBody(w, e.s, toExternalTagKind(kind))
}

// DeleteTagKinds は ids の種類を消す（POST /api/v1/tag-kinds/delete）。重複を除いた ids を順に、画面の
// DELETE と同じ TagStore.DeleteTagKind で 1 つずつ消し、無い id は notFoundIds に返す。その種類のタグは
// 種類なしで残る。途中の想定外の失敗では、それより前の id は消えたまま 500 を返す（deleteCredits と同じ）。
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

// writeTagKindError は種類の操作の誤りを、画面の writeTagKindError と同じ状態と reason へ写す。名前の
// 規則違反はタグの名前と同じ reason（tag_name_*）、名前の重複は 409 tag_kind_name_taken で、その名前を
// 持つ種類の名前を tagName に載せる。
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

// toExternalTagKindRef はタグの種類を応答の形にする。種類の無いタグは nil（null）。
func toExternalTagKindRef(kind *domain.TagKindRef) *extgen.TagKindRef {
	if kind == nil {
		return nil
	}
	return &extgen.TagKindRef{Id: kind.ID, Name: kind.Name}
}
