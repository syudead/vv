package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

func (s *server) GetHealth(w http.ResponseWriter, r *http.Request) {
	reachable := false
	if s.pinger != nil {
		if err := s.pinger.Ping(r.Context()); err != nil {
			s.logger.Warn("cannot reach the store", slog.Any("error", err))
		} else {
			reachable = true
		}
	} else {
		s.logger.Warn("store health check is not configured")
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

	// Health responses must bypass caches to reflect current reachability.
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, status, payload, s.logger)
}
