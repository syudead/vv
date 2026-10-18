package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"mime/multipart"
	"net/http"
	"strconv"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// Person images are owner-only; uploads share the thumbnail upload's body budget, limits and detached timeout.

type PersonImages interface {
	// UploadPersonImage returns ThumbnailUploadTooLargeError for a body over the byte limit and ErrPersonImageInvalid
	// for any other unacceptable image, writing nothing.
	UploadPersonImage(ctx context.Context, id int64, image []byte) (domain.Credit, error)
	PersonImage(ctx context.Context, id int64) (domain.PersonImage, error)
	DeletePersonImage(ctx context.Context, id int64) (domain.Credit, error)
}

func (s *server) GetPersonImage(w http.ResponseWriter, r *http.Request, id gen.PersonId, _ gen.GetPersonImageParams) {
	if !s.personImagesConfigured(w) {
		return
	}
	image, err := s.personImages.PersonImage(r.Context(), id)
	if err != nil {
		s.personImageError(w, err)
		return
	}
	// The revision never repeats for one person, so it identifies the bytes without hashing them.
	etag := `"` + strconv.FormatInt(image.Revision, 10) + `"`
	setRevalidate(w, etag)
	if etagMatches(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Content-Length", strconv.Itoa(len(image.JPEG)))
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(image.JPEG); err != nil {
		s.logger.Debug("could not write the person image", slog.Int64("person", id), slog.Any("error", err))
	}
}

// UploadPersonImage returns only after the fitted face is stored.
func (s *server) UploadPersonImage(w http.ResponseWriter, r *http.Request, id gen.PersonId) {
	if !s.personImagesConfigured(w) {
		return
	}
	weight := thumbnailUploadWeight(r)
	if err := s.thumbnailUploadBuffers.Acquire(r.Context(), weight); err != nil {
		return
	}
	defer s.thumbnailUploadBuffers.Release(weight)
	image, ok := s.readPersonImageUpload(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), thumbnailUploadTimeout)
	defer cancel()
	credit, err := s.personImages.UploadPersonImage(ctx, id, image)
	if err != nil {
		s.personImageError(w, err)
		return
	}
	s.writeCreditBody(w, http.StatusOK, toAPIPerson(credit))
}

func (s *server) DeletePersonImage(w http.ResponseWriter, r *http.Request, id gen.PersonId) {
	if !s.personImagesConfigured(w) {
		return
	}
	credit, err := s.personImages.DeletePersonImage(r.Context(), id)
	if err != nil {
		s.personImageError(w, err)
		return
	}
	s.writeCreditBody(w, http.StatusOK, toAPIPerson(credit))
}

func (s *server) readPersonImageUpload(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	var image []byte
	tooLarge := func() {
		s.personImageError(w, &domain.ThumbnailUploadTooLargeError{Limit: domain.ThumbnailUploadMaxBytes})
	}
	ok := s.readMultipart(w, r, thumbnailUploadMaxBody, tooLarge, func(name string, part *multipart.Part) bool {
		if name != "image" {
			s.invalidRequest(w, "Only image may be sent.")
			return false
		}
		data, ok := s.readUploadImagePart(w, part, s.personImageError)
		image = data
		return ok
	})
	if !ok {
		return nil, false
	}
	if image == nil {
		s.invalidRequest(w, "image is required.")
		return nil, false
	}
	return image, true
}

func (s *server) personImageError(w http.ResponseWriter, err error) {
	var tooLarge *domain.ThumbnailUploadTooLargeError
	switch {
	case errors.As(err, &tooLarge):
		limit := tooLarge.Limit
		reason := reasonPersonImageTooLarge
		s.writeErrorBody(w, http.StatusRequestEntityTooLarge, gen.Error{
			Code: codeInvalidRequest, Reason: &reason, Limit: &limit, Message: "The image must be at most 64 MiB.",
		})
	case errors.Is(err, domain.ErrPersonImageInvalid):
		s.invalidRequestReason(w, reasonPersonImageInvalid,
			"The image must be a readable PNG, JPEG or still WebP of at most 120 megapixels.")
	case errors.Is(err, domain.ErrPersonImageNotFound):
		s.notFoundReason(w, reasonPersonImageNotFound, "The person has no image.")
	case errors.Is(err, domain.ErrNotFound):
		s.notFoundReason(w, peopleAPI.notFound, "The person was not found.")
	default:
		s.internalError(w, "Could not change the person image.", err)
	}
}

func (s *server) personImagesConfigured(w http.ResponseWriter) bool {
	if s.personImages == nil {
		s.internalError(w, "Person images are not configured.", nil)
		return false
	}
	return true
}
