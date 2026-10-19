package httpapi

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/syudead/vv/internal/httpapi/gen"
)

// ThumbnailSources reads the larger source kept with an applied thumbnail image.
type ThumbnailSources interface {
	// ThumbnailImageSource returns nil when no applied image or no source is stored.
	ThumbnailImageSource(ctx context.Context, contentKey string) ([]byte, error)
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

	if params.Size != nil && *params.Size == gen.Large && s.serveThumbnailSource(w, r, video.ContentKey) {
		return
	}

	file, err := s.artifacts.ThumbnailFile(video.ContentKey)
	if err != nil {
		// Done state does not guarantee a surviving file; use the same 404 as ungenerated thumbnails.
		s.notFound(w, "The thumbnail has not been generated yet.")
		return
	}
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

// serveThumbnailSource reports false when there is no source to serve, so the caller falls back to the 640 px file.
func (s *server) serveThumbnailSource(w http.ResponseWriter, r *http.Request, contentKey string) bool {
	if s.thumbnailSources == nil {
		return false
	}
	source, err := s.thumbnailSources.ThumbnailImageSource(r.Context(), contentKey)
	if err != nil {
		s.logger.Warn("cannot read the thumbnail source", slog.Any("error", err))
		return false
	}
	if len(source) == 0 {
		return false
	}
	setRevalidate(w, bytesETag(source))
	w.Header().Set("Content-Type", "image/jpeg")
	http.ServeContent(w, r, "thumbnail.jpg", time.Time{}, bytes.NewReader(source))
	return true
}
