package httpapi

import (
	"encoding/json"
	"fmt"

	"github.com/syudead/vv/internal/domain"
)

// WriteCall implements app.ActionRunner: it validates a schema row's tool arguments against its operation's schema
// and splits them into the path parameters, query and body of the step's call, without calling anything.
func (r *ActionRunner) WriteCall(actionID string, arguments json.RawMessage) (domain.AssistantCall, error) {
	row, ok := domain.LookupAssistantAction(actionID)
	if !ok || !row.Schema {
		return domain.AssistantCall{}, fmt.Errorf("%w: %q is not a schema row", domain.ErrAssistantActionUnknown, actionID)
	}
	op, ok := loadAssistantRegistry().byID[row.Operation]
	if !ok {
		return domain.AssistantCall{}, fmt.Errorf("%w: %q has no operation", domain.ErrAssistantActionUnknown, row.ID)
	}
	args, err := readArguments(op, arguments)
	if err != nil {
		return domain.AssistantCall{}, err
	}
	call := domain.AssistantCall{ActionID: row.ID, Path: pathValues(op, args)}
	if _, err := requestTarget(op, call.Path, nil); err != nil {
		return domain.AssistantCall{}, fmt.Errorf("%w: %w", domain.ErrAssistantInvalidArguments, err)
	}
	for name, values := range queryArguments(op, args) {
		if call.Query == nil {
			call.Query = map[string]string{}
		}
		call.Query[name] = values[0]
	}
	if body, ok := args[assistantBodyArgument].(map[string]any); ok && op.Body {
		call.Body = body
	}
	return call, nil
}
