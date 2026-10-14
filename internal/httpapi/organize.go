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
	// Group は動画 targetID に videoIDs を落としてグループを作る（fromID があればその中の
	// targetID の位置に置く）。結果の GroupID が新しいグループである。
	Group(ctx context.Context, videoIDs []int64, targetID int64, fromID *int64) (domain.OrganizeResult, error)
	// RenameGroup はグループ id の名前を name にし、token がそのグループを作った「グループにする」の
	// 生きているトークンなら、名前の変更をその取り消しに含める。応答と誤りは Groups.RenameGroup と
	// 同じで、トークンが原因で失敗しない。
	RenameGroup(ctx context.Context, id int64, name, token string) (domain.Group, error)
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

// OrganizeGroup はドラッグした動画と落とした先の動画のグループを作る（POST /api/organize/group）。
func (s *server) OrganizeGroup(w http.ResponseWriter, r *http.Request) {
	var body gen.OrganizeGroup
	if !s.readLargeJSONBody(w, r, groupsBodyLimit, &body) {
		return
	}
	videoIDs, ok := s.organizeVideoIDs(w, body.Items)
	if !ok || !s.organizerConfigured(w) {
		return
	}
	result, err := s.organizer.Group(r.Context(), videoIDs, body.Target, body.From)
	if err != nil {
		s.writeGroupError(w, err, "Could not make the group.")
		return
	}
	s.writeOrganizeResult(w, result)
}

// organizeVideoIDs はドラッグしたカード items を動画の id にする。空か多すぎるか、動画でない
// カードがあれば 400 を書いて false を返す。
func (s *server) organizeVideoIDs(w http.ResponseWriter, items gen.OrganizeRefs) ([]int64, bool) {
	switch {
	case len(items) == 0:
		s.invalidRequest(w, "items must contain at least one item.")
		return nil, false
	case len(items) > maxVideoTagsIDs:
		s.invalidRequest(w, fmt.Sprintf("items must contain at most %d items.", maxVideoTagsIDs))
		return nil, false
	}
	ids := make([]int64, 0, len(items))
	for _, item := range items {
		if item.Kind != gen.OrganizeRefKindVideo {
			s.invalidRequest(w, "items must contain only videos.")
			return nil, false
		}
		ids = append(ids, item.Id)
	}
	return ids, true
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
