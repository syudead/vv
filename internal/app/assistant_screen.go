package app

import (
	"cmp"
	"context"
	"encoding/json"
	"slices"
	"strings"

	"github.com/syudead/vv/internal/domain"
)

type screenEntity struct {
	Kind domain.AssistantLabelKind `json:"kind,omitempty"`
	ID   int64                     `json:"id"`
	Name string                    `json:"name,omitempty"`
}

type screenResult struct {
	Route                    domain.AssistantRouteKind `json:"route"`
	Showing                  *screenEntity             `json:"showing,omitempty"`
	SelectedVideoCount       int                       `json:"selectedVideoCount"`
	SelectedVideos           []screenEntity            `json:"selectedVideos,omitempty"`
	SelectedGroupCount       int                       `json:"selectedGroupCount"`
	SelectedGroups           []screenEntity            `json:"selectedGroups,omitempty"`
	SelectedSmartFolderCount int                       `json:"selectedSmartFolderCount"`
	SelectedSmartFolders     []screenEntity            `json:"selectedSmartFolders,omitempty"`
	Filter                   *criteriaResult           `json:"filter,omitempty"`
}

// screenContext answers the screen with ids and names; the system message carries only names and counts. Each
// selection names at most domain.AssistantResultItemMax items, and its count covers the rest.
func (l *turnLoop) screenContext(ctx context.Context, _ json.RawMessage) (builtinOutcome, error) {
	c := l.turn.Context
	result := screenResult{
		Route: c.Route, SelectedVideoCount: len(c.SelectedVideoIDs), SelectedGroupCount: len(c.SelectedGroupIDs),
		SelectedSmartFolderCount: len(c.SelectedSmartFolderIDs),
	}
	if c.RouteID > 0 {
		result.Showing = &screenEntity{Kind: routeKinds[c.Route], ID: c.RouteID}
		if result.Showing.Kind != "" {
			name, err := l.entityName(ctx, result.Showing.Kind, c.RouteID)
			if err != nil {
				return builtinOutcome{}, err
			}
			result.Showing.Name = name
		}
	}
	var err error
	if result.SelectedVideos, err = l.selectedVideos(ctx, listed(c.SelectedVideoIDs)); err != nil {
		return builtinOutcome{}, err
	}
	if result.SelectedGroups, err = l.namedEntities(ctx, domain.AssistantLabelGroup, listed(c.SelectedGroupIDs)); err != nil {
		return builtinOutcome{}, err
	}
	smartFolders := listed(c.SelectedSmartFolderIDs)
	if result.SelectedSmartFolders, err = l.namedEntities(ctx, domain.AssistantLabelSmartFolder, smartFolders); err != nil {
		return builtinOutcome{}, err
	}
	if c.Filter != nil {
		labels, err := l.ruleLabels(ctx, *c.Filter)
		if err != nil {
			return builtinOutcome{}, err
		}
		result.Filter = &criteriaResult{Rule: *c.Filter, Sort: c.Sort, Labels: labels}
	}
	return builtinOutcome{content: encodeResult(result)}, nil
}

// listed keeps the first ids of a selection, the ones screen_context names.
func listed(ids []int64) []int64 {
	return ids[:min(len(ids), domain.AssistantResultItemMax)]
}

func (l *turnLoop) selectedVideos(ctx context.Context, ids []int64) ([]screenEntity, error) {
	if len(ids) == 0 || l.a.videos == nil {
		return nil, nil
	}
	videos, err := l.a.videos.VideosByIDs(ctx, domain.AudienceOwner, ids)
	if err != nil {
		return nil, err
	}
	titles := make(map[int64]string, len(videos))
	for _, v := range videos {
		titles[v.ID] = v.Title
	}
	out := make([]screenEntity, 0, len(ids))
	for _, id := range ids {
		if title, ok := titles[id]; ok {
			out = append(out, screenEntity{ID: id, Name: title})
		}
	}
	return out, nil
}

func (l *turnLoop) namedEntities(ctx context.Context, kind domain.AssistantLabelKind, ids []int64) ([]screenEntity, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	names, err := l.a.vocabulary.EntityNames(ctx, kind, ids)
	if err != nil {
		return nil, err
	}
	out := make([]screenEntity, 0, len(ids))
	for _, id := range ids {
		if name, ok := names[id]; ok {
			out = append(out, screenEntity{ID: id, Name: name})
		}
	}
	return out, nil
}

