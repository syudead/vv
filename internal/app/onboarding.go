package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/syudead/vv/internal/domain"
)

type OnboardingStore interface {
	OnboardingStep(ctx context.Context) (domain.OnboardingStep, error)

	SaveOnboardingStep(ctx context.Context, step domain.OnboardingStep) error

	Interests(ctx context.Context) ([]domain.InterestStatus, error)

	PickInterests(ctx context.Context, interests []domain.Interest) ([]domain.SmartFolder, error)
}

type OnboardingAutoTagging interface {
	QueueLibrary(ctx context.Context, scope domain.AutoTagScope) (int, error)
}

type Onboarding struct {
	store      OnboardingStore
	autoTagger OnboardingAutoTagging
}

func NewOnboarding(store OnboardingStore, autoTagger OnboardingAutoTagging) *Onboarding {
	return &Onboarding{store: store, autoTagger: autoTagger}
}

func (o *Onboarding) Step(ctx context.Context) (domain.OnboardingStep, error) {
	return o.store.OnboardingStep(ctx)
}

var ErrInvalidOnboardingStep = errors.New("invalid onboarding step")

func (o *Onboarding) SetStep(ctx context.Context, step domain.OnboardingStep) (domain.OnboardingStep, error) {
	if !step.Valid() {
		return "", fmt.Errorf("%w: %q", ErrInvalidOnboardingStep, step)
	}
	if err := o.store.SaveOnboardingStep(ctx, step); err != nil {
		return "", err
	}
	return step, nil
}

func (o *Onboarding) Interests(ctx context.Context) ([]domain.InterestStatus, error) {
	return o.store.Interests(ctx)
}

func (o *Onboarding) PickInterests(ctx context.Context, ids []string) (domain.InterestPickResult, error) {
	interests, err := domain.PickInterests(ids)
	if err != nil {
		return domain.InterestPickResult{}, err
	}
	folders, err := o.store.PickInterests(ctx, interests)
	if err != nil {
		return domain.InterestPickResult{}, err
	}
	result := domain.InterestPickResult{Folders: folders}
	if o.autoTagger == nil {
		return result, nil
	}
	queued, err := o.autoTagger.QueueLibrary(ctx, domain.AutoTagScopeAll)
	if errors.Is(err, domain.ErrAutoTaggingOff) {
		return result, nil
	}
	if err != nil {
		return domain.InterestPickResult{}, fmt.Errorf("the folders were made, but the videos could not be queued for auto-tagging: %w", err)
	}
	result.Queued = queued
	return result, nil
}
