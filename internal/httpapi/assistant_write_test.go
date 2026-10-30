package httpapi

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func TestWriteCallSplitsASchemaRowsArgumentsAfterValidation(t *testing.T) {
	runner := &ActionRunner{}
	call, err := runner.WriteCall("moveFaceAppearances",
		json.RawMessage(`{"id":"11","body":{"targetId":12,"appearanceIds":[101,102]}}`))
	if err != nil {
		t.Fatalf("WriteCall() error = %v", err)
	}
	ids, _ := call.Body["appearanceIds"].([]any)
	if call.ActionID != "moveFaceAppearances" || call.Path["id"] != "11" || call.Body["targetId"] != float64(12) || len(ids) != 2 {
		t.Fatalf("call = %+v", call)
	}
	for name, args := range map[string]string{
		"a body without targetId": `{"id":11,"body":{"appearanceIds":[101]}}`,
		"no path id":              `{"body":{"targetId":12,"appearanceIds":[101]}}`,
	} {
		if _, err := runner.WriteCall("moveFaceAppearances", json.RawMessage(args)); !errors.Is(err, domain.ErrAssistantInvalidArguments) {
			t.Errorf("%s: error = %v, want invalid arguments", name, err)
		}
	}
	if _, err := runner.WriteCall("createTag", json.RawMessage(`{}`)); !errors.Is(err, domain.ErrAssistantActionUnknown) {
		t.Errorf("a name-based row: error = %v, want unknown", err)
	}
}
