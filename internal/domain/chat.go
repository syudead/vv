package domain

import "encoding/json"

// ChatRole is the speaker of one chat message; see docs/design-docs/ai-layer.md.
type ChatRole string

const (
	ChatRoleSystem    ChatRole = "system"
	ChatRoleUser      ChatRole = "user"
	ChatRoleAssistant ChatRole = "assistant"
	// ChatRoleTool carries one tool's result back to the model.
	ChatRoleTool ChatRole = "tool"
)

type ChatMessage struct {
	Role    ChatRole
	Content string
	// Images are encoded image files the model sees with this message; only a model with vision reads them.
	Images [][]byte
	// ToolCalls repeats an assistant reply's calls, so the next request shows what the results answer.
	ToolCalls []ToolCall
	// ToolName names the tool whose result a ChatRoleTool message carries.
	ToolName string
}

// ToolDefinition offers one tool to the model; Parameters is a JSON Schema object for its arguments.
type ToolDefinition struct {
	Name        string
	Description string
	Parameters  json.RawMessage
}

// ToolCall is one call the model asked for; Arguments is a JSON object, never null.
type ToolCall struct {
	Name      string
	Arguments json.RawMessage
}

type ChatRequest struct {
	Messages []ChatMessage
	Tools    []ToolDefinition
}

// ChatReply holds either the tool calls the model asked for or its text answer; the model's reasoning is dropped.
type ChatReply struct {
	ToolCalls []ToolCall
	Text      string
	Usage     ChatUsage
}

// ChatUsage is how the provider ended a reply, which explains an empty one; zero values mean it did not say.
type ChatUsage struct {
	// DoneReason is the provider's word for why the reply ended, such as "stop" or "length".
	DoneReason    string
	PromptTokens  int
	OutputTokens  int
	ThinkingChars int
}

// ModelCapabilities is what a model can do beyond text, as the provider reports it.
type ModelCapabilities struct {
	Tools  bool
	Vision bool
}

// OllamaModel is one model at the saved endpoint and what it can do.
type OllamaModel struct {
	Name         string
	Capabilities ModelCapabilities
}
