package httpapi

import (
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// 「同じ動画かもしれない」候補の一覧と「違う動画」の記録
// （specs/030-video-versions/contracts/screen-api.md §5・§6、research.md R-7・R-8）。どちらも
// 所有者だけで、accessRoutes に載せない。候補は取り込みの fingerprint の段階で増減し、画面は
// /api/events の scan（ProcessingChanged）で取り直すので、新しい知らせの種類は足さない。
// 「同じ動画」は POST /api/video-bundles を使い、束ねると保存層が候補を消す。

// ListVersionCandidates は候補を新しい順に返す（GET /api/version-candidates）。各組の動画は
// id の小さい順で、GET /api/videos/{id} と同じ形。
func (s *server) ListVersionCandidates(w http.ResponseWriter, r *http.Request) {
	if s.versions == nil {
		s.internalError(w, "Version storage is not configured.", nil)
		return
	}
	page, err := s.versions.Candidates(r.Context())
	if err != nil {
		s.internalError(w, "Could not load the possible duplicates.", err)
		return
	}

	videos := make([]domain.Video, 0, len(page.Items)*2)
	for _, item := range page.Items {
		videos = append(videos, item.Videos[0], item.Videos[1])
	}
	items := s.versionItems(r, videos)
	payload := gen.VersionCandidatePage{Items: make([]gen.VersionCandidate, 0, len(page.Items)), Total: page.Total}
	for i, item := range page.Items {
		payload.Items = append(payload.Items, gen.VersionCandidate{
			Videos:   []gen.Video{items[2*i], items[2*i+1]},
			Distance: item.Distance,
		})
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, payload, s.logger)
}

// DismissVersionCandidate は 2 本を「違う動画」と記録する（POST /api/version-candidates/dismiss）。
func (s *server) DismissVersionCandidate(w http.ResponseWriter, r *http.Request) {
	var body gen.VersionCandidateDismissRequest
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if len(body.VideoIds) != 2 || body.VideoIds[0] == body.VideoIds[1] {
		s.invalidRequest(w, "videoIds must be two different video ids.")
		return
	}
	if s.versions == nil {
		s.internalError(w, "Version storage is not configured.", nil)
		return
	}
	err := s.versions.Dismiss(r.Context(), [2]int64{body.VideoIds[0], body.VideoIds[1]})
	if !s.checkVersionsError(w, err, "Could not record the videos as different.") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
