package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/syudead/vv/internal/domain"
)

// schemaOperation is the part of an operation of internal/httpapi/gen/assistant_tools.gen.json that a call must
// match: its path parameters, its query names and its body schema.
type schemaOperation struct {
	pathParams []string
	query      []string
	body       *jsonschema.Resolved
}

func loadRequestSchemas(t *testing.T) map[string]schemaOperation {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "httpapi", "gen", "assistant_tools.gen.json"))
	if err != nil {
		t.Fatalf("read the generated tool registry: %v", err)
	}
	var file struct {
		Operations []struct {
			ID         string                  `json:"operationId"`
			PathParams []string                `json:"pathParams"`
			Query      []struct{ Name string } `json:"query"`
			Body       bool                    `json:"body"`
			Parameters json.RawMessage         `json:"parameters"`
		} `json:"operations"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatalf("decode the generated tool registry: %v", err)
	}
	ops := map[string]schemaOperation{}
	for _, op := range file.Operations {
		out := schemaOperation{pathParams: op.PathParams}
		for _, q := range op.Query {
			out.query = append(out.query, q.Name)
		}
		if op.Body {
			var params jsonschema.Schema
			if err := json.Unmarshal(op.Parameters, &params); err != nil {
				t.Fatalf("the schema of %s: %v", op.ID, err)
			}
			if out.body, err = params.Properties["body"].Resolve(nil); err != nil {
				t.Fatalf("the body schema of %s: %v", op.ID, err)
			}
		}
		ops[op.ID] = out
	}
	return ops
}

// schemaRunner stands in for the screen API: it checks each call against its operation's request schema, the
// way the body reaches the handler, and answers what the thumbnail edit stages read.
type schemaRunner struct {
	ops map[string]schemaOperation
	// current is what a settings row reads back before it sends its fields.
	current map[string]any
	called  []string
}

func (r *schemaRunner) Tools() []domain.AssistantTool { return nil }

func (r *schemaRunner) Read(_ context.Context, read domain.AssistantRead) (domain.AssistantReadResult, error) {
	if read.Operation != "getThumbnailEdit" {
		return domain.AssistantReadResult{}, fmt.Errorf("unexpected read %s", read.Operation)
	}
	return domain.AssistantReadResult{Body: []byte(`{"state":"succeeded"}`)}, nil
}

func (r *schemaRunner) Image(context.Context, string) (domain.AssistantImage, error) {
	return domain.AssistantImage{}, errors.New("no images")
}

func (r *schemaRunner) WriteCall(string, json.RawMessage) (domain.AssistantCall, error) {
	return domain.AssistantCall{}, errors.New("no schema rows")
}

func (r *schemaRunner) CheckCall(context.Context, domain.AssistantCall) error {
	return errors.New("the schema cases run their calls")
}

func (r *schemaRunner) Run(_ context.Context, call domain.AssistantCall) (domain.AssistantCallResult, error) {
	row, _ := domain.LookupAssistantAction(call.ActionID)
	operation := row.Operation
	if operation == "" {
		operation = call.Operation
	}
	r.called = append(r.called, operation)
	if err := checkCall(r.ops, row, operation, call, r.current); err != nil {
		return domain.AssistantCallResult{}, err
	}
	if operation == "startThumbnailEdit" {
		return domain.AssistantCallResult{Status: 201, Body: []byte(`{"id":"e1"}`)}, nil
	}
	return domain.AssistantCallResult{Status: 200, Body: []byte(`{}`)}, nil
}

// checkCall refuses a call whose path, query or body the operation does not take.
func checkCall(
	ops map[string]schemaOperation, row domain.AssistantAction, operation string, call domain.AssistantCall,
	current map[string]any,
) error {
	op, ok := ops[operation]
	if !ok {
		return fmt.Errorf("%s is not in api/openapi.yaml", operation)
	}
	for name := range call.Path {
		if !slices.Contains(op.pathParams, name) {
			return fmt.Errorf("%s has no path parameter %s", operation, name)
		}
	}
	for _, name := range op.pathParams {
		if call.Path[name] == "" {
			return fmt.Errorf("%s misses the path parameter %s", operation, name)
		}
	}
	for name := range call.Query {
		if !slices.Contains(op.query, name) {
			return fmt.Errorf("%s has no query parameter %s", operation, name)
		}
	}
	if op.body == nil {
		if call.Body != nil {
			return fmt.Errorf("%s takes no body, but the call sends %v", operation, call.Body)
		}
		return nil
	}
	body, err := row.RequestBody(current, call.Body)
	if err != nil {
		return err
	}
	var instance any
	encoded, err := json.Marshal(body)
	if err == nil {
		err = json.Unmarshal(encoded, &instance)
	}
	if err == nil {
		err = op.body.Validate(instance)
	}
	if err != nil {
		return fmt.Errorf("%s body %s: %w", operation, encoded, err)
	}
	return nil
}

// prefixVocabulary finds a name of a kind when it starts with the kind and a space ("tag Cat"), so each name
// resolves to one entity and any other name is a new one.
type prefixVocabulary struct{ fakeVocabulary }

func (prefixVocabulary) FindNames(_ context.Context, kind domain.AssistantLabelKind, name string) ([]domain.AssistantCandidate, error) {
	if !strings.HasPrefix(name, string(kind)+" ") {
		return nil, nil
	}
	return []domain.AssistantCandidate{{Kind: kind, ID: 3, Name: name}}, nil
}

// planSchemaCase is one representative request of a plan builder.
type planSchemaCase struct {
	row     string
	request string
	// fetch is the body the fetch tool answered for fetchId f1.
	fetch   *fetchedResult
	current map[string]any
}

func named(names ...string) string {
	encoded, _ := json.Marshal(names)
	return fmt.Sprintf(`{"kind":"named","names":%s}`, encoded)
}

func planSchemaCases() []planSchemaCase {
	tag, person, org := named("tag Cat"), named("person Ann"), named("organization Acme")
	return []planSchemaCase{
		{row: "updateVideoTags.add", request: `{"arguments":{"tagNames":["new tag"]}}`},
		{row: "updateVideoTags.remove", request: `{"arguments":{"tagNames":["tag Cat"]}}`},
		{row: "applyVideoEdits.add", request: `{"arguments":{"tagNames":["tag Cat","new tag"],"personNames":["person Ann"],
			"role":"performer","public":true,"dateKind":"release","date":"2024-01-02"}}`},
		{row: "applyVideoEdits.remove", request: `{"arguments":{"tagNames":["tag Cat"],"personNames":["person Ann"],"role":"performer"}}`},
		{row: "updateVideoVisibility", request: `{"arguments":{"public":false}}`},
		{row: "updateFavorites", request: `{"arguments":{"enabled":true}}`},
		{row: "setVideoDisplayName", request: `{"arguments":{"newName":"Shown"}}`},
		{row: "setVideoThumbnailPosition", request: `{"arguments":{"number":1500}}`},
		{row: "reprobeVideo"},
		{row: "autoTagVideo"},
		{row: "confirmVideoTag", request: `{"arguments":{"tagNames":["tag Cat"]}}`},
		{row: "rejectVideoTag", request: `{"arguments":{"tagNames":["tag Cat"]}}`},
		{row: "makeRepresentativeVersion"},
		{row: "unbundleVideo"},
		{row: "createVirtualVideo", request: `{"arguments":{"name":"Clip","url":"https://example.com/v"}}`},
		{row: "applyWorkInfoFetch", request: `{"arguments":{"fetchId":"f1","fields":["title","tags","cover","dates.release",
			"people.performer","organizations.maker","urls","externalIds"]}}`,
			fetch: &fetchedResult{operation: "fetchWorkInfo", body: json.RawMessage(`{"title":"T","tags":["a"],
				"dates":[{"kind":"release","date":"2024"}],"people":[{"role":"performer","name":"Ann"}],
				"organizations":[{"role":"maker","name":"Acme"}],"urls":["https://example.com"],
				"externalIds":[{"source":"s","value":"v"}],"covers":[{"url":"https://example.com/c.jpg"}]}`)}},
		{row: "editVideoThumbnail", request: `{"arguments":{"instruction":"make the sky blue","atMs":1200}}`},

		{row: "createTag", request: `{"arguments":{"name":"Cat"}}`},
		{row: "renameTag", request: `{"target":` + tag + `,"arguments":{"newName":"Kitten"}}`},
		{row: "deleteTag", request: `{"target":` + tag + `}`},
		{row: "mergeTag", request: `{"target":` + tag + `,"arguments":{"names":["tag Kitty"]}}`},
		{row: "addTagSynonym", request: `{"target":` + tag + `,"arguments":{"synonym":"Neko"}}`},
		{row: "removeTagSynonym", request: `{"target":` + tag + `,"arguments":{"synonym":"Neko"}}`},
		{row: "confirmTag", request: `{"target":` + tag + `}`},
		{row: "rejectTag", request: `{"target":` + tag + `}`},
		{row: "forgetRejectedTagName", request: `{"arguments":{"name":"Cat"}}`},
		{row: "batchTags.confirm", request: `{"target":` + tag + `}`},
		{row: "batchTags.reject", request: `{"target":` + tag + `}`},
		{row: "batchTags.delete", request: `{"target":` + tag + `}`},
		{row: "batchTags.setKind", request: `{"target":` + tag + `,"arguments":{"tagKindName":"tagKind Genre"}}`},
		{row: "batchTags.clearKind", request: `{"target":` + tag + `}`},
		{row: "createTagKind", request: `{"arguments":{"name":"Genre"}}`},
		{row: "renameTagKind", request: `{"target":` + named("tagKind Genre") + `,"arguments":{"newName":"Style"}}`},
		{row: "deleteTagKind", request: `{"target":` + named("tagKind Genre") + `}`},
		{row: "setTagKindCardFirst", request: `{"target":` + named("tagKind Genre") + `,"arguments":{"enabled":true}}`},

		{row: "createPerson", request: `{"arguments":{"names":["Ann","Anna"]}}`},
		{row: "updatePerson", request: `{"target":` + person + `,"arguments":{"newName":"Annie"}}`},
		{row: "updatePerson", request: `{"target":` + person + `,"arguments":{"newName":"Annie","names":["Ann","Anna"]}}`},
		{row: "deletePerson", request: `{"target":` + person + `}`},
		{row: "mergePeople", request: `{"target":` + person + `,"arguments":{"names":["person Bob"]}}`},
		{row: "deletePersonImage", request: `{"target":` + person + `}`},
		{row: "confirmFaceCandidate", request: `{"target":` + person + `,"arguments":{"names":["person Bob"]}}`},
		{row: "dismissFaceCandidate", request: `{"target":` + person + `,"arguments":{"names":["person Bob"]}}`},
		{row: "reopenFaceCandidate", request: `{"target":` + person + `,"arguments":{"names":["person Bob"]}}`},
		{row: "applyPersonPluginFetch", request: `{"target":` + person + `,"arguments":{"fetchId":"f1",
			"fields":["image","profile","names","externalIds","links"]}}`,
			fetch: &fetchedResult{operation: "fetchPersonWithPlugin", body: json.RawMessage(`{"person":{
				"names":[{"name":"Ann"}],"externalIds":[{"source":"s","value":"v"}],"imageCount":1,
				"profile":{"gender":"female","heightCm":160,"links":["https://example.com/ann"]}}}`)}},
		{row: "createOrganization", request: `{"arguments":{"names":["Acme"],"parentName":"organization Parent"}}`},
		{row: "updateOrganization", request: `{"target":` + org + `,"arguments":{"names":["Acme Inc"],"parentName":"organization Parent"}}`},
		{row: "deleteOrganization", request: `{"target":` + org + `}`},
		{row: "mergeOrganizations", request: `{"target":` + org + `,"arguments":{"names":["organization Other"]}}`},

		{row: "createPlaylist", request: `{"arguments":{"name":"Mix"}}`},
		{row: "renamePlaylist", request: `{"target":` + named("playlist Mix") + `,"arguments":{"newName":"Best"}}`},
		{row: "deletePlaylist", request: `{"target":` + named("playlist Mix") + `}`},
		{row: "addPlaylistItems", request: `{"arguments":{"name":"playlist Mix"}}`},
		{row: "createGroup", request: `{"arguments":{"name":"Set"}}`},
		{row: "renameGroup", request: `{"target":` + named("group Set") + `,"arguments":{"newName":"Series"}}`},
		{row: "dissolveGroup", request: `{"target":` + named("group Set") + `}`},
		{row: "addGroupItems", request: `{"arguments":{"name":"group Set"}}`},
		{row: "tagGroup", request: `{"target":` + named("group Set") + `}`},
		{row: "deleteCollections", request: `{"target":` + named("group Set", "smartFolder Recent") + `}`},
		{row: "createSmartFolder", request: `{"arguments":{"name":"Long","sort":"addedDesc","parentName":"smartFolder Recent"}}`},
		{row: "deleteSmartFolder", request: `{"target":` + named("smartFolder Recent") + `}`},

		{row: "updateVersionJudgingSettings", request: `{"arguments":{"enabled":true}}`, current: map[string]any{"enabled": false}},
		{row: "updateAutoImportSettings", request: `{"arguments":{"enabled":true}}`, current: map[string]any{"enabled": false}},
		{row: "updateTranscodingSettings", request: `{"arguments":{"encoder":"nvenc"}}`,
			current: map[string]any{"videoEncoder": "auto"}},
		{row: "updateOllamaSettings", request: `{"arguments":{"url":"http://ollama:11434","name":"generation","model":"m2"}}`,
			current: map[string]any{"endpoint": "http://localhost:11434", "model": "m1", "generationModel": ""}},
		{row: "updateAutoTaggingSettings", request: `{"arguments":{"name":"add","number":0.8}}`,
			current: map[string]any{"enabled": true, "afterImport": false, "threshold": 0.5, "tentativeThreshold": 0.3}},
		{row: "updateAutoTaggingSettings", request: `{"arguments":{"name":"automatic","enabled":true}}`,
			current: map[string]any{"enabled": true, "afterImport": false, "threshold": 0.5, "tentativeThreshold": 0.3}},
		{row: "updateComfyUISettings", request: `{"arguments":{"provider":"self_hosted","url":"http://comfy:8188","model":"flux2_klein_4b"}}`,
			current: map[string]any{"provider": "cloud", "model": "qwen_image_edit_2511"}},
		{row: "startScan"},
		{row: "startAutoTagging", request: `{"arguments":{"scope":"all"}}`},
		{row: "startFaceRecognition", request: `{"arguments":{"scope":"untagged"}}`},
	}
}

// buildSchemaCase builds the case's plan on a library screen that selects video 7 and shows a filter, and returns
// the call the worker would send for it.
func buildSchemaCase(t *testing.T, tc planSchemaCase) domain.AssistantCall {
	t.Helper()
	req := domain.AssistantActionRequest{}
	if tc.request != "" {
		if err := json.Unmarshal([]byte(tc.request), &req); err != nil {
			t.Fatalf("the case request: %v", err)
		}
	}
	req.Operation = tc.row
	a := NewAssistant(AssistantOptions{
		Store:      &fakeAssistantStore{},
		Videos:     &fakeLibraryVideos{videos: map[int64]domain.Video{7: {ID: 7, UserKey: "key-7", ContentKey: "content-7", Title: "Clip 7"}}},
		Vocabulary: prefixVocabulary{},
		Now:        func() time.Time { return time.Date(2026, 10, 25, 9, 0, 0, 0, time.UTC) },
	})
	turn := &assistantTurn{a: a, writes: &turnWrites{}, input: domain.AssistantTurnInput{
		Text: "request", Context: domain.AssistantTurnContext{
			Route: domain.AssistantRouteLibrary, SelectedVideoIDs: []int64{7},
			Filter: &domain.SmartRule{Q: "cat", Favorite: true},
		},
	}}
	if tc.fetch != nil {
		turn.writes.fetches = map[string]fetchedResult{"f1": *tc.fetch}
	}
	plan, ok, err := turn.buildPlan(context.Background(), &req)
	if err != nil || !ok {
		t.Fatalf("buildPlan: ok %v, err %v, clarify %+v, failure %q", ok, err, turn.clarification, turn.failure)
	}
	return plan.CallFor([]int64{7})
}

// Every plan builder's call matches the request schema of the operation it calls, so a body field renamed or
// reshaped in api/openapi.yaml fails here instead of when the owner executes an approved proposal.
func TestAssistantPlanBuildersMatchTheRequestSchemas(t *testing.T) {
	ops := loadRequestSchemas(t)
	cases := planSchemaCases()
	for id := range planBuilders {
		if !slices.ContainsFunc(cases, func(tc planSchemaCase) bool { return tc.row == id }) {
			t.Errorf("%s has a plan builder but no case", id)
		}
	}
	for i, tc := range cases {
		t.Run(fmt.Sprintf("%s#%d", tc.row, i), func(t *testing.T) {
			call := buildSchemaCase(t, tc)
			runner := &schemaRunner{ops: ops, current: tc.current}
			a := NewAssistant(AssistantOptions{})
			if _, err := a.runCall(context.Background(), runner, call); err != nil {
				t.Fatalf("%s: %v", tc.row, err)
			}
			if len(runner.called) == 0 {
				t.Fatalf("%s called no operation", tc.row)
			}
		})
	}
}

// The check itself refuses a field the schema does not name, a missing path parameter and an unknown query.
func TestAssistantPlanSchemaCheckRefusesAMismatch(t *testing.T) {
	ops := loadRequestSchemas(t)
	renameTag, _ := domain.LookupAssistantAction("renameTag")
	removeSynonym, _ := domain.LookupAssistantAction("removeTagSynonym")
	cases := []struct {
		name string
		row  domain.AssistantAction
		call domain.AssistantCall
	}{
		{"renamed body field", renameTag, domain.AssistantCall{Path: map[string]string{"id": "3"}, Body: map[string]any{"title": "x"}}},
		{"missing path parameter", renameTag, domain.AssistantCall{Body: map[string]any{"name": "x"}}},
		{"unknown query", removeSynonym, domain.AssistantCall{Path: map[string]string{"id": "3"}, Query: map[string]string{"q": "x"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := checkCall(ops, tc.row, tc.row.Operation, tc.call, nil); err == nil {
				t.Error("checkCall accepted the mismatch")
			}
		})
	}
}
