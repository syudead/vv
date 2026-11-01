package app

import (
	"encoding/json"
	"fmt"
	"slices"
	"unicode/utf8"

	"github.com/syudead/vv/internal/domain"
)

// The chat context is the one the provider ran the model with, which it reports with each reply. A prompt past it makes
// Ollama drop the oldest messages, the request among them, so the turn keeps its estimate under the context less a
// quarter of it, the room for the reply and a reasoning model's thinking.
const (
	// bytesPerToken roughly matches Japanese text, three bytes a character; schemaBytesPerToken matches the English
	// JSON of tool definitions.
	bytesPerToken       = 3
	schemaBytesPerToken = 4
	// imageTokens is a vision model's cost of one image of up to viewImageMaxSide pixels a side.
	imageTokens = 1500
)

// droppedNote marks an old tool result that no longer fits. It asks for nothing: a note that asked the model to call
// again made a model whose results never fit call the same reads without end.
const droppedNote = "this earlier result was removed to fit the context"

// droppedScalarMax bounds a top-level value droppedResult keeps.
const droppedScalarMax = 100

// droppedResult replaces an old tool result that no longer fits, keeping its short top-level values such as total and
// error so the model keeps the counts it read; the grounding evidence keeps the original.
func droppedResult(content string) string {
	out := map[string]json.RawMessage{}
	var object map[string]json.RawMessage
	if json.Unmarshal([]byte(content), &object) == nil {
		for key, value := range object {
			if len(value) > 0 && value[0] != '{' && value[0] != '[' && len(value) <= droppedScalarMax {
				out[key] = value
			}
		}
	}
	out["note"], _ = json.Marshal(droppedNote) // A string always encodes.
	encoded, _ := json.Marshal(out)            // Raw JSON values from a decoded object always encode.
	return string(encoded)
}

// promptBudget is the tokens the prompt may take, or 0 when the provider has not reported the context.
func (l *turnLoop) promptBudget() int {
	return l.usage.ContextTokens - l.usage.ContextTokens/4
}

// fitContext makes the messages and tools fit the context the last reply reported; before any reply, or when the
// provider does not say, it changes nothing. It first replaces the oldest tool results with droppedResult, keeping
// the latest round since dropping it makes the model call the same tools again, and then cuts the latest round's
// longest results.
func (l *turnLoop) fitContext(tools []domain.ToolDefinition) {
	budget := l.promptBudget()
	if budget <= 0 {
		return
	}
	encoded, _ := json.Marshal(tools) // Tool definitions are plain data and always encode.
	used := len(encoded) / schemaBytesPerToken
	for _, m := range l.messages {
		used += messageTokens(m)
	}
	round := l.lastRound()
	for i := range round {
		if used <= budget {
			return
		}
		m := &l.messages[i]
		if m.Role != domain.ChatRoleTool {
			continue
		}
		dropped := droppedResult(m.Content)
		if len(dropped) >= len(m.Content) {
			continue
		}
		used -= messageTokens(*m)
		m.Content = dropped
		used += messageTokens(*m)
	}
	used, round = l.dropRounds(used, budget, round)
	l.cutLatest(round, used-budget)
}

// droppedRounds stands for the earlier rounds dropRounds removed.
const droppedRounds = "Earlier tool calls and their results were removed to fit the context."

// dropRounds removes whole earlier rounds after the request, oldest first, until the prompt fits, and puts one
// note in their place; the system message, the request and the latest round stay. It answers the tokens used and
// the latest round's new index.
func (l *turnLoop) dropRounds(used, budget, round int) (int, int) {
	const kept = 2 // The system message and the request.
	end := kept
	if end < len(l.messages) && l.messages[end].Content == droppedRounds {
		end++
	}
	start := end
	// A round ends where the next assistant message starts, so no tool result outlives its call.
	for end < round && (used > budget || l.messages[end].Role != domain.ChatRoleAssistant) {
		used -= messageTokens(l.messages[end])
		end++
	}
	if end == start {
		return used, round
	}
	note := domain.ChatMessage{Role: domain.ChatRoleUser, Content: droppedRounds}
	if start == kept {
		used += messageTokens(note)
	}
	l.messages = append(append(l.messages[:kept:kept], note), l.messages[end:]...)
	return used, round - (end - kept) + 1
}

// cutLatest takes over tokens from the latest round's tool results, longest first, ending each cut with a note.
func (l *turnLoop) cutLatest(round, over int) {
	latest := []int{}
	for i := round; i < len(l.messages); i++ {
		if l.messages[i].Role == domain.ChatRoleTool {
			latest = append(latest, i)
		}
	}
	slices.SortFunc(latest, func(a, b int) int { return len(l.messages[b].Content) - len(l.messages[a].Content) })
	for _, i := range latest {
		if over <= 0 {
			return
		}
		m := &l.messages[i]
		keep := max(len(m.Content)-over*bytesPerToken, 0)
		for keep > 0 && !utf8.RuneStart(m.Content[keep]) {
			keep--
		}
		over -= (len(m.Content) - keep) / bytesPerToken
		m.Content = fmt.Sprintf("%s…[cut to fit the context: %d bytes in all; the rest is not shown]",
			m.Content[:keep], len(m.Content))
	}
}

// minResultTokens is the least room the prompt must leave for tool results. Below it every result is cut away, and a
// model that never sees its results calls the same reads without end.
const minResultTokens = 1024

// contextShortfall says, when the instructions, the request and the tools leave the prompt less than minResultTokens,
// why the turn cannot run in the model's context; it answers "" when they fit or the context is not known yet.
func (l *turnLoop) contextShortfall(tools []domain.ToolDefinition) string {
	budget := l.promptBudget()
	if budget <= 0 || len(l.messages) < 2 {
		return ""
	}
	encoded, _ := json.Marshal(tools) // Tool definitions are plain data and always encode.
	fixed := len(encoded)/schemaBytesPerToken + messageTokens(l.messages[0]) + messageTokens(l.messages[1])
	if fixed+minResultTokens <= budget {
		return ""
	}
	need := (fixed + minResultTokens) * 4 / 3
	return fmt.Sprintf("the model runs with a context of %d tokens; the instructions, the request and the tools take "+
		"about %d of the %d tokens the prompt may use, which leaves too little room for tool results; it needs a "+
		"context of at least %d tokens", l.usage.ContextTokens, fixed, budget, need)
}

func messageTokens(m domain.ChatMessage) int {
	size := len(m.Content)
	for _, call := range m.ToolCalls {
		size += len(call.Name) + len(call.Arguments)
	}
	return size/bytesPerToken + len(m.Images)*imageTokens
}

// lastRound is the index of the latest assistant message; the messages from it on are the round the model acts on.
func (l *turnLoop) lastRound() int {
	for i := len(l.messages) - 1; i >= 0; i-- {
		if l.messages[i].Role == domain.ChatRoleAssistant {
			return i
		}
	}
	return len(l.messages)
}
