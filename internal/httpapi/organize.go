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
	// Move はカードを move.To へ足し、move.From から外す。ドラッグしたグループが To か To の祖先なら
	// domain.ErrGroupCycle。
	Move(ctx context.Context, move domain.OrganizeMove) (domain.OrganizeResult, error)
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
	case !namedGroupID(body.From) || !namedGroupID(body.To):
		// グループの id は 1 から始まる。明示した 0 以下を省略と取り違えると、無い移動先を
		// ライブラリへ外すことにしてしまう。
		s.writeGroupError(w, domain.ErrGroupTargetNotFound, "Could not move the cards.")
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

// namedGroupID は省ける id が、省かれたか、あり得るグループの id（1 以上）かを返す。
func namedGroupID(id *int64) bool {
	return id == nil || *id > 0
}

// optionalID は省ける id を、無ければ 0 にする。
func optionalID(id *int64) int64 {
	if id == nil {
		return 0
	}
	return *id
}

// OrganizeMerge はドラッグしたグループと動画をグループへマージする（POST /api/organize/merge）。
func (s *server) OrganizeMerge(w http.ResponseWriter, r *http.Request) {
	var body gen.OrganizeMerge
	if !s.readLargeJSONBody(w, r, groupsBodyLimit, &body) {
		return
	}
	items, problem := organizeRefs(body.Items)
	if problem == "" && !slices.ContainsFunc(items, func(item domain.OrganizeRef) bool { return item.Kind == domain.OrganizeGroup }) {
		problem = "items must contain at least one group."
	}
	if problem != "" {
		s.invalidRequest(w, problem)
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
