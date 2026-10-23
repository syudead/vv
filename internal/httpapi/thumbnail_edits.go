package httpapi

import (
	"context"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"unicode/utf8"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// Thumbnail edit sessions are owner-only and stored in internal/app memory.

// The mask cap fits an uncompressed 2048×2048 RGBA image.
const thumbnailEditMaxMaskBytes = 20 << 20

// The body cap includes all part limits plus multipart overhead.
const thumbnailEditMaxRunBody = thumbnailEditMaxMaskBytes + domain.ThumbnailEditMaxReferenceBytes + 1<<20

// Finished sessions return ErrThumbnailEditNotFound.
type ThumbnailEdits interface {
	Start(ctx context.Context, video domain.Video, path string, start domain.ThumbnailEditStart) (domain.ThumbnailEdit, error)
	Get(ctx context.Context, id string) (domain.ThumbnailEdit, error)
	Frame(ctx context.Context, id string) ([]byte, error)
	Result(ctx context.Context, id string) ([]byte, error)
	StartRun(ctx context.Context, id string, input domain.ThumbnailEditRunInput) (domain.ThumbnailEdit, error)
	Cancel(ctx context.Context, id string) (domain.ThumbnailEdit, error)
	Apply(ctx context.Context, id string) (domain.Video, error)
	Discard(ctx context.Context, id string)
	VideoFrame(ctx context.Context, path string, positionMs int64) (domain.ThumbnailFrame, error)
}

// GetVideoFrame answers a bounded PNG of the video at positionMs, for framing a person's face.
func (s *server) GetVideoFrame(w http.ResponseWriter, r *http.Request, id gen.VideoId, params gen.GetVideoFrameParams) {
	if s.thumbnailEdits == nil || s.files == nil {
		s.internalError(w, "Frame extraction is not configured.", nil)
		return
	}
	video, ok := s.lookupVideo(w, r, id)
	if !ok {
		return
	}
	if err := domain.CheckThumbnailPosition(video, params.PositionMs); err != nil {
		s.thumbnailPositionError(w, video, err)
		return
	}
	_, path, ok := s.resolveMediaFile(r, video)
	if !ok {
		s.notFoundReason(w, reasonFileUnavailable, "Cannot open this video's file.")
		return
	}
	frame, err := s.thumbnailEdits.VideoFrame(r.Context(), path, params.PositionMs)
	if errors.Is(err, domain.ErrThumbnailFrameUnavailable) {
		s.conflictReason(w, reasonThumbnailFrameUnavailable, "No frame could be read at this position.")
		return
	}
	if err != nil {
		s.internalError(w, "Could not read the frame.", err)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	_, _ = w.Write(frame.PNG)
}

func (s *server) StartThumbnailEdit(w http.ResponseWriter, r *http.Request, id gen.VideoId) {
	var body gen.StartThumbnailEditRequest
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if !body.Source.Valid() || body.Source == gen.ThumbnailEditSourceImage {
		s.invalidRequest(w, "source must be frame or thumbnail.")
		return
	}
	if s.thumbnailEdits == nil {
		s.internalError(w, "Thumbnail editing is not configured.", nil)
		return
	}
	video, ok := s.lookupFrameVideo(w, r, id)
	if !ok {
		return
	}
	start := domain.ThumbnailEditStart{Source: domain.ThumbnailEditSource(body.Source)}
	if start.Source == domain.ThumbnailEditFromFrame {
		start.PositionMs = body.PositionMs
		// Validate position before opening media, matching SetVideoThumbnailPosition.
		if err := checkThumbnailEditPosition(video, body.PositionMs); err != nil {
			s.thumbnailPositionError(w, video, err)
			return
		}
	}
	if s.files == nil {
		s.internalError(w, "Media file access is not configured.", nil)
		return
	}
	// Use validated resolved paths; edits from an applied thumbnail may proceed without readable source media, while frame extraction still requires it.
	_, path, ok := s.resolveMediaFile(r, video)
	if !ok && start.Source == domain.ThumbnailEditFromFrame {
		s.notFoundReason(w, reasonFileUnavailable, "Cannot open this video's file.")
		return
	}
	edit, err := s.thumbnailEdits.Start(r.Context(), video, path, start)
	if err != nil {
		if errors.Is(err, domain.ErrMediaFileUnavailable) {
			s.notFoundReason(w, reasonFileUnavailable, "Cannot open this video's file.")
			return
		}
		if !s.thumbnailEditError(w, err) {
			s.thumbnailPositionError(w, video, err)
		}
		return
	}
	s.writeThumbnailEdit(w, http.StatusCreated, edit)
}

// StartThumbnailEditFromImage starts a session from an image the page drew; it needs no readable media file.
func (s *server) StartThumbnailEditFromImage(w http.ResponseWriter, r *http.Request, id gen.VideoId) {
	if s.thumbnailEdits == nil {
		s.internalError(w, "Thumbnail editing is not configured.", nil)
		return
	}
	video, ok := s.lookupFrameVideo(w, r, id)
	if !ok {
		return
	}
	image, ok := s.readThumbnailEditImage(w, r)
	if !ok {
		return
	}
	start := domain.ThumbnailEditStart{Source: domain.ThumbnailEditFromImage, Image: image}
	edit, err := s.thumbnailEdits.Start(r.Context(), video, "", start)
	switch {
	case errors.Is(err, domain.ErrThumbnailUploadInvalid):
		s.thumbnailUploadError(w, err)
	case err != nil:
		s.thumbnailEditFailure(w, err)
	default:
		s.writeThumbnailEdit(w, http.StatusCreated, edit)
	}
}

func (s *server) readThumbnailEditImage(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	tooLarge := func() {
		s.thumbnailUploadError(w, &domain.ThumbnailUploadTooLargeError{Limit: domain.ThumbnailEditMaxReferenceBytes})
	}
	var image []byte
	ok := s.readMultipart(w, r, domain.ThumbnailEditMaxReferenceBytes+1<<20, tooLarge, func(name string, part *multipart.Part) bool {
		if name != "image" {
			s.invalidRequest(w, "Only image may be sent.")
			return false
		}
		data, ok := readPart(part, domain.ThumbnailEditMaxReferenceBytes)
		if !ok {
			tooLarge()
			return false
		}
		image = data
		return true
	})
	if ok && image == nil {
		s.invalidRequest(w, "image is required.")
		return nil, false
	}
	return image, ok
}

// A missing frame position is rejected like an out-of-range position.
func checkThumbnailEditPosition(video domain.Video, positionMs *int64) error {
	if positionMs == nil {
		return domain.ErrThumbnailPositionOutOfRange
	}
	return domain.CheckThumbnailPosition(video, *positionMs)
}

func (s *server) GetThumbnailEdit(w http.ResponseWriter, r *http.Request, editID gen.ThumbnailEditId) {
	if s.thumbnailEdits == nil {
		s.internalError(w, "Thumbnail editing is not configured.", nil)
		return
	}
	edit, err := s.thumbnailEdits.Get(r.Context(), editID)
	if err != nil {
		s.thumbnailEditFailure(w, err)
		return
	}
	s.writeThumbnailEdit(w, http.StatusOK, edit)
}

func (s *server) GetThumbnailEditFrame(w http.ResponseWriter, r *http.Request, editID gen.ThumbnailEditId) {
	if s.thumbnailEdits == nil {
		s.internalError(w, "Thumbnail editing is not configured.", nil)
		return
	}
	frame, err := s.thumbnailEdits.Frame(r.Context(), editID)
	if err != nil {
		s.thumbnailEditFailure(w, err)
		return
	}
	s.writeImage(w, "image/png", frame)
}

func (s *server) GetThumbnailEditResult(w http.ResponseWriter, r *http.Request, editID gen.ThumbnailEditId) {
	if s.thumbnailEdits == nil {
		s.internalError(w, "Thumbnail editing is not configured.", nil)
		return
	}
	result, err := s.thumbnailEdits.Result(r.Context(), editID)
	if err != nil {
		s.thumbnailEditFailure(w, err)
		return
	}
	s.writeImage(w, http.DetectContentType(result), result)
}

func (s *server) RunThumbnailEdit(w http.ResponseWriter, r *http.Request, editID gen.ThumbnailEditId) {
	if s.thumbnailEdits == nil {
		s.internalError(w, "Thumbnail editing is not configured.", nil)
		return
	}
	input, ok := s.readThumbnailEditRun(w, r)
	if !ok {
		return
	}
	edit, err := s.thumbnailEdits.StartRun(r.Context(), editID, input)
	if err != nil {
		s.thumbnailEditFailure(w, err)
		return
	}
	s.writeThumbnailEdit(w, http.StatusAccepted, edit)
}

func (s *server) CancelThumbnailEdit(w http.ResponseWriter, r *http.Request, editID gen.ThumbnailEditId) {
	if s.thumbnailEdits == nil {
		s.internalError(w, "Thumbnail editing is not configured.", nil)
		return
	}
	edit, err := s.thumbnailEdits.Cancel(r.Context(), editID)
	if err != nil {
		s.thumbnailEditFailure(w, err)
		return
	}
	s.writeThumbnailEdit(w, http.StatusOK, edit)
}

func (s *server) ApplyThumbnailEdit(w http.ResponseWriter, r *http.Request, editID gen.ThumbnailEditId) {
	if s.thumbnailEdits == nil {
		s.internalError(w, "Thumbnail editing is not configured.", nil)
		return
	}
	video, err := s.thumbnailEdits.Apply(r.Context(), editID)
	if err != nil {
		s.thumbnailEditFailure(w, err)
		return
	}
	s.writeVideoDetail(w, r, video)
}

func (s *server) DiscardThumbnailEdit(w http.ResponseWriter, r *http.Request, editID gen.ThumbnailEditId) {
	if s.thumbnailEdits == nil {
		s.internalError(w, "Thumbnail editing is not configured.", nil)
		return
	}
	s.thumbnailEdits.Discard(r.Context(), editID)
	w.Header().Set("Cache-Control", cacheNoStore)
	w.WriteHeader(http.StatusNoContent)
}

// All run parts are optional, but duplicates and unknown names are invalid; oversize masks/references use their specific reasons.
func (s *server) readThumbnailEditRun(w http.ResponseWriter, r *http.Request) (domain.ThumbnailEditRunInput, bool) {
	var input domain.ThumbnailEditRunInput
	ok := s.readMultipart(w, r, thumbnailEditMaxRunBody, nil, func(name string, part *multipart.Part) bool {
		return s.readThumbnailEditPart(w, name, part, &input)
	})
	return input, ok
}

// readMultipart enforces total size and unique parts for edits, uploads, and plugins; callbacks write their own failures, with optional tooLarge handling for multipart overhead.
func (s *server) readMultipart(
	w http.ResponseWriter, r *http.Request, maxBody int64, tooLarge func(),
	readPart func(name string, part *multipart.Part) bool,
) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/form-data" {
		s.invalidRequest(w, "Content-Type must be multipart/form-data.")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	reader, err := r.MultipartReader()
	if err != nil {
		s.invalidRequest(w, "The request body is not valid multipart/form-data.")
		return false
	}
	seen := map[string]bool{}
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			return true
		}
		var bodyTooLarge *http.MaxBytesError
		if errors.As(err, &bodyTooLarge) && tooLarge != nil {
			tooLarge()
			return false
		}
		if err != nil {
			s.invalidRequest(w, "The request body is not valid multipart/form-data.")
			return false
		}
		name := part.FormName()
		if seen[name] {
			s.invalidRequest(w, "Each part may be sent only once.")
			return false
		}
		seen[name] = true
		if !readPart(name, part) {
			return false
		}
	}
}

