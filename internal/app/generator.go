package app

import (
	"context"
	"encoding/json"

	"github.com/syudead/vv/internal/domain"
)

// StructuredGenerator hides the provider from the assistant, as Classifier does for decisions; see
// docs/design-docs/ai-layer.md. Errors wrap the domain.ErrOllama* and domain.ErrGeneration* causes.
// It is not named Generator because the media Generator in ingest.go holds that name.
type StructuredGenerator interface {
	// Generate returns one JSON value that the provider constrained to req.Schema.
	Generate(ctx context.Context, req domain.GenerationRequest) (json.RawMessage, error)
}
