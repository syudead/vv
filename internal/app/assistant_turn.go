package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/syudead/vv/internal/domain"
)

// AssistantVideos reads the library as the screens do, so every number the assistant shows is the screens' number.
type AssistantVideos interface {
	ListVideos(ctx context.Context, audience domain.Audience, q domain.VideoQuery) (domain.VideoPage, error)
	VideosByIDs(ctx context.Context, audience domain.Audience, ids []int64) ([]domain.Video, error)
}

// AssistantSmartFolders expands smart folders to their member videos, as the bulk operations do.
type AssistantSmartFolders interface {
	SmartFolderVideoIDs(ctx context.Context, ids []int64) ([]int64, error)
}

// AssistantHistory reads the watch history, newest first.
type AssistantHistory interface {
	ListWatchHistory(
		ctx context.Context, audience domain.Audience, query domain.WatchHistoryQuery, cursor string, limit int,
	) (domain.WatchHistoryPage, error)
}

// assistantTurn is the state of one turn's interpretation.
type assistantTurn struct {
	a             *Assistant
	input         domain.AssistantTurnInput
	clarification *domain.AssistantClarification
	created       []string
}

// Turn interprets one request with one generator call and answers one outcome; an action stores a proposal. A
// follow-up discards the previous proposal once the new request is understood. Generator failures other than an
// unreadable answer are returned, and nothing is stored.
func (a *Assistant) Turn(ctx context.Context, in domain.AssistantTurnInput) (domain.AssistantTurnResult, error) {
	if a.generator == nil || a.vocabulary == nil || a.videos == nil {
		return domain.AssistantTurnResult{}, errors.New("the assistant cannot interpret requests: it is not configured")
	}
	t := &assistantTurn{a: a, input: in}
	schema, err := domain.AssistantOutcomeSchema()
	if err != nil {
		return domain.AssistantTurnResult{}, err
	}
	prompt, err := t.prompt(ctx)
	if err != nil {
		return domain.AssistantTurnResult{}, err
	}
	raw, err := a.generator.Generate(ctx, domain.GenerationRequest{Prompt: prompt, Schema: schema})
	if errors.Is(err, domain.ErrGenerationUnreadable) {
		return domain.AssistantTurnResult{Kind: domain.AssistantTurnUnreadable}, nil
	}
	if err != nil {
		return domain.AssistantTurnResult{}, err
	}
	outcome, err := domain.ParseAssistantOutcome(raw)
	if err != nil {
		a.logger.Info("the model's answer does not match the outcome schema", slog.Any("error", err))
		return domain.AssistantTurnResult{Kind: domain.AssistantTurnUnreadable}, nil
	}
	result, err := t.interpret(ctx, outcome)
	if err != nil {
		return domain.AssistantTurnResult{}, err
	}
	if in.PreviousProposalID > 0 {
		a.discard(ctx, in.PreviousProposalID)
	}
	return result, nil
}

// discard replaces the previous proposal; one that was approved meanwhile or already expired is left as it is.
func (a *Assistant) discard(ctx context.Context, id int64) {
	err := a.store.DiscardProposal(ctx, id, a.now())
	if err != nil && !errors.Is(err, domain.ErrNotFound) && !errors.Is(err, domain.ErrAssistantProposalNotPending) {
		a.logger.Warn("cannot discard the previous assistant proposal", slog.Int64("proposal", id), slog.Any("error", err))
	}
}

func (t *assistantTurn) interpret(ctx context.Context, o domain.AssistantOutcome) (domain.AssistantTurnResult, error) {
	var result domain.AssistantTurnResult
	var err error
	switch o.Kind {
	case domain.AssistantOutcomeFilter:
		result, err = t.filterOutcome(ctx, o.Filter)
	case domain.AssistantOutcomeNavigate:
		result, err = t.navigate(ctx, o.Navigate)
	case domain.AssistantOutcomeAnswer:
		result, err = t.answer(ctx, o.Answer)
	case domain.AssistantOutcomeAction:
		result, err = t.action(ctx, o.Action)
	case domain.AssistantOutcomeOrganize:
		return t.organize(ctx, o.Organize)
	case domain.AssistantOutcomeClarify:
		return domain.AssistantTurnResult{
			Kind: domain.AssistantTurnClarify, Clarify: &domain.AssistantClarification{About: o.Clarify.About},
		}, nil
	default:
		return domain.AssistantTurnResult{Kind: domain.AssistantTurnUnsupported}, nil
	}
	if err != nil {
		return domain.AssistantTurnResult{}, err
	}
	if t.clarification == nil && result.Kind == "" {
		t.clarify("value", nil)
	}
	if t.clarification != nil {
		return domain.AssistantTurnResult{Kind: domain.AssistantTurnClarify, Clarify: t.clarification}, nil
	}
	return result, nil
}

func (t *assistantTurn) filterOutcome(ctx context.Context, f *domain.AssistantFilter) (domain.AssistantTurnResult, error) {
	criteria, ok, err := t.criteria(ctx, f)
	if !ok || err != nil {
		return domain.AssistantTurnResult{}, err
	}
	return domain.AssistantTurnResult{Kind: domain.AssistantTurnFilter, Criteria: &criteria}, nil
}

// criteria resolves a filter's names to ids; a condition out of order or a name that is not one entity asks.
func (t *assistantTurn) criteria(ctx context.Context, f *domain.AssistantFilter) (domain.AssistantCriteria, bool, error) {
	var c domain.AssistantCriteria
	if f == nil {
		return c, true, nil
	}
	c.Rule = domain.SmartRule{Q: f.Q, Favorite: f.Favorite, VideoConditions: domain.VideoConditions{
		MinDurationMs: f.MinDurationMs, MaxDurationMs: f.MaxDurationMs,
		AddedFrom: domain.CalendarDay(f.AddedFrom), AddedTo: domain.CalendarDay(f.AddedTo),
		WatchState: domain.WatchState(f.WatchState),
	}}
	c.Sort = domain.VideoSort(f.Sort)
	if err := c.Rule.Validate(); err != nil {
		t.clarify("value", nil)
		return c, false, nil
	}
	tags, ok, err := t.each(ctx, domain.AssistantLabelTag, f.Tags, valueName)
	if !ok || err != nil {
		return c, false, err
	}
	c.Rule.Tags, c.Labels = labelIDs(tags), tags
	for _, ref := range []struct {
		kind domain.AssistantLabelKind
		name string
		id   *int64
	}{
		{domain.AssistantLabelTagKind, f.TagKind, &c.Rule.TagKind},
		{domain.AssistantLabelPerson, f.Person, &c.Rule.Person},
		{domain.AssistantLabelOrganization, f.Organization, &c.Rule.Organization},
	} {
		if ref.name == "" {
			continue
		}
		label, ok, err := t.one(ctx, ref.kind, ref.name, valueName)
		if !ok || err != nil {
			return c, false, err
		}
		*ref.id = label.ID
		c.Labels = append(c.Labels, label)
	}
	return c, true, nil
}

// count runs the filter as the library does and answers its total.
func (t *assistantTurn) count(ctx context.Context, q domain.VideoQuery) (int, error) {
	q.Limit = 1
	page, err := t.a.videos.ListVideos(ctx, domain.AudienceOwner, q)
	if err != nil {
		return 0, fmt.Errorf("cannot count the videos: %w", err)
	}
	return page.Total, nil
}