func (s *server) readThumbnailEditPart(w http.ResponseWriter, name string, part io.Reader, input *domain.ThumbnailEditRunInput) bool {
	switch name {
	case "prompt":
		// Four UTF-8 bytes per code point suffice to check the character limit.
		text, ok := readPart(part, domain.ThumbnailEditMaxInstruction*4)
		if !ok || !utf8.Valid(text) || utf8.RuneCount(text) > domain.ThumbnailEditMaxInstruction {
			s.invalidRequest(w, "prompt must be valid text of at most 4000 characters.")
			return false
		}
		input.Instruction = string(text)
	case "mask":
		mask, ok := readPart(part, thumbnailEditMaxMaskBytes)
		if !ok {
			s.invalidRequestReason(w, reasonThumbnailEditMaskInvalid, "The mask must be a PNG of the frame's size.")
			return false
		}
		input.Mask = mask
	case "reference":
		reference, ok := readPart(part, domain.ThumbnailEditMaxReferenceBytes)
		if !ok {
			s.invalidRequestReason(w, reasonThumbnailEditReferenceInvalid, "The reference image must be a PNG, JPEG or WebP image of at most 20 MiB.")
			return false
		}
		input.Reference = reference
	default:
		s.invalidRequest(w, "Only prompt, mask and reference may be sent.")
		return false
	}
	return true
}

