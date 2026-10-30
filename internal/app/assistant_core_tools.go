package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/syudead/vv/internal/domain"
)

// The built-in tools that are not screen operations; they are offered on every model call with the core reads.
const (
	toolScreenContext = "screen_context"
	toolAppHelp       = "app_help"
	toolShowInLibrary = "show_in_library"
	toolOpenPage      = "open_page"
	toolStartReview   = "start_review"
)

// builtinOutcome is what a built-in tool answers: the tool message, or why it failed, and what its progress item
// shows besides the tool.
type builtinOutcome struct {
	content     string
	failure     string
	labels      []domain.AssistantLabel
	reviewRunID int64
}

type builtinTool struct {
	definition domain.ToolDefinition
	schema     *jsonschema.Resolved
	run        func(l *turnLoop, ctx context.Context, args json.RawMessage) (builtinOutcome, error)
}

// navigationKinds are the destinations of open_page; resume opens the video watched last that is in progress.
var navigationKinds = []string{
	"video", "person", "organization", "playlist", "group", "smartFolder", "tag", "library", "folders", "tags",
	"people", "organizations", "playlists", "history", "duplicates", "activity", "settings", "resume",
}

var settingsPages = []string{
	"scan", "library", "auto-tagging", "face-recognition", "connections", "plugins", "conversion", "network",
	"pairing", "account", "api", "danger-zone",
}

var videoSorts = []domain.VideoSort{
	domain.SortAddedAsc, domain.SortAddedDesc, domain.SortModifiedAsc, domain.SortModifiedDesc,
	domain.SortCreatedAsc, domain.SortCreatedDesc, domain.SortTitleAsc, domain.SortTitleDesc,
	domain.SortDurationAsc, domain.SortDurationDesc, domain.SortSizeAsc, domain.SortSizeDesc,
	domain.SortPlayedAsc, domain.SortPlayedDesc, domain.SortFavoritedAsc, domain.SortFavoritedDesc, domain.SortRandom,
}

func enumOf[T ~string](values ...T) []any {
	out := make([]any, len(values))
	for i, v := range values {
		out[i] = string(v)
	}
	return out
}

func stringSchema(description string) *jsonschema.Schema {
	return &jsonschema.Schema{Type: "string", Description: description}
}

func oneOf(description string, values []any) *jsonschema.Schema {
	return &jsonschema.Schema{Type: "string", Description: description, Enum: values}
}

// object is a closed object schema, so a misspelt argument comes back to the model as an error.
func object(required []string, properties map[string]*jsonschema.Schema) *jsonschema.Schema {
	return &jsonschema.Schema{
		Type: "object", Properties: properties, Required: required, AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
	}
}

func filterSchema() *jsonschema.Schema {
	bound := func(description string) *jsonschema.Schema {
		return &jsonschema.Schema{Type: "integer", Minimum: jsonschema.Ptr(0.0), Description: description}
	}
	return object(nil, map[string]*jsonschema.Schema{
		"q":             stringSchema("Free text searched in titles, tags, people, organizations and work information."),
		"favorite":      {Type: "boolean", Description: "Only favorites."},
		"tags":          {Type: "array", Items: stringSchema("A tag name."), Description: "Tag names; every one must match."},
		"tagKind":       stringSchema("A tag kind name; videos with a tag of that kind."),
		"person":        stringSchema("A person's name."),
		"organization":  stringSchema("An organization's name."),
		"minDurationMs": bound("The shortest length in milliseconds; 30 minutes is 1800000."),
		"maxDurationMs": bound("The longest length in milliseconds."),
		"addedFrom":     stringSchema("The first day the video was added, YYYY-MM-DD."),
		"addedTo":       stringSchema("The last day the video was added, YYYY-MM-DD."),
		"watchState": oneOf("Videos never played, partly played, or played to the end.",
			enumOf(domain.WatchStateUnwatched, domain.WatchStateInProgress, domain.WatchStateWatched)),
		"sort": oneOf("The order of the library.", enumOf(videoSorts...)),
	})
}

