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
	if s.reprobe == nil {
		s.internalError(w, "読み取りのやり直し先が設定されていません", nil)
		return
	}

	// シーク用プレビューの置き場の有無はファイルの事実なので、ここで確かめて渡す。
	seekMissing := !seekThumbnailDirExists(s.thumbnailsDir, video.ContentKey)
	err := s.reprobe.RetryProbe(r.Context(), video.ID, seekMissing)
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
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusAccepted, withProgress(s.apiVideo(r.Context(), updated), progress, updated.ContentKey), s.logger)
}
