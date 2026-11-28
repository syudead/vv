package app

import (
	"context"
	"encoding/json"

	"github.com/syudead/vv/internal/domain"
)

// Classifier hides provider endpoints and batch limits from features; see
// docs/design-docs/ai-layer.md.
type Classifier interface {
	// Classify returns yes probabilities from zero to one and omits unanswered questions.
	Classify(ctx context.Context, req domain.ClassifierRequest) (map[string]float64, error)
}

// ChoiceClassifier ranks options for one input; a provider that cannot is left unset and its callers ask every
// option as a yes/no question.
type ChoiceClassifier interface {
	// Choose returns each option's probability from zero to one and omits unanswered options.
	Choose(ctx context.Context, req domain.ClassifierChoiceRequest) (map[string]float64, error)
}

// ClassifierWire sends a request in the provider's own question format and answers the provider's JSON as it is, for
// a caller whose model reads the answers itself: the assistant's clef tool.
type ClassifierWire interface {
	// WireFormat tells the model that writes the request how to write it and how to read the answer.
	WireFormat() domain.ClassifierWireFormat
	// SendWire returns an error with the provider's error text for an answer that is not a success.
	SendWire(ctx context.Context, req domain.ClassifierWireRequest) (json.RawMessage, error)
}
