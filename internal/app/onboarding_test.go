package app

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

type fakeOnboardingStore struct {
	step   domain.OnboardingStep
	picked [][]domain.Interest
	err    error
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
	if s.err != nil {
		return nil, s.err
	}
	s.picked = append(s.picked, interests)
	folders := make([]domain.SmartFolder, len(interests))
	for i, interest := range interests {
		folders[i] = domain.SmartFolder{ID: int64(i + 1), Name: interest.Name}
	}
	return folders, nil
}

func newTestOnboarding(settings *fakeAutoTagSettings) (*Onboarding, *fakeOnboardingStore, *fakeAutoTagQueue) {
	store := &fakeOnboardingStore{step: domain.OnboardingFolders}
	queue := newFakeAutoTagQueue()
	tagger := NewAutoTagger(AutoTaggerOptions{Queue: queue, Settings: settings, Classifier: &fakeClassifier{}})
	return NewOnboarding(store, tagger), store, queue
}

func TestPickInterestsQueuesEveryVideoWhenAutoTaggingIsOn(t *testing.T) {
	onboarding, store, queue := newTestOnboarding(enabledAutoTagSettings())

	result, err := onboarding.PickInterests(context.Background(), []string{"anime", "music"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Folders) != 2 || result.Queued != 3 {
		t.Fatalf("result = %+v, want 2 folders and the 3 queued videos", result)
	}
	if !slices.Equal(queue.modes, []domain.AutoTagQueueMode{domain.AutoTagScopeAll.QueueMode()}) {
		t.Fatalf("queue modes = %v, want one queue of every video", queue.modes)
	}
	if len(store.picked) != 1 || store.picked[0][0].ID != "anime" || store.picked[0][1].ID != "music" {
		t.Fatalf("picked = %+v", store.picked)
	}
}

func TestPickInterestsMakesFoldersWithoutQueuingWhenAutoTaggingIsOff(t *testing.T) {
	onboarding, _, queue := newTestOnboarding(&fakeAutoTagSettings{settings: domain.DefaultAutoTagSettings()})

	result, err := onboarding.PickInterests(context.Background(), []string{"travel"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Folders) != 1 || result.Queued != 0 || len(queue.modes) != 0 {
		t.Fatalf("result = %+v, queue modes = %v; want 1 folder and nothing queued", result, queue.modes)
	}
}

func TestPickInterestsRefusesBadPicksBeforeQueuing(t *testing.T) {
	onboarding, store, queue := newTestOnboarding(enabledAutoTagSettings())
	ctx := context.Background()

	if _, err := onboarding.PickInterests(ctx, []string{"opera"}); !errors.Is(err, domain.ErrInvalidInterestPick) {
		t.Fatalf("unknown id error = %v", err)
	}
	store.err = errors.New("store failure")
	if _, err := onboarding.PickInterests(ctx, []string{"news"}); !errors.Is(err, store.err) {
		t.Fatalf("store error = %v", err)
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
