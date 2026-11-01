package app

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/syudead/vv/internal/domain"
)

// toolVideoStats counts and measures videos in one call, so the model never pages through a library to count it.
const toolVideoStats = "video_stats"

const (
	// statsPage is the page the tool reads, the largest listVideos takes.
	statsPage = 200
	// statsTopDefault and statsTopMax bound the groups the tool answers.
	statsTopDefault = 20
	statsTopMax     = 100
)

var statsGroups = []any{"tag", "person", "organization"}

// statsSchema takes the library filter by names, without its order, and the scope and grouping.
func statsSchema() *jsonschema.Schema {
	schema := filterSchema()
	delete(schema.Properties, "sort")
	schema.Properties["scope"] = oneOf("screen: the videos on the owner's screen (the selection, else the screen's "+
		"filter or page), narrowed by the other conditions; library (default): the whole library.", []any{"screen", "library"})
	schema.Properties["groupBy"] = oneOf("Also count the videos per tag, performer or organization, most first.", statsGroups)
	schema.Properties["top"] = &jsonschema.Schema{Type: "integer", Minimum: jsonschema.Ptr(1.0),
		Maximum: jsonschema.Ptr(float64(statsTopMax)), Description: "How many groups to answer; 20 by default."}
	return schema
}

// statsArgs are the tool's arguments: the library filter by names, the scope and the grouping.
type statsArgs struct {
	domain.AssistantFilter
	Scope   string `json:"scope"`
	GroupBy string `json:"groupBy"`
	Top     int    `json:"top"`
}

// videoRef names one video in the tool's answer.
type videoRef struct {
	ID         int64  `json:"id"`
	Title      string `json:"title"`
	DurationMs int64  `json:"durationMs"`
}

type statsGroup struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// videoStats is the tool's answer; durations count only the videos whose length is known.
type videoStats struct {
	Count           int          `json:"count"`
	Favorites       int          `json:"favorites"`
	WithDuration    int          `json:"withDuration"`
	TotalDurationMs int64        `json:"totalDurationMs"`
	Longest         *videoRef    `json:"longest,omitempty"`
	Shortest        *videoRef    `json:"shortest,omitempty"`
	GroupCount      *int         `json:"groupCount,omitempty"`
	Groups          []statsGroup `json:"groups,omitempty"`
	counts          map[string]int
}

func (s *videoStats) add(v videoRef, favorite, hasDuration bool, groups []string) {
	s.Count++
	if favorite {
		s.Favorites++
	}
	if hasDuration {
		s.WithDuration++
		s.TotalDurationMs += v.DurationMs
		if s.Longest == nil || v.DurationMs > s.Longest.DurationMs {
			s.Longest = &v
		}
		if s.Shortest == nil || v.DurationMs < s.Shortest.DurationMs {
			s.Shortest = &v
		}
	}
	for _, g := range groups {
		s.counts[g]++
	}
}

// finish keeps the top groups, most first, ties by name.
func (s *videoStats) finish(groupBy string, top int) {
	if groupBy == "" {
		return
	}
	names := slices.Collect(maps.Keys(s.counts))
	slices.SortFunc(names, func(a, b string) int { return cmp.Or(s.counts[b]-s.counts[a], strings.Compare(a, b)) })
	n := len(names)
	s.GroupCount = &n
	s.Groups = []statsGroup{}
	for _, name := range names[:min(len(names), top)] {
		s.Groups = append(s.Groups, statsGroup{Name: name, Count: s.counts[name]})
	}
}

func (l *turnLoop) videoStats(ctx context.Context, raw json.RawMessage) (builtinOutcome, error) {
	var args statsArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return builtinOutcome{failure: err.Error()}, nil
	}
	top := statsTopDefault
	if args.Top > 0 {
		top = min(args.Top, statsTopMax)
	}
	stats := videoStats{counts: map[string]int{}}
	screen := args.Scope == "screen"
	if ids := l.turn.Context.SelectedVideoIDs; screen && len(ids) > 0 {
		failure, err := l.selectionStats(ctx, &stats, ids, args)
		if failure != "" || err != nil {
			return builtinOutcome{failure: failure}, err
		}
	} else {
		t := l.resolver()
		criteria, ok, err := t.criteria(ctx, &args.AssistantFilter, false)
		if err != nil {
			return builtinOutcome{}, err
		}
		if !ok {
			return builtinOutcome{failure: t.clarifyFailure()}, nil
		}
		rule := criteria.Rule
		if screen {
			rule = mergeRules(l.screenRule(), rule)
		}
		if failure, err := l.pageStats(ctx, &stats, rule, args.GroupBy); failure != "" || err != nil {
			return builtinOutcome{failure: failure}, err
		}
	}
	stats.finish(args.GroupBy, top)
	return builtinOutcome{content: encodeResult(stats)}, nil
}

// selectionStats measures the selected videos; their tags and credits are not read, and the filter does not apply.
func (l *turnLoop) selectionStats(ctx context.Context, s *videoStats, ids []int64, args statsArgs) (string, error) {
	if args.GroupBy != "" || !reflect.DeepEqual(args.AssistantFilter, domain.AssistantFilter{}) {
		return "for the selected videos, video_stats takes neither conditions nor groupBy; leave them out", nil
	}
	videos, failure, err := l.a.videosByIDs(ctx, l.runner, ids)
	if failure != "" || err != nil {
		return failure, err
	}
	for _, v := range videos {
		ref := videoRef{ID: v.ID, Title: v.Title}
		if v.DurationMs != nil {
			ref.DurationMs = *v.DurationMs
		}
		s.add(ref, v.Favorite, v.DurationMs != nil, nil)
	}
	return "", nil
}

