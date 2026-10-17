package httpapi

import (
	"context"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// HomeOrganizer はホームの整理役である。internal/app の *HomeOrganizer が満たす
// （docs/design-docs/home.md、research.md R-9）。POST /api/home/organize は所有者だけで、
// accessRoutes に載せない。
type HomeOrganizer interface {
	// Reorganize は rebuild を待ちに入れ、入れたあとの状態を返す。
	Reorganize(ctx context.Context) (domain.HomeOrganizerState, error)
	// State は今の状態を返す。
	State(ctx context.Context) (domain.HomeOrganizerState, error)
}

// OrganizeHome は rebuild を待ちに入れ、202 で整理役の状態を返す。
func (s *server) OrganizeHome(w http.ResponseWriter, r *http.Request) {
	if s.homeOrganizer == nil {
		s.internalError(w, "The home organizer is not configured.", nil)
		return
	}
	state, err := s.homeOrganizer.Reorganize(r.Context())
	if err != nil {
		s.internalError(w, "Could not queue the home rebuild.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusAccepted, toAPIHomeOrganizer(state), s.logger)
}

// toAPIHomeOrganizer は整理役の状態を応答の形にする（data-model.md「Organizer state」）。
func toAPIHomeOrganizer(state domain.HomeOrganizerState) gen.HomeOrganizer {
	out := gen.HomeOrganizer{Running: state.Running}
	if state.Pending != "" {
		pending := gen.HomeOrganizerPending(state.Pending)
		out.Pending = &pending
	}
	if last := state.Last; last != nil {
		out.Last = &gen.HomeOrganizeRun{
			Run: gen.HomeOrganizeRunRun(last.Run), Outcome: gen.HomeOrganizeRunOutcome(last.Outcome),
			FinishedAt: last.FinishedAt, Added: last.Added, Removed: last.Removed,
		}
	}
	return out
}
