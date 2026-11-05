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

// Person faces and organization logos are owner-only; uploads share the thumbnail upload's body budget, limits and
// detached timeout.

type PersonImages interface {
	// UploadCreditImage returns ThumbnailUploadTooLargeError for a body over the byte limit and ErrPersonImageInvalid
	// for any other unreadable image, writing nothing.
	UploadCreditImage(ctx context.Context, kind domain.CreditKind, id int64, image []byte) (domain.Credit, error)
	CreditImage(ctx context.Context, kind domain.CreditKind, id int64) (domain.PersonImage, error)
	DeleteCreditImage(ctx context.Context, kind domain.CreditKind, id int64) (domain.Credit, error)
}

func (s *server) GetPersonImage(w http.ResponseWriter, r *http.Request, id gen.PersonId, _ gen.GetPersonImageParams) {
	s.getCreditImage(w, r, peopleAPI, id)
}

func (s *server) GetOrganizationImage(w http.ResponseWriter, r *http.Request, id gen.OrganizationId, _ gen.GetOrganizationImageParams) {
	s.getCreditImage(w, r, organizationsAPI, id)
}

func (s *server) getCreditImage(w http.ResponseWriter, r *http.Request, api creditKindAPI, id int64) {
	if !s.personImagesConfigured(w) {
		return
	}
	image, err := s.personImages.CreditImage(r.Context(), api.kind, id)
	if err != nil {
		s.creditImageError(w, api, err)
		return
	}
	// The revision never repeats for one entity, so it identifies the bytes without hashing them.
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
		s.logger.Debug("could not write the image", slog.String("kind", string(api.kind)), slog.Int64("id", id),
			slog.Any("error", err))
	}
}

func (s *server) UploadPersonImage(w http.ResponseWriter, r *http.Request, id gen.PersonId) {
	s.uploadCreditImage(w, r, peopleAPI, id)
}

func (s *server) UploadOrganizationImage(w http.ResponseWriter, r *http.Request, id gen.OrganizationId) {
	s.uploadCreditImage(w, r, organizationsAPI, id)
}

// uploadCreditImage returns only after the fitted image is stored.
func (s *server) uploadCreditImage(w http.ResponseWriter, r *http.Request, api creditKindAPI, id int64) {
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
	credit, err := s.personImages.UploadCreditImage(ctx, api.kind, id, image)
	if err != nil {
		s.creditImageError(w, api, err)
		return
	}
	s.writeCreditBody(w, http.StatusOK, api.one(credit))
}

func (s *server) DeletePersonImage(w http.ResponseWriter, r *http.Request, id gen.PersonId) {
	s.deleteCreditImage(w, r, peopleAPI, id)
}

func (s *server) DeleteOrganizationImage(w http.ResponseWriter, r *http.Request, id gen.OrganizationId) {
	s.deleteCreditImage(w, r, organizationsAPI, id)
}

func (s *server) deleteCreditImage(w http.ResponseWriter, r *http.Request, api creditKindAPI, id int64) {
	if !s.personImagesConfigured(w) {
		return
	}
	credit, err := s.personImages.DeleteCreditImage(r.Context(), api.kind, id)
	if err != nil {
		s.creditImageError(w, api, err)
		return
	}
	s.writeCreditBody(w, http.StatusOK, api.one(credit))
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

// personImageError maps the upload's errors, which name no entity.
func (s *server) personImageError(w http.ResponseWriter, err error) {
	s.creditImageError(w, peopleAPI, err)
}

// creditImageError maps image errors; the image reasons are shared by both kinds, the missing entity is api's.
func (s *server) creditImageError(w http.ResponseWriter, api creditKindAPI, err error) {
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
		s.notFoundReason(w, reasonPersonImageNotFound, "The "+string(api.kind)+" has no image.")
	case errors.Is(err, domain.ErrNotFound):
		s.notFoundReason(w, api.notFound, "The "+string(api.kind)+" was not found.")
	default:
		s.internalError(w, "Could not change the "+string(api.kind)+" image.", err)
	}
}

func (s *server) personImagesConfigured(w http.ResponseWriter) bool {
	if s.personImages == nil {
		s.internalError(w, "Person images are not configured.", nil)
		return false
	}
	return true
}
