package app

import (
	"encoding/json"

	"github.com/syudead/vv/internal/domain"
)

// The chat context is Ollama's num_ctx (see internal/ollama/chat.go). A prompt past it makes Ollama drop the oldest
// messages, the request among them, so the turn keeps its estimate under the context less a reserve for the reply
// and a reasoning model's thinking.
const (
	assistantContextTokens = 16384
	assistantReplyReserve  = 4096
	// bytesPerToken roughly matches Japanese text, three bytes a character; schemaBytesPerToken matches the English
	// JSON of tool definitions.
	bytesPerToken       = 3
	schemaBytesPerToken = 4
	// imageTokens is a vision model's cost of one image of up to viewImageMaxSide pixels a side.
	imageTokens = 1500
)

// droppedResult replaces an old tool result that no longer fits; the grounding evidence keeps the original.
const droppedResult = `{"note":"this earlier result was dropped to fit the context; call the tool again if you need it"}`

// fitContext replaces the oldest tool results with droppedResult until the messages and tools fit the context. The
// results of the latest round stay, since dropping them makes the model call the same tools again.
func (l *turnLoop) fitContext(tools []domain.ToolDefinition) {
	encoded, _ := json.Marshal(tools) // Tool definitions are plain data and always encode.
	used := len(encoded) / schemaBytesPerToken
	for _, m := range l.messages {
		used += messageTokens(m)
	}
	budget := assistantContextTokens - assistantReplyReserve
	for i := range l.lastRound() {
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