var builtinTools = sync.OnceValue(func() map[string]*builtinTool {
	tools := []*builtinTool{
		{definition: domain.ToolDefinition{Name: toolScreenContext, Description: "Read what the owner sees: the " +
			"screen and the entity it shows, the selected videos (id and title), the selected groups and smart " +
			"folders, and the library filter. Call it when the request says this, these, here or the selected ones."},
			run: (*turnLoop).screenContext},
		{definition: domain.ToolDefinition{Name: toolAppHelp, Description: "Explain how to do something in the app. " +
			"Answers the app's pages (route, title, what it is for) and the operations that match, with the page " +
			"where the owner does what the assistant cannot do. Use it for how-to questions, then answer from it."},
			run: (*turnLoop).appHelp},
		{definition: domain.ToolDefinition{Name: toolShowInLibrary, Description: "Show videos in the library: when " +
			"the request ends, the owner's library opens with these conditions, without asking. Names are matched " +
			"by the server. Answers the conditions as resolved and how many videos match."},
			run: (*turnLoop).showInLibrary},
		{definition: domain.ToolDefinition{Name: toolOpenPage, Description: "Open a page of the app when the " +
			"request ends, without asking: an entity's page by its name, a list, a settings page, or resume for " +
			"the video watched last that is still in progress."},
			run: (*turnLoop).openPage},
		{definition: domain.ToolDefinition{Name: toolStartReview, Description: "Start a background review of the " +
			"whole library: tags finds duplicate or similar tags, people the same person under several entries, " +
			"works videos of the same work. The owner adopts or rejects its candidates on the review list. Use it " +
			"for library-wide tidying, not for a few named items."},
			run: (*turnLoop).startReview},
	}
	schemas := map[string]*jsonschema.Schema{
		toolScreenContext: object(nil, map[string]*jsonschema.Schema{}),
		toolAppHelp: object([]string{"query"}, map[string]*jsonschema.Schema{
			"query": stringSchema(`English words naming the feature, such as "favorite" or "smart folder".`),
		}),
		toolShowInLibrary: filterSchema(),
		toolOpenPage: object([]string{"to"}, map[string]*jsonschema.Schema{
			"to":           oneOf("The page to open.", enumOf(navigationKinds...)),
			"name":         stringSchema("The entity's name or the video's title; leave it out for the one on screen."),
			"settingsPage": oneOf("For to settings, the settings page.", enumOf(settingsPages...)),
		}),
		toolStartReview: object([]string{"kind"}, map[string]*jsonschema.Schema{
			"kind": oneOf("What to review.", enumOf(domain.AssistantReviewTags, domain.AssistantReviewPeople,
				domain.AssistantReviewWorks)),
		}),
	}
	out := make(map[string]*builtinTool, len(tools))
	for _, tool := range tools {
		schema := schemas[tool.definition.Name]
		raw, err := json.Marshal(schema)
		if err != nil {
			panic(fmt.Sprintf("the schema of %s: %v", tool.definition.Name, err))
		}
		if tool.schema, err = schema.Resolve(nil); err != nil {
			panic(fmt.Sprintf("the schema of %s: %v", tool.definition.Name, err))
		}
		tool.definition.Parameters = raw
		out[tool.definition.Name] = tool
	}
	return out
})

// builtinOrder is the order the built-in tools are offered in.
var builtinOrder = []string{toolScreenContext, toolAppHelp, toolShowInLibrary, toolOpenPage, toolStartReview}

// builtinDefinitions are the built-in tools this assistant can run; start_review needs the review store.
func (a *Assistant) builtinDefinitions() []domain.ToolDefinition {
	var out []domain.ToolDefinition
	for _, name := range builtinOrder {
		if name == toolStartReview && a.reviews == nil {
			continue
		}
		out = append(out, builtinTools()[name].definition)
	}
	return out
}

// runBuiltin validates the arguments against the tool's schema and runs it; an error is a storage failure.
func (l *turnLoop) runBuiltin(ctx context.Context, tool *builtinTool, raw json.RawMessage) (builtinOutcome, error) {
	args, err := builtinArguments(tool.schema, raw)
	if err != nil {
		return builtinOutcome{failure: err.Error()}, nil
	}
	return tool.run(l, ctx, args)
}

