package httpapi

import (
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// GetAutoImportSettings は自動の取り込みの選択と、フォルダの監視の今の状態を返す
// （specs/042-folder-watch-import/contracts/screen-api.md）。
func (s *server) GetAutoImportSettings(w http.ResponseWriter, _ *http.Request) {
	if s.autoImport == nil {
		s.internalError(w, "Auto-import settings are not configured.", nil)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, toAPIAutoImportSettings(s.autoImport.Status()), s.logger)
}

// UpdateAutoImportSettings は選択を保存し、監視を張る／外す。走査は始めない。
func (s *server) UpdateAutoImportSettings(w http.ResponseWriter, r *http.Request) {
	if s.autoImport == nil {
		s.internalError(w, "Auto-import settings are not configured.", nil)
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
	status, err := s.autoImport.SetEnabled(r.Context(), *body.Enabled)
	if err != nil {
		s.internalError(w, "Could not save the auto-import setting.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, toAPIAutoImportSettings(status), s.logger)
}

func toAPIAutoImportSettings(status domain.AutoImportStatus) gen.AutoImportSettings {
	watch := gen.FolderWatch{State: gen.FolderWatchState(status.State)}
	if status.State == domain.FolderWatchLimited && status.Problem != nil {
		problem := gen.FolderWatchProblem(status.Problem.Kind)
		watch.Problem = &problem
		if status.Problem.Path != "" {
			path := status.Problem.Path
			watch.Path = &path
		}
	}
	return gen.AutoImportSettings{Enabled: status.Enabled, Watch: watch}
}
