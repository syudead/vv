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

// ClefWire sends Clef's own request and answers the endpoint's JSON as it is, for a caller that reads Clef's answers
// itself: the assistant's clef tool.
type ClefWire interface {
	// SystemOne returns an error with the endpoint's error text for an answer that is not a success.
	SystemOne(ctx context.Context, req domain.ClefRequest) (json.RawMessage, error)
}
