package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/syudead/vv/internal/domain"
)

// toolFind searches the registry and loads what it finds into the turn.
const toolFind = "find_tools"

// assistantCoreTools are the operations sent on every model call.
var assistantCoreTools = []string{"listVideos", "getVideo", "listTags", "listPeople", "listOrganizations"}

// findToolsLimit bounds the tools one search answers.
const findToolsLimit = 15

// assistantWriteNote ends a write tool's description.
const assistantWriteNote = "Proposed for the owner's approval; it does not run now."

var findToolsDefinition = domain.ToolDefinition{
	Name: toolFind,
	Description: "Search the app's other tools by English words over their names and descriptions. Answers up to " +
		"15 tools with their kind (read, write or excluded) and summary; the read and write tools it finds can be " +
		"called after it. An excluded tool says why and on which page the owner does it instead.",
	Parameters: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string",` +
		`"description":"English words, such as \"favorite videos\" or \"merge tags\"."}},"required":["query"]}`),
}

// emptyParameters is the schema of a tool whose arguments the registry does not give.
var emptyParameters = json.RawMessage(`{"type":"object","properties":{}}`)

// turnTools holds the registry and the tools offered so far in one turn, in the order they were offered.
type turnTools struct {
	byName  map[string]domain.AssistantTool
	all     []domain.AssistantTool
	offered []string
	// builtins are the built-in tools offered on every call besides find_tools.
	builtins []domain.ToolDefinition
}

func newTurnTools(registry []domain.AssistantTool) *turnTools {
	t := &turnTools{byName: make(map[string]domain.AssistantTool, len(registry)), all: registry}
	for _, tool := range registry {
		t.byName[tool.Name] = tool
	}
	for _, name := range assistantCoreTools {
		if tool, ok := t.byName[name]; ok && tool.Kind == domain.AssistantToolRead {
			t.offered = append(t.offered, name)
		}
	}
	return t
}

func (t *turnTools) isOffered(name string) bool { return slices.Contains(t.offered, name) }

func (t *turnTools) load(name string) {
	if !t.isOffered(name) {
		t.offered = append(t.offered, name)
	}
}

// definitions are the tools the next model call receives: find_tools, the built-in and core tools and those loaded.
func (t *turnTools) definitions() []domain.ToolDefinition {
	out := append([]domain.ToolDefinition{findToolsDefinition}, t.builtins...)
	for _, name := range t.offered {
		tool := t.byName[name]
		description := strings.TrimSpace(tool.Summary + "\n" + tool.Description)
		if tool.Kind == domain.AssistantToolWrite {
			description += "\n" + assistantWriteNote
		}
		parameters := tool.Parameters
		if tool.Row != nil && !tool.Row.Schema {
			parameters = domain.AssistantRowParameters(*tool.Row)
		}
		if tool.Row != nil && tool.Row.Action != "" {
			// Variants share their operation's description, which names every action.
			description += "\nThis tool does " + tool.Row.Action + " only."
		}
		if len(parameters) == 0 {
			parameters = emptyParameters
		}
		out = append(out, domain.ToolDefinition{Name: tool.Name, Description: description, Parameters: parameters})
	}
	return out
}

// foundTool is one tool find_tools answers.
type foundTool struct {
	Name    string                   `json:"name"`
	Kind    domain.AssistantToolKind `json:"kind"`
	Summary string                   `json:"summary"`
	Reason  string                   `json:"reason,omitempty"`
	Page    string                   `json:"page,omitempty"`
}

// find ranks the tools by how many of the words their name, summary and description contain, ties by name, and
// loads the read and write tools among the first 15. It also reports whether the first tool contains more of the
// words than any other.
func (t *turnTools) find(query string) ([]foundTool, bool) {
	candidates := slices.DeleteFunc(slices.Clone(t.all), func(tool domain.AssistantTool) bool {
		return slices.Contains(assistantCoreTools, tool.Name) ||
			(tool.Kind == domain.AssistantToolWrite && !proposable(tool))
	})
	slices.SortStableFunc(candidates, func(a, b domain.AssistantTool) int { return strings.Compare(a.Name, b.Name) })
	found := []foundTool{}
	matches := ranked(candidates, searchWords(query), toolSearchText, findToolsLimit)
	for _, match := range matches {
		tool := match.item
		item := foundTool{Name: tool.Name, Kind: tool.Kind, Summary: tool.Summary}
		if tool.Exclusion != nil {
			item.Reason, item.Page = tool.Exclusion.Reason, tool.Exclusion.Page
		} else {
			t.load(tool.Name)
		}
		found = append(found, item)
	}
	return found, len(matches) == 1 || (len(matches) > 1 && matches[0].score > matches[1].score)
}

// toolError is the result the model receives for a call that did not run or failed.
func toolError(message string) string {
	encoded, _ := json.Marshal(map[string]string{"error": message}) // A string map always encodes.
	return string(encoded)
}

// call runs one tool call, records its progress item, and answers the tool message's content. It returns an error
// only for a storage failure.
func (l *turnLoop) call(ctx context.Context, call domain.ToolCall) (string, error) {
	item := domain.AssistantProgressItem{Tool: call.Name, Kind: domain.AssistantProgressBuiltin, At: l.a.now()}
	tool, known := l.tools.byName[call.Name]
	if known {
		item.Kind = progressKind(tool)
		item.Labels = l.labels(ctx, tool, call.Arguments)
	}
	if err := l.record(ctx, item); err != nil {
		return "", err
	}
	content, failure, err := l.dispatch(ctx, call, tool, known)
	if err != nil {
		return "", err
	}
	l.note(call.Name, tool, known, failure)
	if failure != "" {
		l.turn.Progress[len(l.turn.Progress)-1].Error = failure
		if err := l.a.turns.RecordTurnProgress(ctx, l.turn.ID, l.turn.Progress); err != nil && ctx.Err() == nil {
			return "", err
		}
		return toolError(failure), nil
	}
	return content, nil
}

