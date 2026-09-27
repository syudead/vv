package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// GetVideoSeekThumbnail はシーク用サムネイルのスプライトの配置情報を返す
// （specs/021-seek-thumbnail-sprite/contracts/seek-sprite-api.md §2）。
func (s *server) GetVideoSeekThumbnail(
	w http.ResponseWriter,
	r *http.Request,
	id gen.VideoId,
	_ gen.GetVideoSeekThumbnailParams,
) {
	video, sprite, ok := s.lookupSeekSprite(w, r, id)
	if !ok {
		return
	}
	payload := gen.SeekThumbnailSprite{
		IntervalMs:  sprite.IntervalMs,
		FrameCount:  sprite.FrameCount,
		Columns:     sprite.Columns,
		Rows:        sprite.Rows,
		FrameWidth:  sprite.FrameWidth,
		FrameHeight: sprite.FrameHeight,
		Sheets:      make([]string, sprite.SheetCount),
	}
	for sheet := range sprite.SheetCount {
		payload.Sheets[sheet] = seekThumbnailSheetURL(video, sheet)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		s.internalError(w, "Could not build the seek preview layout.", err)
		return
	}
	body = append(body, '\n')

	// 版の有無によらず、使うたびに確かめさせる（contracts/guest-api.md §5）。
	etag := bytesETag(body)
	setRevalidate(w, etag)
	if etagMatches(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", contentTypeJSON)
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(body); err != nil {
		s.logger.Debug("could not write the seek preview layout", slog.Int64("video", video.ID), slog.Any("error", err))
	}
}

// GetVideoSeekThumbnailSheet はシーク用サムネイルのスプライトのシートを返す
// （contracts/seek-sprite-api.md §3）。
func (s *server) GetVideoSeekThumbnailSheet(
	w http.ResponseWriter,
	r *http.Request,
	id gen.VideoId,
	sheet int,
	_ gen.GetVideoSeekThumbnailSheetParams,
) {
	if sheet < 0 {
		s.invalidRequest(w, "sheet must be a non-negative integer.")
		return
	}
	video, sprite, ok := s.lookupSeekSprite(w, r, id)
	if !ok {
		return
	}
	if sheet >= sprite.SheetCount {
		s.notFound(w, "No such sheet.")
		return
	}
	image, err := s.artifacts.SeekSpriteSheet(video.ContentKey, sheet)
	switch {
	case err != nil && errors.Is(r.Context().Err(), context.Canceled):
		return
	case err != nil:
		s.internalError(w, "Could not read the seek preview.", err)
		return
	case len(image) == 0:
		s.internalError(w, "Could not read the seek preview.", errors.New("sheet is empty"))
		return
	}

	etag := bytesETag(image)
	setRevalidate(w, etag)
	if etagMatches(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Content-Length", strconv.Itoa(len(image)))
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(image); err != nil {
		s.logger.Debug("could not write the seek preview", slog.Int64("video", video.ID), slog.Any("error", err))
	}
}

// lookupSeekSprite は動画と、完成したスプライトの配置情報を引く。解析が終わって
// いない・尺が無い、またはスプライトが完成していなければ 409 を書く。完成の
// 判断（配置情報の有無）は Catalog.SeekThumbnailState が done を返す条件と同じで、
// 置き場（internal/artifacts）が持つ。
func (s *server) lookupSeekSprite(
	w http.ResponseWriter, r *http.Request, id int64,
) (domain.Video, domain.SeekSprite, bool) {
	video, ok := s.lookupVideo(w, r, id)
	if !ok {
		return domain.Video{}, domain.SeekSprite{}, false
	}
	if !video.HasSeekThumbnail() {
		s.conflictReason(w, reasonProbeInfoMissing, "This video lacks the media information needed for the seek preview.")
		return domain.Video{}, domain.SeekSprite{}, false
	}
	if s.artifacts == nil {
		s.internalError(w, "Seek preview storage is not configured.", nil)
		return domain.Video{}, domain.SeekSprite{}, false
	}
	sprite, err := s.artifacts.SeekSprite(video.ContentKey)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		s.logger.Info("seek preview is being generated",
			slog.Int64("video", video.ID), slog.Any("error", err))
		s.conflictReason(w, reasonSeekPreviewGenerating, "The seek preview is being generated.")
		return domain.Video{}, domain.SeekSprite{}, false
	case err != nil && errors.Is(r.Context().Err(), context.Canceled):
		return domain.Video{}, domain.SeekSprite{}, false
	case err != nil:
		s.internalError(w, "Could not read the seek preview layout.", err)
		return domain.Video{}, domain.SeekSprite{}, false
	}
	return video, sprite, true
}
