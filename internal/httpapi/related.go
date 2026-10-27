package httpapi

import (
	"context"
	"net/http"
	"slices"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// VideoCatalog owns ranking; the first page also includes a bounded group-member window around the current video.
// With a peer link, the screens' list holds the videos of both nodes (peer_related.go); the peer's own request
// gets this node's videos only.
func (s *server) GetRelatedVideos(w http.ResponseWriter, r *http.Request, id gen.VideoId, params gen.GetRelatedVideosParams) {
	limit, ok := s.relatedLimit(w, params.Limit)
	if !ok {
		return
	}
	if domain.IsRemoteID(id) {
		s.remoteRelatedVideos(w, r, id, params, limit)
		return
	}
	if s.peerLink != nil && !isPeerRequest(r.Context()) {
		s.mergedRelatedVideos(w, r, id, params, limit)
		return
	}
	opened := videoContext(params.Group, params.SmartFolder)
	page := domain.RelatedPage{Limit: limit, GroupID: opened.GroupID, SmartFolderID: opened.SmartFolderID}
	if params.Cursor != nil {
		cursor, err := domain.ParseRelatedCursor(*params.Cursor)
		if err != nil {
			s.invalidCursor(w)
			return
		}
		page.After = &cursor
	}
	video, ok := s.lookupVideo(w, r, id)
	if !ok || !s.requireCatalog(w) {
		return
	}
	audience := audienceFrom(r.Context())

	related, err := s.catalog.RelatedVideos(r.Context(), audience, video, page)
	if err != nil {
		s.internalError(w, "Could not load related videos.", err)
		return
	}

	// Batch progress/tag reads for related and group-member videos together.
	present := s.presenter(r.Context(), audience, withGroupMembers(related.Items, related.Group))
	cursor := ""
	if related.NextCursor != nil {
		cursor = related.NextCursor.String()
	}
	payload := relatedBody(present(r.Context(), related.Items), presentGroup(r.Context(), present, related.Group),
		cursor, related.NextID, related.PrevID)
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, payload, s.logger)
}

func (s *server) relatedLimit(w http.ResponseWriter, limit *int) (int, bool) {
	if limit == nil {
		return domain.DefaultRelatedPageSize, true
	}
	if *limit < 1 || *limit > domain.MaxRelatedPageSize {
		s.invalidRequest(w, "limit must be between 1 and 100.")
		return 0, false
	}
	return *limit, true
}

func (s *server) invalidCursor(w http.ResponseWriter) {
	s.invalidRequest(w, "cursor is not a value returned as nextCursor.")
}

func (s *server) requireCatalog(w http.ResponseWriter) bool {
	if s.catalog == nil {
		s.internalError(w, "Related video queries are not configured.", nil)
		return false
	}
	return true
}

func withGroupMembers(items []domain.Video, group *domain.VideoGroup) []domain.Video {
	if group == nil {
		return items
	}
	return append(slices.Clone(items), group.Members...)
}

func presentGroup(ctx context.Context, present func(context.Context, []domain.Video) []gen.Video, group *domain.VideoGroup) *gen.RelatedGroup {
	if group == nil {
		return nil
	}
	return &gen.RelatedGroup{
		Id:     group.ID,
		Name:   group.Name,
		Items:  present(ctx, group.Members),
		Offset: group.Offset,
		Total:  group.Total(),
	}
}

// relatedBody leaves out an empty cursor and zero ids.
func relatedBody(items []gen.Video, group *gen.RelatedGroup, cursor string, nextID, prevID int64) gen.RelatedVideos {
	payload := gen.RelatedVideos{Items: items, Group: group}
	if cursor != "" {
		payload.NextCursor = &cursor
	}
	if nextID != 0 {
		payload.NextId = &nextID
	}
	if prevID != 0 {
		payload.PrevId = &prevID
	}
	return payload
}

func (s *server) ListVideoGroupMembers(w http.ResponseWriter, r *http.Request, id gen.VideoId, params gen.ListVideoGroupMembersParams) {
	window := domain.GroupWindow{Limit: domain.GroupMemberWindow}
	if params.Offset != nil {
		if *params.Offset < 0 {
			s.invalidRequest(w, "offset must be 0 or more.")
			return
		}
		window.Offset = *params.Offset
	}
	if params.Limit != nil {
		if *params.Limit < 1 || *params.Limit > domain.MaxGroupMemberPage {
			s.invalidRequest(w, "limit must be between 1 and 200.")
			return
		}
		window.Limit = *params.Limit
	}
	video, ok := s.lookupVideo(w, r, id)
	if !ok || !s.requireCatalog(w) {
		return
	}
	audience := audienceFrom(r.Context())
	group, grouped, err := s.catalog.VideoGroup(r.Context(), audience, video, videoContext(params.Group, params.SmartFolder), window)
	if err != nil {
		s.internalError(w, "Could not load the group members.", err)
		return
	}
	if !grouped {
		s.notFound(w, "The video is not in a group.")
		return
	}
	present := s.presenter(r.Context(), audience, group.Members)
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.GroupMemberPage{
		Items:  present(r.Context(), group.Members),
		Offset: group.Offset,
		Total:  group.Total(),
	}, s.logger)
}

func (s *server) presenter(ctx context.Context, audience domain.Audience, shown []domain.Video) func(context.Context, []domain.Video) []gen.Video {
	extras := s.extrasFor(ctx, shown)
	return func(ctx context.Context, videos []domain.Video) []gen.Video {
		out := make([]gen.Video, 0, len(videos))
		for _, view := range s.presentVideos(ctx, videos) {
			out = append(out, forAudience(audience, extras.item(view)))
		}
		return out
	}
}

// Return exactly one context reference, group or smartFolder.
func videoContextRefs(group domain.VideoGroup, videoID int64) (*gen.VideoGroupRef, *gen.VideoSmartFolderRef) {
	position, count := group.Position(videoID), group.Total()
	if group.SmartFolder {
		return nil, &gen.VideoSmartFolderRef{Id: group.ID, Name: group.Name, Position: position, Count: count}
	}
	return &gen.VideoGroupRef{
		Id: group.ID, Folder: apiGroupFolder(group.Folder), Name: group.Name, Position: position, Count: count,
	}, nil
}

// Manual groups and unregistered automatic paths have no folder metadata.
func apiGroupFolder(folder *domain.VideoFolder) *gen.VideoFolder {
	if folder == nil {
		return nil
	}
	return &gen.VideoFolder{RootId: folder.RootID, Path: folder.Path}
}
