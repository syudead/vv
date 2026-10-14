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
	// Merge はドラッグしたカード items を画面の順にたどり、グループはグループ into へマージし、
	// 動画は into の末尾へ足す。ドラッグしたグループが into か into の祖先なら domain.ErrGroupCycle。
	Merge(ctx context.Context, items []domain.OrganizeRef, into int64) (domain.OrganizeResult, error)
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

// OrganizeMerge はドラッグしたグループと動画をグループへマージする（POST /api/organize/merge）。
func (s *server) OrganizeMerge(w http.ResponseWriter, r *http.Request) {
	var body gen.OrganizeMerge
	if !s.readLargeJSONBody(w, r, groupsBodyLimit, &body) {
		return
	}
	items, ok := s.organizeRefs(w, body.Items)
	if !ok {
		return
	}
	if !slices.ContainsFunc(items, func(item domain.OrganizeRef) bool { return item.Kind == domain.OrganizeGroup }) {
		s.invalidRequest(w, "items must contain at least one group.")
		return
	}
	if !s.organizerConfigured(w) {
		return
	}
	result, err := s.organizer.Merge(r.Context(), items, body.Into)
	if err != nil {
		s.writeGroupError(w, err, "Could not merge the groups.")
		return
	}
	s.writeOrganizeResult(w, result)
}

// organizeRefs はドラッグしたカードの並び refs を確かめて domain の形にする。空か 20000 枚を
// 超えるか、知らない種類があれば 400 を書いて false を返す。
func (s *server) organizeRefs(w http.ResponseWriter, refs gen.OrganizeRefs) ([]domain.OrganizeRef, bool) {
	switch {
	case len(refs) == 0:
		s.invalidRequest(w, "items must contain at least one card.")
		return nil, false
	case len(refs) > maxVideoTagsIDs:
		s.invalidRequest(w, fmt.Sprintf("items must contain at most %d cards.", maxVideoTagsIDs))
		return nil, false
	}
	out := make([]domain.OrganizeRef, 0, len(refs))
	for _, ref := range refs {
		if !ref.Kind.Valid() {
			s.invalidRequest(w, "kind must be video or group.")
			return nil, false
		}
		out = append(out, domain.OrganizeRef{Kind: domain.OrganizeRefKind(ref.Kind), ID: ref.Id})
	}
	return out, true
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
