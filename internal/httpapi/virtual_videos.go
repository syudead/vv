package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// Virtual-video writes are owner-only. Saves and deletion carry the contentVersion the screen read, which for a
// virtual video is its whole user key, so a stale screen never writes into a later video that reused the row id.

type VirtualVideoStore interface {
	// Create returns ErrInvalidVirtualTitle for an empty title and InvalidDisplayNameError for an unusable one.
	Create(ctx context.Context, title string) (domain.Video, error)
	// CreateFromLink stores link as the video's link and starts its fetch; an empty title comes from the link. It
	// returns ErrInvalidVirtualLink for a link that is not an http or https URL, and Create's title errors.
	CreateFromLink(ctx context.Context, title, link string) (domain.Video, error)
	// FinishLinkFetch records the end of a running link fetch.
	FinishLinkFetch(ctx context.Context, key, plugin, failure string) error
	// LinkFetch reads the fetch of a video created from a link; false for any other video.
	LinkFetch(ctx context.Context, key string) (domain.LinkFetch, bool, error)
	// SaveTitle, SaveDetails and Delete return ErrNotFound for a missing video, ErrNotVirtual for a video with a
	// file and ErrContentChanged for another contentVersion.
	SaveTitle(ctx context.Context, videoID int64, contentVersion, title string) (domain.Video, error)
	// SaveDetails writes the title and the work information atomically.
	SaveDetails(ctx context.Context, videoID int64, contentVersion, title string, info domain.WorkInfo) (domain.Video, error)
	Delete(ctx context.Context, videoID int64, contentVersion string) error
	// Merge returns the file video. Besides the errors of Delete for the virtual video, it returns ErrNotFound and
	// ErrContentChanged for the file video, ErrVirtualVideo when videoID has no file, and
	// ErrMergedDescriptionTooLong or ErrMergedTooManyURLs when the combined work information exceeds its limits.
	Merge(ctx context.Context, virtualID int64, contentVersion string, videoID int64, videoContentVersion string) (domain.Video, error)
}

func (s *server) CreateVirtualVideo(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title *string `json:"title"`
		URL   *string `json:"url"`
	}
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if body.Title == nil && body.URL == nil {
		s.invalidRequest(w, "title or url is required.")
		return
	}
	if !s.virtualVideosConfigured(w) {
		return
	}
	video, err := s.createVirtualVideo(r.Context(), body.Title, body.URL)
	if errors.Is(err, domain.ErrInvalidVirtualLink) {
		s.invalidRequest(w, "url must be an http or https URL of at most 2048 characters.")
		return
	}
	if err != nil {
		s.writeVirtualVideoError(w, err, "", "Could not create the video.")
		return
	}
	payload, err := s.videoDetail(r, video, domain.VideoContext{})
	if err != nil {
		s.internalError(w, "Could not load the video.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusCreated, payload, s.logger)
}

// createVirtualVideo creates from the title alone, or from the link with an optional title and then starts the
// link's fetch, which ends after the response.
func (s *server) createVirtualVideo(ctx context.Context, title, link *string) (domain.Video, error) {
	if link == nil {
		return s.virtualVideos.Create(ctx, *title)
	}
	var typed string
	if title != nil {
		typed = *title
	}
	video, err := s.virtualVideos.CreateFromLink(ctx, typed, *link)
	if err != nil {
		return domain.Video{}, err
	}
	normalized, _ := domain.NormalizeVirtualLink(*link)
	if s.workInfoPlugins == nil {
		err := s.virtualVideos.FinishLinkFetch(ctx, video.ContentKey, "", "Plugins are not available.")
		return video, err
	}
	s.workInfoPlugins.StartLinkFetch(video, normalized)
	return video, nil
}

func (s *server) DeleteVideo(w http.ResponseWriter, r *http.Request, id gen.VideoId, params gen.DeleteVideoParams) {
	if !s.virtualVideosConfigured(w) {
		return
	}
	err := s.virtualVideos.Delete(r.Context(), id, params.ContentVersion)
	if errors.Is(err, domain.ErrNotVirtual) {
		s.conflictReason(w, reasonNotVirtual, "Only a video without a file can be deleted.")
		return
	}
	if err != nil {
		s.writeVirtualVideoError(w, err, "", "Could not delete the video.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) MergeVirtualVideo(w http.ResponseWriter, r *http.Request, id gen.VideoId) {
	var body struct {
		ContentVersion      *string `json:"contentVersion"`
		VideoID             *int64  `json:"videoId"`
		VideoContentVersion *string `json:"videoContentVersion"`
	}
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if body.ContentVersion == nil || body.VideoID == nil || body.VideoContentVersion == nil {
		s.invalidRequest(w, "contentVersion, videoId and videoContentVersion are required.")
		return
	}
	if !s.virtualVideosConfigured(w) {
		return
	}
	video, err := s.virtualVideos.Merge(r.Context(), id, *body.ContentVersion, *body.VideoID, *body.VideoContentVersion)
	if err != nil {
		s.writeMergeError(w, err)
		return
	}
	s.writeVideoDetail(w, r, video)
}

func (s *server) writeMergeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrNotVirtual):
		s.conflictReason(w, reasonNotVirtual, "Only a video without a file can be merged.")
	case errors.Is(err, domain.ErrVirtualVideo):
		s.conflictReason(w, reasonVirtualVideo, "A video without a file cannot be merged into.")
	case errors.Is(err, domain.ErrMergedDescriptionTooLong):
		s.conflictReason(w, reasonDescriptionTooLong, "The combined description is too long.")
	case errors.Is(err, domain.ErrMergedTooManyURLs):
		s.conflictReason(w, reasonTooManyURLs, "The combined links are too many.")
	default:
		s.writeVirtualVideoError(w, err, "", "Could not merge the video.")
	}
}

