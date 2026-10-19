package httpapi

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// ThumbnailSources reads the larger source kept with an applied thumbnail image.
type ThumbnailSources interface {
	// ThumbnailImageSource returns nil when no applied image or no source is stored.
	ThumbnailImageSource(ctx context.Context, contentKey string) ([]byte, error)
}

// Posters extracts the player's poster from the video for a thumbnail that is a frame.
type Posters interface {
	// Poster receives the resolved source path and original location for ownership rechecks; it returns
	// ErrThumbnailFrameUnavailable for an applied image.
	Poster(ctx context.Context, videoID int64, location, path string) error
}

// Unready thumbnails return 404 so the client displays an empty frame.
func (s *server) GetVideoThumbnail(
	w http.ResponseWriter, r *http.Request, id gen.VideoId, params gen.GetVideoThumbnailParams,
) {
	video, ok := s.lookupVideo(w, r, id)
	if !ok {
		return
	}

	if s.artifacts == nil || !video.HasThumbnail() {
		s.notFound(w, "The thumbnail has not been generated yet.")
		return
	}

	if params.Size != nil && *params.Size == gen.Large && s.serveLargeThumbnail(w, r, video) {
		return
	}

	file, err := s.artifacts.ThumbnailFile(video.ContentKey)
	if err != nil {
		// Done state does not guarantee a surviving file; use the same 404 as ungenerated thumbnails.
		s.notFound(w, "The thumbnail has not been generated yet.")
		return
	}
	s.serveJPEGFile(w, r, file)
}

// serveLargeThumbnail reports false when there is nothing larger to serve, so the caller falls back
// to the 640 px file: an applied image serves its stored source, and a frame its poster, extracted
// on first request.
func (s *server) serveLargeThumbnail(w http.ResponseWriter, r *http.Request, video domain.Video) bool {
	if s.thumbnailSources != nil {
		source, err := s.thumbnailSources.ThumbnailImageSource(r.Context(), video.ContentKey)
		if err != nil {
			s.logger.Warn("cannot read the thumbnail source", slog.Any("error", err))
			return false
		}
		if len(source) > 0 {
			setRevalidate(w, bytesETag(source))
			w.Header().Set("Content-Type", "image/jpeg")
			http.ServeContent(w, r, "thumbnail.jpg", time.Time{}, bytes.NewReader(source))
			return true
		}
	}
	file, err := s.artifacts.PosterFile(video.ContentKey)
	if errors.Is(err, fs.ErrNotExist) && s.extractPoster(r, video) {
		file, err = s.artifacts.PosterFile(video.ContentKey)
	}
	if err != nil {
		return false
	}
	s.serveJPEGFile(w, r, file)
	return true
}

func (s *server) extractPoster(r *http.Request, video domain.Video) bool {
	if s.posters == nil || s.files == nil {
		return false
	}
	location, path, ok := s.resolveMediaFile(r, video)
	if !ok {
		return false
	}
	err := s.posters.Poster(r.Context(), video.ID, location, path)
	switch {
	case err == nil:
		return true
	case errors.Is(err, domain.ErrThumbnailFrameUnavailable), errors.Is(err, domain.ErrMediaFileUnavailable),
		errors.Is(err, domain.ErrContentChanged), errors.Is(err, domain.ErrNotFound),
		errors.Is(err, context.Canceled):
	default:
		s.logger.Warn("cannot extract the poster", slog.Int64("video", video.ID), slog.Any("error", err))
	}
	return false
}

func (s *server) serveJPEGFile(w http.ResponseWriter, r *http.Request, file *os.File) {
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		s.notFound(w, "The thumbnail has not been generated yet.")
		return
	}

	// Always revalidate with content ETags so same-size/time rebuilds replace cached images.
	etag, err := readerETag(file)
	if err != nil {
		s.notFound(w, "The thumbnail has not been generated yet.")
		return
	}
	setRevalidate(w, etag)
	w.Header().Set("Content-Type", "image/jpeg")

	// Omit modification time to prevent stale same-second If-Modified-Since 304 responses.
	http.ServeContent(w, r, info.Name(), time.Time{}, file)
}
