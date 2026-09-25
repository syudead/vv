package httpapi

import (
	"context"
	"net/http"
	"slices"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// GetRelatedVideos は関連動画を返す（GET /api/videos/{id}/related）。
//
// 並べ方と問い合わせの流れはアプリケーション層（VideoCatalog）が持つ。ここは
// 動画を引き、並んだ結果を契約の形へ写すだけである。グループのメンバーなら、
// グループの全メンバーも関連動画と同じ形で載せる。
func (s *server) GetRelatedVideos(w http.ResponseWriter, r *http.Request, id gen.VideoId) {
	video, ok := s.lookupVideo(w, r, id)
	if !ok {
		return
	}
	if s.catalog == nil {
		s.internalError(w, "関連動画の問い合わせ先が設定されていません", nil)
		return
	}
	audience := audienceFrom(r.Context())

	related, err := s.catalog.RelatedVideos(r.Context(), audience, video)
	if err != nil {
		s.internalError(w, "関連動画を取得できませんでした", err)
		return
	}

	// グループのメンバーにも関連動画と同じく再生位置とタグを載せるので、まとめて引く。
	shown := related.Items
	if related.Group != nil {
		shown = append(slices.Clone(related.Items), related.Group.Members...)
	}
	progress := s.progressFor(r.Context(), shown)
	tags := s.tagsFor(r.Context(), shown)
	present := func(ctx context.Context, videos []domain.Video) []gen.Video {
		out := make([]gen.Video, 0, len(videos))
		for _, view := range s.presentVideos(ctx, videos) {
			item := withTags(withProgress(toAPIVideo(view), progress, view.Video.ContentKey), tags, view.Video.ContentKey)
			out = append(out, forAudience(audience, item))
		}
		return out
	}
	payload := gen.RelatedVideos{Items: present(r.Context(), related.Items)}
	if group := related.Group; group != nil {
		payload.Group = &gen.RelatedGroup{
			Folder: gen.VideoFolder{RootId: group.Folder.RootID, Path: group.Folder.Path},
			Name:   group.Name,
			Items:  present(r.Context(), group.Members),
		}
	}
	if related.NextID != 0 {
		next := related.NextID
		payload.NextId = &next
	}
	if related.PrevID != 0 {
		prev := related.PrevID
		payload.PrevId = &prev
	}

	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, payload, s.logger)
}
