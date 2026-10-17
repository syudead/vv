package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/syudead/vv/internal/domain"
)

// OnboardingStore はオンボードの段階と分野の保存先である。internal/store の *HomeStore が満たす
// （docs/design-docs/home.md）。
type OnboardingStore interface {
	// OnboardingStep は段階を返す。無ければメディアフォルダの有無で決まる既定である。
	OnboardingStep(ctx context.Context) (domain.OnboardingStep, error)
	// SaveOnboardingStep は段階を step にする。
	SaveOnboardingStep(ctx context.Context, step domain.OnboardingStep) error
	// Interests は分野の一覧を、棚がもうホームにあるかを添えて返す。
	Interests(ctx context.Context) ([]domain.InterestStatus, error)
	// PickInterests は分野を 1 つの取引でタグと所有者の棚にし、足した棚を返す。ホームが
	// domain.MaxHomeShelves を超えるなら domain.ErrHomeShelvesFull。
	PickInterests(ctx context.Context, interests []domain.Interest) ([]domain.SmartFolder, error)
}

// OnboardingAutoTagging は分野を選んだあとに動画を自動タグ付けに回す口である。*AutoTagger が満たす。
type OnboardingAutoTagging interface {
	// QueueLibrary は動画を scope に従って判定に回し、積んだ件数を返す。機能が切なら
	// domain.ErrAutoTaggingOff。
	QueueLibrary(ctx context.Context, scope domain.AutoTagScope) (int, error)
}

// Onboarding はオンボードの段階と分野の選び方である（docs/design-docs/home.md〜R-6）。
type Onboarding struct {
	store      OnboardingStore
	autoTagger OnboardingAutoTagging
}

// NewOnboarding はオンボードを組み立てる。autoTagger が nil なら、分野を選んでも動画を積まない。
func NewOnboarding(store OnboardingStore, autoTagger OnboardingAutoTagging) *Onboarding {
	return &Onboarding{store: store, autoTagger: autoTagger}
}

// Step は段階を返す。
func (o *Onboarding) Step(ctx context.Context) (domain.OnboardingStep, error) {
	return o.store.OnboardingStep(ctx)
}

// ErrInvalidOnboardingStep は知らない段階を書こうとしたことを表す。API では 400 invalid_request である。
var ErrInvalidOnboardingStep = errors.New("invalid onboarding step")

// SetStep は段階を step にして、書いた段階を返す。名前の段階をそのまま書くので、2 つの端末からの
// 書き込みは後のものが残る（research.md R-3）。知らない段階なら ErrInvalidOnboardingStep。
func (o *Onboarding) SetStep(ctx context.Context, step domain.OnboardingStep) (domain.OnboardingStep, error) {
	if !step.Valid() {
		return "", fmt.Errorf("%w: %q", ErrInvalidOnboardingStep, step)
	}
	if err := o.store.SaveOnboardingStep(ctx, step); err != nil {
		return "", err
	}
	return step, nil
}

// Interests は分野の一覧を一覧の順に返す。
func (o *Onboarding) Interests(ctx context.Context) ([]domain.InterestStatus, error) {
	return o.store.Interests(ctx)
}

// PickInterests は id の分野をタグと所有者の棚にし（research.md R-6）、自動タグ付けが入なら
// 「Tag all videos again」と同じくすべての動画を判定に回す。id が規則に合わなければ
// domain.ErrInvalidInterestPick、棚が上限を超えるなら domain.ErrHomeShelvesFull で、どちらも
// 何も書かない。自動タグ付けが切なら Queued は 0 である。
func (o *Onboarding) PickInterests(ctx context.Context, ids []string) (domain.InterestPickResult, error) {
	interests, err := domain.PickInterests(ids)
	if err != nil {
		return domain.InterestPickResult{}, err
	}
	shelves, err := o.store.PickInterests(ctx, interests)
	if err != nil {
		return domain.InterestPickResult{}, err
	}
	result := domain.InterestPickResult{Shelves: shelves}
	if o.autoTagger == nil {
		return result, nil
	}
	queued, err := o.autoTagger.QueueLibrary(ctx, domain.AutoTagScopeAll)
	if errors.Is(err, domain.ErrAutoTaggingOff) {
		return result, nil
	}
	if err != nil {
		return domain.InterestPickResult{}, fmt.Errorf("the shelves were made, but the videos could not be queued for auto-tagging: %w", err)
	}
	result.Queued = queued
	return result, nil
}
