package httpapi

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// Keep the ID limit aligned with the OpenAPI schema.
const maxVideoTagsIDs = 20000

func (s *server) UpdateVideoTags(w http.ResponseWriter, r *http.Request) {
	var body gen.VideoTagsRequest
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if !validVideoTagsIDs(body.VideoIds) {
		s.tooManyVideos(w)
		return
	}
	tagID, tagName, ok := parseTagInput(body.Tag)
	if !ok {
		s.invalidRequest(w, "tag must have exactly one of id or name.")
		return
	}
	if s.tags == nil {
		s.internalError(w, "Tag storage is not configured.", nil)
		return
	}

	var (
		ref     domain.TagRef
		applied int
		err     error
	)
	switch body.Action {
	case gen.Add:
		if tagID != nil {
			ref, applied, err = s.tags.AttachTagByID(r.Context(), body.VideoIds, *tagID)
		} else {
			ref, applied, err = s.tags.AttachTagByName(r.Context(), body.VideoIds, *tagName)
		}
	case gen.Remove:
		// Removal accepts IDs only because the UI already knows the existing tag ID.
		if tagID == nil {
			s.invalidRequest(w, "Removing a tag requires tag.id.")
			return
		}
		ref, applied, err = s.tags.DetachTag(r.Context(), body.VideoIds, *tagID)
	default:
		s.invalidRequest(w, "Unknown action.")
		return
	}
	if err != nil {
		s.writeVideoTagsError(w, err)
		return
	}

	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.VideoTagsResponse{
		Tag:     toAPITagRef(ref),
		Applied: applied,
	}, s.logger)
}

func (s *server) SummarizeVideoTags(w http.ResponseWriter, r *http.Request) {
	var body gen.VideoTagsSummaryRequest
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if !validVideoTagsIDs(body.VideoIds) {
		s.tooManyVideos(w)
		return
	}
	if s.tags == nil {
		s.internalError(w, "Tag storage is not configured.", nil)
		return
	}

	summary, err := s.tags.Summary(r.Context(), body.VideoIds)
	if err != nil {
		s.internalError(w, "Could not load the tag summary.", err)
		return
	}

	items := make([]gen.VideoTagsSummaryItem, 0, len(summary.Items))
	for _, item := range summary.Items {
		items = append(items, gen.VideoTagsSummaryItem{
			Tag:         toAPITagRef(item.Tag),
			Count:       item.Count,
			ManualCount: item.ManualCount,
		})
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.VideoTagsSummary{Total: summary.Total, Items: items}, s.logger)
}

func (s *server) parseIDsQuery(w http.ResponseWriter, params videoQueryParams) (domain.VideoQuery, bool) {
	query := domain.VideoQuery{FavoriteOnly: params.favorite != nil && *params.favorite}
	filters, ok := s.parseListFilters(w, listFilterParams{watch: params.watch, playable: params.playable})
	if !ok {
		return domain.VideoQuery{}, false
	}
	query.Watch, query.PlayableOnly = filters.watch, filters.playableOnly
	if query.Query, ok = s.parseSearchQuery(w, params.query); !ok {
		return domain.VideoQuery{}, false
	}
	if query.TagIDs, ok = s.parseTagFilter(w, params.tag); !ok {
		return domain.VideoQuery{}, false
	}
	if !s.parseIDFilters(w, params, &query) {
		return domain.VideoQuery{}, false
	}
	if !s.parseAddedAndLength(w, params, &query) {
		return domain.VideoQuery{}, false
	}
	return query, true
}

// Omit group and smart-folder IDs when absent.
func (s *server) writeVideoIDs(w http.ResponseWriter, ids, missingTagIDs []int64, groups []gen.LibraryGroupIds, smartFolderIDs []int64) {
	if ids == nil {
		ids = []int64{}
	}
	payload := gen.VideoIdsResponse{Ids: ids}
	if len(missingTagIDs) > 0 {
		payload.MissingTagIds = &missingTagIDs
	}
	if len(groups) > 0 {
		payload.Groups = &groups
	}
	if len(smartFolderIDs) > 0 {
		payload.SmartFolderIds = &smartFolderIDs
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, payload, s.logger)
}

func validVideoTagsIDs(videoIDs []int64) bool {
	return len(videoIDs) >= 1 && len(videoIDs) <= maxVideoTagsIDs
}

func parseTagInput(tag gen.TagInput) (id *int64, name *string, ok bool) {
	if (tag.Id == nil) == (tag.Name == nil) {
		return nil, nil, false
	}
	return tag.Id, tag.Name, true
}

func (s *server) tooManyVideos(w http.ResponseWriter) {
	s.invalidRequestLimit(w, reasonTooManyVideos, maxVideoTagsIDs,
		fmt.Sprintf("videoIds must contain between 1 and %d items.", maxVideoTagsIDs))
}

func (s *server) writeVideoTagsError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidTagName):
		s.invalidTagName(w, err, "")
	case errors.Is(err, domain.ErrTagNotFound):
		s.writeError(w, http.StatusNotFound, codeTagNotFound, "Tag not found.")
	default:
		s.internalError(w, "Could not update the tag.", err)
	}
}
