package httpapi

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/extgen"
)

// ExternalVideos always reads as owner and therefore has no audience argument.
type ExternalVideos interface {
	// ListExternalVideos includes only registered locations and orders by (added_at, id) ascending.
	ListExternalVideos(ctx context.Context, q domain.ExternalVideoQuery) (domain.ExternalVideoPage, error)
	// Unregistered-only locations are not found.
	LookupExternalVideo(ctx context.Context, ref domain.VideoRef) (domain.ExternalVideo, error)
}

// External list limits reject out-of-range values rather than clamping, as required by this API's contract.
func (e *externalServer) ListVideos(w http.ResponseWriter, r *http.Request, params extgen.ListVideosParams) {
	if e.s.externalVideos == nil {
		e.internalError(w, "Video storage is not configured.", nil)
		return
	}
	query := domain.ExternalVideoQuery{Limit: domain.ExternalVideoDefaultLimit}
	if params.Limit != nil {
		if *params.Limit < 1 || *params.Limit > domain.MaxLimit {
			e.invalidRequest(w, nil, "limit must be between 1 and 200.")
			return
		}
		query.Limit = *params.Limit
	}
	if params.Cursor != nil {
		query.Cursor = *params.Cursor
	}
	page, err := e.s.externalVideos.ListExternalVideos(r.Context(), query)
	if errors.Is(err, domain.ErrInvalidCursor) {
		reason := extgen.InvalidCursor
		e.invalidRequest(w, &reason, "Cannot read the cursor. Read the list again from the start.")
		return
	}
	if err != nil {
		e.internalError(w, "Could not load the videos.", err)
		return
	}
	out := extgen.ExternalVideoPage{Items: make([]extgen.ExternalVideo, 0, len(page.Items)), NextCursor: page.NextCursor}
	for _, item := range page.Items {
		out.Items = append(out.Items, toExternalVideo(item))
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, out, e.s.logger)
}

// Count reference parameters by presence, including invalid empty values.
func (e *externalServer) LookupVideo(w http.ResponseWriter, r *http.Request, params extgen.LookupVideoParams) {
	if e.s.externalVideos == nil {
		e.internalError(w, "Video storage is not configured.", nil)
		return
	}
	var ref domain.VideoRef
	given := 0
	if params.Id != nil {
		given++
		ref.ID = *params.Id
	}
	if params.ContentKey != nil {
		given++
		ref.ContentKey = *params.ContentKey
	}
	if params.Path != nil {
		given++
		ref.Path = *params.Path
	}
	if given != 1 {
		e.invalidRequest(w, nil, "Give exactly one of id, contentKey and path.")
		return
	}
	if (params.ContentKey != nil && ref.ContentKey == "") || (params.Path != nil && ref.Path == "") {
		e.invalidRequest(w, nil, "contentKey and path must not be empty.")
		return
	}
	// Nonpositive IDs are lookup misses, producing 404.
	video, err := e.s.externalVideos.LookupExternalVideo(r.Context(), ref)
	if errors.Is(err, domain.ErrNotFound) {
		reason := extgen.VideoNotFound
		writeExternalError(w, e.s, http.StatusNotFound, extgen.Error{
			Code: extgen.ErrorCodeNotFound, Reason: &reason, Message: "The video is not in the library.",
		})
		return
	}
	if err != nil {
		e.internalError(w, "Could not look up the video.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, toExternalVideo(video), e.s.logger)
}

func (e *externalServer) invalidRequest(w http.ResponseWriter, reason *extgen.ErrorReason, message string) {
	writeExternalError(w, e.s, http.StatusBadRequest, extgen.Error{
		Code: extgen.ErrorCodeInvalidRequest, Reason: reason, Message: message,
	})
}

func toExternalVideo(item domain.ExternalVideo) extgen.ExternalVideo {
	out := extgen.ExternalVideo{
		Id:                  item.Video.ID,
		ContentKey:          item.Video.ContentKey,
		Title:               item.Video.Title,
		FileTitle:           item.Video.FileTitle,
		DisplayName:         optionalString(item.Video.DisplayName),
		ThumbnailPositionMs: item.Video.ThumbnailPositionMs,
		DurationMs:          item.Video.DurationMs,
		AddedAt:             item.Video.AddedAt.UTC(),
		UpdatedAt:           item.Video.EditedAt.UTC(),
		FileCreatedAt:       item.Video.FileCreatedAt.UTC(),
		Locations:           make([]extgen.ExternalVideoLocation, 0, len(item.Locations)),
		Tags:                toExternalVideoTags(item.Tags),
		WorkInfo:            toExternalWorkInfo(item.Video.WorkInfo),
	}
	for _, path := range item.Locations {
		out.Locations = append(out.Locations, extgen.ExternalVideoLocation{Path: path, FileName: filepath.Base(path)})
	}
	return out
}

func toExternalVideoTags(tags []domain.VideoTag) []extgen.ExternalVideoTag {
	out := make([]extgen.ExternalVideoTag, 0, len(tags))
	for _, tag := range tags {
		out = append(out, extgen.ExternalVideoTag{
			Id: tag.ID, Name: tag.Name, Manual: tag.Manual, FromFolder: tag.FromFolder, Tentative: tag.Tentative,
		})
	}
	return out
}