// Empty parts return non-nil empty bytes; unreadable or oversized parts return false.
func readPart(part io.Reader, limit int64) ([]byte, bool) {
	data, err := io.ReadAll(io.LimitReader(part, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, false
	}
	if data == nil {
		data = []byte{}
	}
	return data, true
}

func (s *server) thumbnailEditFailure(w http.ResponseWriter, err error) {
	if !s.thumbnailEditError(w, err) {
		s.internalError(w, "Could not handle the thumbnail edit.", err)
	}
}

// Never include provider text in response messages.
func (s *server) thumbnailEditError(w http.ResponseWriter, err error) bool {
	var failure *domain.ImageEditError
	switch {
	case errors.Is(err, domain.ErrThumbnailEditNotFound):
		s.notFoundReason(w, reasonThumbnailEditNotFound, "The thumbnail edit has ended.")
	case errors.Is(err, domain.ErrThumbnailEditNotReady):
		s.conflictReason(w, reasonThumbnailEditNotReady, "No ComfyUI connection is saved.")
	case errors.Is(err, domain.ErrThumbnailEditMaskInvalid):
		s.invalidRequestReason(w, reasonThumbnailEditMaskInvalid, "The mask must be a PNG of the frame's size.")
	case errors.Is(err, domain.ErrThumbnailEditReferenceInvalid):
		s.invalidRequestReason(w, reasonThumbnailEditReferenceInvalid, "The reference image must be a PNG, JPEG or WebP image of at most 20 MiB.")
	case errors.Is(err, domain.ErrThumbnailEditNoResult):
		s.conflictReason(w, reasonThumbnailEditNoResult, "The thumbnail edit has no generated image.")
	case errors.Is(err, domain.ErrContentChanged):
		s.conflictReason(w, reasonContentChanged, "The video is gone or now holds other content.")
	case errors.Is(err, domain.ErrThumbnailImageInvalid):
		s.conflictReason(w, reasonThumbnailImageInvalid, "The generated image cannot be decoded.")
	case errors.As(err, &failure):
		s.writeReasonError(w, http.StatusBadGateway, codeProviderFailed, gen.ErrorReason(failure.Reason),
			"ComfyUI did not accept the run.")
	default:
		return false
	}
	return true
}

func (s *server) writeThumbnailEdit(w http.ResponseWriter, status int, edit domain.ThumbnailEdit) {
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, status, thumbnailEditBody(edit), s.logger)
}

