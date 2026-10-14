package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// ドラッグの操作と取り消し（specs/050-drag-organize/contracts/screen-api.md）。どれも /api/* の既定
// どおり所有者だけである。

// Organizer はドラッグの操作とその取り消しの先である。internal/app の *Organizer がこれを満たす。
// 操作は 1 つの取引で行い、取り消しのトークンを返す。要求が名指したグループや項目が無ければ
// domain.ErrGroupTargetNotFound で、何も書かない。
type Organizer interface {
	// Reorder はグループ groupID の項目 itemIDs を、その順のまままとめて項目 afterItemID の
	// すぐ後ろへ移す（nil なら先頭）。
	Reorder(ctx context.Context, groupID int64, itemIDs []int64, afterItemID *int64) (domain.OrganizeResult, error)
	// Move はカードを move.To へ足し、move.From から外す。ドラッグしたグループが To か To の祖先なら
	// domain.ErrGroupCycle。
	Move(ctx context.Context, move domain.OrganizeMove) (domain.OrganizeResult, error)
	// Undo はトークン token の操作を取り消す。行が変わっていれば domain.ErrUndoConflict、トークンが
	// 無い・使い済み・古ければ domain.ErrUndoExpired。
	Undo(ctx context.Context, token string) error
}

// OrganizeReorder はグループの項目をまとめて並べ替える（POST /api/organize/reorder）。
func (s *server) OrganizeReorder(w http.ResponseWriter, r *http.Request) {
	var body gen.OrganizeReorder
	if !s.readLargeJSONBody(w, r, groupsBodyLimit, &body) {
		return
	}
	switch {
	case len(body.ItemIds) == 0:
		s.invalidRequest(w, "itemIds must contain at least one item.")
		return
	case len(body.ItemIds) > maxVideoTagsIDs:
		s.invalidRequest(w, fmt.Sprintf("itemIds must contain at most %d items.", maxVideoTagsIDs))
		return
	case body.After != nil && slices.Contains(body.ItemIds, *body.After):
		s.invalidRequest(w, "Items cannot be moved after one of themselves.")
		return
	}
	if !s.organizerConfigured(w) {
		return
	}
	result, err := s.organizer.Reorder(r.Context(), body.GroupId, body.ItemIds, body.After)
	if err != nil {
		s.writeGroupError(w, err, "Could not reorder the group.")
		return
	}
	s.writeOrganizeResult(w, result)
}

// OrganizeMove はカードをグループへ入れるか、グループの間で動かすか、グループから外す
// （POST /api/organize/move）。
func (s *server) OrganizeMove(w http.ResponseWriter, r *http.Request) {
	var body gen.OrganizeMove
	if !s.readLargeJSONBody(w, r, groupsBodyLimit, &body) {
		return
	}
	items, problem := organizeRefs(body.Items)
	from, to := optionalID(body.From), optionalID(body.To)
	switch {
	case problem != "":
		s.invalidRequest(w, problem)
		return
	case from == 0 && to == 0:
		s.invalidRequest(w, "Name the group the cards leave or the group they join.")
		return
	case from == to:
		s.invalidRequest(w, "Cards cannot be moved into the group they leave.")
		return
	}
	if !s.organizerConfigured(w) {
		return
	}
	result, err := s.organizer.Move(r.Context(), domain.OrganizeMove{Items: items, From: from, To: to})
	if err != nil {
		s.writeGroupError(w, err, "Could not move the cards.")
		return
	}
	s.writeOrganizeResult(w, result)
}

// organizeRefs はドラッグしたカードの本文を domain の並びにする。空か多すぎるか、知らない種類が
// あれば、画面に出してよい英語の文を返す。
func organizeRefs(refs gen.OrganizeRefs) ([]domain.OrganizeRef, string) {
	switch {
	case len(refs) == 0:
		return nil, "items must contain at least one card."
	case len(refs) > maxVideoTagsIDs:
		return nil, fmt.Sprintf("items must contain at most %d cards.", maxVideoTagsIDs)
	}
	out := make([]domain.OrganizeRef, 0, len(refs))
	for _, ref := range refs {
		if !ref.Kind.Valid() {
			return nil, "Each card's kind must be video or group."
		}
		out = append(out, domain.OrganizeRef{Kind: domain.OrganizeRefKind(ref.Kind), ID: ref.Id})
	}
	return out, ""
}

// optionalID は省ける id を、無ければ 0 にする。
func optionalID(id *int64) int64 {
	if id == nil {
		return 0
	}
	return *id
}

// OrganizeUndo はドラッグの操作を取り消す（POST /api/organize/undo）。
func (s *server) OrganizeUndo(w http.ResponseWriter, r *http.Request) {
	var body gen.OrganizeUndo
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if body.Token == "" {
		s.invalidRequest(w, "token is required.")
		return
	}
	if !s.organizerConfigured(w) {
		return
	}
	err := s.organizer.Undo(r.Context(), body.Token)
	switch {
	case errors.Is(err, domain.ErrUndoConflict):
		s.conflictReason(w, reasonUndoConflict, "It changed after the operation, so it cannot be undone.")
		return
	case errors.Is(err, domain.ErrUndoExpired):
		s.writeReasonError(w, http.StatusGone, codeNotFound, reasonUndoExpired, "This can no longer be undone.")
		return
	case err != nil:
		s.internalError(w, "Could not undo.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	w.WriteHeader(http.StatusNoContent)
}

// organizerConfigured は操作の先が無ければ 500 を書いて false を返す。
func (s *server) organizerConfigured(w http.ResponseWriter) bool {
	if s.organizer == nil {
		s.internalError(w, "Drag operations are not configured.", nil)
		return false
	}
	return true
}

// writeOrganizeResult は操作の結果を 200 OrganizeResult にする。
func (s *server) writeOrganizeResult(w http.ResponseWriter, result domain.OrganizeResult) {
	out := gen.OrganizeResult{UndoToken: result.UndoToken}
	if result.GroupID != 0 {
		id := result.GroupID
		out.GroupId = &id
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, out, s.logger)
}
