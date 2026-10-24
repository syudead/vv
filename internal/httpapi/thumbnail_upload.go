package httpapi

import (
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"time"

	"golang.org/x/sync/semaphore"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// Thumbnail uploads are owner-only, buffered in memory, and validated/applied by internal/app.

// Allow one MiB of multipart overhead beyond the image cap.
const thumbnailUploadMaxBody = domain.ThumbnailUploadMaxBytes + 1<<20

// Bound aggregate buffered uploads to four maximum bodies; keep this wider than the decode gate so a generation-lock wait cannot block unrelated uploads.
const thumbnailUploadBufferBudget = 4 * thumbnailUploadMaxBody

func newThumbnailUploadBuffers() *semaphore.Weighted {
	return semaphore.NewWeighted(thumbnailUploadBufferBudget)
}

// The post-body timeout includes gate/lock waits and two adjustments of up to sixty seconds each.
const thumbnailUploadTimeout = 5 * time.Minute

type ThumbnailUploads interface {
	// UploadThumbnailImage compares the initial content key and leaves storage unchanged on size, format, content-change, or adjustment failures.
	UploadThumbnailImage(ctx context.Context, videoID int64, contentKey string, image []byte) (domain.Video, error)
}

// Return only after the thumbnail is published.
func (s *server) UploadThumbnailImage(w http.ResponseWriter, r *http.Request, id gen.VideoId) {
	if s.thumbnailUploads == nil {
		s.internalError(w, "Thumbnail uploads are not configured.", nil)
		return
	}
	// Read content identity before the body so concurrent reassignment becomes content_changed.
	video, ok := s.lookupVideo(w, r, id)
	if !ok {
		return
	}
	// Reserve worst-case body memory before reading and retain it through response completion; cancellation while waiting performs no work.
	weight := thumbnailUploadWeight(r)
	if err := s.thumbnailUploadBuffers.Acquire(r.Context(), weight); err != nil {
		return
	}
	defer s.thumbnailUploadBuffers.Release(weight)
	image, contentVersion, ok := s.readThumbnailUpload(w, r)
	if !ok {
		return
	}
	// Reject stale preview content before acquiring the decode gate or generation lock; compare the contentVersion the
	// detail sends, which is a virtual video's whole key.
	if video.ContentKey == "" || contentVersion != domain.ContentVersion(video.ContentKey) {
		s.thumbnailUploadError(w, domain.ErrContentChanged)
		return
	}
	// After receiving the body, finish validation/application even if the page leaves.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), thumbnailUploadTimeout)
	defer cancel()
	saved, err := s.thumbnailUploads.UploadThumbnailImage(ctx, video.ID, video.ContentKey, image)
	if err != nil {
		s.thumbnailUploadError(w, err)
		return
	}
	s.writeVideoDetail(w, r, saved)
}

// Reserve Content-Length when within the cap; unknown or oversized lengths reserve the full cap.
func thumbnailUploadWeight(r *http.Request) int64 {
	if r.ContentLength > 0 && r.ContentLength <= thumbnailUploadMaxBody {
		return r.ContentLength
	}
	return thumbnailUploadMaxBody
}

const thumbnailUploadMaxContentVersion = 64

func (s *server) readThumbnailUpload(w http.ResponseWriter, r *http.Request) ([]byte, string, bool) {
	var image, contentVersion []byte
	tooLarge := func() {
		s.thumbnailUploadError(w, &domain.ThumbnailUploadTooLargeError{Limit: domain.ThumbnailUploadMaxBytes})
	}
	ok := s.readMultipart(w, r, thumbnailUploadMaxBody, tooLarge, func(name string, part *multipart.Part) bool {
		switch name {
		case "image":
			data, ok := s.readUploadImagePart(w, part, s.thumbnailUploadError)
			image = data
			return ok
		case "contentVersion":
			data, ok := readPart(part, thumbnailUploadMaxContentVersion)
			if !ok {
				s.invalidRequest(w, "contentVersion must be at most 64 bytes.")
				return false
			}
			contentVersion = data
			return true
		default:
			s.invalidRequest(w, "Only image and contentVersion may be sent.")
			return false
		}
	})
	if !ok {
		return nil, "", false
	}
	if image == nil {
		s.invalidRequest(w, "image is required.")
		return nil, "", false
	}
	if contentVersion == nil {
		s.invalidRequest(w, "contentVersion is required.")
		return nil, "", false
	}
	return image, string(contentVersion), true
}

// readUploadImagePart reads an upload's image part, writing an oversized part through tooLarge.
func (s *server) readUploadImagePart(w http.ResponseWriter, part io.Reader, tooLarge func(http.ResponseWriter, error)) ([]byte, bool) {
	data, err := readUploadPart(part, domain.ThumbnailUploadMaxBytes)
	switch {
	case errors.Is(err, domain.ErrThumbnailUploadTooLarge):
		tooLarge(w, err)
		return nil, false
	case err != nil:
		s.invalidRequest(w, "The request body could not be read.")
		return nil, false
	}
	return data, true
}

// readUploadPart enforces both part and whole-body limits and returns non-nil bytes for empty parts.
func readUploadPart(part io.Reader, limit int64) ([]byte, error) {
	tooLarge := &domain.ThumbnailUploadTooLargeError{Limit: int(limit)}
	data, err := io.ReadAll(io.LimitReader(part, limit+1))
	var bodyTooLarge *http.MaxBytesError
	switch {
	case errors.As(err, &bodyTooLarge):
		return nil, tooLarge
	case err != nil:
		return nil, err
	case int64(len(data)) > limit:
		return nil, tooLarge
	case data == nil:
		return []byte{}, nil
	}
	return data, nil
}

func (s *server) thumbnailUploadError(w http.ResponseWriter, err error) {
	var tooLarge *domain.ThumbnailUploadTooLargeError
	switch {
	case errors.As(err, &tooLarge):
		s.invalidRequestLimit(w, reasonThumbnailUploadTooLarge, tooLarge.Limit,
			"The image must be at most 64 MiB and 120 megapixels.")
	case errors.Is(err, domain.ErrThumbnailUploadInvalid):
		s.invalidRequestReason(w, reasonThumbnailUploadInvalid, "The image must be a readable PNG, JPEG or still WebP.")
	case errors.Is(err, domain.ErrContentChanged):
		s.conflictReason(w, reasonContentChanged, "The video is gone or now holds other content.")
	case errors.Is(err, domain.ErrThumbnailImageInvalid):
		s.conflictReason(w, reasonThumbnailImageInvalid, "The image cannot be fitted as the thumbnail.")
	default:
		s.internalError(w, "Could not set the thumbnail image.", err)
	}
}
