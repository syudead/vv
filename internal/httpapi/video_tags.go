package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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
	if !validVideoTargetCounts(body.VideoIds, body.SmartFolderIds, maxVideoTagsIDs) {
		s.tooManyVideos(w, maxVideoTagsIDs)
		return
	}
	tagID, tagName, valid := parseTagInput(body.Tag)
	switch {
	case !valid:
		s.invalidRequest(w, "tag must have exactly one of id or name.")
		return
	case body.Action != gen.Add && body.Action != gen.Remove:
		s.invalidRequest(w, "Unknown action.")
		return
	case body.Action == gen.Remove && tagID == nil:
		// Removal accepts IDs only because the UI already knows the existing tag ID.
		s.invalidRequest(w, "Removing a tag requires tag.id.")
		return
	case s.tags == nil:
		s.internalError(w, "Tag storage is not configured.", nil)
		return
	}
	sel, ok := s.resolveSelection(w, r, body.VideoIds, body.SmartFolderIds, maxVideoTagsIDs)
	if !ok {
		return
	}
	edit := domain.VideoEdit{TagsAdd: []domain.TagInput{{ID: tagID, Name: tagName}}}
	if body.Action == gen.Remove {
		edit = domain.VideoEdit{TagsRemove: []int64{*tagID}}
	}
	parts, ok := s.splitVideoEdit(w, r, edit, sel)
	if !ok {
		return
	}
	out := gen.VideoTagsResponse{Tag: gen.TagRef{Name: valueOrEmpty(tagName)}}
	if tagID != nil {
		out.Tag.Id = *tagID
	}
	if parts.local != nil {
		ref, applied, err := s.applyTag(r.Context(), sel.local, *parts.local)
		if err != nil {
			s.writeVideoTagsError(w, err)
			return
		}
		out.Tag, out.Applied = toAPITagRef(ref), applied
	}
	if sel.hasPeer() {
		ref, applied := s.tagOnPeer(r, body.Action, sel, &parts, &out)
		if ref != nil && parts.local == nil {
			out.Tag = *ref
		}
		out.Applied += applied
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, out, s.logger)
}

// applyTag applies the one tag of edit, an addition or a removal, to this node's videos.
func (s *server) applyTag(ctx context.Context, videoIDs []int64, edit domain.VideoEdit) (domain.TagRef, int, error) {
	if len(edit.TagsRemove) > 0 {
		return s.tags.DetachTag(ctx, videoIDs, edit.TagsRemove[0])
	}
	if input := edit.TagsAdd[0]; input.ID != nil {
		return s.tags.AttachTagByID(ctx, videoIDs, *input.ID)
	}
	return s.tags.AttachTagByName(ctx, videoIDs, *edit.TagsAdd[0].Name)
}

// tagOnPeer applies the peer's part of a tag change and sets its result in out; it returns the peer's tag in this
// node's ids, when the peer answered one, and the videos it changed.
func (s *server) tagOnPeer(r *http.Request, action gen.VideoTagsAction, sel selection, parts *videoEditParts, out *gen.VideoTagsResponse) (*gen.TagRef, int) {
	if parts.peerErr != nil || parts.peer == nil {
		out.Remote = s.remoteSelectionResult(0, sel.peer, parts.peerErr)
		return nil, 0
	}
	ids, folders := sel.peerTargets()
	request := gen.VideoTagsRequest{Action: action, VideoIds: ids, SmartFolderIds: folders}
	if len(parts.peer.TagsRemove) > 0 {
		request.Tag.Id = &parts.peer.TagsRemove[0]
	} else {
		request.Tag = gen.TagInput{Id: parts.peer.TagsAdd[0].ID, Name: parts.peer.TagsAdd[0].Name}
	}
	var answer gen.VideoTagsResponse
	err := s.peerCall(r, http.MethodPost, "/api/video-tags", nil, request, &answer)
	out.Remote = s.remoteSelectionResult(answer.Applied, sel.peer, err)
	if err != nil {
		return nil, 0
	}
	join, err := s.vocabularyJoin(r.Context())
	if err != nil {
		s.logger.Warn("could not join the other VVMDM's tag", slog.Any("error", err))
		return nil, answer.Applied
	}
	ref := peerTagRef(answer.Tag, join)
	return &ref, answer.Applied
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
	sel, ok := s.resolveSelection(w, r, body.VideoIds, body.SmartFolderIds, maxVideoTagsIDs)
	if !ok {
		return
	}
	summary := domain.TagSummary{Items: []domain.TagSummaryItem{}}
	if len(sel.local) > 0 {
		var err error
		if summary, err = s.tags.Summary(r.Context(), sel.local); err != nil {
			s.internalError(w, "Could not load the tag summary.", err)
			return
		}
	}
	var peer gen.VideoTagsSummary
	if sel.hasPeer() && s.peerSelectionRead(r, "/api/video-tags/summary", sel, &peer) {
		join, err := s.vocabularyJoin(r.Context())
		if err != nil {
			s.internalError(w, "Could not load the tag summary.", err)
			return
		}
		summary = domain.MergeTagSummaries(summary, domain.TagSummary{Total: peer.Total, Items: peerTagSummaryItems(peer.Items, join)})
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
func validVideoTargetCounts(videoIDs []int64, smartFolderIDs *[]int64, limit int) bool {
	folders := 0
	if smartFolderIDs != nil {
		folders = len(*smartFolderIDs)
	}
	return len(videoIDs)+folders >= 1 && len(videoIDs) <= limit && folders <= limit
}

// resolveVideoTargets adds the current members of smartFolderIDs to videoIDs and applies limit to the distinct
// result, so a request cannot exceed it through folders. It writes the error response when it returns false.
func (s *server) resolveVideoTargets(
	w http.ResponseWriter, r *http.Request, videoIDs []int64, smartFolderIDs *[]int64, limit int,
) ([]int64, bool) {
	if !validVideoTargetCounts(videoIDs, smartFolderIDs, limit) {
		s.tooManyVideos(w, limit)
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
	if len(resolved) < 1 || len(resolved) > limit {
		s.tooManyVideos(w, limit)
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

func (s *server) tooManyVideos(w http.ResponseWriter, limit int) {
	s.invalidRequestLimit(w, reasonTooManyVideos, limit,
		fmt.Sprintf("videoIds and the smart folders' videos must total between 1 and %d distinct videos.", limit))
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
