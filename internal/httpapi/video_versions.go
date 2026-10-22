package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// Address bundles by video ID, never bundle ID. Store mutations are atomic; VideoBundleChanged publishes SSE after commit.

type VersionStore interface {
	// Versions filters by audience and puts the effective representative first; an unbundled video returns itself.
	Versions(ctx context.Context, audience domain.Audience, videoID int64) (domain.VideoVersions, error)
	// Validate the entire bundle before writing.
	Bundle(ctx context.Context, videoIDs []int64, representativeID int64) (domain.VideoVersions, error)
	MakeRepresentative(ctx context.Context, videoID int64) (domain.VideoVersions, error)
	Unbundle(ctx context.Context, videoID int64) (domain.Video, error)
	// Candidates require both videos to remain registered; return the newest pairs to owners only.
	Candidates(ctx context.Context) (domain.VersionCandidatePage, error)
	// Dismiss candidates atomically; a missing candidate aborts the operation.
	Dismiss(ctx context.Context, videoIDs [2]int64) error
}

// Use the same ID limit as tag mutations.
const maxBundleVideoIDs = maxVideoTagsIDs

func (s *server) ListVideoVersions(w http.ResponseWriter, r *http.Request, id gen.VideoId) {
	if s.versions == nil {
		s.internalError(w, "Version storage is not configured.", nil)
		return
	}
	versions, err := s.versions.Versions(r.Context(), audienceFrom(r.Context()), id)
	if !s.checkVersionsError(w, err, "Could not load the versions.") {
		return
	}
	s.writeVideoVersions(w, r, versions)
}

func (s *server) BundleVideos(w http.ResponseWriter, r *http.Request) {
	var body gen.VideoBundleRequest
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if len(body.VideoIds) > maxBundleVideoIDs {
		s.invalidRequestLimit(w, reasonTooManyVideos, maxBundleVideoIDs,
			fmt.Sprintf("Bundle at most %d videos at a time.", maxBundleVideoIDs))
		return
	}
	if s.versions == nil {
		s.internalError(w, "Version storage is not configured.", nil)
		return
	}
	versions, err := s.versions.Bundle(r.Context(), body.VideoIds, body.RepresentativeId)
	if !s.checkVersionsError(w, err, "Could not bundle the videos.") {
		return
	}
	s.writeVideoVersions(w, r, versions)
}

func (s *server) MakeRepresentativeVersion(w http.ResponseWriter, r *http.Request, id gen.VideoId) {
	if s.versions == nil {
		s.internalError(w, "Version storage is not configured.", nil)
		return
	}
	versions, err := s.versions.MakeRepresentative(r.Context(), id)
	if !s.checkVersionsError(w, err, "Could not change the version shown in the library.") {
		return
	}
	s.writeVideoVersions(w, r, versions)
}

func (s *server) UnbundleVideo(w http.ResponseWriter, r *http.Request, id gen.VideoId) {
	if s.versions == nil {
		s.internalError(w, "Version storage is not configured.", nil)
		return
	}
	video, err := s.versions.Unbundle(r.Context(), id)
	if !s.checkVersionsError(w, err, "Could not unbundle the video.") {
		return
	}
	s.writeVideoDetail(w, r, video)
}

func (s *server) checkVersionsError(w http.ResponseWriter, err error, message string) bool {
	switch {
	case err == nil:
		return true
	case errors.Is(err, domain.ErrNotFound):
		s.notFoundReason(w, reasonVideoNotFound, "Video not found.")
	case errors.Is(err, domain.ErrTooFewVersions):
		s.invalidRequestReason(w, reasonTooFewVideos, "Select at least two videos to bundle.")
	case errors.Is(err, domain.ErrRepresentativeNotSelected):
		s.invalidRequestReason(w, reasonRepresentativeNotSelected, "representativeId must be one of videoIds.")
	case errors.Is(err, domain.ErrNotBundled):
		s.invalidRequestReason(w, reasonNotBundled, "The video is not bundled with other videos.")
	case errors.Is(err, domain.ErrVirtualVideo):
		s.virtualVideoInBundle(w)
	default:
		s.internalError(w, message, err)
	}
	return false
}

// A bundle chooses a file to play, so a virtual video never joins one.
func (s *server) virtualVideoInBundle(w http.ResponseWriter) {
	s.conflictReason(w, reasonVirtualVideo, "A video without a file cannot be bundled with versions.")
}

func (s *server) writeVideoVersions(w http.ResponseWriter, r *http.Request, versions domain.VideoVersions) {
	payload := gen.VideoVersions{RepresentativeId: versions.RepresentativeID, Items: s.versionItems(r, versions.Items)}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, payload, s.logger)
}

// Use the detail shape without group or seek-preview state to avoid per-item queries.
func (s *server) versionItems(r *http.Request, videos []domain.Video) []gen.Video {
	ctx := r.Context()
	audience := audienceFrom(ctx)
	progress := s.progressFor(ctx, videos)
	tags := s.tagsFor(ctx, videos)
	roots := s.registeredRoots(ctx)
	openable := s.canOpen(r)

	items := make([]gen.Video, 0, len(videos))
	for _, view := range s.presentVideos(ctx, videos) {
		video := view.Video
		item := withTags(withProgress(toAPIVideo(view), progress, video.UserKey), tags, video.UserKey)
		item.Location = &gen.VideoLocation{Path: video.Path, Openable: openable}
		item.Folder = detailFolder(roots, video.Path)
		items = append(items, forAudience(audience, item))
	}
	return items
}

func apiVersionsRef(ref *domain.VideoVersionsRef) *gen.VideoVersionsRef {
	if ref == nil {
		return nil
	}
	return &gen.VideoVersionsRef{Count: ref.Count, RepresentativeId: ref.RepresentativeID}
}