// builtinArguments drops the null and empty string arguments, which chat models write for an argument they mean to
// leave out, reads "true", "false" and a number written as a string as the boolean or integer the schema asks for,
// and validates the rest.
func builtinArguments(schema *jsonschema.Resolved, raw json.RawMessage) (json.RawMessage, error) {
	args := map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("the arguments are not a JSON object: %w", err)
		}
	}
	for name, value := range args {
		if value == nil || value == "" {
			delete(args, name)
			continue
		}
		if property, ok := schema.Schema().Properties[name]; ok {
			args[name] = coerceScalar(property.Type, value)
		}
	}
	if err := schema.Validate(args); err != nil {
		return nil, fmt.Errorf("the arguments do not match the schema: %w", err)
	}
	return json.Marshal(args)
}

// coerceScalar reads a string as the boolean or integer a property takes, and leaves any other value as it is.
func coerceScalar(kind string, value any) any {
	text, ok := value.(string)
	if !ok {
		return value
	}
	switch kind {
	case "boolean":
		if parsed, err := strconv.ParseBool(text); err == nil {
			return parsed
		}
	case "integer":
		if parsed, err := strconv.ParseInt(text, 10, 64); err == nil {
			return parsed
		}
	}
	return value
}

// resolver resolves names the way the shipped filter and navigation did, for this turn's request and screen.
func (l *turnLoop) resolver() *assistantTurn {
	return &assistantTurn{a: l.a, input: domain.AssistantTurnInput{
		Text: l.turn.Request, Context: l.turn.Context, PreviousTurnID: l.turn.PreviousTurnID,
	}}
}

// clarifyFailure tells the model what a name or value did not settle, so it can call again or ask the owner.
func clarifyFailure(c *domain.AssistantClarification) string {
	if c == nil {
		return "the request is not complete"
	}
	if len(c.Candidates) > 0 {
		names := make([]string, len(c.Candidates))
		for i, candidate := range c.Candidates {
			names[i] = fmt.Sprintf("%s (%s, %d videos)", candidate.Name, candidate.Kind, candidate.Count)
		}
		return "the name matches several entities: " + strings.Join(names, ", ") +
			"; call again with one of these names, or ask the owner which one"
	}
	if len(c.Names) > 0 {
		return "nothing is named " + strings.Join(c.Names, ", ") + "; check the name with a read tool"
	}
	if c.About == "target" {
		return "the screen shows no such entity; give its name"
	}
	return "the conditions are not valid: check that each bound is not past the other and dates are YYYY-MM-DD"
}

// encodeResult bounds a built-in tool's result as a read's result is bounded, so that it fits the model's context.
func encodeResult(value any) string {
	encoded, _ := json.Marshal(value) // The result types hold only plain values, which always encode.
	return domain.TrimAssistantToolResult(encoded)
}

// criteriaResult is a resolved library filter in the names the library URL uses, with the entities it names.
type criteriaResult struct {
	Rule   domain.SmartRule        `json:"rule"`
	Sort   domain.VideoSort        `json:"sort,omitempty"`
	Labels []domain.AssistantLabel `json:"labels"`
}

// directiveNote ends a directive's result: small models otherwise call the same tool again.
const directiveNote = "Done: it applies when you answer. Answer the owner now, without calling it again."

