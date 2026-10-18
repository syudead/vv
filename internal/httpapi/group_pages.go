package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// Group pages are guest-readable but aggregate only publicly visible videos (docs/design-docs/catalog-organization.md).

func (s *server) GetGroup(w http.ResponseWriter, r *http.Request, id gen.GroupId, params gen.GetGroupParams) {
	if s.library == nil {
		s.internalError(w, "Group queries are not configured.", nil)
		return
	}
	var trail []int64
	if params.Trail != nil {
		trail = *params.Trail
	}
	audience := audienceFrom(r.Context())
	page, err := s.library.Group(r.Context(), audience, id, trail)
	if !s.groupReadOK(w, err) {
		return
	}
	group, items, ok := s.presentGroupPage(r.Context(), audience, page)
	if !ok {
		s.notFound(w, "The group does not exist.")
		return
	}
	payload := gen.GroupDetail{Group: group, Trail: make([]gen.GroupRef, 0, len(page.Trail)), Items: items}
	for _, ref := range page.Trail {
		payload.Trail = append(payload.Trail, gen.GroupRef{Id: ref.ID, Name: ref.Name})
	}
	if page.NextCursor != "" {
		next := page.NextCursor
		payload.NextCursor = &next
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, payload, s.logger)
}

func (s *server) ListGroupItems(w http.ResponseWriter, r *http.Request, id gen.GroupId, params gen.ListGroupItemsParams) {
	if s.library == nil {
		s.internalError(w, "Group queries are not configured.", nil)
		return
	}
	limit := domain.GroupItemsDefaultLimit
	if params.Limit != nil {
		if *params.Limit < 1 || *params.Limit > domain.GroupItemsMaxLimit {
			s.invalidRequest(w, "limit must be between 1 and 200.")
			return
		}
		limit = *params.Limit
	}
	cursor := ""
	if params.Cursor != nil {
		cursor = *params.Cursor
	}
	audience := audienceFrom(r.Context())
	page, err := s.library.GroupItems(r.Context(), audience, id, cursor, limit)
	if !s.groupReadOK(w, err) {
		return
	}
	_, items, ok := s.presentGroupPage(r.Context(), audience, page)
	if !ok {
		s.notFound(w, "The group does not exist.")
		return
	}
	payload := gen.GroupItemPage{Items: items}
	if page.NextCursor != "" {
		next := page.NextCursor
		payload.NextCursor = &next
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, payload, s.logger)
}

func (s *server) groupReadOK(w http.ResponseWriter, err error) bool {
	switch {
	case err == nil:
		return true
	case errors.Is(err, domain.ErrNotFound):
		s.notFound(w, "The group does not exist.")
	case errors.Is(err, domain.ErrInvalidCursor):
		s.invalidRequestReason(w, reasonInvalidCursor, "Cannot read the cursor. Reload the group.")
	default:
		s.internalError(w, "Could not load the group.", err)
	}
	return false
}

// presentGroupPage uses library item presentation and returns false when no videos are visible.
func (s *server) presentGroupPage(ctx context.Context, audience domain.Audience, page domain.GroupPage) (gen.LibraryGroup, []gen.GroupItem, bool) {
	var shown []domain.Video
	tagged := append([]domain.Video{}, page.Group.Members...)
	for _, item := range page.Items {
		if item.Video != nil {
			shown = append(shown, *item.Video)
			tagged = append(tagged, *item.Video)
		} else {
			tagged = append(tagged, item.Group.Members...)
		}
	}
	lookup := s.itemLookups(ctx, shown, tagged)
	group, ok := lookup.group(ctx, audience, page.Roots, page.Group)
	if !ok {
		return gen.LibraryGroup{}, nil, false
	}
	items := make([]gen.GroupItem, 0, len(page.Items))
	for _, item := range page.Items {
		if item.Video != nil {
			video := lookup.video(ctx, audience, page.Roots, *item.Video)
			items = append(items, gen.GroupItem{Id: item.ID, Kind: gen.GroupItemKindVideo, Video: &video})
			continue
		}
		child, ok := lookup.group(ctx, audience, page.Roots, *item.Group)
		if !ok {
			continue
		}
		items = append(items, gen.GroupItem{Id: item.ID, Kind: gen.GroupItemKindGroup, Group: &child})
	}
	return group, items, true
}
