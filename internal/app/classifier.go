package app

import (
	"context"

	"github.com/syudead/vv/internal/domain"
)

// Classifier hides provider endpoints and batch limits from features; see
// docs/design-docs/ai-layer.md.
type Classifier interface {
	// Classify returns yes probabilities from zero to one and omits unanswered questions.
	Classify(ctx context.Context, req domain.ClassifierRequest) (map[string]float64, error)
}