// statsItem is what the tool reads of a listVideos item.
type statsItem struct {
	ID          int64      `json:"id"`
	Title       string     `json:"title"`
	DurationMs  *int64     `json:"durationMs"`
	Favorite    bool       `json:"favorite"`
	Tags        []namedRef `json:"tags"`
	CardCredits struct {
		Performers   []namedRef `json:"performers"`
		Organization *namedRef  `json:"organization"`
	} `json:"cardCredits"`
}

type namedRef struct {
	Name string `json:"name"`
}

func (i statsItem) groups(groupBy string) []string {
	var refs []namedRef
	switch groupBy {
	case "tag":
		refs = i.Tags
	case "person":
		refs = i.CardCredits.Performers
	case "organization":
		if i.CardCredits.Organization != nil {
			refs = []namedRef{*i.CardCredits.Organization}
		}
	}
	names := make([]string, len(refs))
	for k, ref := range refs {
		names[k] = ref.Name
	}
	return names
}

// pageStats reads every page of listVideos for the rule through the screen's own handler, which also lists the
// paired node's videos, and measures them; a failed read is the tool's failure.
func (l *turnLoop) pageStats(ctx context.Context, s *videoStats, rule domain.SmartRule, groupBy string) (string, error) {
	args := ruleArgs(rule)
	args["limit"] = statsPage
	for {
		encoded, _ := json.Marshal(args) // Plain values always encode.
		result, err := l.runner.Read(ctx, domain.AssistantRead{Operation: "listVideos", Arguments: encoded})
		if err != nil {
			return fmt.Sprintf("listVideos with %s: %s", trimArguments(encoded), readFailure(err)), nil
		}
		var page struct {
			Items      []statsItem `json:"items"`
			NextCursor string      `json:"nextCursor"`
		}
		if err := json.Unmarshal(result.Body, &page); err != nil {
			return "", fmt.Errorf("cannot read a page of videos: %w", err)
		}
		for _, item := range page.Items {
			ref := videoRef{ID: item.ID, Title: item.Title}
			if item.DurationMs != nil {
				ref.DurationMs = *item.DurationMs
			}
			s.add(ref, item.Favorite, item.DurationMs != nil, item.groups(groupBy))
		}
		if page.NextCursor == "" || ctx.Err() != nil {
			return "", ctx.Err()
		}
		args["cursor"] = page.NextCursor
	}
}

// ruleArgs are listVideos' query arguments for the rule.
func ruleArgs(r domain.SmartRule) map[string]any {
	args := map[string]any{}
	set := func(name string, value any, ok bool) {
		if ok {
			args[name] = value
		}
	}
	set("query", r.Q, r.Q != "")
	set("favorite", true, r.Favorite)
	set("tag", r.Tags, len(r.Tags) > 0)
	set("tagKind", r.TagKind, r.TagKind != 0)
	set("person", r.Person, r.Person != 0)
	set("organization", r.Organization, r.Organization != 0)
	set("minDurationMs", r.MinDurationMs, r.MinDurationMs != nil)
	set("maxDurationMs", r.MaxDurationMs, r.MaxDurationMs != nil)
	set("addedFrom", r.AddedFrom, r.AddedFrom != "")
	set("addedTo", r.AddedTo, r.AddedTo != "")
	set("watchState", r.WatchState, r.WatchState != "")
	return args
}

// screenRule is the screen's filter, narrowed to the tag, person or organization the page shows.
func (l *turnLoop) screenRule() domain.SmartRule {
	c := l.turn.Context
	var rule domain.SmartRule
	if c.Filter != nil {
		rule = *c.Filter
		rule.Tags = slices.Clone(rule.Tags)
	}
	switch routeKinds[c.Route] {
	case domain.AssistantLabelTag:
		if !slices.Contains(rule.Tags, c.RouteID) {
			rule.Tags = append(rule.Tags, c.RouteID)
		}
	case domain.AssistantLabelPerson:
		rule.Person = c.RouteID
	case domain.AssistantLabelOrganization:
		rule.Organization = c.RouteID
	}
	return rule
}

// mergeRules narrows the screen's rule with the call's conditions; a value the call gives replaces the screen's.
func mergeRules(screen, call domain.SmartRule) domain.SmartRule {
	out := screen
	out.Q = strings.TrimSpace(screen.Q + " " + call.Q)
	out.Favorite = screen.Favorite || call.Favorite
	for _, id := range call.Tags {
		if !slices.Contains(out.Tags, id) {
			out.Tags = append(out.Tags, id)
		}
	}
	out.TagKind = cmp.Or(call.TagKind, screen.TagKind)
	out.Person = cmp.Or(call.Person, screen.Person)
	out.Organization = cmp.Or(call.Organization, screen.Organization)
	c := call.VideoConditions
	if c.MinDurationMs != nil {
		out.MinDurationMs = c.MinDurationMs
	}
	if c.MaxDurationMs != nil {
		out.MaxDurationMs = c.MaxDurationMs
	}
	out.AddedFrom = cmp.Or(c.AddedFrom, screen.AddedFrom)
	out.AddedTo = cmp.Or(c.AddedTo, screen.AddedTo)
	out.WatchState = cmp.Or(c.WatchState, screen.WatchState)
	return out
}
