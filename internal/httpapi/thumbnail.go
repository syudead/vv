package httpapi

import (
	"net/http"
	"time"

	"github.com/syudead/vv/internal/httpapi/gen"
)

// Unready thumbnails return 404 so the client displays an empty frame.
func (s *server) GetVideoThumbnail(
	w http.ResponseWriter, r *http.Request, id gen.VideoId, _ gen.GetVideoThumbnailParams,
) {
	video, ok := s.lookupVideo(w, r, id)
	if !ok {
		return
	}

	if s.artifacts == nil || !video.HasThumbnail() {
		s.notFound(w, "The thumbnail has not been generated yet.")
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
