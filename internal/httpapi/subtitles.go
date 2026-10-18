package httpapi

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strconv"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// List subtitles from the current stream location without reading contents; malformed files may list successfully and fail on retrieval (docs/design-docs/sidecar-subtitles.md).
func (s *server) ListVideoSubtitles(w http.ResponseWriter, r *http.Request, id gen.VideoId) {
	video, r, release, ok := s.lookupServedVideo(w, r, id)
	if !ok {
		return
	}
	defer release()

	if s.files == nil {
		s.internalError(w, "Media file access is not configured.", nil)
		return
	}
	_, sidecars, ok := s.subtitleSidecars(r, video)
	if !ok {
		s.notFoundReason(w, reasonFileUnavailable, "Cannot open this video's file.")
		return
	}

	tracks := make([]gen.SubtitleTrack, 0, len(sidecars))
	for _, sidecar := range sidecars {
		tracks = append(tracks, gen.SubtitleTrack{
			File:   sidecar.File,
			Label:  sidecar.Label,
			Format: gen.SubtitleTrackFormat(sidecar.Format),
		})
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.SubtitleTrackList{Subtitles: tracks}, s.logger)
}

// Open only exact names from a fresh sidecar listing; never construct paths from request strings.
func (s *server) GetVideoSubtitle(
	w http.ResponseWriter,
	r *http.Request,
	id gen.VideoId,
	file string,
	params gen.GetVideoSubtitleParams,
) {
	var offsetMs int64
	if params.OffsetMs != nil {
		offsetMs = *params.OffsetMs
	}
	if offsetMs < 0 {
		s.invalidRequest(w, "offsetMs must be a non-negative integer.")
		return
	}

	video, r, release, ok := s.lookupServedVideo(w, r, id)
	if !ok {
		return
	}
	defer release()

	if s.files == nil {
		s.internalError(w, "Media file access is not configured.", nil)
		return
	}
	if s.subtitles == nil {
		s.internalError(w, "Subtitle conversion is not configured.", nil)
		return
	}
	location, sidecars, ok := s.subtitleSidecars(r, video)
	if !ok {
		s.notFoundReason(w, reasonFileUnavailable, "Cannot open this video's file.")
		return
	}

	body, err := s.readSubtitle(r, location, sidecars, file, offsetMs)
	if err != nil {
		s.logger.Warn("cannot serve the subtitle",
			slog.Int64("video", video.ID), slog.String("file", file), slog.Any("error", err))
		s.notFoundReason(w, reasonSubtitleUnavailable, "Cannot read this subtitle file.")
		return
	}

	etag := bytesETag(body)
	setRevalidate(w, etag)
	if etagMatches(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(body); err != nil {
		s.logger.Debug("could not write the subtitle", slog.Int64("video", video.ID), slog.Any("error", err))
	}
}

var errSubtitleNotListed = errors.New("not a subtitle file of this video")

var errSubtitleTooLarge = errors.New("subtitle file exceeds the size limit")

func (s *server) readSubtitle(
	r *http.Request,
	location string,
	sidecars []domain.SubtitleSidecar,
	file string,
	offsetMs int64,
) ([]byte, error) {
	var sidecar domain.SubtitleSidecar
	found := false
	for _, candidate := range sidecars {
		if candidate.File == file {
			sidecar, found = candidate, true
			break
		}
	}
	if !found {
		return nil, errSubtitleNotListed
	}
	roots, err := s.mediaFolderPaths(r)
	if err != nil {
		return nil, err
	}
	opened, _, err := s.files.OpenSidecarFile(roots, location, sidecar.File)
	if err != nil {
		return nil, err
	}
	defer func() { _ = opened.Close() }()
	// Enforce the read cap even if the file grows after listing.
	src, err := io.ReadAll(io.LimitReader(opened, domain.SubtitleFileLimit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(src)) > domain.SubtitleFileLimit {
		return nil, errSubtitleTooLarge
	}
	return s.subtitles.Convert(src, sidecar.Format, offsetMs)
}

// Try locations in streaming order; unreadable folders return an empty list and log the cause.
func (s *server) subtitleSidecars(r *http.Request, video domain.Video) (string, []domain.SubtitleSidecar, bool) {
	locations, err := s.videos.VideoLocations(r.Context(), video.ID)
	if err != nil {
		return "", nil, false
	}
	roots, err := s.mediaFolderPaths(r)
	if err != nil {
		return "", nil, false
	}
	for _, location := range locations {
		entries, err := s.files.ListSidecarFiles(roots, location.Path)
		switch {
		case err == nil:
			return location.Path, domain.SubtitleSidecars(filepath.Base(location.Path), entries), true
		case errors.Is(err, domain.ErrDirectoryUnavailable):
			s.logger.Warn("cannot read the folder next to the video",
				slog.Int64("video", video.ID), slog.Any("error", err))
			return location.Path, nil, true
		case errors.Is(err, domain.ErrMediaFileOutsideRoot):
			s.logger.Warn("link target is outside the media folders",
				slog.Int64("video", video.ID), slog.String("path", location.Path))
		default:
			s.logger.Debug("cannot open the media file", slog.Int64("video", video.ID), slog.Any("error", err))
		}
	}
	return "", nil, false
}
