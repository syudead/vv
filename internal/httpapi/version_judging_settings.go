package httpapi

import (
	"net/http"

	"github.com/syudead/vv/internal/httpapi/gen"
)

// GetVersionJudgingSettings は重複の自動判定の入切を返す。保存前は切である
// （specs/054-auto-dup/contracts/screen-api.md）。
func (s *server) GetVersionJudgingSettings(w http.ResponseWriter, r *http.Request) {
	if s.versionJudging == nil {
		s.internalError(w, "Duplicate judging settings are not configured.", nil)
		return
	}
	enabled, err := s.versionJudging.Enabled(r.Context())
	if err != nil {
		s.internalError(w, "Could not read the duplicate judging setting.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.VersionJudgingSettings{Enabled: enabled}, s.logger)
}

// UpdateVersionJudgingSettings は入切を保存し、判定の処理を起こす。入れると待っていた候補を判定し始める。
func (s *server) UpdateVersionJudgingSettings(w http.ResponseWriter, r *http.Request) {
	if s.versionJudging == nil {
		s.internalError(w, "Duplicate judging settings are not configured.", nil)
		return
	}
	// enabled は必須なので、無いことを偽と区別して読む。
	var body struct {
		Enabled *bool `json:"enabled"`
	}
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if body.Enabled == nil {
		s.invalidRequest(w, "enabled must be true or false.")
		return
	}
	if err := s.versionJudging.SaveEnabled(r.Context(), *body.Enabled); err != nil {
		s.internalError(w, "Could not save the duplicate judging setting.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.VersionJudgingSettings{Enabled: *body.Enabled}, s.logger)
}
