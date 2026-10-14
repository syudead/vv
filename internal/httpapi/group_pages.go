package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// グループの画面の読み取り（GET /api/groups/{id} と GET /api/groups/{id}/items、
// specs/048-groups/contracts/screen-api.md）。所有者とゲストが読め（accessRoutes）、ゲストには
// 公開の動画だけから作ったグループを返す。

// GetGroup はグループ 1 つと、その道筋と項目の最初のページを返す（GET /api/groups/{id}）。
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

// ListGroupItems はグループの項目の続きのページを返す（GET /api/groups/{id}/items）。
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

// groupReadOK はグループの読み取りの誤りを応答にし、続けてよいかを返す。
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

// presentGroupPage はグループの値と項目を応答の形にする。項目の動画はライブラリの動画の項目と
// 同じく再生位置・タグ・フォルダを載せ、子グループはライブラリのグループの項目と同じ形にする。
// グループに見せる動画が無ければ false を返す。
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
