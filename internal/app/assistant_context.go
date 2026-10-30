package app

import (
	"encoding/json"
	"fmt"
	"slices"
	"unicode/utf8"

	"github.com/syudead/vv/internal/domain"
)

// The chat context is the model's own length, which the provider reports with each reply. A prompt past it makes
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

// droppedResult replaces an old tool result that no longer fits; the grounding evidence keeps the original.
const droppedResult = `{"note":"this earlier result was dropped to fit the context; call the tool again if you need it"}`

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
		if m.Role != domain.ChatRoleTool || len(m.Content) <= len(droppedResult) {
			continue
		}
		used -= messageTokens(*m)
		m.Content = droppedResult
		used += messageTokens(*m)
	}
	l.cutLatest(round, used-budget)
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
		m.Content = fmt.Sprintf("%s…[cut to fit the context: %d bytes in all; narrow the call to read the rest]",
			m.Content[:keep], len(m.Content))
	}
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
