package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const specPath = "../../api/openapi.yaml"

// loadChanged generates the registry from a copy of the real spec with one exact text replaced.
func loadChanged(t *testing.T, old, replacement string) Registry {
	t.Helper()
	raw, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(raw), old) != 1 {
		t.Fatalf("the spec holds %q %d times, want once", old, strings.Count(string(raw), old))
	}
	path := filepath.Join(t.TempDir(), "openapi.yaml")
	if err := os.WriteFile(path, []byte(strings.Replace(string(raw), old, replacement, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	registry, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return registry
}

func operation(t *testing.T, registry Registry, id string) Operation {
	t.Helper()
	for _, op := range registry.Operations {
		if op.ID == id {
			return op
		}
	}
	t.Fatalf("%s is not in the registry", id)
	return Operation{}
}

func kinds(op Operation) map[string]string {
	out := map[string]string{}
	for _, id := range op.IDs {
		out[id.In+" "+id.Field] = id.Kind
	}
	return out
}

const moveTargetID = `        targetId:
          type: integer
          format: int64
          minimum: 1
          x-vv-entity: person
`

func TestLoadTakesIDKindsFromParameterNamesAndAnnotations(t *testing.T) {
	registry, err := Load(specPath)
	if err != nil {
		t.Fatal(err)
	}
	got := kinds(operation(t, registry, "moveFaceAppearances"))
	want := map[string]string{"path id": "person", "body /targetId": "person", "body /appearanceIds": "faceAppearance"}
	if len(got) != len(want) {
		t.Errorf("ids = %v, want %v", got, want)
	}
	for field, kind := range want {
		if got[field] != kind {
			t.Errorf("%s = %q, want %q", field, got[field], kind)
		}
	}
	// An annotation on the object a shared $ref sits in names that object's own id.
	details := kinds(operation(t, registry, "setVideoDetails"))
	if details["body /workInfo/people/*/person/id"] != "person" ||
		details["body /workInfo/organizations/*/organization/id"] != "organization" {
		t.Errorf("setVideoDetails ids = %v", details)
	}

	unannotated := loadChanged(t, moveTargetID, strings.Replace(moveTargetID, "          x-vv-entity: person\n", "", 1))
	if kind := kinds(operation(t, unannotated, "moveFaceAppearances"))["body /targetId"]; kind != "" {
		t.Errorf("targetId without its annotation = %q, want no kind", kind)
	}
}

func TestLoadMarksResponsesThatReachACredential(t *testing.T) {
	registry, err := Load(specPath)
	if err != nil {
		t.Fatal(err)
	}
	if !operation(t, registry, "getPluginProxySettings").Secret {
		t.Error("getPluginProxySettings is not marked secret")
	}
	// The new token is a string property named token, which no annotation marks.
	if !operation(t, registry, "createApiToken").Secret {
		t.Error("createApiToken is not marked secret")
	}
	if operation(t, registry, "getOllamaSettings").Secret {
		t.Error("getOllamaSettings is marked secret")
	}

	unmarked := loadChanged(t, "          maxLength: 2048\n          x-vv-secret: true\n", "          maxLength: 2048\n")
	if operation(t, unmarked, "getPluginProxySettings").Secret {
		t.Error("the proxy URL without x-vv-secret still marks the operation")
	}
}

func TestLoadDescribesParametersBodyAndResponse(t *testing.T) {
	registry, err := Load(specPath)
	if err != nil {
		t.Fatal(err)
	}
	frame := operation(t, registry, "getVideoFrame")
	if frame.Response != responseImage || !frame.Owner {
		t.Errorf("getVideoFrame = %s owner %v, want an owner image", frame.Response, frame.Owner)
	}
	if stream := operation(t, registry, "streamEvents"); stream.Response != responseMedia {
		t.Errorf("streamEvents = %s, want media", stream.Response)
	}
	if login := operation(t, registry, "login"); login.Owner {
		t.Error("login, which needs no session, is an owner operation")
	}
	move := operation(t, registry, "moveFaceAppearances")
	properties, _ := move.Parameters["properties"].(map[string]any)
	body, _ := properties[BodyArgument].(map[string]any)
	if _, ok := properties["id"]; !ok || !move.Body || move.BodyType != bodyJSON || body["type"] != "object" {
		t.Errorf("moveFaceAppearances parameters = %v, want the path id and the JSON body", move.Parameters)
	}
	// A multipart body keeps its text fields and drops its file parts, which the assistant cannot send.
	run := operation(t, registry, "runThumbnailEdit")
	runBody, _ := run.Parameters["properties"].(map[string]any)[BodyArgument].(map[string]any)
	fields, _ := runBody["properties"].(map[string]any)
	if _, ok := fields["prompt"]; !ok || len(fields) != 1 || !run.Body || run.BodyType != bodyMultipart {
		t.Errorf("runThumbnailEdit body = %v (%s), want the prompt field of a multipart body", runBody, run.BodyType)
	}
	if image := operation(t, registry, "uploadPersonImage"); image.Body {
		t.Error("uploadPersonImage, whose only part is a file, takes a body")
	}
	if strings.Contains(mustEncode(t, registry), "$ref") || strings.Contains(mustEncode(t, registry), "x-vv-") {
		t.Error("the registry keeps a $ref or an x-vv- extension")
	}
}

func mustEncode(t *testing.T, registry Registry) string {
	t.Helper()
	encoded, err := Encode(registry)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}
