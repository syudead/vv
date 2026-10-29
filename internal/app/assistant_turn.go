package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"unicode"

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

// AssistantGroups reads the member videos of groups, so an operation that takes a selected group does not also
// receive its members.
type AssistantGroups interface {
	GroupVideoIDs(ctx context.Context, ids []int64) ([]int64, error)
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
	criteria, ok, err := t.criteria(ctx, f, false)
	if !ok || err != nil {
		return domain.AssistantTurnResult{}, err
	}
	return domain.AssistantTurnResult{Kind: domain.AssistantTurnFilter, Criteria: &criteria}, nil
}

// criteria resolves a filter's names to ids; a condition out of order or a name that is not one entity asks. A filter
// that only reads (strict false) instead searches a tag, person or organization name that matches nothing as free
// text, or drops it when it is a field word such as "tags", so one loose name does not fail the request; a filter
// that picks what a change acts on stays strict (docs/design-docs/assistant.md).
func (t *assistantTurn) criteria(
	ctx context.Context, f *domain.AssistantFilter, strict bool,
) (domain.AssistantCriteria, bool, error) {
	var c domain.AssistantCriteria
	if f == nil {
		return c, true, nil
	}
	c.Rule = domain.SmartRule{Q: f.Q, Favorite: f.Favorite, VideoConditions: domain.VideoConditions{
		MinDurationMs: positiveDuration(f.MinDurationMs), MaxDurationMs: positiveDuration(f.MaxDurationMs),
		AddedFrom: calendarDay(f.AddedFrom, strict), AddedTo: calendarDay(f.AddedTo, strict),
		WatchState: domain.WatchState(f.WatchState),
	}}
	c.Sort = domain.VideoSort(f.Sort)
	use := searchedName
	if strict {
		use = valueName
	}
	tags, ok, err := t.each(ctx, domain.AssistantLabelTag, f.Tags, use)
	if !ok || err != nil {
		return c, false, err
	}
	c.Labels = []domain.AssistantLabel{}
	for _, tag := range tags {
		if c.Rule.Q, ok = searchUnresolved(c.Rule.Q, tag); ok {
			c.Rule.Tags = append(c.Rule.Tags, tag.ID)
			c.Labels = append(c.Labels, tag)
		}
	}
	for _, ref := range []struct {
		kind domain.AssistantLabelKind
		name string
		id   *int64
		use  nameUse
	}{
		{domain.AssistantLabelTagKind, f.TagKind, &c.Rule.TagKind, valueName},
		{domain.AssistantLabelPerson, f.Person, &c.Rule.Person, use},
		{domain.AssistantLabelOrganization, f.Organization, &c.Rule.Organization, use},
	} {
		if ref.name == "" {
			continue
		}
		label, ok, err := t.one(ctx, ref.kind, ref.name, ref.use)
		if !ok || err != nil {
			return c, false, err
		}
		if c.Rule.Q, ok = searchUnresolved(c.Rule.Q, label); ok {
			*ref.id = label.ID
			c.Labels = append(c.Labels, label)
		}
	}
	if err := c.Rule.Validate(); err != nil {
		t.clarify("value", nil)
		return c, false, nil
	}
	return c, true, nil
}

// calendarDay drops, from a filter that only reads, a day that is not a date, such as the "0000-00-00" a model
// writes for a day the request does not set; a change's target keeps it, so the validation asks instead of
// widening the target.
func calendarDay(day string, strict bool) domain.CalendarDay {
	parsed := domain.CalendarDay(day)
	if strict || (domain.VideoConditions{AddedFrom: parsed}).Validate() == nil {
		return parsed
	}
	return ""
}

// positiveDuration drops a bound of 0, which a model writes for a bound the request does not set.
func positiveDuration(ms *int64) *int64 {
	if ms == nil || *ms <= 0 {
		return nil
	}
	return ms
}

// filterFieldWords are words, folded, that a model copies from the schema instead of a name.
var filterFieldWords = foldedSet(
	"tag", "tags", "tag kind", "tagKind", "tagNames", "person", "people", "persons", "organization",
	"organizations", "name", "names", "q", "filter", "video", "videos",
)

func foldedSet(words ...string) map[string]bool {
	set := make(map[string]bool, len(words))
	for _, word := range words {
		set[domain.FoldForMatch(word)] = true
	}
	return set
}

// searchUnresolved reports whether the label names an entity. A label without an id instead joins q as a search
// term, quoted when it holds spaces, unless it is a field word or q already holds it.
func searchUnresolved(q string, label domain.AssistantLabel) (string, bool) {
	if label.ID != 0 {
		return q, true
	}
	term := strings.TrimSpace(strings.ReplaceAll(label.Name, `"`, ""))
	key := domain.FoldForMatch(term)
	if key == "" || filterFieldWords[key] || strings.Contains(domain.FoldForMatch(q), key) {
		return q, false
	}
	if strings.ContainsFunc(term, unicode.IsSpace) {
		term = `"` + term + `"`
	}
	return strings.TrimSpace(q + " " + term), false
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
