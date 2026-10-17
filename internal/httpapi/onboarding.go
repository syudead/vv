package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// Onboarding はオンボードの段階と分野の選び方である。internal/app の *Onboarding が満たす
// （specs/057-home/contracts/screen-api.md）。経路はどれも所有者だけで、accessRoutes に載せない。
type Onboarding interface {
	Step(ctx context.Context) (domain.OnboardingStep, error)
	// SetStep は段階を書き、書いた段階を返す。段階は呼ぶ側が確かめる。
	SetStep(ctx context.Context, step domain.OnboardingStep) (domain.OnboardingStep, error)
	Interests(ctx context.Context) ([]domain.InterestStatus, error)
	// PickInterests は分野をタグと棚にする。id が規則に合わなければ domain.ErrInvalidInterestPick、
	// 棚が上限を超えるなら domain.ErrHomeShelvesFull。
	PickInterests(ctx context.Context, ids []string) (domain.InterestPickResult, error)
}

// GetOnboarding はオンボードの段階を返す。
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

// UpdateOnboarding は名前の段階を書く（research.md R-3）。
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

// ListOnboardingInterests は分野の一覧を一覧の順に返す。
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
		interests[i] = gen.Interest{Id: status.ID, Name: status.Name, Synonym: status.Synonym, OnHome: status.OnHome}
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.InterestList{Interests: interests}, s.logger)
}

// PickOnboardingInterests は選んだ分野をタグと棚にする（research.md R-6）。
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
	case errors.Is(err, domain.ErrHomeShelvesFull):
		s.writeError(w, http.StatusConflict, codeConflict,
			"The home holds at most "+strconv.Itoa(domain.MaxHomeShelves)+" shelves.")
		return
	case err != nil:
		s.internalError(w, "Could not add the interests.", err)
		return
	}
	shelves := make([]gen.HomeShelf, len(result.Shelves))
	for i, folder := range result.Shelves {
		shelves[i] = toAPIFolderShelf(folder)
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.InterestPickResult{Shelves: shelves, Queued: result.Queued}, s.logger)
}

// toAPIFolderShelf はフォルダの棚を所有者への応答の形にする。
func toAPIFolderShelf(folder domain.SmartFolder) gen.HomeShelf {
	return toAPIHomeShelf(domain.AudienceOwner, domain.FolderShelf(folder))
}
