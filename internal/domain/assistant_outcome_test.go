package domain

import (
	"encoding/json"
	"errors"
	"reflect"
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
		"kind without its member":       `{"kind":"organize","action":{"operation":"mergeTag","target":{"kind":"named","names":["a"]}}}`,
		"not JSON":                      `I could not do that.`,
	}
	for name, raw := range cases {
		if _, err := ParseAssistantOutcome(json.RawMessage(raw)); !errors.Is(err, ErrGenerationUnreadable) {
			t.Fatalf("%s: error = %v, want ErrGenerationUnreadable", name, err)
		}
	}
}

// Real models of every size add a member of another kind beside the right one, and put an answer's filter at the
// top; the answers below are what Llama 3.3 70B, Llama 3.2 3B and Mistral Small wrote.
func TestParseAssistantOutcomeKeepsOnlyTheKindsOwnMember(t *testing.T) {
	cases := map[string]struct {
		raw  string
		want AssistantOutcome
	}{
		"filter beside an answer is its filter": {
			`{"kind":"answer","filter":{"person":"山田太郎"},"answer":{"question":"count"}}`,
			AssistantOutcome{Kind: AssistantOutcomeAnswer, Answer: &AssistantQuestion{
				Question: "count", Filter: &AssistantFilter{Person: "山田太郎"},
			}},
		},
		"action and clarify beside an answer are dropped": {
			`{"kind":"answer","filter":{"q":"cat"},"answer":{"question":"count","filter":{"tags":["cat"]}},` +
				`"action":{"operation":"startScan"},"clarify":{"about":"operation"}}`,
			AssistantOutcome{Kind: AssistantOutcomeAnswer, Answer: &AssistantQuestion{
				Question: "count", Filter: &AssistantFilter{Tags: []string{"cat"}},
			}},
		},
		"a member beside unsupported is dropped": {
			`{"kind":"unsupported","answer":{"question":"appearances","limit":1}}`,
			AssistantOutcome{Kind: AssistantOutcomeUnsupported},
		},
		"a filter beside an action is dropped": {
			`{"action":{"operation":"updateFavorites","target":{"kind":"selection"}},"filter":{"favorite":true},"kind":"action"}`,
			AssistantOutcome{Kind: AssistantOutcomeAction, Action: &AssistantActionRequest{
				Operation: "updateFavorites", Target: &AssistantTargetRequest{Kind: "selection"},
			}},
		},
	}
	for name, c := range cases {
		got, err := ParseAssistantOutcome(json.RawMessage(c.raw))
		if err != nil {
			t.Fatalf("%s: error = %v", name, err)
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Fatalf("%s: outcome = %+v, want %+v", name, got, c.want)
		}
	}
}
