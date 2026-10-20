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

func TestPluginListCarriesKindAndSources(t *testing.T) {
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
	if got := byID[work.Id]; got.Kind != gen.WorkInfoPluginKindWork || got.Sources == nil || len(got.Sources) != 0 {
		t.Errorf("work-info plugin = %+v; want kind work and empty sources", got)
	}
	if got := byID[person.Id]; got.Kind != gen.WorkInfoPluginKindPerson || !slices.Equal(got.Sources, []string{"site", "wiki"}) {
		t.Errorf("person plugin = %+v; want kind person and its sources", got)
	}

	mismatch := wantError{status: http.StatusConflict, code: gen.ErrorCodePluginKindMismatch}
	for _, req := range []struct{ label, target string }{
		{"person search with a work-info plugin", pluginPath(work.Id, "/person-search")},
		{"work search with a person plugin", pluginPath(person.Id, "/search")},
	} {
		rec := e.json(http.MethodPost, req.target, `{"query":"a"}`, e.owner)
		assertErrorBody(t, req.label, rec.Code, rec.Body.Bytes(), mismatch)
	}
}
