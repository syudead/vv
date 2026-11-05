package app

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/syudead/vv/internal/domain"
)

// testOutcome is one resolution request in the shape the one-shot build sent; the tests drive the name, filter,
// navigation, write and review resolution that the loop's tools reuse through it.
type testOutcome struct {
	Kind     string                           `json:"kind"`
	Filter   *domain.AssistantFilter          `json:"filter"`
	Navigate *domain.AssistantNavigation      `json:"navigate"`
	Action   *domain.AssistantActionRequest   `json:"action"`
	Organize *domain.AssistantOrganizeRequest `json:"organize"`
}

// resolve resolves the fixture's outcome for the input; a part that needs the owner answers clarify.
func (f turnFixture) resolve(ctx context.Context, in domain.AssistantTurnInput) (domain.AssistantResolution, error) {
	var o testOutcome
	if err := json.Unmarshal([]byte(f.outcome), &o); err != nil {
		return domain.AssistantResolution{}, fmt.Errorf("the test outcome: %w", err)
	}
	t := &assistantTurn{a: f.a, input: in}
	var result domain.AssistantResolution
	var err error
	switch o.Kind {
	case "filter":
		var criteria domain.AssistantCriteria
		var ok bool
		if criteria, ok, err = t.criteria(ctx, o.Filter, false); ok && err == nil {
			result = domain.AssistantResolution{Kind: domain.AssistantResolvedFilter, Criteria: &criteria}
		}
	case "navigate":
		result, err = t.navigate(ctx, o.Navigate)
	case "action":
		result, err = t.action(ctx, o.Action)
	case "organize":
		return t.organize(ctx, o.Organize)
	default:
		return domain.AssistantResolution{}, fmt.Errorf("the test outcome kind %q", o.Kind)
	}
	if err != nil {
		return domain.AssistantResolution{}, err
	}
	if t.clarification == nil && result.Kind == "" {
		t.clarify("value", nil)
	}
	if t.clarification != nil {
		return domain.AssistantResolution{Kind: domain.AssistantResolvedClarify, Clarify: t.clarification}, nil
	}
	return result, nil
}
