package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// GetHealth は稼働状態とビルド情報を返す（GET /api/health）。
//
// 保存層まで疎通できていれば 200 と status=ok、疎通できなければ 503 と
// status=degraded を返す。判定は要求のたびに行い、状態を保持しない。
func (s *server) GetHealth(w http.ResponseWriter, r *http.Request) {
	reachable := false
	if s.pinger != nil {
		if err := s.pinger.Ping(r.Context()); err != nil {
			s.logger.Warn("保存層へ疎通できません", slog.Any("error", err))
		} else {
			reachable = true
		}
	} else {
		s.logger.Warn("保存層への疎通確認が設定されていません")
	}

	health := domain.NewHealth(s.build, reachable)

	payload := gen.Health{
		Status:  gen.HealthStatus(health.Status),
		Version: health.Version,
	}
	if health.Commit != "" {
		payload.Commit = &health.Commit
	}
	if !health.BuiltAt.IsZero() {
		builtAt := health.BuiltAt
		payload.BuiltAt = &builtAt
	}

	status := http.StatusOK
	if health.Status != domain.StatusOK {
		status = http.StatusServiceUnavailable
	}

	// 稼働確認は常に最新の判定を返す必要があるため中間キャッシュを禁止する。
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, status, payload, s.logger)
}
