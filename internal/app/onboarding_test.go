package app

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// fakeOnboardingStore は段階と、選ばれた分野を覚える。
type fakeOnboardingStore struct {
	step   domain.OnboardingStep
	picked [][]domain.Interest
	full   bool
}

func (s *fakeOnboardingStore) OnboardingStep(context.Context) (domain.OnboardingStep, error) {
	return s.step, nil
}

func (s *fakeOnboardingStore) SaveOnboardingStep(_ context.Context, step domain.OnboardingStep) error {
	s.step = step
	return nil
}

func (s *fakeOnboardingStore) Interests(context.Context) ([]domain.InterestStatus, error) {
	return nil, nil
}

func (s *fakeOnboardingStore) PickInterests(_ context.Context, interests []domain.Interest) ([]domain.SmartFolder, error) {
	if s.full {
		return nil, domain.ErrHomeShelvesFull
	}
	s.picked = append(s.picked, interests)
	shelves := make([]domain.SmartFolder, len(interests))
	for i, interest := range interests {
		shelves[i] = domain.SmartFolder{ID: int64(i + 1), Name: interest.Name, OwnerKept: true}
	}
	return shelves, nil
}

func newTestOnboarding(settings *fakeAutoTagSettings) (*Onboarding, *fakeOnboardingStore, *fakeAutoTagQueue) {
	store := &fakeOnboardingStore{step: domain.OnboardingFolders}
	queue := newFakeAutoTagQueue()
	tagger := NewAutoTagger(AutoTaggerOptions{Queue: queue, Settings: settings, Classifier: &fakeClassifier{}})
	return NewOnboarding(store, tagger), store, queue
}

// 自動タグ付けが入なら、分野を選ぶと「Tag all videos again」と同じくすべての動画を積む。
func TestPickInterestsQueuesEveryVideoWhenAutoTaggingIsOn(t *testing.T) {
	onboarding, store, queue := newTestOnboarding(enabledAutoTagSettings())

	result, err := onboarding.PickInterests(context.Background(), []string{"anime", "music"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Shelves) != 2 || result.Queued != 3 {
		t.Fatalf("result = %+v, want 2 shelves and the 3 queued videos", result)
	}
	if !slices.Equal(queue.modes, []domain.AutoTagQueueMode{domain.AutoTagScopeAll.QueueMode()}) {
		t.Fatalf("queue modes = %v, want one queue of every video", queue.modes)
	}
	if len(store.picked) != 1 || store.picked[0][0].ID != "anime" || store.picked[0][1].ID != "music" {
		t.Fatalf("picked = %+v", store.picked)
	}
}

// 自動タグ付けが切なら、何も積まずに棚はでき、Queued は 0 である。
func TestPickInterestsMakesShelvesWithoutQueuingWhenAutoTaggingIsOff(t *testing.T) {
	onboarding, _, queue := newTestOnboarding(&fakeAutoTagSettings{settings: domain.DefaultAutoTagSettings()})

	result, err := onboarding.PickInterests(context.Background(), []string{"travel"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Shelves) != 1 || result.Queued != 0 || len(queue.modes) != 0 {
		t.Fatalf("result = %+v, queue modes = %v; want 1 shelf and nothing queued", result, queue.modes)
	}
}

// 規則に合わない id と上限を超える棚は、何も積まずに誤りを返す。
func TestPickInterestsRefusesBadPicksBeforeQueuing(t *testing.T) {
	onboarding, store, queue := newTestOnboarding(enabledAutoTagSettings())
	ctx := context.Background()

	if _, err := onboarding.PickInterests(ctx, []string{"opera"}); !errors.Is(err, domain.ErrInvalidInterestPick) {
		t.Fatalf("unknown id error = %v", err)
	}
	store.full = true
	if _, err := onboarding.PickInterests(ctx, []string{"news"}); !errors.Is(err, domain.ErrHomeShelvesFull) {
		t.Fatalf("full home error = %v", err)
	}
	if len(store.picked) != 0 || len(queue.modes) != 0 {
		t.Fatalf("picked = %+v, queue modes = %v; want neither", store.picked, queue.modes)
	}
}

func TestSetStepWritesTheNamedStep(t *testing.T) {
	onboarding, store, _ := newTestOnboarding(enabledAutoTagSettings())
	ctx := context.Background()

	if step, err := onboarding.SetStep(ctx, domain.OnboardingInterests); err != nil || step != domain.OnboardingInterests {
		t.Fatalf("SetStep = %q, %v", step, err)
	}
	if _, err := onboarding.SetStep(ctx, "later"); !errors.Is(err, ErrInvalidOnboardingStep) {
		t.Fatalf("unknown step error = %v", err)
	}
	if step, err := onboarding.Step(ctx); err != nil || step != domain.OnboardingInterests || store.step != step {
		t.Fatalf("Step = %q, %v", step, err)
	}
}
