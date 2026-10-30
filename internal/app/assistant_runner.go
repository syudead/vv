package app

import (
	"context"
	"encoding/json"

	"github.com/syudead/vv/internal/domain"
)

// ActionRunner runs one catalog row or read tool through the screen API's own handler as the owner, so the
// operation keeps its validation, audience rules, events and activity entries; internal/httpapi implements it.
type ActionRunner interface {
	// Tools lists every owner operation as a read, write or excluded tool, built from the screen API schema.
	Tools() []domain.AssistantTool
	// Read validates the arguments against the tool's schema before any call: domain.ErrAssistantInvalidArguments
	// refuses them, a *domain.AssistantExcludedError an excluded operation, and domain.ErrAssistantNotReadTool any
	// other operation that is not a read tool. A non-2xx response is a *domain.AssistantCallError.
	Read(ctx context.Context, read domain.AssistantRead) (domain.AssistantReadResult, error)
	// WriteCall validates a schema row's tool arguments against its operation's schema, before any call, and splits
	// them into the request of the row's step; domain.ErrAssistantInvalidArguments refuses them.
	WriteCall(actionID string, arguments json.RawMessage) (domain.AssistantCall, error)
	// Run returns domain.ErrAssistantActionUnknown before any call for an id outside the catalog, and a
	// *domain.AssistantCallError for a non-2xx response.
	Run(ctx context.Context, call domain.AssistantCall) (domain.AssistantCallResult, error)
}