// virtualSaveTarget reports whether the video is virtual, writing 404 for a missing video and 400 when a virtual
// video's save lacks contentVersion. A file video saved with a virtual video's version answers 409 content_changed:
// the screen read a deleted virtual video whose row id the file video reused.
func (s *server) virtualSaveTarget(w http.ResponseWriter, r *http.Request, id int64, contentVersion *string) (bool, bool) {
	video, ok := s.lookupVideo(w, r, id)
	if !ok {
		return false, false
	}
	if !video.Virtual() {
		if contentVersion != nil && domain.IsVirtualKey(*contentVersion) {
			s.writeVirtualVideoError(w, domain.ErrNotVirtual, "", "Could not save the video.")
			return false, false
		}
		return false, true
	}
	if contentVersion == nil {
		s.invalidRequest(w, "contentVersion is required for a video without a file.")
		return false, false
	}
	return true, s.virtualVideosConfigured(w)
}

// The store checks the title again; checking it here answers before the work information is read.
func (s *server) virtualTitleUsable(w http.ResponseWriter, title, field string) bool {
	_, err := domain.NormalizeVirtualTitle(title)
	if err != nil {
		s.writeVirtualVideoError(w, err, field, "Could not save the title.")
	}
	return err == nil
}

func (s *server) saveVirtualTitle(w http.ResponseWriter, r *http.Request, id int64, contentVersion, title string) {
	if !s.virtualTitleUsable(w, title, "") {
		return
	}
	video, err := s.virtualVideos.SaveTitle(r.Context(), id, contentVersion, title)
	if err != nil {
		s.writeVirtualVideoError(w, err, "", "Could not save the title.")
		return
	}
	s.writeVideoDetail(w, r, video)
}

// writeVirtualVideoError answers a title, missing-video or version error; field names the title in the request.
// A video that gained a file since the screen read it is also content_changed.
func (s *server) writeVirtualVideoError(w http.ResponseWriter, err error, field, message string) {
	if failure, ok := displayNameFailureOf(err); ok {
		failure.field = field
		s.writeWorkInfoFailure(w, failure)
		return
	}
	switch {
	case errors.Is(err, domain.ErrInvalidVirtualTitle):
		s.writeWorkInfoFailure(w, workInfoFailure{
			status: http.StatusBadRequest, code: string(codeInvalidRequest), reason: string(reasonInvalidTitle),
			field: field, message: "Enter a title.",
		})
	case errors.Is(err, domain.ErrNotFound):
		s.notFoundReason(w, reasonVideoNotFound, "Video not found.")
	case errors.Is(err, domain.ErrContentChanged), errors.Is(err, domain.ErrNotVirtual):
		s.conflictReason(w, reasonContentChanged, "The video changed in the meantime.")
	default:
		s.internalError(w, message, err)
	}
}

func (s *server) virtualVideosConfigured(w http.ResponseWriter) bool {
	if s.virtualVideos == nil {
		s.internalError(w, "Virtual video storage is not configured.", nil)
		return false
	}
	return true
}
