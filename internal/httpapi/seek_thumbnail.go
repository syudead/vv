package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

const seekThumbnailTimeout = 10 * time.Second

// GetVideoSeekThumbnail generates one JPEG for the requested logical time.
func (s *server) GetVideoSeekThumbnail(
	w http.ResponseWriter,
	r *http.Request,
	id gen.VideoId,
	params gen.GetVideoSeekThumbnailParams,
) {
	video, ok := s.lookupVideo(w, r, id)
	if !ok {
		return
	}
	if video.ProbeState != domain.ProbeStateDone || video.DurationMs == nil || *video.DurationMs <= 0 {
		s.writeError(w, http.StatusConflict, codeConflict, "この動画はプレビューに必要な解析情報がありません")
		return
	}
	if params.PositionMs < 0 || params.PositionMs >= *video.DurationMs {
		s.invalidRequest(w, "positionMsは動画の範囲内で指定してください")
		return
	}
	if s.seekThumbnails == nil {
		s.internalError(w, "シークプレビュー生成が設定されていません", nil)
		return
	}

	file, _, _, ok := s.openMediaFile(r, video)
	if !ok {
		s.notFound(w, "この動画の実体を開けません")
		return
	}
	defer func() { _ = file.Close() }()

	ctx, cancel := context.WithTimeout(r.Context(), seekThumbnailTimeout)
	defer cancel()
	image, err := s.seekThumbnails.Extract(ctx, file.Name(), params.PositionMs)
	switch {
	case errors.Is(err, domain.ErrSeekFrameUnavailable):
		s.logger.Info("シークプレビューのframeを取得できません",
			slog.Int64("video", video.ID), slog.Any("error", err))
		s.writeError(w, http.StatusConflict, codeConflict, "この位置のプレビューを生成できません")
		return
	case err != nil && errors.Is(r.Context().Err(), context.Canceled):
		return
	case err != nil:
		s.internalError(w, "シークプレビューを生成できませんでした", err)
		return
	}
	if len(image) == 0 {
		s.internalError(w, "シークプレビューを生成できませんでした", errors.New("生成画像が空です"))
		return
	}

	if params.V != nil && *params.V != "" {
		w.Header().Set("Cache-Control", cacheImmutable)
	} else {
		w.Header().Set("Cache-Control", cacheNoStore)
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Content-Length", strconv.Itoa(len(image)))
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(image); err != nil {
		s.logger.Debug("シークプレビューを書き出せませんでした", slog.Int64("video", video.ID), slog.Any("error", err))
	}
}
