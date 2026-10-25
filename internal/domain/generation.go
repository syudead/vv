package domain

import "encoding/json"

// GenerationRequest asks a generator for one JSON value that satisfies Schema; see
// docs/design-docs/ai-layer.md.
type GenerationRequest struct {
	Prompt string
	// Schema is a JSON Schema object the answer must satisfy.
	Schema json.RawMessage
}