func (l *turnLoop) showInLibrary(ctx context.Context, raw json.RawMessage) (builtinOutcome, error) {
	var filter domain.AssistantFilter
	if err := json.Unmarshal(raw, &filter); err != nil {
		return builtinOutcome{failure: err.Error()}, nil
	}
	t := l.resolver()
	criteria, ok, err := t.criteria(ctx, &filter, false)
	if err != nil {
		return builtinOutcome{}, err
	}
	if !ok {
		return builtinOutcome{failure: clarifyFailure(t.clarification)}, nil
	}
	result := struct {
		Criteria criteriaResult `json:"criteria"`
		Total    *int           `json:"total,omitempty"`
		Note     string         `json:"note"`
	}{Criteria: criteriaResult{Rule: criteria.Rule, Sort: criteria.Sort, Labels: criteria.Labels}, Note: directiveNote}
	if l.a.videos != nil {
		q := criteria.Rule.Query()
		q.Limit = 1
		page, err := l.a.videos.ListVideos(ctx, domain.AudienceOwner, q)
		if err != nil {
			return builtinOutcome{}, fmt.Errorf("cannot count the filtered videos: %w", err)
		}
		result.Total = &page.Total
	}
	l.turn.Directive = &domain.AssistantDirective{Kind: string(domain.AssistantResolvedFilter), Criteria: &criteria}
	return builtinOutcome{content: encodeResult(result), labels: criteria.Labels}, nil
}

func (l *turnLoop) openPage(ctx context.Context, raw json.RawMessage) (builtinOutcome, error) {
	var nav domain.AssistantNavigation
	if err := json.Unmarshal(raw, &nav); err != nil {
		return builtinOutcome{failure: err.Error()}, nil
	}
	if nav.To == "settings" && nav.SettingsPage == "" {
		return builtinOutcome{failure: "to settings needs settingsPage"}, nil
	}
	t := l.resolver()
	resolved, err := t.navigate(ctx, &nav)
	if err != nil {
		return builtinOutcome{}, err
	}
	if t.clarification != nil || resolved.Destination == nil {
		return builtinOutcome{failure: clarifyFailure(t.clarification)}, nil
	}
	dest := resolved.Destination
	result := struct {
		Kind         string `json:"kind"`
		ID           int64  `json:"id,omitempty"`
		Name         string `json:"name,omitempty"`
		SettingsPage string `json:"settingsPage,omitempty"`
		Note         string `json:"note"`
	}{Kind: dest.Kind, ID: dest.ID, SettingsPage: dest.SettingsPage, Note: directiveNote}
	var labels []domain.AssistantLabel
	if dest.ID > 0 {
		kind := domain.AssistantLabelKind(dest.Kind)
		name, err := l.entityName(ctx, kind, dest.ID)
		if err != nil {
			return builtinOutcome{}, err
		}
		result.Name = name
		labels = []domain.AssistantLabel{{Kind: kind, ID: dest.ID, Name: name}}
	}
	l.turn.Directive = &domain.AssistantDirective{Kind: string(domain.AssistantResolvedNavigate), Destination: dest}
	return builtinOutcome{content: encodeResult(result), labels: labels}, nil
}

func (l *turnLoop) startReview(ctx context.Context, raw json.RawMessage) (builtinOutcome, error) {
	var req domain.AssistantOrganizeRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return builtinOutcome{failure: err.Error()}, nil
	}
	resolved, err := l.resolver().organize(ctx, &req)
	if err != nil {
		return builtinOutcome{}, err
	}
	if resolved.Review == nil {
		return builtinOutcome{failure: "review runs are not available"}, nil
	}
	run := resolved.Review
	result := struct {
		ID     int64                        `json:"id"`
		Kind   domain.AssistantReviewKind   `json:"kind"`
		Status domain.AssistantReviewStatus `json:"status"`
		Note   string                       `json:"note"`
	}{ID: run.ID, Kind: run.Kind, Status: run.Status, Note: "The run judges in the background, and the panel links " +
		"its review list under your answer; do not open a page for it. Answer the owner now."}
	return builtinOutcome{content: encodeResult(result), reviewRunID: run.ID}, nil
}

// entityName reads the current name of an entity, or a video's title; empty when it is gone.
func (l *turnLoop) entityName(ctx context.Context, kind domain.AssistantLabelKind, id int64) (string, error) {
	if kind != domain.AssistantLabelVideo {
		names, err := l.a.vocabulary.EntityNames(ctx, kind, []int64{id})
		return names[id], err
	}
	if l.a.videos == nil {
		return "", nil
	}
	videos, err := l.a.videos.VideosByIDs(ctx, domain.AudienceOwner, []int64{id})
	if err != nil || len(videos) == 0 {
		return "", err
	}
	return videos[0].Title, nil
}
