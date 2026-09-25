package httpapi

import (
	"errors"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// ReprobeVideo は読み取りに失敗した動画を読み取り直す（POST /api/videos/{id}/probe）。
//
// 状態を戻すこととジョブを積むことは、保存層が1つの取引で行う。失敗していない
// 動画への要求は 409 probe_not_failed で、連打や別タブからの二度目もこれになる。
// 画面は 409 を「すでにやり直し中」とみなす。
func (s *server) ReprobeVideo(w http.ResponseWriter, r *http.Request, id gen.VideoId) {
	video, ok := s.lookupVideo(w, r, id)
	if !ok {
		return
	}
	if s.catalog == nil {
		s.internalError(w, "読み取りのやり直し先が設定されていません", nil)
		return
	}

	err := s.catalog.RetryProbe(r.Context(), video)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		s.notFound(w, "その動画はありません")
		return
	case errors.Is(err, domain.ErrProbeNotFailed):
		s.writeError(w, http.StatusConflict, codeProbeNotFailed, "この動画は読み取り中か、読み取り済みです")
		return
	case err != nil:
		s.internalError(w, "読み取りをやり直せませんでした", err)
		return
	}

	updated, ok := s.lookupVideo(w, r, id)
	if !ok {
		return
	}
	progress := s.progressFor(r.Context(), []domain.Video{updated})
	tags := s.tagsFor(r.Context(), []domain.Video{updated})
	payload := withTags(withProgress(s.apiVideo(r.Context(), updated), progress, updated.ContentKey), tags, updated.ContentKey)
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusAccepted, payload, s.logger)
}
