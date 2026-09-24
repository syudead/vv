package httpapi

import (
	"net/http"

	"github.com/syudead/vv/internal/httpapi/gen"
)

// GetRelatedVideos は関連動画を返す（GET /api/videos/{id}/related）。
//
// 並べ方と問い合わせの流れはアプリケーション層（VideoCatalog）が持つ。ここは
// 動画を引き、並んだ結果を契約の形へ写すだけである。
func (s *server) GetRelatedVideos(w http.ResponseWriter, r *http.Request, id gen.VideoId) {
	video, ok := s.lookupVideo(w, r, id)
	if !ok {
		return
	}
	if s.catalog == nil {
		s.internalError(w, "関連動画の問い合わせ先が設定されていません", nil)
		return
	}

	related, err := s.catalog.RelatedVideos(r.Context(), video)
	if err != nil {
		s.internalError(w, "関連動画を取得できませんでした", err)
		return
	}

	progress := s.progressFor(r.Context(), related.Items)
	tags := s.tagsFor(r.Context(), related.Items)
	payload := gen.RelatedVideos{Items: make([]gen.Video, 0, len(related.Items))}
	for _, view := range s.presentVideos(r.Context(), related.Items) {
		item := withTags(withProgress(toAPIVideo(view), progress, view.Video.ContentKey), tags, view.Video.ContentKey)
		payload.Items = append(payload.Items, item)
	}
	if related.NextID != 0 {
		next := related.NextID
		payload.NextId = &next
	}

	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, payload, s.logger)
}