// dispatch runs a built-in tool, recording what its progress item shows, or any other tool; an error is a storage
// failure.
func (l *turnLoop) dispatch(
	ctx context.Context, call domain.ToolCall, tool domain.AssistantTool, known bool,
) (string, string, error) {
	builtin, ok := builtinTools()[call.Name]
	if !ok || !slices.ContainsFunc(l.tools.builtins, func(d domain.ToolDefinition) bool { return d.Name == call.Name }) {
		return l.runTool(ctx, call, tool, known)
	}
	out, err := l.runBuiltin(ctx, builtin, call.Arguments)
	if err != nil || (len(out.labels) == 0 && out.reviewRunID == 0) {
		return out.content, out.failure, err
	}
	item := &l.turn.Progress[len(l.turn.Progress)-1]
	item.Labels, item.ReviewRunID = out.labels[:min(len(out.labels), progressLabelLimit)], out.reviewRunID
	if item.Labels == nil {
		item.Labels = []domain.AssistantLabel{}
	}
	return out.content, out.failure, l.a.turns.RecordTurnProgress(ctx, l.turn.ID, l.turn.Progress)
}

// passiveTools only search or read the screen; a turn that ran nothing else did nothing for the request.
var passiveTools = []string{toolFind, toolAppHelp, toolScreenContext}

// note keeps the excluded operation the model asked for by name, and whether the turn did anything else.
func (l *turnLoop) note(name string, tool domain.AssistantTool, known bool, failure string) {
	switch {
	case known && tool.Kind == domain.AssistantToolExcluded:
		l.askedExcluded(tool)
	case failure == "" && !slices.Contains(passiveTools, name):
		l.acted = true
	}
}

// askedExcluded keeps the first excluded operation the turn asked for.
func (l *turnLoop) askedExcluded(tool domain.AssistantTool) {
	if l.excluded == nil && tool.Exclusion != nil {
		l.excluded = &domain.AssistantExcludedRef{
			Operation: tool.Name, Reason: tool.Exclusion.Reason, Page: tool.Exclusion.Page,
		}
	}
}

// record commits the turn's progress with the new item, so the panel shows the call while it runs.
func (l *turnLoop) record(ctx context.Context, item domain.AssistantProgressItem) error {
	if item.Labels == nil {
		item.Labels = []domain.AssistantLabel{}
	}
	l.turn.Progress = append(l.turn.Progress, item)
	return l.a.turns.RecordTurnProgress(ctx, l.turn.ID, l.turn.Progress)
}

func progressKind(tool domain.AssistantTool) domain.AssistantProgressKind {
	switch {
	case tool.Kind == domain.AssistantToolExcluded:
		return domain.AssistantProgressExcluded
	case tool.Kind == domain.AssistantToolWrite:
		return domain.AssistantProgressWrite
	case tool.Image:
		return domain.AssistantProgressImage
	}
	return domain.AssistantProgressRead
}

// runTool answers the content of a call, or the reason it failed; it returns an error only for a storage failure.
func (l *turnLoop) runTool(
	ctx context.Context, call domain.ToolCall, tool domain.AssistantTool, known bool,
) (string, string, error) {
	switch {
	case call.Name == toolFind:
		var args struct {
			Query string `json:"query"`
		}
		if err := json.Unmarshal(call.Arguments, &args); err != nil || strings.TrimSpace(args.Query) == "" {
			return "", "find_tools needs a query of English words", nil
		}
		found, clear := l.tools.find(args.Query)
		if clear && found[0].Kind == domain.AssistantToolExcluded {
			// A clear best match is what the request asked for.
			l.askedExcluded(l.tools.byName[found[0].Name])
		}
		return encodeResult(map[string]any{"tools": found}), "", nil
	case !known:
		return "", fmt.Sprintf("there is no tool named %s; call find_tools to look for one", call.Name), nil
	case tool.Kind == domain.AssistantToolExcluded:
		return "", fmt.Sprintf("%s is excluded: %s; the owner does it on the page %s", tool.Name,
			tool.Exclusion.Reason, tool.Exclusion.Page), nil
	case !l.tools.isOffered(call.Name):
		return "", fmt.Sprintf("%s is not loaded; call find_tools first", call.Name), nil
	case tool.Kind == domain.AssistantToolWrite:
		return l.write(ctx, call, tool)
	}
	result, err := l.runner.Read(ctx, domain.AssistantRead{Operation: tool.Operation, Arguments: call.Arguments})
	if err != nil {
		return "", readFailure(err), nil
	}
	l.writes.remember(tool.Operation, result.Body)
	return domain.TrimAssistantToolResult(result.Body), "", nil
}

// readFailure says why a read did not answer, in words the model can act on.
func readFailure(err error) string {
	var call *domain.AssistantCallError
	switch {
	case errors.Is(err, domain.ErrAssistantInvalidArguments):
		return err.Error()
	case errors.As(err, &call):
		return strings.TrimSuffix(fmt.Sprintf("the operation answered %d %s: %s", call.Status, call.Code, call.Message), ": ")
	}
	return "the operation failed"
}
