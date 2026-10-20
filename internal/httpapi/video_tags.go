package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"slices"

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
	if !validVideoTargetCounts(body.VideoIds, body.SmartFolderIds) {
		s.tooManyVideos(w)
		return
	}
	tagID, tagName, valid := parseTagInput(body.Tag)
	if !valid {
		s.invalidRequest(w, "tag must have exactly one of id or name.")
		return
	}
	if s.tags == nil {
		s.internalError(w, "Tag storage is not configured.", nil)
		return
	}
	videoIDs, ok := s.resolveVideoTargets(w, r, body.VideoIds, body.SmartFolderIds)
	if !ok {
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
			ref, applied, err = s.tags.AttachTagByID(r.Context(), videoIDs, *tagID)
		} else {
			ref, applied, err = s.tags.AttachTagByName(r.Context(), videoIDs, *tagName)
		}
	case gen.Remove:
		// Removal accepts IDs only because the UI already knows the existing tag ID.
		if tagID == nil {
			s.invalidRequest(w, "Removing a tag requires tag.id.")
			return
		}
		ref, applied, err = s.tags.DetachTag(r.Context(), videoIDs, *tagID)
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
	if s.tags == nil {
		s.internalError(w, "Tag storage is not configured.", nil)
		return
	}
	videoIDs, ok := s.resolveVideoTargets(w, r, body.VideoIds, body.SmartFolderIds)
	if !ok {
		return
	}

	summary, err := s.tags.Summary(r.Context(), videoIDs)
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

// validVideoTargetCounts checks the request's own array lengths; resolveVideoTargets also limits the resolved total.
func validVideoTargetCounts(videoIDs []int64, smartFolderIDs *[]int64) bool {
	folders := 0
	if smartFolderIDs != nil {
		folders = len(*smartFolderIDs)
	}
	return len(videoIDs)+folders >= 1 && len(videoIDs) <= maxVideoTagsIDs && folders <= maxVideoTagsIDs
}

// resolveVideoTargets adds the current members of smartFolderIDs to videoIDs and applies the video limit to the
// distinct result, so a request cannot exceed it through folders. It writes the error response when it returns false.
func (s *server) resolveVideoTargets(w http.ResponseWriter, r *http.Request, videoIDs []int64, smartFolderIDs *[]int64) ([]int64, bool) {
	if !validVideoTargetCounts(videoIDs, smartFolderIDs) {
		s.tooManyVideos(w)
		return nil, false
	}
	if smartFolderIDs == nil || len(*smartFolderIDs) == 0 {
		return videoIDs, true
	}
	if !s.smartFoldersConfigured(w) {
		return nil, false
	}
	members, err := s.smartFolders.SmartFolderVideoIDs(r.Context(), *smartFolderIDs)
	if err != nil {
		s.internalError(w, "Could not read the smart folder videos.", err)
		return nil, false
	}
	resolved := make([]int64, 0, len(videoIDs)+len(members))
	seen := make(map[int64]bool, cap(resolved))
	for _, id := range slices.Concat(videoIDs, members) {
		if !seen[id] {
			seen[id] = true
			resolved = append(resolved, id)
		}
	}
	// Folders that resolve to nothing leave no target, which would otherwise create an unused tag by name.
	if len(resolved) < 1 || len(resolved) > maxVideoTagsIDs {
		s.tooManyVideos(w)
		return nil, false
	}
	return resolved, true
}

func parseTagInput(tag gen.TagInput) (id *int64, name *string, ok bool) {
	if (tag.Id == nil) == (tag.Name == nil) {
		return nil, nil, false
	}
	return tag.Id, tag.Name, true
}

func (s *server) tooManyVideos(w http.ResponseWriter) {
	s.invalidRequestLimit(w, reasonTooManyVideos, maxVideoTagsIDs,
		fmt.Sprintf("videoIds and the smart folders' videos must total between 1 and %d distinct videos.", maxVideoTagsIDs))
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
