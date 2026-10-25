package app

import (
	"context"

	"github.com/syudead/vv/internal/domain"
)

// ActionRunner runs one catalog row through the screen API's own handler as the owner, so the
// operation keeps its validation, events and activity entries; internal/httpapi implements it.
type ActionRunner interface {
	// Run returns domain.ErrAssistantActionUnknown before any call for an id outside the catalog, and a
	// *domain.AssistantCallError for a non-2xx response.
	Run(ctx context.Context, call domain.AssistantCall) (domain.AssistantCallResult, error)
}
