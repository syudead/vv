package app

import (
	"context"
	"strings"

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

// assistantTurn resolves the names of one request against the library and records the first clarification.
type assistantTurn struct {
	a             *Assistant
	input         domain.AssistantTurnInput
	clarification *domain.AssistantClarification
	created       []string
	// clarifyName and clarifyKind are the name the first clarification asks about, when a name asked it.
	clarifyName string
	clarifyKind domain.AssistantLabelKind
	// failure says why a write call cannot become a step, for the model to fix the call.
	failure string
	// idPins are the ids a write call gave with its names, by folded name; each wins only among the name's matches.
	idPins map[string]int64
	// writes are the turn's earlier steps, whose created names later steps resolve to.
	writes *turnWrites
	// runner reads the peer's videos; without it a remote video id cannot be resolved.
	runner ActionRunner
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
		MinDurationMs: duration(f.MinDurationMs, strict), MaxDurationMs: duration(f.MaxDurationMs, strict),
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

// duration drops, from a filter that only reads, a bound of 0, which a model writes for a bound the request does
// not set; a change's target keeps it, since an explicit 0 is a bound the validation checks against the other.
func duration(ms *int64, strict bool) *int64 {
	if strict || ms == nil || *ms > 0 {
		return ms
	}
	return nil
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

// searchUnresolved reports whether the label names an entity. A label without an id instead joins q as a literal
// search term, unless it is a field word or q already requires it.
func searchUnresolved(q string, label domain.AssistantLabel) (string, bool) {
	if label.ID != 0 {
		return q, true
	}
	term := strings.TrimSpace(strings.Map(dropSearchQuote, label.Name))
	key := domain.FoldForMatch(term)
	if key == "" || filterFieldWords[key] || requiresSearchTerm(domain.ParseSearchQuery(q), key) {
		return q, false
	}
	return strings.TrimSpace(q + " " + literalSearchTerm(term, key)), false
}

// dropSearchQuote removes the quotes, ASCII and fullwidth, that would end a quoted phrase after NFKC.
func dropSearchQuote(r rune) rune {
	if r == '"' || r == '\uFF02' {
		return -1
	}
	return r
}

// requiresSearchTerm reports whether every match of the query must contain the positive term key.
func requiresSearchTerm(expr domain.SearchExpr, key string) bool {
	for _, clause := range expr.Clauses {
		if len(clause.Terms) == 1 && clause.Terms[0] == (domain.SearchTerm{Text: key}) {
			return true
		}
	}
	return false
}

// literalSearchTerm quotes the term unless it already reads as one plain word; OR stays quoted because a term
// appended after it would turn it into an operator.
func literalSearchTerm(term, key string) string {
	expr := domain.ParseSearchQuery(term)
	if term != "OR" && len(expr.Clauses) == 1 && requiresSearchTerm(expr, key) {
		return term
	}
	return `"` + term + `"`
}
