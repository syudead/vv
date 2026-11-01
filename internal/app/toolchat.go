package app

import (
	"context"

	"github.com/syudead/vv/internal/domain"
)

// ToolChat hides the provider from the assistant's tool loop; see docs/design-docs/ai-layer.md. Errors wrap
// the domain.ErrOllama* causes.
type ToolChat interface {
	// Chat sends the conversation and the offered tools, and returns the model's tool calls or its text.
	Chat(ctx context.Context, req domain.ChatRequest) (domain.ChatReply, error)
}

// ContextReporter is a ToolChat that tells the context of the loaded model before the first call; 0 means unknown.
type ContextReporter interface {
	LoadedContext(ctx context.Context) int
}