// ruleLabels names the tags, tag kind, person and organization a filter holds by id.
func (l *turnLoop) ruleLabels(ctx context.Context, rule domain.SmartRule) ([]domain.AssistantLabel, error) {
	labels := []domain.AssistantLabel{}
	for _, ref := range []struct {
		kind domain.AssistantLabelKind
		ids  []int64
	}{
		{domain.AssistantLabelTag, rule.Tags},
		{domain.AssistantLabelTagKind, nonZero(rule.TagKind)},
		{domain.AssistantLabelPerson, nonZero(rule.Person)},
		{domain.AssistantLabelOrganization, nonZero(rule.Organization)},
	} {
		entities, err := l.namedEntities(ctx, ref.kind, ref.ids)
		if err != nil {
			return nil, err
		}
		for _, e := range entities {
			labels = append(labels, domain.AssistantLabel{Kind: ref.kind, ID: e.ID, Name: e.Name})
		}
	}
	return labels, nil
}

func nonZero(id int64) []int64 {
	if id == 0 {
		return nil
	}
	return []int64{id}
}

// Limits of one app_help answer.
const (
	helpPageLimit      = 5
	helpOperationLimit = 10
)

type helpPage struct {
	Route   string `json:"route"`
	Title   string `json:"title"`
	Purpose string `json:"purpose,omitempty"`
}

type helpOperation struct {
	Name    string                   `json:"name"`
	Kind    domain.AssistantToolKind `json:"kind"`
	Summary string                   `json:"summary"`
	Reason  string                   `json:"reason,omitempty"`
	Page    *helpPage                `json:"page,omitempty"`
}

type helpResult struct {
	Pages      []helpPage      `json:"pages"`
	Operations []helpOperation `json:"operations"`
	// Note and AllPages answer a query that matched nothing, so the model can try the page titles.
	Note     string     `json:"note,omitempty"`
	AllPages []helpPage `json:"allPages,omitempty"`
}

// appHelp searches the page table and the operations' summaries by the words of the query. An excluded operation
// carries its reason and the page that does it.
func (l *turnLoop) appHelp(_ context.Context, raw json.RawMessage) (builtinOutcome, error) {
	var args struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return builtinOutcome{failure: err.Error()}, nil
	}
	words := searchWords(args.Query)
	result := helpResult{Pages: []helpPage{}, Operations: []helpOperation{}}
	for _, match := range ranked(domain.AssistantPages(), words, func(p domain.AssistantPage) string {
		return p.Title + " " + p.Purpose
	}, helpPageLimit) {
		page := match.item
		result.Pages = append(result.Pages, helpPage{Route: page.Route, Title: page.Title, Purpose: page.Purpose})
	}
	for _, match := range ranked(l.tools.all, words, toolSearchText, helpOperationLimit) {
		tool := match.item
		op := helpOperation{Name: domain.AssistantModelToolName(tool.Name), Kind: tool.Kind, Summary: tool.Summary}
		if tool.Exclusion != nil {
			op.Reason = tool.Exclusion.Reason
			op.Page = &helpPage{Route: tool.Exclusion.Page, Title: domain.AssistantPageTitle(tool.Exclusion.Page)}
		}
		result.Operations = append(result.Operations, op)
	}
	if len(result.Pages) == 0 && len(result.Operations) == 0 {
		result.Note = "Nothing matched. Search again with English words, such as a page title below."
		for _, page := range domain.AssistantPages() {
			result.AllPages = append(result.AllPages, helpPage{Route: page.Route, Title: page.Title})
		}
	}
	return builtinOutcome{content: encodeResult(result)}, nil
}

func toolSearchText(tool domain.AssistantTool) string {
	return tool.Name + " " + tool.Summary + " " + tool.Description
}

// searchFillers are words of a how-to question that name no feature.
var searchFillers = []string{"how", "the", "and", "for", "with", "what", "can", "use", "using", "into", "from", "app"}

// searchWords folds the query into the words a search counts: words of fewer than three letters and fillers are
// dropped, and a plural ending in s also matches its singular.
func searchWords(query string) []string {
	var words []string
	for _, word := range strings.Fields(domain.FoldForMatch(query)) {
		if len([]rune(word)) < 3 || slices.Contains(searchFillers, word) {
			continue
		}
		if len(word) > 3 && strings.HasSuffix(word, "s") && !strings.HasSuffix(word, "ss") {
			word = strings.TrimSuffix(word, "s")
		}
		if !slices.Contains(words, word) {
			words = append(words, word)
		}
	}
	return words
}

// scored is an item a search found, with how many of the words it contains.
type scored[T any] struct {
	item  T
	score int
}

// ranked keeps the items whose folded text contains at least one word, most words first and in their original
// order on a tie, up to limit.
func ranked[T any](items []T, words []string, textOf func(T) string, limit int) []scored[T] {
	var matches []scored[T]
	for _, item := range items {
		folded := domain.FoldForMatch(textOf(item))
		score := 0
		for _, word := range words {
			if strings.Contains(folded, word) {
				score++
			}
		}
		if score > 0 {
			matches = append(matches, scored[T]{item, score})
		}
	}
	slices.SortStableFunc(matches, func(a, b scored[T]) int { return cmp.Compare(b.score, a.score) })
	return matches[:min(len(matches), limit)]
}
