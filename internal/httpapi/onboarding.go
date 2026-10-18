package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

type Onboarding interface {
	Step(ctx context.Context) (domain.OnboardingStep, error)

	SetStep(ctx context.Context, step domain.OnboardingStep) (domain.OnboardingStep, error)
	Interests(ctx context.Context) ([]domain.InterestStatus, error)

	PickInterests(ctx context.Context, ids []string) (domain.InterestPickResult, error)
}

func (s *server) GetOnboarding(w http.ResponseWriter, r *http.Request) {
	if s.onboarding == nil {
		s.internalError(w, "Onboarding is not configured.", nil)
		return
	}
	step, err := s.onboarding.Step(r.Context())
	if err != nil {
		s.internalError(w, "Could not read the onboarding step.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.Onboarding{Step: gen.OnboardingStep(step)}, s.logger)
}

func (s *server) UpdateOnboarding(w http.ResponseWriter, r *http.Request) {
	if s.onboarding == nil {
		s.internalError(w, "Onboarding is not configured.", nil)
		return
	}
	var body struct {
		Step *string `json:"step"`
	}
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if body.Step == nil || !domain.OnboardingStep(*body.Step).Valid() {
		s.invalidRequest(w, "step must be folders, scan, interests or done.")
		return
	}
	step, err := s.onboarding.SetStep(r.Context(), domain.OnboardingStep(*body.Step))
	if err != nil {
		s.internalError(w, "Could not save the onboarding step.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.Onboarding{Step: gen.OnboardingStep(step)}, s.logger)
}

func (s *server) ListOnboardingInterests(w http.ResponseWriter, r *http.Request) {
	if s.onboarding == nil {
		s.internalError(w, "Onboarding is not configured.", nil)
		return
	}
	statuses, err := s.onboarding.Interests(r.Context())
	if err != nil {
		s.internalError(w, "Could not read the interests.", err)
		return
	}
	interests := make([]gen.Interest, len(statuses))
	for i, status := range statuses {
		interests[i] = gen.Interest{Id: status.ID, Name: status.Name, Synonym: status.Synonym, HasFolder: status.HasFolder}
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.InterestList{Interests: interests}, s.logger)
}

func (s *server) PickOnboardingInterests(w http.ResponseWriter, r *http.Request) {
	if s.onboarding == nil {
		s.internalError(w, "Onboarding is not configured.", nil)
		return
	}
	var body struct {
		IDs *[]string `json:"ids"`
	}
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if body.IDs == nil {
		s.invalidRequest(w, "ids is required.")
		return
	}
	result, err := s.onboarding.PickInterests(r.Context(), *body.IDs)
	switch {
	case errors.Is(err, domain.ErrInvalidInterestPick):
		s.invalidRequest(w, "ids must name known interests, each once.")
		return
	case err != nil:
		s.internalError(w, "Could not add the interests.", err)
		return
	}
	folders := make([]gen.SmartFolderSummary, len(result.Folders))
	for i, folder := range result.Folders {
		folders[i] = gen.SmartFolderSummary{Id: folder.ID, Name: folder.Name, ParentId: parentIDRef(folder.ParentID)}
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.InterestPickResult{Folders: folders, Queued: result.Queued}, s.logger)
}
