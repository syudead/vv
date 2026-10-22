package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// Display names use one store transaction. Thumbnail mutations go through Ingest under the same generation lock as background jobs.

type OverrideStore interface {
	// SetDisplayName clears normalized empty names and returns ErrNotFound for missing or unregistered videos.
	SetDisplayName(ctx context.Context, videoID int64, name string) (domain.Video, error)
	// SetDisplayNames is atomic; indexed errors preserve input order and prevent all writes.
	SetDisplayNames(ctx context.Context, changes []domain.DisplayNameChange) ([]domain.Video, error)
}

func (s *server) SetVideoDisplayName(w http.ResponseWriter, r *http.Request, id gen.VideoId) {
	// Require displayName presence to prevent accidental clearing.
	var body struct {
		DisplayName    *string `json:"displayName"`
		ContentVersion *string `json:"contentVersion"`
	}
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if body.DisplayName == nil {
		s.invalidRequest(w, "displayName is required.")
		return
	}
	if s.overrides == nil {
		s.internalError(w, "Video override storage is not configured.", nil)
		return
	}
	virtual, ok := s.virtualSaveTarget(w, r, id, body.ContentVersion)
	if !ok {
		return
	}
	if virtual {
		s.saveVirtualTitle(w, r, id, *body.ContentVersion, *body.DisplayName)
		return
	}

	video, err := s.overrides.SetDisplayName(r.Context(), id, *body.DisplayName)
	switch {
	case errors.Is(err, domain.ErrInvalidDisplayName):
		s.invalidDisplayName(w, err)
		return
	case errors.Is(err, domain.ErrNotFound):
		s.notFoundReason(w, reasonVideoNotFound, "Video not found.")
		return
	case err != nil:
		s.internalError(w, "Could not save the display name.", err)
		return
	}
	s.writeVideoDetail(w, r, video)
}

func (s *server) invalidDisplayName(w http.ResponseWriter, err error) {
	failure, ok := displayNameFailureOf(err)
	if !ok {
		s.invalidRequest(w, "The display name cannot be used.")
		return
	}
	s.writeWorkInfoFailure(w, failure)
}

// displayNameFailureOf leaves field empty; only the details endpoint adds it.
func displayNameFailureOf(err error) (workInfoFailure, bool) {
	var invalid *domain.InvalidDisplayNameError
	if !errors.As(err, &invalid) {
		return workInfoFailure{}, false
	}
	failure := workInfoFailure{status: http.StatusBadRequest, code: string(codeInvalidRequest)}
	switch invalid.Problem {
	case domain.DisplayNameControlCharacters:
		failure.reason = string(reasonDisplayNameControlCharacters)
		failure.message = "Display names cannot contain control characters."
	case domain.DisplayNameTooLong:
		failure.reason, failure.limit = string(reasonDisplayNameTooLong), domain.DisplayNameMaxLength
		failure.message = "Display names must be at most " + strconv.Itoa(domain.DisplayNameMaxLength) + " characters."
	default:
		return workInfoFailure{}, false
	}
	return failure, true
}

type ThumbnailPicker interface {
	// SetThumbnailPosition receives the resolved source path and original location for ownership rechecks. A nil position restores automatic selection.
	SetThumbnailPosition(
		ctx context.Context, videoID int64, location, path string, positionMs *int64,
	) (domain.Video, error)
	// Reverting regenerates the thumbnail even without an applied image.
	RevertThumbnailImage(ctx context.Context, videoID int64, location, path string) (domain.Video, error)
}

// Wait for generation to finish before responding.
func (s *server) SetVideoThumbnailPosition(w http.ResponseWriter, r *http.Request, id gen.VideoId) {
	// Distinguish an omitted position from null, which clears it.
	var body struct {
		PositionMs json.RawMessage `json:"positionMs"`
	}
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if len(body.PositionMs) == 0 {
		s.invalidRequest(w, "positionMs is required.")
		return
	}
	var positionMs *int64
	if err := json.Unmarshal(body.PositionMs, &positionMs); err != nil {
		s.invalidRequest(w, "positionMs must be an integer or null.")
		return
	}
	if s.thumbnails == nil {
		s.internalError(w, "Thumbnail generation is not configured.", nil)
		return
	}

	video, ok := s.lookupVideo(w, r, id)
	if !ok {
		return
	}
	// Validate position before source errors; the application revalidates under the generation lock.
	if positionMs != nil {
		if err := domain.CheckThumbnailPosition(video, *positionMs); err != nil {
			s.thumbnailPositionError(w, video, err)
			return
		}
	}
	if s.files == nil {
		s.internalError(w, "Media file access is not configured.", nil)
		return
	}
	// Pass the resolved path to prevent a symlink swap from reading another file or escaping the root.
	location, path, ok := s.resolveMediaFile(r, video)
	if !ok {
		s.notFoundReason(w, reasonFileUnavailable, "Cannot open this video's file.")
		return
	}

	saved, err := s.thumbnails.SetThumbnailPosition(r.Context(), id, location, path, positionMs)
	if err != nil {
		s.thumbnailPositionError(w, video, err)
		return
	}
	s.writeVideoDetail(w, r, saved)
}

func (s *server) RevertThumbnailImage(w http.ResponseWriter, r *http.Request, id gen.VideoId) {
	if s.thumbnails == nil {
		s.internalError(w, "Thumbnail generation is not configured.", nil)
		return
	}
	video, ok := s.lookupVideo(w, r, id)
	if !ok {
		return
	}
	if s.files == nil {
		s.internalError(w, "Media file access is not configured.", nil)
		return
	}
	location, path, ok := s.resolveMediaFile(r, video)
	if !ok {
		s.notFoundReason(w, reasonFileUnavailable, "Cannot open this video's file.")
		return
	}

	saved, err := s.thumbnails.RevertThumbnailImage(r.Context(), id, location, path)
	if err != nil {
		s.thumbnailPositionError(w, video, err)
		return
	}
	s.writeVideoDetail(w, r, saved)
}

// Use duration as the limit for out-of-range positions.
func (s *server) thumbnailPositionError(w http.ResponseWriter, video domain.Video, err error) {
	switch {
	case errors.Is(err, domain.ErrDurationUnknown):
		s.conflictReason(w, reasonDurationUnknown, "The video's duration is not known yet.")
	case errors.Is(err, domain.ErrThumbnailPositionOutOfRange):
		if video.DurationMs == nil {
			s.invalidRequestReason(w, reasonThumbnailPositionOutOfRange, "The position is outside the video.")
			return
		}
		s.invalidRequestLimit(w, reasonThumbnailPositionOutOfRange, int(*video.DurationMs),
			"The position must be at least 0 and less than "+strconv.FormatInt(*video.DurationMs, 10)+" ms.")
	case errors.Is(err, domain.ErrThumbnailFrameUnavailable):
		// Log external ffmpeg failures without exposing their details.
		s.logger.Warn("cannot extract a frame for the thumbnail",
			slog.Int64("video", video.ID), slog.Any("error", err))
		s.conflictReason(w, reasonThumbnailFrameUnavailable, "Cannot make a thumbnail from this position.")
	case errors.Is(err, domain.ErrMediaFileUnavailable):
		// The location may have been reassigned or removed during generation.
		s.notFoundReason(w, reasonFileUnavailable, "Cannot open this video's file.")
	case errors.Is(err, domain.ErrNotFound):
		s.notFoundReason(w, reasonVideoNotFound, "Video not found.")
	default:
		s.internalError(w, "Could not change the thumbnail.", err)
	}
}
