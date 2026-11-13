package httpapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// ImageSearch searches the library by one uploaded file, streamed as the request body arrives.
type ImageSearch interface {
	// Search reads file to its end, then calls onlyFile and returns its error unchanged; nothing it reads is kept.
	Search(ctx context.Context, audience domain.Audience, file io.Reader, onlyFile func() error) (domain.ImageSearchResult, error)
}

// Allow one MiB of multipart overhead beyond the largest video.
const imageSearchMaxBody = domain.ImageSearchVideoMaxBytes + 1<<20

var (
	errImageSearchSecondPart = errors.New("the request holds more than the file")
	errImageSearchBody       = errors.New("the request body could not be read")
)

// SearchByImage streams the one file part to the use case, which keeps a video in the upload area only until it
// returns; a cancelled request writes no response.
func (s *server) SearchByImage(w http.ResponseWriter, r *http.Request) {
	if s.imageSearch == nil {
		s.internalError(w, "Image search is not configured.", nil)
		return
	}
	reader, ok := s.multipartReader(w, r, imageSearchMaxBody)
	if !ok {
		return
	}
	part, err := reader.NextPart()
	switch {
	case errors.Is(err, io.EOF):
		s.invalidRequest(w, "file is required.")
		return
	case err != nil:
		s.imageSearchError(w, errors.Join(errImageSearchBody, err))
		return
	case part.FormName() != "file":
		s.invalidRequest(w, "Only file may be sent.")
		return
	}
	onlyFile := func() error {
		if _, err := reader.NextPart(); !errors.Is(err, io.EOF) {
			return errImageSearchSecondPart
		}
		return nil
	}
	audience := audienceFrom(r.Context())
	result, err := s.imageSearch.Search(r.Context(), audience, bodyReader{part}, onlyFile)
	if r.Context().Err() != nil {
		return
	}
	if err != nil {
		s.imageSearchError(w, err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, s.imageSearchBody(r.Context(), audience, result), s.logger)
}

// bodyReader marks read failures as the request's, so they are not reported as server errors.
type bodyReader struct{ r io.Reader }

func (b bodyReader) Read(p []byte) (int, error) {
	n, err := b.r.Read(p)
	if err != nil && !errors.Is(err, io.EOF) {
		err = errors.Join(errImageSearchBody, err)
	}
	return n, err
}

func (s *server) imageSearchBody(ctx context.Context, audience domain.Audience, result domain.ImageSearchResult) gen.ImageSearchResult {
	videos := make([]domain.Video, len(result.Items))
	for i, item := range result.Items {
		videos[i] = item.Video
	}
	presented := s.presenter(ctx, audience, videos)(ctx, videos)
	body := gen.ImageSearchResult{
		Query:          gen.ImageSearchQuery{Kind: gen.ImageSearchQueryKind(result.Kind)},
		Bands:          make([]gen.ImageSearchBand, len(result.Bands)),
		Items:          make([]gen.ImageSearchItem, len(result.Items)),
		UnindexedCount: result.UnindexedCount,
	}
	for i, band := range result.Bands {
		body.Bands[i] = gen.ImageSearchBand{
			Id: gen.ImageSearchBandId(band.ID), MinPercent: band.MinPercent, MaxPercent: band.MaxPercent,
		}
	}
	if result.Kind == domain.ImageSearchQueryVideo {
		body.Query.DurationMs = &result.DurationMs
	}
	for i, item := range result.Items {
		body.Items[i] = gen.ImageSearchItem{Video: presented[i], Match: imageSearchMatchBody(item)}
	}
	return body
}

func imageSearchMatchBody(item domain.ImageSearchItem) gen.ImageSearchMatch {
	match := item.Match
	body := gen.ImageSearchMatch{
		VideoId: match.VideoID, Rate: match.Rate, Kind: gen.ImageSearchMatchKind(match.Kind), PositionMs: match.PositionMs,
		Band: gen.ImageSearchBandId(item.Band), Frames: make([]gen.ImageSearchMatchFrame, len(item.Frames)),
		FrameCount: item.FrameCount,
	}
	for i, frame := range item.Frames {
		body.Frames[i] = gen.ImageSearchMatchFrame{PositionMs: frame.PositionMs, Rate: frame.Rate}
	}
	if match.Kind != domain.ImageSearchMatchFrame {
		body.StartMs, body.EndMs = &match.StartMs, &match.EndMs
		body.QueryStartMs, body.QueryEndMs = &match.QueryStartMs, &match.QueryEndMs
	}
	return body
}

func (s *server) imageSearchError(w http.ResponseWriter, err error) {
	var tooLarge *domain.ImageSearchTooLargeError
	var tooLong *domain.ImageSearchTooLongError
	var bodyTooLarge *http.MaxBytesError
	switch {
	case errors.As(err, &bodyTooLarge):
		s.imageSearchTooLarge(w, domain.ImageSearchVideoMaxBytes)
	case errors.As(err, &tooLarge):
		s.imageSearchTooLarge(w, tooLarge.Limit)
	case errors.As(err, &tooLong):
		s.invalidRequestLimit(w, gen.ErrorReasonImageSearchTooLong, int(tooLong.LimitMs),
			fmt.Sprintf("The video must be at most %d hours long.", tooLong.LimitMs/3_600_000))
	case errors.Is(err, errImageSearchSecondPart):
		s.invalidRequest(w, "Send exactly one file.")
	case errors.Is(err, errImageSearchBody):
		s.invalidRequest(w, "The request body could not be read.")
	case errors.Is(err, domain.ErrImageSearchUnreadable):
		s.invalidRequestReason(w, gen.ErrorReasonImageSearchUnreadable,
			"The file must be a readable PNG, JPEG or still WebP image, or a video of at least one second.")
	case errors.Is(err, domain.ErrImageSearchUninformative):
		s.invalidRequestReason(w, gen.ErrorReasonImageSearchUninformative,
			"The file is too plain to search by.")
	default:
		s.internalError(w, "Could not search by the file.", err)
	}
}

func (s *server) imageSearchTooLarge(w http.ResponseWriter, limit int64) {
	reason := gen.ErrorReasonImageSearchTooLarge
	value := int(limit)
	message := fmt.Sprintf("The file must be at most %d MiB.", limit>>20)
	if limit == domain.ImageSearchImageMaxPixels {
		message = fmt.Sprintf("The image must be at most %d megapixels.", limit/1_000_000)
	}
	s.writeErrorBody(w, http.StatusRequestEntityTooLarge, gen.Error{
		Code: codeInvalidRequest, Reason: &reason, Limit: &value, Message: message,
	})
}
