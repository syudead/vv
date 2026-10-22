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

// Organizer writes atomically and returns undo tokens; missing group targets leave storage unchanged.
type Organizer interface {
	Reorder(ctx context.Context, groupID int64, itemIDs []int64, afterItemID *int64) (domain.OrganizeResult, error)
	Group(ctx context.Context, videoIDs []int64, targetID int64, fromID *int64) (domain.OrganizeResult, error)
	// A live creation token incorporates the rename into undo; token problems never fail the rename itself.
	RenameGroup(ctx context.Context, id int64, name, token string) (domain.Group, error)
	Move(ctx context.Context, move domain.OrganizeMove) (domain.OrganizeResult, error)
	// Bundle expands existing groups before enforcing MaxOrganizeBundleVideos and chooses the drop target as representative.
	Bundle(ctx context.Context, videoIDs []int64, targetID int64) (domain.OrganizeResult, error)
	// Merge follows screen order, appending videos and rejecting self/ancestor group cycles.
	Merge(ctx context.Context, items []domain.OrganizeRef, into, from int64) (domain.OrganizeResult, error)
	// Undo reports changed rows as conflict and missing/used/expired tokens as expired.
	Undo(ctx context.Context, token string) error
}

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
		// Explicit nonpositive group IDs must not become omitted destinations that remove items into the library.
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

func (s *server) OrganizeBundle(w http.ResponseWriter, r *http.Request) {
	var body gen.OrganizeBundle
	if !s.readJSONBody(w, r, &body) {
		return
	}
	ids := uniqueInt64s(append([]int64{body.Target}, body.VideoIds...))
	switch {
	case body.Target <= 0:
		// Reject missing required IDs before zero becomes a misleading lookup 404.
		s.invalidRequest(w, "target is required.")
		return
	case len(body.VideoIds) == 0 || len(ids) < 2:
		s.invalidRequest(w, "videoIds must contain a video other than the target.")
		return
	case len(ids) > domain.MaxOrganizeBundleVideos:
		s.tooManyBundleVideos(w)
		return
	}
	if !s.organizerConfigured(w) {
		return
	}
	result, err := s.organizer.Bundle(r.Context(), body.VideoIds, body.Target)
	switch {
	case errors.Is(err, domain.ErrTooManyVersions):
		s.tooManyBundleVideos(w)
		return
	case errors.Is(err, domain.ErrTooFewVersions):
		s.invalidRequest(w, "videoIds must contain a video other than the target.")
		return
	case errors.Is(err, domain.ErrVirtualVideo):
		s.virtualVideoInBundle(w)
		return
	case err != nil:
		s.writeGroupError(w, err, "Could not bundle the videos.")
		return
	}
	s.writeOrganizeResult(w, result)
}

func (s *server) tooManyBundleVideos(w http.ResponseWriter) {
	s.invalidRequestLimit(w, reasonTooManyVideos, domain.MaxOrganizeBundleVideos,
		fmt.Sprintf("Bundle at most %d videos at a time.", domain.MaxOrganizeBundleVideos))
}

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

func namedGroupID(id *int64) bool {
	return id == nil || *id > 0
}

func optionalID(id *int64) int64 {
	if id == nil {
		return 0
	}
	return *id
}

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
	if !namedGroupID(body.From) {
		// Explicit nonpositive from IDs must not become omitted sources that leave the original membership behind.
		s.writeGroupError(w, domain.ErrGroupTargetNotFound, "Could not merge the groups.")
		return
	}
	if !s.organizerConfigured(w) {
		return
	}
	result, err := s.organizer.Merge(r.Context(), items, body.Into, optionalID(body.From))
	if err != nil {
		s.writeGroupError(w, err, "Could not merge the groups.")
		return
	}
	s.writeOrganizeResult(w, result)
}

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

func (s *server) organizerConfigured(w http.ResponseWriter) bool {
	if s.organizer == nil {
		s.internalError(w, "Drag operations are not configured.", nil)
		return false
	}
	return true
}

func (s *server) writeOrganizeResult(w http.ResponseWriter, result domain.OrganizeResult) {
	out := gen.OrganizeResult{UndoToken: result.UndoToken}
	if result.GroupID != 0 {
		id := result.GroupID
		out.GroupId = &id
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, out, s.logger)
}
