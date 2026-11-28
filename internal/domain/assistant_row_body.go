package domain

import (
	"fmt"
	"maps"
)

// assistantActionKeys are body fields whose objects, or array elements, take the row's action as their key (add
// or remove); a key of another action is refused. Other rows with an action send it as the body's action field.
var assistantActionKeys = map[string][]string{"applyVideoEdits": {"tags", "credits"}}

var assistantActionNames = []string{"add", "remove"}

// RequestBody is the body the row sends: the fields over the current values a settings row read back, and the
// row's action as the body's action field or as the key of its edits.
func (row AssistantAction) RequestBody(current, fields map[string]any) (map[string]any, error) {
	body := map[string]any{}
	maps.Copy(body, current)
	maps.Copy(body, fields)
	keys, keyed := assistantActionKeys[row.Operation]
	if row.Action != "" && !keyed {
		body["action"] = row.Action
	}
	if err := checkActionKeys(keys, row.Action, body); err != nil {
		return nil, err
	}
	return body, nil
}

func checkActionKeys(fields []string, action string, body map[string]any) error {
	for _, field := range fields {
		var edits []any
		switch value := body[field].(type) {
		case []any:
			edits = value
		case nil:
		default:
			edits = []any{value}
		}
		for _, edit := range edits {
			values, _ := edit.(map[string]any)
			for _, key := range assistantActionNames {
				if _, ok := values[key]; ok && key != action {
					return fmt.Errorf("%s holds %s, which is not the action %s", field, key, action)
				}
			}
		}
	}
	return nil
}
