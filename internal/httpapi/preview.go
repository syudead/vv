package httpapi

import (
	"net/http"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// GetVideoPreview serves only the persisted, content-keyed preview MP4.
func (s *server) GetVideoPreview(
	w http.ResponseWriter, r *http.Request, id gen.VideoId, _ gen.GetVideoPreviewParams,
) {
	// Register guest requests so making the video private cancels them.
	video, r, release, ok := s.lookupServedVideo(w, r, id)
	if !ok {
		return
	}
	defer release()

	if video.PreviewState != domain.PreviewStateDone || s.artifacts == nil {
		s.notFound(w, "The preview has not been generated yet.")
		return
	}

	// Missing, manifest-mismatched, or incomplete artifacts all appear absent.
	file, digest, err := s.artifacts.PreviewFile(video.ContentKey)
	if err != nil {
		s.notFound(w, "The preview has not been generated yet.")
		return
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		s.notFound(w, "The preview has not been generated yet.")
		return
	}

	// Revalidate only by content ETag, without modification time, so same-second rebuilds cannot return stale If-Modified-Since results.
	setRevalidate(w, digestETag("preview", digest))
	w.Header().Set("Content-Type", "video/mp4")
	http.ServeContent(w, r, info.Name(), time.Time{}, file)
}
