package domain

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestParseAssistantOutcomeAcceptsEachKind(t *testing.T) {
	examples := map[AssistantOutcomeKind]string{
		AssistantOutcomeFilter: `{"kind":"filter","filter":{"tags":["cat"],"maxDurationMs":600000,` +
			`"addedFrom":"2026-01-01","watchState":"unwatched","sort":"addedDesc"}}`,
		AssistantOutcomeNavigate: `{"kind":"navigate","navigate":{"to":"settings","settingsPage":"connections"}}`,
		AssistantOutcomeAnswer: `{"kind":"answer","answer":{"question":"appearances",` +
			`"entity":{"kind":"person","name":"Alice"}}}`,
		AssistantOutcomeAction: `{"kind":"action","action":{"operation":"updateVideoTags.add",` +
			`"target":{"kind":"selection"},"arguments":{"tagNames":["cat"]}}}`,
		AssistantOutcomeOrganize:    `{"kind":"organize","organize":{"kind":"tags"}}`,
		AssistantOutcomeClarify:     `{"kind":"clarify","clarify":{"about":"target"}}`,
		AssistantOutcomeUnsupported: `{"kind":"unsupported"}`,
	}
	for kind, raw := range examples {
		outcome, err := ParseAssistantOutcome(json.RawMessage(raw))
		if err != nil {
			t.Fatalf("%s: ParseAssistantOutcome() error = %v", kind, err)
		}
		if outcome.Kind != kind {
			t.Fatalf("%s: kind = %q", kind, outcome.Kind)
		}
	}
}

func TestParseAssistantOutcomeRejectsWhatTheSchemaDoesNotDefine(t *testing.T) {
	cases := map[string]string{
		"operation outside the catalog": `{"kind":"action","action":{"operation":"deleteVideo"}}`,
		"text field at the top":         `{"kind":"unsupported","message":"Done! I deleted everything."}`,
		"text field in a member":        `{"kind":"clarify","clarify":{"about":"value","question":"Which tag?"}}`,
		"member of another kind":        `{"kind":"unsupported","filter":{"q":"cat"}}`,
		"not JSON":                      `I could not do that.`,
	}
	for name, raw := range cases {
		if _, err := ParseAssistantOutcome(json.RawMessage(raw)); !errors.Is(err, ErrGenerationUnreadable) {
			t.Fatalf("%s: error = %v, want ErrGenerationUnreadable", name, err)
		}
	}
}