// Session images change over time and must not be cached.
func (s *server) writeImage(w http.ResponseWriter, contentType string, data []byte) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", cacheNoStore)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(data); err != nil {
		s.logger.Debug("could not write the image", "error", err)
	}
}

func thumbnailEditBody(edit domain.ThumbnailEdit) gen.ThumbnailEdit {
	base := "/api/thumbnail-edits/" + edit.ID
	body := gen.ThumbnailEdit{
		Id: edit.ID, VideoId: edit.VideoID, State: gen.ThumbnailEditState(edit.State),
		Source: gen.ThumbnailEditSource(edit.Source), PositionMs: edit.PositionMs,
		FrameUrl: base + "/frame", FrameWidth: edit.FrameWidth, FrameHeight: edit.FrameHeight,
	}
	if edit.Elapsed != nil {
		elapsed := edit.Elapsed.Milliseconds()
		body.ElapsedMs = &elapsed
	}
	if edit.Progress != nil {
		body.Progress = &gen.ThumbnailEditProgress{Value: edit.Progress.Value, Max: edit.Progress.Max}
	}
	if edit.Error != nil {
		failure := gen.ThumbnailEditError{Reason: gen.ThumbnailEditFailure(edit.Error.Reason)}
		if edit.Error.Message != "" {
			message := edit.Error.Message
			failure.Message = &message
		}
		body.Error = &failure
	}
	if edit.HasResult {
		result := base + "/result"
		body.ResultUrl = &result
	}
	return body
}
