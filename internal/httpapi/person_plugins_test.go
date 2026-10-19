package httpapi

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/httpapi/gen"
)

const personPluginManifest = `{"contract":1,"kind":"person","name":"example.people","version":"1.0.0",` +
	`"operations":["search","fetch"],"sources":["site","wiki"]}`

func TestPluginListCarriesKindSourcesAndAutoLookup(t *testing.T) {
	e := newPluginEnv(t)
	work := decodePlugin(t, "work", e.uploadParts([]byte(pluginManifest), 1, e.owner), http.StatusCreated)
	person := decodePlugin(t, "person", e.uploadParts([]byte(personPluginManifest), 1, e.owner), http.StatusCreated)

	rec := e.get("/api/work-info-plugins", e.owner)
	var list gen.WorkInfoPluginList
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &list) != nil || len(list.Plugins) != 2 {
		t.Fatalf("list = %d %s", rec.Code, rec.Body)
	}
	byID := map[int64]gen.WorkInfoPlugin{}
	for _, p := range list.Plugins {
		byID[p.Id] = p
	}
	if got := byID[work.Id]; got.Kind != gen.WorkInfoPluginKindWork || got.Sources == nil || len(got.Sources) != 0 ||
		got.AutoLookup {
		t.Errorf("work-info plugin = %+v; want kind work, empty sources and autoLookup false", got)
	}
	if got := byID[person.Id]; got.Kind != gen.WorkInfoPluginKindPerson ||
		!slices.Equal(got.Sources, []string{"site", "wiki"}) || !got.AutoLookup {
		t.Errorf("person plugin = %+v; want kind person, its sources and autoLookup true", got)
	}

	off := decodePlugin(t, "auto-lookup off",
		e.json(http.MethodPut, pluginPath(person.Id, "/auto-lookup"), `{"enabled":false}`, e.owner), http.StatusOK)
	if off.AutoLookup || off.Kind != gen.WorkInfoPluginKindPerson {
		t.Fatalf("auto-lookup off = %+v", off)
	}
	mismatch := wantError{status: http.StatusConflict, code: gen.ErrorCodePluginKindMismatch}
	for _, req := range []struct{ label, method, target, body string }{
		{"auto-lookup on a work-info plugin", http.MethodPut, pluginPath(work.Id, "/auto-lookup"), `{"enabled":true}`},
		{"person search with a work-info plugin", http.MethodPost, pluginPath(work.Id, "/person-search"), `{"query":"a"}`},
		{"work search with a person plugin", http.MethodPost, pluginPath(person.Id, "/search"), `{"query":"a"}`},
	} {
		rec := e.json(req.method, req.target, req.body, e.owner)
		assertErrorBody(t, req.label, rec.Code, rec.Body.Bytes(), mismatch)
	}
	for _, req := range []struct {
		label, target, body string
		want                wantError
	}{
		{"missing enabled", pluginPath(person.Id, "/auto-lookup"), `{}`,
			wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest}},
		{"missing plugin", pluginPath(person.Id+work.Id, "/auto-lookup"), `{"enabled":true}`,
			wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound}},
	} {
		rec := e.json(http.MethodPut, req.target, req.body, e.owner)
		assertErrorBody(t, req.label, rec.Code, rec.Body.Bytes(), req.want)
	}
	assertUnauthenticated(t, "auto-lookup as a guest",
		e.json(http.MethodPut, pluginPath(person.Id, "/auto-lookup"), `{"enabled":true}`))
}
