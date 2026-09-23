package httpapi

import (
	"net/http"
	"path/filepath"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// GetRelatedVideos は関連動画を返す（GET /api/videos/{id}/related）。
//
// 並べ方は internal/domain の OrderRelated が決める。ここは、代表の所在の
// ディレクトリ直下の動画と、追加日時の近い動画を読み、選ばれた動画の本体を
// 返す順に並べるだけである。
func (s *server) GetRelatedVideos(w http.ResponseWriter, r *http.Request, id gen.VideoId) {
	video, ok := s.lookupVideo(w, r, id)
	if !ok {
		return
	}
	if s.related == nil {
		s.internalError(w, "関連動画の問い合わせ先が設定されていません", nil)
		return
	}

	siblings, err := s.related.DirectVideoPaths(r.Context(), filepath.Dir(video.Path))
	if err != nil {
		s.internalError(w, "関連動画を取得できませんでした", err)
		return
	}
	neighbors, err := s.related.VideosAddedNear(r.Context(), video.ID, video.AddedAt, domain.MaxRelatedVideos)
	if err != nil {
		s.internalError(w, "関連動画を取得できませんでした", err)
		return
	}
	order := domain.OrderRelated(domain.RelatedSelf{VideoID: video.ID, Path: video.Path, AddedAt: video.AddedAt},
		siblings, neighbors)
	videos, err := s.related.VideosByIDs(r.Context(), order.IDs)
	if err != nil {
		s.internalError(w, "関連動画を取得できませんでした", err)
		return
	}

	progress := s.progressFor(r.Context(), videos)
	payload := gen.RelatedVideos{Items: make([]gen.Video, 0, len(videos))}
	for _, item := range videos {
		payload.Items = append(payload.Items, withProgress(toAPIVideo(item, s.thumbnailsDir), progress, item.ContentKey))
	}
	if order.NextID != 0 {
		next := order.NextID
		payload.NextId = &next
	}

	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, payload, s.logger)
}
