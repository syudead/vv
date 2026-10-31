package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// A thumbnail edit of the peer's video runs on this node's ComfyUI from the frame or thumbnail the peer gives, and
// its apply uploads the result as the screens upload a thumbnail (docs/design-docs/peer-library.md).

// startRemoteThumbnailEdit starts a session for the peer's video id from the peer's frame at start.PositionMs or
// its current thumbnail.
func (s *server) startRemoteThumbnailEdit(w http.ResponseWriter, r *http.Request, id int64, start domain.ThumbnailEditStart) {
	video, ok := s.remoteFrameVideo(w, r, id)
	if !ok {
		return
	}
	suffix, query := "/thumbnail", url.Values{"size": {"large"}}
	if start.Source == domain.ThumbnailEditFromFrame {
		if start.PositionMs == nil {
			s.thumbnailPositionError(w, video, domain.ErrThumbnailPositionOutOfRange)
			return
		}
		suffix, query = "/frame", url.Values{"positionMs": {strconv.FormatInt(*start.PositionMs, 10)}}
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, peerVideoPath(domain.FromRemoteID(id), suffix, query), nil)
	if err != nil {
		s.internalError(w, "Could not read the image of the other VVMDM's video.", err)
		return
	}
	// The peer may extract the frame from the file, as a forwarded frame read does.
	image, err := s.peerSend(req, false, true)
	if err != nil {
		s.peerFailure(w, err, "Could not read the image of the other VVMDM's video.")
		return
	}
	start.Image = image
	s.startFromPeerImage(w, r, video, start)
}

// startFromPeerImage starts the session for the peer's video from start.Image; its apply uploads to the peer.
func (s *server) startFromPeerImage(w http.ResponseWriter, r *http.Request, video domain.Video, start domain.ThumbnailEditStart) {
	edit, err := s.thumbnailEdits.StartFrom(r.Context(), video, start, s.peerThumbnailWrite(video))
	switch {
	case errors.Is(err, domain.ErrThumbnailUploadInvalid):
		s.thumbnailUploadError(w, err)
	case errors.Is(err, domain.ErrThumbnailFrameUnavailable):
		s.thumbnailPositionError(w, video, err)
	case err != nil:
		s.thumbnailEditFailure(w, err)
	default:
		s.writeThumbnailEdit(w, http.StatusCreated, edit)
	}
}

// remoteFrameVideo reads the peer's video id as a session needs it: its content version, which the upload
// compares, and its duration. A virtual video has no frames and answers 409 virtual_video, as a local one does.
func (s *server) remoteFrameVideo(w http.ResponseWriter, r *http.Request, id int64) (domain.Video, bool) {
	var peer gen.Video
	if err := s.peerGet(r, peerVideoPath(domain.FromRemoteID(id), "", nil), url.Values{}, &peer); err != nil {
		s.peerFailure(w, err, "Could not read the other VVMDM's video.")
		return domain.Video{}, false
	}
	switch {
	case peer.Virtual:
		s.conflictReason(w, reasonVirtualVideo, "A video without a file has no frames; upload an image as its thumbnail.")
		return domain.Video{}, false
	case peer.ContentVersion == nil:
		s.notFound(w, "The video was not found.")
		return domain.Video{}, false
	}
	return domain.Video{ID: id, ContentKey: *peer.ContentVersion, DurationMs: peer.DurationMs}, true
}

// peerThumbnailWrite uploads an applied result to the peer's video; the peer's 404 or content_changed means the
// video no longer holds the content the session started from.
func (s *server) peerThumbnailWrite(video domain.Video) domain.ThumbnailImageWrite {
	return func(ctx context.Context, image []byte) (domain.Video, error) {
		saved, err := s.uploadPeerThumbnail(ctx, video.ID, video.ContentKey, image)
		var refusal *peerRefusal
		if errors.As(err, &refusal) &&
			(refusal.status == http.StatusNotFound || (refusal.reason != nil && *refusal.reason == reasonContentChanged)) {
			return domain.Video{}, errors.Join(domain.ErrContentChanged, err)
		}
		return saved, err
	}
}

// writeAppliedRemoteVideo answers an applied edit with the peer's video; the thumbnail is already saved, so a
// failed read still answers 200 with the bare id, as a local apply answers when it cannot read part of the video.
func (s *server) writeAppliedRemoteVideo(w http.ResponseWriter, r *http.Request, id int64) {
	video, err := s.peerVideoDetail(r, id)
	if err != nil {
		s.logger.Warn("cannot read the other VVMDM's video after applying its thumbnail edit",
			slog.Int64("video", id), slog.Any("error", err))
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, video, s.logger)
}

// peerFailure answers err from a call to the peer with the peer's own error when it gave one and as unreachable
// when it did not; an error that did not come from the peer is a 500 with message.
func (s *server) peerFailure(w http.ResponseWriter, err error, message string) {
	var refusal *peerRefusal
	switch {
	case errors.As(err, &refusal) && refusal.problem != nil:
		s.writeErrorBody(w, refusal.status, *refusal.problem)
	case errors.As(err, &refusal) && refusal.status == http.StatusNotFound:
		s.notFound(w, "The video was not found.")
	case errors.As(err, &refusal) || errors.Is(err, domain.ErrPeerUnreachable):
		s.peerUnreachable(w)
	default:
		s.internalError(w, message, err)
	}
}
